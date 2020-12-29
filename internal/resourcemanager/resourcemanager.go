package resourcemanager

import (
	"context"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/resourcemonitor"
	"log"
	"os"
)

type ResourceManager interface {
	Run() error // 同步运行函数
}

type Config struct {
	MonitorConfig resourcemonitor.Config
	Watcher       ProcessGroupWatcher
}

type ProcessGroupCondition string

var (
	ProcessGroupStatusAdd    ProcessGroupCondition = "add"
	ProcessGroupStatusUpdate ProcessGroupCondition = "update"
	ProcessGroupStatusRemove ProcessGroupCondition = "remove"
)

type ProcessGroupStatus struct {
	Group  core.ProcessGroup
	Status ProcessGroupCondition
}

type ProcessGroupWatcher interface {
	Watch() <-chan *ProcessGroupStatus
	StopWatch(ch <-chan *ProcessGroupStatus)
}

type impl struct {
	watcher ProcessGroupWatcher
	monitor resourcemonitor.Monitor
	logger  *log.Logger
}

var _ ResourceManager = &impl{}

func New(config *Config) (ResourceManager, error) {
	m := resourcemonitor.New(context.Background(), resourcemonitor.DefaultConfig())

	return &impl{
		watcher: config.Watcher,
		monitor: m,
		logger:  log.New(os.Stdout, "ResourceManager", log.LstdFlags|log.Lshortfile|log.Lmsgprefix),
	}, nil
}

func (i *impl) Run() error {
	panic("implement me")
}
