package watcher

import (
	"context"
	"fmt"
	"github.com/mitchellh/go-ps"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/utils"
	"log"
	"os"
	"sync"
	"time"
)

const tickTime = 200 * time.Millisecond

type processWatcher struct {
	base        *baseChannelWatcher
	cancelFunc  context.CancelFunc
	targetCmd   []string
	oldGroupMap map[int]*core.ProcessGroup
}

func (p *processWatcher) Watch() <-chan *ProcessGroupStatus {
	if p.cancelFunc == nil {
		ctx, cancel := context.WithCancel(context.Background())
		p.cancelFunc = cancel
		go p.pollRoutine(ctx)
	}
	return p.base.Watch()
}

func (p *processWatcher) StopWatch(ch <-chan *ProcessGroupStatus) {
	p.base.StopWatch(ch)
	if len(p.base.channels) == 0 && p.cancelFunc != nil {
		p.cancelFunc()
		p.cancelFunc = nil
	}
}

func diffFamily(oldFamily, newFamily map[int]*core.ProcessGroup) []*ProcessGroupStatus {
	wg := sync.WaitGroup{}
	addUpdateStatus := []*ProcessGroupStatus{}
	lock := sync.Mutex{}
	// 寻找插入与更新的
	for k, v := range newFamily {
		wg.Add(1)
		go func(rpid int, group *core.ProcessGroup) {
			defer wg.Done()
			process, _ := ps.FindProcess(rpid)
			if process == nil {
				return
			}
			status := &ProcessGroupStatus{
				Group: *group,
			}
			// 检查是否发生变化
			old := oldFamily[rpid]
			if old == nil {
				status.Status = ProcessGroupStatusAdd
			} else if utils.IntListDifferent(old.Pid, group.Pid) {
				status.Status = ProcessGroupStatusUpdate
			} else {
				// 没有变化，直接返回
				return
			}
			lock.Lock()
			addUpdateStatus = append(addUpdateStatus, status)
			lock.Unlock()
		}(k, v)
	}

	// 寻找被删除的
	deletedStatus := []*ProcessGroupStatus{}
	for rpid, old := range oldFamily {
		if newFamily[rpid] == nil {
			status := &ProcessGroupStatus{
				Group:  *old,
				Status: ProcessGroupStatusRemove,
			}
			status.Group.Pid = []int{}
			deletedStatus = append(deletedStatus, status)
		}
	}

	wg.Wait()

	return append(addUpdateStatus, deletedStatus...)
}

func (p *processWatcher) pollRoutine(ctx context.Context) {
	logger := log.New(os.Stdout, "Process Watcher: ", log.LstdFlags|log.Lshortfile|log.Lmsgprefix)
	logger.Println("进程监控者启动")
	tick := time.Tick(tickTime)
	set := &processUnionSet{
		processMap: make(map[int]*processUnionSetEntry),
		targetCmd:  p.targetCmd,
	}

	for {
		select {
		case <-tick:
			processes, err := ps.Processes()
			if err != nil {
				panic(err)
			}
			family := set.update(processes)
			diff := diffFamily(p.oldGroupMap, family)
			for _, status := range diff {
				p.base.notifyAll(status)
			}
			p.oldGroupMap = family
		case <-ctx.Done():
			logger.Println("进程监控者结束")
			return
		}
	}
}

// 用于监控本机进程的工具。
// 目前Pin有Bug无法使用Docker，为了实验需要，先使用直接监控本机进程的方式
func NewProcessWatcher(targetCmd []string) ProcessGroupWatcher {
	return &processWatcher{
		base:        &baseChannelWatcher{channels: []chan *ProcessGroupStatus{}},
		targetCmd:   targetCmd,
		oldGroupMap: map[int]*core.ProcessGroup{},
	}
}

type processUnionSetEntry struct {
	ps.Process
	root   int
	target bool
}

type processUnionSet struct {
	processMap map[int]*processUnionSetEntry
	targetCmd  []string // 目标进程的指令，本进程集仅关注目标进程及其子进程
}

func (p *processUnionSet) isTargetProcess(process ps.Process) bool {
	// FIXME 优化性能
	for _, cmd := range p.targetCmd {
		if process.Executable() == cmd {
			return true
		}
	}
	return false
}

func (p *processUnionSet) shouldSkip(process ps.Process) bool {
	// 跳过条件：
	// 1. 若父进程是init，并且不是我们关心的目标进程，则跳过

	return (process.PPid() == 1 && !p.isTargetProcess(process))
}

func (p *processUnionSet) update(processList []ps.Process) map[int]*core.ProcessGroup {
	exist := make(map[int]struct{})
	for _, process := range processList {
		if p.shouldSkip(process) {
			delete(p.processMap, process.Pid())
			continue
		}

		exist[process.Pid()] = struct{}{}
		storedProcess := p.processMap[process.Pid()]
		if storedProcess == nil || storedProcess.Executable() != process.Executable() || storedProcess.PPid() != process.PPid() {
			ent := &processUnionSetEntry{
				Process: process,
			}
			// 若是目标进程，其自身就是root，否则root初始化为其父亲
			if p.isTargetProcess(process) {
				ent.root = process.Pid()
				ent.target = true
			} else {
				ent.root = process.PPid()
			}
			p.processMap[process.Pid()] = ent
		}
	}

	result := make(map[int]*core.ProcessGroup)
	for _, ent := range p.processMap {
		if _, ok := exist[ent.Pid()]; !ok {
			delete(p.processMap, ent.Pid())
		} else {
			rpid := p.findRoot(ent.Pid())

			if rpid == 1 {
				delete(p.processMap, ent.Pid())
				continue
			}

			group := result[rpid]
			if result[rpid] == nil {
				rootProcess, _ := p.processMap[rpid]
				if rootProcess == nil {
					delete(p.processMap, ent.Pid())
					continue
				}

				group = &core.ProcessGroup{
					Id:  fmt.Sprintf("%s-%d", rootProcess.Executable(), rpid),
					Pid: make([]int, 0, 10),
				}
				result[rpid] = group
			}
			group.Pid = append(group.Pid, ent.Pid())
		}
	}
	return result
}

// 查找祖先同时路径压缩
func (p *processUnionSet) findRoot(pid int) int {
	self, ok := p.processMap[pid]
	if !ok {
		// 所有查找不到的进程root都是1
		return 1
	}
	if self.root == self.Pid() {
		return self.Pid()
	}
	self.root = p.findRoot(self.root)
	return self.root
}
