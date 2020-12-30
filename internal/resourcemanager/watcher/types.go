package watcher

import "github.com/packagewjx/resourcemanager/internal/core"

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
