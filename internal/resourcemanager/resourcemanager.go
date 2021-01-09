package resourcemanager

import (
	"context"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/classifier"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/resourcemanager/watcher"
	"github.com/packagewjx/resourcemanager/internal/sampler/perf"
	"github.com/packagewjx/resourcemanager/internal/sampler/pin"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type ResourceManager interface {
	Run() error // 同步运行函数
}

type Config struct {
	Watcher watcher.ProcessGroupWatcher
}

type processGroupState string

var (
	processGroupStateNew         processGroupState = "new"
	processGroupStateClassifying processGroupState = "classifying"
	processGroupStateRunning     processGroupState = "running"
	processGroupStateErrored     processGroupState = "error"
)

type processGroupContext struct {
	group            *core.ProcessGroup
	classifyResult   map[int]*processCharacteristic
	state            processGroupState
	cancelManageFunc context.CancelFunc
}

type processCharacteristic struct {
	pid            int
	characteristic classifier.MemoryCharacteristic
	mrc            []float32
	perfStat       *perf.PerfStatResult
}

func (p *processCharacteristic) Clone() core.Cloneable {
	newMrc := make([]float32, len(p.mrc))
	copy(newMrc, p.mrc)
	return &processCharacteristic{
		pid:            p.pid,
		characteristic: p.characteristic,
		mrc:            newMrc,
		perfStat:       p.perfStat.Clone().(*perf.PerfStatResult),
	}
}

type processGroupMap sync.Map

func (m *processGroupMap) get(name string) (*processGroupContext, bool) {
	val, ok := ((*sync.Map)(m)).Load(name)
	return val.(*processGroupContext), ok
}

func (m *processGroupMap) store(p *processGroupContext) {
	(*sync.Map)(m).Store(p.group.Id, p)
}

func (m *processGroupMap) remove(name string) {
	(*sync.Map)(m).Delete(name)
}

func (m *processGroupMap) traverse(s func(name string, group *processGroupContext) bool) {
	(*sync.Map)(m).Range(func(key, value interface{}) bool {
		return s(key.(string), value.(*processGroupContext))
	})
}

type impl struct {
	watcher                      watcher.ProcessGroupWatcher
	classifier                   classifier.Classifier
	reAllocTimerRoutine          *timedRoutine
	processGroups                *processGroupMap
	processChangeCountWhenUpdate int
	logger                       *log.Logger
	wg                           sync.WaitGroup
}

var _ ResourceManager = &impl{}

func New(config *Config) (ResourceManager, error) {
	r := &impl{
		classifier: classifier.New(&classifier.Config{
			MemTraceConfig: &pin.Config{
				BufferSize:     core.RootConfig.MemTrace.BufferSize,
				WriteThreshold: core.RootConfig.MemTrace.WriteThreshold,
				PinToolPath:    core.RootConfig.MemTrace.PinToolPath,
				TraceCount:     core.RootConfig.MemTrace.TraceCount,
				ConcurrentMax:  core.RootConfig.MemTrace.ConcurrentMax,
			},
			ReservoirSize: core.RootConfig.MemTrace.ReservoirSize,
		}),
		watcher:       config.Watcher,
		processGroups: (*processGroupMap)(&sync.Map{}),
		logger:        log.New(os.Stdout, "ResourceManager", log.LstdFlags|log.Lshortfile|log.Lmsgprefix),
		wg:            sync.WaitGroup{},
	}
	r.reAllocTimerRoutine = newTimerRoutine(core.RootConfig.Manager.AllocCoolDown, core.RootConfig.Manager.AllocSquash, r.doReAlloc)
	return r, nil
}

func (i *impl) gracefulShutdown() {

}

func (i *impl) handleProcessStatus(ctx context.Context, status *watcher.ProcessGroupStatus) {
	switch status.Status {
	case watcher.ProcessGroupStatusAdd:
		childCtx, cancel := context.WithCancel(ctx)
		processGroupCtx := &processGroupContext{
			group:            status.Group.Clone().(*core.ProcessGroup),
			classifyResult:   nil,
			state:            processGroupStateNew,
			cancelManageFunc: cancel,
		}
		i.processGroups.store(processGroupCtx)
		i.wg.Add(1)
		go i.classifyRoutine(childCtx, processGroupCtx)
	case watcher.ProcessGroupStatusRemove:
		processGroup, ok := i.processGroups.get(status.Group.Id)
		if !ok {
			i.logger.Printf("错误，移除进程组时没有找到进程组 %s", status.Group.Id)
			return
		}
		// 当进程已经退出的时候，CLOS自然会被清空，因此这里不需要做太多的工作，移除本进程组即可
		if processGroup.cancelManageFunc != nil {
			// 暂停正在进行的任何活动
			processGroup.cancelManageFunc()
			processGroup.cancelManageFunc = nil
		}
		i.processChangeCountWhenUpdate += len(processGroup.group.Pid)
		i.processGroups.remove(status.Group.Id)
		i.logger.Printf("成功移除进程组 %s", status.Group.Id)
	case watcher.ProcessGroupStatusUpdate:
		processGroup, ok := i.processGroups.get(status.Group.Id)
		if !ok {
			i.logger.Printf("错误，更新时没有找到进程组 %s", status.Group.Id)
			return
		}
		// 对于进程组更新，只有当前进程更改的次数达到一个阈值以后才会进行处理。如果每次更新进程都处理，会导致分配方案频繁变更，可能
		// 会有不好的后果。
		// 再分配触发时重置此计数。
		// 目前先不实现再次进行分类的逻辑。
		oldGroup := processGroup.group
		add, removed := diffIntArray(oldGroup.Pid, status.Group.Pid)
		i.processChangeCountWhenUpdate += len(add) + len(removed)
		for _, removedPid := range removed {
			delete(processGroup.classifyResult, removedPid)
		}
	}
	if i.processChangeCountWhenUpdate > core.RootConfig.Manager.ChangeProcessCountThreshold {
		i.reAllocTimerRoutine.requestRun()
	}
}

func (i *impl) doReAlloc() {
	// 首先获取快照，防止processGroups修改产生的一些意外后果
	managedProcess := make([]*processCharacteristic, 0, 10)
	i.processGroups.traverse(func(name string, group *processGroupContext) bool {
		if group.state == processGroupStateClassifying || group.state == processGroupStateErrored {
			return true
		}
		pGroup := group.group.Clone().(*core.ProcessGroup)
		for _, pid := range pGroup.Pid {
			r, ok := group.classifyResult[pid]
			if !ok || r.characteristic == classifier.MemoryCharacteristicNonCritical ||
				r.characteristic == classifier.MemoryCharacteristicBully || r.characteristic == classifier.MemoryCharacteristicSquanderer {
				continue
			}

			managedProcess = append(managedProcess, r.Clone().(*processCharacteristic))
		}
		return true
	})

	// 开始使用DCAPS计算分配方案
}

func (i *impl) classifyRoutine(ctx context.Context, groupContext *processGroupContext) {
	defer func() {
		i.wg.Done()
		groupContext.cancelManageFunc = nil
	}()
	groupContext.state = processGroupStateClassifying
	ch := i.classifier.Classify(ctx, groupContext.group)
	i.logger.Printf("对进程组 %s 进行分类", groupContext.group.Pid)
	result := <-ch // 这里直接等待这个，而没有ctx.Done，因为ctx结束时，理论上会返回结果
	if result.Error != nil {
		i.logger.Printf("对进程组 %s 的监控出错： %v", groupContext.group.Id, result.Error)
		groupContext.state = processGroupStateErrored
		return
	}
	groupContext.state = processGroupStateRunning
	cMap := make(map[int]*processCharacteristic)
	for _, processResult := range result.Processes {
		if processResult.Error != nil {
			i.logger.Printf("进程组 %s 的进程 %d 监控出错： %v", groupContext.group.Id, processResult.Pid, processResult.Error)
			cMap[processResult.Pid] = &processCharacteristic{
				pid:            processResult.Pid,
				characteristic: classifier.MemoryCharacteristicToDetermine,
			}
		} else {
			cMap[processResult.Pid] = &processCharacteristic{
				pid:            processResult.Pid,
				characteristic: processResult.Characteristic,
				perfStat:       processResult.StatResult,
				mrc:            processResult.WeightedAverageMRC,
			}
		}

	}
	groupContext.classifyResult = cMap
	i.logger.Printf("进程组 %s 分类完成，准备执行分配")
	i.reAllocTimerRoutine.requestRun()
}

func (i *impl) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
	}()
	// 注册信号处理
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM)
	// 注册进程监视函数
	watchChannel := i.watcher.Watch()

	for {
		select {
		case sig := <-sigCh:
			signal.Ignore(sig) // 防止重复进入本函数
			if sig == syscall.SIGTERM || sig == syscall.SIGINT {
				i.logger.Println("接收到结束信号，正在关闭并回收所有资源")
				i.gracefulShutdown()
				return nil
			} else if sig == syscall.SIGKILL {
				i.logger.Println("接收到中止信号，正在强制退出")
				return fmt.Errorf("Kill By Signal")
			} else {
				i.logger.Printf("接收到信号%v，不处理", sig)
				signal.Reset(sig)
				continue
			}
		case processStatus := <-watchChannel:
			i.logger.Printf("接收到进程组新状态：ID %s ，状态 %s ，Pid列表： %v", processStatus.Group.Id,
				watcher.ProcessGroupConditionDisplayName[processStatus.Status], processStatus.Group.Pid)
			i.handleProcessStatus(ctx, processStatus)
		}
	}

}

func diffIntArray(a, b []int) (add []int, remove []int) {
	am := map[int]struct{}{}
	bm := map[int]struct{}{}
	for _, i := range a {
		am[i] = struct{}{}
	}
	for _, i := range b {
		if _, ok := am[i]; !ok {
			add = append(add, i)
		}
		bm[i] = struct{}{}
	}
	for _, i := range a {
		if _, ok := bm[i]; !ok {
			remove = append(remove, i)
		}
	}
	return
}
