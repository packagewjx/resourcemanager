package resourcemanager

import (
	"context"
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/packagewjx/resourcemanager/internal/classifier"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/resourcemanager/watcher"
	"github.com/packagewjx/resourcemanager/internal/sampler/perf"
	"log"
	"sync"
)

type processGroupMap sync.Map

type processGroupContext struct {
	group            *core.ProcessGroup
	processes        map[int]*processCharacteristic
	state            processGroupState
	cancelManageFunc context.CancelFunc
}

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

func (m *processGroupMap) getProgramMetricList() []*algorithm.ProgramMetric {
	res := make([]*algorithm.ProgramMetric, 0, 10)
	m.traverse(func(name string, group *processGroupContext) bool {
		for pid, characteristic := range group.processes {
			res = append(res, &algorithm.ProgramMetric{
				Pid:      pid,
				MRC:      characteristic.mrc,
				PerfStat: characteristic.perfStat,
			})
		}
		return true
	})
	return res
}

type ResourceManager interface {
	Run() error // 同步运行函数
}

type Config struct {
	Watcher watcher.ProcessGroupWatcher
}

type processCharacteristic struct {
	pid            int
	characteristic classifier.MemoryCharacteristic
	mrc            []float32
	perfStat       *perf.StatResult
}

func (p *processCharacteristic) Clone() core.Cloneable {
	newMrc := make([]float32, len(p.mrc))
	copy(newMrc, p.mrc)
	return &processCharacteristic{
		pid:            p.pid,
		characteristic: p.characteristic,
		mrc:            newMrc,
		perfStat:       p.perfStat.Clone().(*perf.StatResult),
	}
}
