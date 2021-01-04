package watcher

import "github.com/packagewjx/resourcemanager/internal/core"

type ProcessGroupCondition string

var (
	ProcessGroupStatusAdd    ProcessGroupCondition = "add"
	ProcessGroupStatusUpdate ProcessGroupCondition = "update"
	ProcessGroupStatusRemove ProcessGroupCondition = "remove"
)

var ProcessGroupConditionDisplayName = map[ProcessGroupCondition]string{
	ProcessGroupStatusAdd:    "添加",
	ProcessGroupStatusUpdate: "更新",
	ProcessGroupStatusRemove: "移除",
}

type ProcessGroupStatus struct {
	Group  core.ProcessGroup
	Status ProcessGroupCondition
}

type ProcessGroupWatcher interface {
	Watch() <-chan *ProcessGroupStatus
	StopWatch(ch <-chan *ProcessGroupStatus)
}
