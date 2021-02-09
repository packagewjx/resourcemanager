package resourcemanager

import (
	"bufio"
	"context"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/classifier"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/pqos"
	"github.com/packagewjx/resourcemanager/internal/resourcemanager/watcher"
	"github.com/packagewjx/resourcemanager/internal/sampler/memrecord"
	"github.com/packagewjx/resourcemanager/internal/utils"
	"github.com/pkg/errors"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type processGroupState string

var (
	processGroupStateNew         processGroupState = "new"
	processGroupStateClassifying processGroupState = "classifying"
	processGroupStateRunning     processGroupState = "running"
	processGroupStateErrored     processGroupState = "error"
)

var numWays, numSets, _ = utils.GetL3Cap()

type impl struct {
	watcher                      watcher.ProcessGroupWatcher
	classifier                   classifier.Classifier
	memRecorder                  memrecord.MemRecorder
	reAllocTimerRoutine          *timedRoutine
	processGroups                *processGroupMap
	processChangeCountWhenUpdate int
	logger                       *log.Logger
	wg                           sync.WaitGroup
	currentSchemes               []*pqos.CLOSScheme
}

var _ ResourceManager = &impl{}

func New(config *Config) (ResourceManager, error) {
	c, err := classifier.New(&classifier.Config{})
	if err != nil {
		return nil, errors.Wrap(err, "创建分类器出错")
	}
	recorder, err := memrecord.NewPinMemRecorder(&memrecord.Config{
		BufferSize:     core.RootConfig.MemTrace.PinConfig.BufferSize,
		WriteThreshold: core.RootConfig.MemTrace.PinConfig.WriteThreshold,
		PinToolPath:    core.RootConfig.MemTrace.PinConfig.PinToolPath,
		TraceCount:     core.RootConfig.MemTrace.TraceCount,
		ConcurrentMax:  core.RootConfig.MemTrace.ConcurrentMax,
	})
	if err != nil {
		return nil, errors.Wrap(err, "创建内存追踪器出错")
	}
	r := &impl{
		watcher:                      config.Watcher,
		classifier:                   c,
		memRecorder:                  recorder,
		processGroups:                (*processGroupMap)(&sync.Map{}),
		processChangeCountWhenUpdate: 0,
		logger:                       log.New(os.Stdout, "ResourceManager: ", log.LstdFlags|log.Lshortfile|log.Lmsgprefix),
		wg:                           sync.WaitGroup{},
	}

	r.reAllocTimerRoutine = newTimerRoutine(core.RootConfig.Manager.AllocCoolDown, core.RootConfig.Manager.AllocSquash, r.doReAlloc)
	return r, nil
}

func (r *impl) gracefulShutdown() {

}

func (r *impl) handleProcessStatus(ctx context.Context, status *watcher.ProcessGroupStatus) {
	switch status.Status {
	case watcher.ProcessGroupStatusAdd:
		//childCtx, cancel := context.WithCancel(ctx)
		processGroupCtx := &processGroupContext{
			group:     status.Group.Clone().(*core.ProcessGroup),
			state:     processGroupStateNew,
			processes: map[int]*processCharacteristic{},
			//cancelManageFunc: cancel,
		}
		for _, pid := range processGroupCtx.group.Pid {
			processGroupCtx.processes[pid] = &processCharacteristic{
				pid:            pid,
				characteristic: classifier.MemoryCharacteristicToDetermine,
			}
		}
		r.processGroups.store(processGroupCtx)
		r.wg.Add(1)
		go func() {
			defer func() {
				r.wg.Done()
				processGroupCtx.cancelManageFunc = nil
			}()
			//if r.classify(childCtx, processGroupCtx) != nil {
			//	return
			//}
			//r.memTrace(childCtx, processGroupCtx)
			r.reAllocTimerRoutine.requestRun()
		}()
	case watcher.ProcessGroupStatusRemove:
		processGroup, ok := r.processGroups.get(status.Group.Id)
		if !ok {
			r.logger.Printf("错误，移除进程组时没有找到进程组 %s", status.Group.Id)
			return
		}
		// 当进程已经退出的时候，CLOS自然会被清空，因此这里不需要做太多的工作，移除本进程组即可
		if processGroup.cancelManageFunc != nil {
			// 暂停正在进行的任何活动
			processGroup.cancelManageFunc()
			processGroup.cancelManageFunc = nil
		}
		r.processChangeCountWhenUpdate += len(processGroup.group.Pid)
		r.processGroups.remove(status.Group.Id)
		r.logger.Printf("成功移除进程组 %s", status.Group.Id)
	case watcher.ProcessGroupStatusUpdate:
		processGroup, ok := r.processGroups.get(status.Group.Id)
		if !ok {
			r.logger.Printf("错误，更新时没有找到进程组 %s", status.Group.Id)
			return
		}
		// 对于进程组更新，只有当前进程更改的次数达到一个阈值以后才会进行处理。如果每次更新进程都处理，会导致分配方案频繁变更，可能
		// 会有不好的后果。
		// 再分配触发时重置此计数。
		// 目前先不实现再次进行分类的逻辑。
		oldGroup := processGroup.group
		add, removed := diffIntArray(oldGroup.Pid, status.Group.Pid)
		r.processChangeCountWhenUpdate += len(add) + len(removed)
		for _, removedPid := range removed {
			delete(processGroup.processes, removedPid)
		}
	}
	if r.processChangeCountWhenUpdate > core.RootConfig.Manager.ChangeProcessCountThreshold {
		r.reAllocTimerRoutine.requestRun()
	}
}

func (r *impl) doReAlloc() {
	// 首先获取快照，防止processGroups修改产生的一些意外后果
	r.logger.Println("正在计算分配方案")
	r.logger.Println("分配方案计算完成，正在执行分配")
	schemes := r.directAlloc()

	for _, scheme := range schemes {
		r.logger.Printf("CLOS [%d] WayBit [%x] Process [%v]", scheme.CLOSNum, scheme.WayBit, scheme.Processes)
	}
	err := pqos.SetCLOSScheme(schemes)
	if err != nil {
		r.logger.Println("无法设置CLOS分配", err)
	}
	r.logger.Println("资源分配完成")
}

// 调试用,用于采集信息
func (r *impl) writeResult() {
	var perfStatCsv *os.File
	name := "perfstat.csv"
	if _, err := os.Stat(name); os.IsNotExist(err) {
		perfStatCsv, err = os.Create(name)
		if err != nil {
			r.logger.Println("创建perfstat输出文件失败", err)
			return
		}
		_, _ = perfStatCsv.WriteString("groupId,pid,instructions,cycles,allStores,allLoads,LLCMiss,LLCHit,MemAnyCycles,LLCMissCycles,characteristic\n")
	} else {
		perfStatCsv, err = os.OpenFile(name, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			r.logger.Println("打开perfstat输出文件失败", err)
			return
		}
	}

	r.processGroups.traverse(func(name string, group *processGroupContext) bool {
		for pid, characteristic := range group.processes {
			if characteristic.characteristic == classifier.MemoryCharacteristicToDetermine {
				continue
			}

			if len(characteristic.mrc) != 0 {
				mrcCsv, err := os.Create(fmt.Sprintf("%s-%d.mrc.csv", group.group.Id, pid))
				if err != nil {
					r.logger.Println("创建MRC CSV 失败")
				} else {
					writer := bufio.NewWriter(mrcCsv)
					for cacheSize, missRate := range characteristic.mrc {
						_, _ = writer.WriteString(fmt.Sprintf("%d,%.4f\n", cacheSize, missRate))
					}
					_ = writer.Flush()
					_ = mrcCsv.Close()
				}
			}
			if characteristic.perfStat == nil {
				r.logger.Printf("进程组 %s 进程 %d perf stat 为空", group.group.Id, pid)
			} else {
				_, _ = perfStatCsv.WriteString(fmt.Sprintf("%s,%d,%d,%d,%d,%d,%d,%d,%d,%d,%s\n", group.group.Id,
					characteristic.perfStat.Pid, characteristic.perfStat.Instructions, characteristic.perfStat.Cycles,
					characteristic.perfStat.AllStores, characteristic.perfStat.AllLoads, characteristic.perfStat.LLCMiss,
					characteristic.perfStat.LLCHit, characteristic.perfStat.MemAnyCycles, characteristic.perfStat.LLCMissCycles,
					characteristic.characteristic))
			}
		}
		return true
	})
	_ = perfStatCsv.Close()
	r.logger.Println("结果写入完成")
}

func (r *impl) classify(ctx context.Context, groupContext *processGroupContext) error {
	r.logger.Printf("等待 %s 后对 %s 进程组进行分类", core.RootConfig.Manager.ClassifyAfter.String(), groupContext.group.Id)
	select {
	case <-time.After(core.RootConfig.Manager.ClassifyAfter):
	case <-ctx.Done():
		r.logger.Println("等待分类时被结束")
		return fmt.Errorf("等待分类时被结束")
	}

	groupContext.state = processGroupStateClassifying
	ch := r.classifier.Classify(ctx, groupContext.group)
	r.logger.Printf("对进程组 %s 进行分类", groupContext.group.Id)
	result := <-ch // 这里直接等待这个，而没有ctx.Done，因为ctx结束时，理论上会返回结果
	if result.Error != nil {
		r.logger.Printf("对进程组 %s 的分类出错： %v", groupContext.group.Id, result.Error)
		groupContext.state = processGroupStateErrored
		return result.Error
	}
	for _, processResult := range result.Processes {
		p := groupContext.processes[processResult.Pid]
		if processResult.Error != nil {
			r.logger.Printf("进程组 %s 的进程 %d 监控出错： %v", groupContext.group.Id, processResult.Pid, processResult.Error)
			p.characteristic = classifier.MemoryCharacteristicToDetermine
		} else {
			p.characteristic = processResult.Characteristic
			p.perfStat = processResult.StatResultAllWays
		}
	}
	r.logger.Printf("进程组 %s 分类完成", groupContext.group.Id)
	groupContext.state = processGroupStateRunning
	return nil
}

func (r *impl) memTrace(ctx context.Context, group *processGroupContext) {
	wg := sync.WaitGroup{}
	for _, c := range group.processes {
		if c.characteristic == classifier.MemoryCharacteristicSensitive ||
			c.characteristic == classifier.MemoryCharacteristicMedium {
			wg.Add(1)
			go func(p *processCharacteristic) {
				r.logger.Printf("对进程组 %s 进程 %d 开始内存追踪", group.group.Id, p.pid)
				consumer := memrecord.NewRTHCalculatorConsumer(memrecord.GetCalculatorFromRootConfig())
				ch, _ := r.memRecorder.RecordProcess(ctx, &memrecord.AttachRequest{
					BaseRequest: memrecord.BaseRequest{
						Consumer: consumer,
						Name:     fmt.Sprintf("%s-%d", group.group.Id, p.pid),
					},
					Pid: p.pid,
				})
				result := <-ch
				if result.Err != nil {
					r.logger.Printf("对进程组 %s 进程 %d 的内存追踪错误：%v", group.group.Id, p.pid, result.Err)
					p.mrc = []float32{}
				} else {
					p.mrc = WeightedAverageMRC(consumer.GetCalculatorMap(), result.ThreadInstructionCount,
						result.TotalInstructions, core.RootConfig.MemTrace.MaxRthTime, numWays*numSets)
				}
				wg.Done()
			}(c)
		}
	}
	wg.Wait()
}

func (r *impl) Run() error {
	pqos.PqosInit()
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		pqos.PqosFini()
	}()
	// 注册信号处理
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM, syscall.SIGQUIT)
	// 注册进程监视函数
	watchChannel := r.watcher.Watch()
	r.reAllocTimerRoutine.start(ctx)

	for {
		select {
		case sig := <-sigCh:
			signal.Ignore(sig) // 防止重复进入本函数
			if sig == syscall.SIGTERM || sig == syscall.SIGINT || sig == syscall.SIGQUIT {
				r.logger.Println("接收到结束信号，正在关闭并回收所有资源")
				r.gracefulShutdown()
				return nil
			} else if sig == syscall.SIGKILL {
				r.logger.Println("接收到中止信号，正在强制退出")
				return fmt.Errorf("Kill By Signal")
			} else {
				r.logger.Printf("接收到信号%v，不处理", sig)
				signal.Reset(sig)
				continue
			}
		case processStatus := <-watchChannel:
			r.logger.Printf("接收到进程组新状态：ID %s ，状态 %s ，Pid列表： %v", processStatus.Group.Id,
				watcher.ProcessGroupConditionDisplayName[processStatus.Status], processStatus.Group.Pid)
			r.handleProcessStatus(ctx, processStatus)
		}
	}

}

func (r *impl) directAlloc() []*pqos.CLOSScheme {
	clos := make([]*pqos.CLOSScheme, 3)
	clos[0] = &pqos.CLOSScheme{
		CLOSNum: 1,
		WayBit:  0xF,
	}
	clos[1] = &pqos.CLOSScheme{
		CLOSNum: 2,
		WayBit:  0x7F0,
	}
	clos[2] = &pqos.CLOSScheme{
		CLOSNum: 3,
		WayBit:  0x7FF,
	}
	r.processGroups.traverse(func(name string, group *processGroupContext) bool {
		r.logger.Printf("ocess Group %s: Process [%v]", name, group.processes)
		closPos := 0
		if strings.HasPrefix(name, "mcf") || strings.HasPrefix(name, "omnetpp") ||
			strings.HasPrefix(name, "xz") || strings.HasPrefix(name, "cpugcc") {
			closPos = 0 // bully, squanderer
		} else {
			closPos = 1
		}
		// non-critial e all ways
		for pid := range group.processes {
			clos[closPos].Processes = append(clos[closPos].Processes, pid)
		}
		return true
	})
	return clos
}
