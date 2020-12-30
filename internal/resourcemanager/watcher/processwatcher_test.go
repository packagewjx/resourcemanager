package watcher

import (
	"github.com/mitchellh/go-ps"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/stretchr/testify/assert"
	"os/exec"
	"testing"
)

type myProcess struct {
	pid        int
	ppid       int
	executable string
}

func (m myProcess) Pid() int {
	return m.pid
}

func (m myProcess) PPid() int {
	return m.ppid
}

func (m myProcess) Executable() string {
	return m.executable
}

func TestProcessUnionSet(t *testing.T) {
	processList := []ps.Process{
		myProcess{
			pid:        1,
			ppid:       1,
			executable: "init",
		},
		myProcess{
			pid:        2,
			ppid:       1,
			executable: "bash",
		},
		myProcess{
			pid:        3,
			ppid:       2,
			executable: "ps",
		},
		myProcess{
			pid:        4,
			ppid:       1,
			executable: "noploop",
		},
		myProcess{
			pid:        5,
			ppid:       1,
			executable: "nginx",
		},
		myProcess{
			pid:        6,
			ppid:       5,
			executable: "nginx-worker",
		},
		myProcess{
			pid:        7,
			ppid:       5,
			executable: "nginx-worker",
		},
		myProcess{
			pid:        8,
			ppid:       6,
			executable: "nginx-worker-child",
		},
	}

	set := &processUnionSet{
		processMap: make(map[int]*processUnionSetEntry),
		targetCmd:  []string{"noploop", "nginx"},
	}

	family := set.update(processList)
	assert.Equal(t, 2, len(family))
	group := family[5]
	assert.NotNil(t, group)
	assert.Equal(t, 4, len(group.Pid))
	group = family[4]
	assert.NotNil(t, group)
	assert.Equal(t, 1, len(group.Pid))

	// 更新一次，加入新的进程，删去其中一个
	processList = []ps.Process{
		myProcess{
			pid:        1,
			ppid:       1,
			executable: "init",
		},
		myProcess{
			pid:        2,
			ppid:       1,
			executable: "bash",
		},
		myProcess{
			pid:        9,
			ppid:       2,
			executable: "ls",
		},
		myProcess{
			pid:        5,
			ppid:       1,
			executable: "nginx",
		},
		myProcess{
			pid:        6,
			ppid:       5,
			executable: "nginx-worker",
		},
		myProcess{
			pid:        10,
			ppid:       1,
			executable: "nginx",
		},
		myProcess{
			pid:        11,
			ppid:       10,
			executable: "nginx-worker",
		},
	}

	family = set.update(processList)
	assert.Equal(t, 2, len(family))
	group = family[5]
	assert.NotNil(t, group)
	assert.Equal(t, 2, len(group.Pid))
	group = family[10]
	assert.NotNil(t, group)
	assert.Equal(t, 2, len(group.Pid))
}

func TestDiffFamily(t *testing.T) {
	oldFamily := map[int]*core.ProcessGroup{
		1: {
			Id:  "init",
			Pid: []int{1, 5, 7},
		},
		2: {
			Id:  "kworker",
			Pid: []int{2, 3, 4},
		},
		6: {
			Id:  "bash",
			Pid: []int{6},
		},
	}

	newFamily := map[int]*core.ProcessGroup{
		1: {
			Id:  "init",
			Pid: []int{1, 5},
		},
		2: {
			Id:  "kworker",
			Pid: []int{2, 3, 4},
		},
		10: {
			Id:  "noploop",
			Pid: []int{10},
		},
	}

	diff := diffFamily(oldFamily, newFamily)
	assert.Equal(t, 3, len(diff))
	for _, status := range diff {
		if status.Group.Id == "init" {
			assert.Equal(t, ProcessGroupStatusUpdate, status.Status)
		} else if status.Group.Id == "bash" {
			assert.Equal(t, ProcessGroupStatusRemove, status.Status)
		} else if status.Group.Id == "noploop" {
			assert.Equal(t, ProcessGroupStatusAdd, status.Status)
		}
	}
}

func TestWatcher(t *testing.T) {
	go func() {
		for i := 0; i < 3; i++ {
			_ = exec.Command("sleep", "1").Run()
		}
	}()
	watcher := NewProcessWatcher([]string{"sleep"})
	ch := watcher.Watch()
	for i := 0; i < 6; i++ {
		status := <-ch
		t.Log(status)
	}
	watcher.StopWatch(ch)
}
