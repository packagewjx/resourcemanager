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

type processWatcher struct {
	base        *baseChannelWatcher
	cancelFunc  context.CancelFunc
	targetCmd   []string
	oldGroupMap map[int]*core.ProcessGroup
	tickTime    time.Duration
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
	tick := time.Tick(p.tickTime)
	set := newProcessUnionSet(p.targetCmd)

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
func NewProcessWatcher(targetCmd []string, tickTime time.Duration) ProcessGroupWatcher {
	return &processWatcher{
		base:        &baseChannelWatcher{channels: []chan *ProcessGroupStatus{}},
		targetCmd:   targetCmd,
		oldGroupMap: map[int]*core.ProcessGroup{},
		tickTime:    tickTime,
	}
}

type processUnionSetEntry struct {
	ps.Process
	root int
}

type processUnionSet struct {
	processMap map[int]*processUnionSetEntry
	targetCmd  map[string]struct{} // 目标进程的指令，本进程集仅关注目标进程及其子进程
}

func newProcessUnionSet(targetCmd []string) *processUnionSet {
	cmdMap := make(map[string]struct{})
	for _, s := range targetCmd {
		cmdMap[s] = struct{}{}
	}
	return &processUnionSet{
		processMap: make(map[int]*processUnionSetEntry),
		targetCmd:  cmdMap,
	}
}

// 判断这个进程是否是目标的进程，目标进程包括根进程及其子进程，返回是否。若是，则同时返回根进程pid
// rootProcesses用于缓存中间结果
func (p *processUnionSet) inTargetProcessTree(process ps.Process, currentProcesses map[int]ps.Process, rootProcesses map[int]int) (bool, int) {
	curr := process
	rpid := 1
	inTargetProcessTree := false
	for curr != nil {
		if currRootPid, ok := rootProcesses[curr.Pid()]; ok {
			// 有中间结果，看情况更新 rpid 与 inTargetProcessTree
			if inTargetProcessTree {
				if currRootPid != 1 {
					rpid = currRootPid
				}
			} else {
				rpid = currRootPid
				inTargetProcessTree = rpid != 1
			}
			break
		} else if _, ok := p.targetCmd[curr.Executable()]; ok {
			inTargetProcessTree = true
			rpid = curr.Pid()
		}
		curr = currentProcesses[curr.PPid()]
	}
	rootProcesses[process.Pid()] = rpid
	return inTargetProcessTree, rpid
}

// 判断属于这个新进程的pid，是否与保存的进程不是一个进程。因为进程号会复用
func (p *processUnionSet) isProcessChanged(process ps.Process) bool {
	storedProcess := p.processMap[process.Pid()]
	return storedProcess == nil || storedProcess.PPid() != process.PPid() || storedProcess.Executable() != process.Executable()
}

func (p *processUnionSet) shouldSkip(process ps.Process) bool {
	// 跳过条件：
	// 1. 若父进程是init，并且不是我们关心的目标进程，则跳过
	_, ok := p.targetCmd[process.Executable()]
	return process.Pid() == 1 || process.Pid() == 2 || ((process.PPid() == 1 || process.PPid() == 2) && !ok)
}

func (p *processUnionSet) update(processList []ps.Process) map[int]*core.ProcessGroup {
	currentProcesses := make(map[int]ps.Process)
	for _, process := range processList {
		// 跳过一些不可能是目标进程及其子进程的进程
		if p.shouldSkip(process) {
			continue
		}
		currentProcesses[process.Pid()] = process
	}

	rootProcesses := make(map[int]int)
	for _, process := range currentProcesses {
		if p.isProcessChanged(process) {
			if ok, rpid := p.inTargetProcessTree(process, currentProcesses, rootProcesses); ok {
				p.processMap[process.Pid()] = &processUnionSetEntry{
					Process: process,
					root:    rpid,
				}
			} else {
				delete(p.processMap, process.Pid())
			}
		}
	}

	result := make(map[int]*core.ProcessGroup)
	for _, ent := range p.processMap {
		if _, ok := currentProcesses[ent.Pid()]; !ok {
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
