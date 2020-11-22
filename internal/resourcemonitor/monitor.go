package resourcemonitor

import (
	"container/heap"
	"context"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/utils"
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
	ErrRemoved   = fmt.Errorf("进程组在监控完成之前被移出监控队列")
	ErrDuplicate = fmt.Errorf("重复进程组")
)

type Monitor interface {
	AddProcess(rq *Request)         // 添加进程到监控队列。若监控队列已满，将会把进程放入等待队列。完成监控时将会调用onFinish函数。出错则调用onError函数
	RemoveProcess(requestId string) // 移除当前在监控队列中的进程。若成功移除，将会调用添加时提供的onError函数，err为ErrRemoved
	Start(ctx context.Context)      // 启动Monitor
	ShutDownNow()                   // 立即结束，并等待资源回收。若ctx过期，也需要调用此函数进行资源回收
}

type FinishFunc func(requestId string, pid []int, file string)
type ErrorFunc func(requestId string, pid []int, err error)

var _ Monitor = &monitorImpl{}

type Request struct {
	requestId string
	pidList   []int
	duration  time.Duration
	onFinish  FinishFunc
	onError   ErrorFunc
}

type monitorContext struct {
	requestId      string
	pidList        []int
	fileName       string
	cContext       *C.struct_ProcessMonitorContext
	monitorEndTime time.Time
	onFinish       FinishFunc
	onError        ErrorFunc
}

type monitorImpl struct {
	interval    int
	maxRmid     int
	requestCh   chan *Request
	removePidCh chan string
	pMonitor    *C.struct_ProcessMonitor
	logger      *log.Logger
	wg          sync.WaitGroup
	cancelFunc  context.CancelFunc
}

func (m *monitorImpl) ShutDownNow() {
	m.cancelFunc()
	m.wg.Wait()
}

func New(interval int) (Monitor, error) {
	return &monitorImpl{
		interval:    interval,
		requestCh:   make(chan *Request),
		removePidCh: make(chan string),
		logger:      log.New(os.Stdout, "Monitor", log.LstdFlags|log.Lshortfile|log.Lmsgprefix),
		wg:          sync.WaitGroup{},
	}, nil
}

func (m *monitorImpl) AddProcess(rq *Request) {
	m.requestCh <- rq
}

func (m *monitorImpl) RemoveProcess(requestId string) {
	m.removePidCh <- requestId
}

func (m *monitorImpl) Start(ctx context.Context) {
	m.pMonitor = C.rm_monitor_create(C.uint(m.interval))
	m.maxRmid = int(C.rm_monitor_get_max_process(m.pMonitor))
	myCtx, cancel := context.WithCancel(ctx)
	m.wg.Add(1)
	m.cancelFunc = cancel
	go m.routine(myCtx)
}

