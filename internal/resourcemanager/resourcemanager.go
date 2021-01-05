package resourcemanager

import (
	"context"
	"github.com/packagewjx/resourcemanager/internal/classifier"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/resourcemanager/watcher"
	"github.com/packagewjx/resourcemanager/internal/sampler/perf"
	"log"
	"os"
	"os/signal"
	"syscall"
)

type ResourceManager interface {
	Run() error // 同步运行函数
}

type Config struct {
	Watcher watcher.ProcessGroupWatcher
}

type groupCharacteristic struct {
	group             *core.ProcessGroup
	statRecord        *perf.PerfStatResult
	memCharacteristic classifier.MemoryCharacteristic
	rth               []int
}

type impl struct {
	watcher    watcher.ProcessGroupWatcher
	classifier classifier.Classifier
	gcMap      map[string]*groupCharacteristic
	logger     *log.Logger
	ctx        context.Context
	cancelFunc context.CancelFunc
}

var _ ResourceManager = &impl{}

func New(config *Config) (ResourceManager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	return &impl{
		watcher:    config.Watcher,
		gcMap:      make(map[string]*groupCharacteristic),
		logger:     log.New(os.Stdout, "ResourceManager", log.LstdFlags|log.Lshortfile|log.Lmsgprefix),
		ctx:        ctx,
		cancelFunc: cancel,
	}, nil
}

func (i *impl) gracefulShutdown() {
	panic("implement me")
}

func (i *impl) shutdownNow() {
	panic("implement me")
}

func (i *impl) handleProcessStatus(status *watcher.ProcessGroupStatus) {
	panic("implement me")
}

func (i *impl) Run() error {
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
				os.Exit(0)
			} else if sig == syscall.SIGKILL {
				i.logger.Println("接收到中止信号，正在强制退出")
				i.shutdownNow()
				os.Exit(1)
			} else {
				i.logger.Printf("接收到信号%v，不处理", sig)
				signal.Reset(sig)
				continue
			}
		case processStatus := <-watchChannel:
			i.logger.Println("接收到进程组新状态：ID %s ，状态 %s ，Pid列表： %v", processStatus.Group.Id,
				watcher.ProcessGroupConditionDisplayName[processStatus.Status], processStatus.Group.Pid)
			i.handleProcessStatus(processStatus)
		}
	}

}
