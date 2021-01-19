package resourcemanager

import (
	"github.com/packagewjx/resourcemanager/internal/resourcemanager/watcher"
	"log"
	"sync"
)

type processGroupMap sync.Map

func (m *processGroupMap) get(name string) (*processGroupContext, bool) {
	val, ok := ((*sync.Map)(m)).Load(name)
	if !ok {
		return nil, false
	} else {
		return val.(*processGroupContext), ok
	}
}

func (m *processGroupMap) store(p *processGroupContext) {
	(*sync.Map)(m).Store(p.group.Id, p)
}

func (m *processGroupMap) remove(name string) {
	(*sync.Map)(m).Delete(name)
}

func (m *processGroupMap) traverse(s func(name string, group *processGroupContext) bool) {
	(*sync.Map)(m).Range(func(key, value interface{}) bool {
		if value == nil {
			log.Printf("警告： processGroupMap 键为 %s 的值为空", key)
			return true
		}
		return s(key.(string), value.(*processGroupContext))
	})
}

type ResourceManager interface {
	Run() error // 同步运行函数
}

type Config struct {
	Watcher watcher.ProcessGroupWatcher
}