func (m *monitorImpl) routine(ctx context.Context) {
	m.logger.Println("正在启动")

	waitingRequestQueue := make([]*Request, 0)
	mQueue := queue(make([]*monitorContext, 0))
	heap.Init(&mQueue)
	var waitCh <-chan time.Time
	var firstInLine *monitorContext // 保存队列的原第一个，用于避免多次更新waitCh

	// 在优先队列发生变化时，更新waitCh
	updateWaitCh := func() {
		if mQueue.Len() == 0 {
			firstInLine = nil
		} else if mQueue[0] != firstInLine {
			var originRequestId string
			if firstInLine != nil {
				originRequestId = firstInLine.requestId
			}
			firstInLine = mQueue[0]
			waitTime := firstInLine.monitorEndTime.Sub(time.Now())
			waitCh = time.After(waitTime)
			m.logger.Printf("监控队列第一个更新为进程组%s，等待时间%s，原为%s\n",
				firstInLine.requestId, waitTime.String(), originRequestId)
		}
	}

	doAddMonitoringProcess := func(rq *Request) error {
		m.logger.Printf("正在将进程组%s加入监控队列\n", rq.requestId)
		pidListPointer := utils.MallocCPidList(rq.pidList)
		fileName := fmt.Sprintf("%s.csv", rq.requestId)
		cName := C.CString(fileName)
		var cCtx *C.struct_ProcessMonitorContext
		res := int(C.rm_monitor_add_process_group(m.pMonitor, (*C.pid_t)(pidListPointer), C.int(len(rq.pidList)),
			cName, &cCtx))
		if res != 0 {
			m.logger.Printf("添加进程组%s失败，返回码为%d\n", rq.requestId, res)
			return fmt.Errorf("添加进程组%s失败，返回码为%d\n", rq.requestId, res)
		}

		heap.Push(&mQueue, &monitorContext{
			requestId:      rq.requestId,
			pidList:        rq.pidList,
			monitorEndTime: time.Now().Add(rq.duration),
			onFinish:       rq.onFinish,
			onError:        rq.onError,
			cContext:       cCtx,
			fileName:       fileName,
		})

		updateWaitCh()
		return nil
	}

	waitingLineToMonitoringLine := func() {
		for len(waitingRequestQueue) > 0 {
			waiting := waitingRequestQueue[0]
			waitingRequestQueue = waitingRequestQueue[1:]
			m.logger.Printf("将等待队列中的进程组%s加入监控队列", waiting.requestId)
			err := doAddMonitoringProcess(waiting)
			if err != nil {
				if waiting.onError != nil {
					go waiting.onError(waiting.requestId, waiting.pidList, err)
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
			m.logger.Printf("进程组%s监控时间结束，正在回收资源\n", first.requestId)
			// 队头弹出之后，从等待队列插入一个
			waitingLineToMonitoringLine()

			updateWaitCh()
			res := C.rm_monitor_remove_process_group(m.pMonitor, first.cContext)
			if res != 0 {
				m.logger.Printf("系统监控移除进程组%s失败，返回值为%d\n", first.requestId, int(res))
			}
			if first.onFinish != nil {
				go first.onFinish(first.requestId, first.pidList, first.fileName)
			}

		case rq := <-m.requestCh:
			m.logger.Printf("接收到监控进程组%s的请求，监控时长%s\n", rq.requestId, rq.duration.String())

			if mQueue.Len() >= m.maxRmid {
				m.logger.Printf("监控队列已满，进程组%s加入等待队列\n", rq.requestId)
				waitingRequestQueue = append(waitingRequestQueue, rq)
				continue outerLoop
			}
			// 检查是否有重复的进程组
			for _, ctx := range mQueue {
				if ctx.requestId == rq.requestId {
					rq.onError(rq.requestId, rq.pidList, ErrDuplicate)
					continue outerLoop
				}
			}
			for _, wrq := range waitingRequestQueue {
				if wrq.requestId == rq.requestId {
					rq.onError(rq.requestId, rq.pidList, ErrDuplicate)
					continue outerLoop
				}
			}

			err := doAddMonitoringProcess(rq)
			if err != nil && rq.onError != nil {
				go rq.onError(rq.requestId, rq.pidList, err)
			}

		case removeId := <-m.removePidCh:
			m.logger.Printf("接收到移除监控进程组%s的请求\n", removeId)
			// 检查当前正在监控的队列
			for i, ctx := range mQueue {
				if ctx.requestId == removeId {
					m.logger.Printf("移除在监控队列中的进程组%s\n", removeId)
					removed := heap.Remove(&mQueue, i).(*monitorContext)
					if removed.onError != nil {
						go removed.onError(removed.requestId, removed.pidList, ErrRemoved)
					}
					waitingLineToMonitoringLine()
					continue outerLoop
				}
			}
			// 检查等待中的队列
			for i := 0; i < len(waitingRequestQueue); i++ {
				if waitingRequestQueue[i].requestId == removeId {
					m.logger.Printf("移除在等待队列中的进程组%s\n", removeId)
					removed := waitingRequestQueue[i]
					go removed.onError(removed.requestId, removed.pidList, ErrRemoved)
					waitingRequestQueue = append(waitingRequestQueue[:i], waitingRequestQueue[i+1:]...)
					continue outerLoop
				}
			}
			m.logger.Printf("无法移除，目前没有监控进程组%s\n", removeId)

		case <-ctx.Done():
			m.logger.Println("正在退出")
			close(m.requestCh)
			for mQueue.Len() > 0 {
				ctx := heap.Pop(&mQueue).(*monitorContext)
				C.rm_monitor_remove_process_group(m.pMonitor, ctx.cContext)
			}
			C.rm_monitor_destroy(m.pMonitor)

			break outerLoop
		}
	}
	m.wg.Done()
}
