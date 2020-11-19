package monitor

import (
	"container/heap"
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

/*
#cgo LDFLAGS: -L/usr/local/lib -Wl,-rpath=/usr/local/lib -lresource_manager  -lpqos
#include <resource_manager.h>
*/
import "C"

var (
	ErrRemoved = fmt.Errorf("进程在监控完成之前被移出监控队列")
)

type Monitor interface {
	AddProcess(rq *Request)    // 添加进程到监控队列。若监控队列已满，将会把进程放入等待队列。完成监控时将会调用onFinish函数。出错则调用onError函数
	RemoveProcess(pid uint)    // 移除当前在监控队列中的进程。若成功移除，将会调用添加时提供的onError函数，err为ErrRemoved
	Start(ctx context.Context) // 启动Monitor
	ShutDownNow()              // 立即结束，并等待资源回收。若ctx过期，也需要调用此函数进行资源回收
}

type FinishFunc func(pid uint, file string)
type ErrorFunc func(pid uint, err error)

var _ Monitor = &monitorImpl{}

type Request struct {
	pid      uint
	duration time.Duration
	onFinish FinishFunc
	onError  ErrorFunc
}

type monitorContext struct {
	pid            uint
	monitorEndTime time.Time
	onFinish       FinishFunc
	onError        ErrorFunc
}

type monitorImpl struct {
	interval    int
	maxRmid     uint
	requestCh   chan *Request
	removePidCh chan uint
	pMonitor    *C.struct_ProcessMonitor
	logger      *log.Logger
	wg          sync.WaitGroup
	cancelFunc  context.CancelFunc
}

func (m *monitorImpl) ShutDownNow() {
	m.cancelFunc()
	m.wg.Wait()
}

func NewMonitor(interval int) (Monitor, error) {
	return &monitorImpl{
		interval:    interval,
		requestCh:   make(chan *Request),
		removePidCh: make(chan uint),
		logger:      log.New(os.Stdout, "Monitor", log.LstdFlags|log.Lshortfile|log.Lmsgprefix),
		wg:          sync.WaitGroup{},
	}, nil
}

func (m *monitorImpl) AddProcess(rq *Request) {
	m.requestCh <- rq
}

func (m *monitorImpl) RemoveProcess(pid uint) {
	m.removePidCh <- pid
}

func (m *monitorImpl) Start(ctx context.Context) {
	m.pMonitor = C.rm_monitor_create(C.uint(m.interval))
	m.maxRmid = uint(C.rm_monitor_get_max_process(m.pMonitor))
	myCtx, cancel := context.WithCancel(ctx)
	m.wg.Add(1)
	m.cancelFunc = cancel
	go m.routine(myCtx)
}

func (m *monitorImpl) routine(ctx context.Context) {
	m.logger.Println("正在启动")

	requestQueue := make([]*Request, 0)
	mQueue := monitoringQueue(make([]*monitorContext, 0))
	heap.Init(&mQueue)
	var waitCh <-chan time.Time
	var firstInLine *monitorContext // 保存队列的原第一个，用于避免多次更新waitCh

	// 在优先队列发生变化时，更新waitCh
	updateWaitCh := func() {
		if mQueue.Len() == 0 {
			firstInLine = nil
		} else if mQueue[0] != firstInLine {
			var originPid uint = 0
			if firstInLine != nil {
				originPid = firstInLine.pid
			}
			firstInLine = mQueue[0]
			waitTime := firstInLine.monitorEndTime.Sub(time.Now())
			waitCh = time.After(waitTime)
			m.logger.Printf("监控队列第一个更新为进程%d，等待时间%s，原为%d（0为不存在）\n",
				firstInLine.pid, waitTime.String(), originPid)
		}
	}

	doAddMonitoringProcess := func(rq *Request) error {
		m.logger.Printf("正在将进程%d加入监控队列\n", rq.pid)
		res := int(C.rm_monitor_add_process(m.pMonitor, C.int(rq.pid)))
		if res != 0 {
			m.logger.Printf("添加进程%d失败，返回码为%d\n", rq.pid, res)
			return fmt.Errorf("添加进程%d失败，返回码为%d\n", rq.pid, res)
		}

		heap.Push(&mQueue, &monitorContext{
			pid:            rq.pid,
			monitorEndTime: time.Now().Add(rq.duration),
			onFinish:       rq.onFinish,
			onError:        rq.onError,
		})

		updateWaitCh()
		return nil
	}

	waitingLineToMonitoringLine := func() {
		for len(requestQueue) > 0 {
			waiting := requestQueue[0]
			requestQueue = requestQueue[1:]
			m.logger.Printf("将等待队列中的进程%d加入监控队列", waiting.pid)
			err := doAddMonitoringProcess(waiting)
			if err != nil {
				if waiting.onError != nil {
					go waiting.onError(waiting.pid, err)
				}
			} else {
				break
			}
		}
	}

outerLoop:
	for {
		select {
		case <-waitCh:
			// 队头的到时间了，调用回调函数
			first := heap.Pop(&mQueue).(*monitorContext)
			m.logger.Printf("进程%d监控时间结束，正在回收资源\n", first.pid)
			// 队头弹出之后，从等待队列插入一个
			waitingLineToMonitoringLine()

			updateWaitCh()
			res := C.rm_monitor_remove_process(m.pMonitor, C.int(first.pid))
			if res != 0 {
				m.logger.Printf("系统监控移除进程%d失败，返回值为%d\n", first.pid, res)
			}
			if first.onFinish != nil {
				go first.onFinish(first.pid, fmt.Sprintf("%d.csv", first.pid))
			}

		case rq := <-m.requestCh:
			m.logger.Printf("接收到监控进程%d的请求，监控时长%s\n", rq.pid, rq.duration.String())

			if uint(mQueue.Len()) >= m.maxRmid {
				m.logger.Printf("监控队列已满，进程%d加入等待队列\n", rq.pid)
				requestQueue = append(requestQueue, rq)
				continue outerLoop
			}

			err := doAddMonitoringProcess(rq)
			if err != nil && rq.onError != nil {
				go rq.onError(rq.pid, err)
			}

		case rpid := <-m.removePidCh:
			m.logger.Printf("接收到移除监控进程%d的请求\n", rpid)
			// 检查当前正在监控的队列
			for i, ctx := range mQueue {
				if ctx.pid == rpid {
					m.logger.Printf("移除在监控队列中的进程%d\n", rpid)
					removed := heap.Remove(&mQueue, i).(*monitorContext)
					go removed.onError(removed.pid, ErrRemoved)
					waitingLineToMonitoringLine()
					continue outerLoop
				}
			}
			// 检查等待中的队列
			for i := 0; i < len(requestQueue); i++ {
				if requestQueue[i].pid == rpid {
					m.logger.Printf("移除在等待队列中的进程%d\n", rpid)
					removed := requestQueue[i]
					go removed.onError(removed.pid, ErrRemoved)
					requestQueue = append(requestQueue[:i], requestQueue[i+1:]...)
					continue outerLoop
				}
			}
			m.logger.Printf("无法移除，目前没有监控进程%d\n", rpid)

		case <-ctx.Done():
			m.logger.Println("正在退出")
			close(m.requestCh)
			for mQueue.Len() > 0 {
				ctx := heap.Pop(&mQueue).(*monitorContext)
				C.rm_monitor_remove_process(m.pMonitor, C.int(ctx.pid))
			}
			C.rm_monitor_destroy(m.pMonitor)

			break outerLoop
		}
	}
	m.wg.Done()
}
