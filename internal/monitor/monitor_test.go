package monitor

import (
	"bytes"
	"context"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/stretchr/testify/assert"
	"log"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

type testBufWriter bytes.Buffer

func (b *testBufWriter) Write(p []byte) (n int, err error) {
	_, _ = os.Stdout.Write(p)
	return (*bytes.Buffer)(b).Write(p)
}

func TestMain(m *testing.M) {
	if code := core.LibInit(); code != 0 {
		fmt.Printf("启动失败，返回码为%d\n", code)
		os.Exit(1)
	}

	retVal := m.Run()
	core.LibFinalize()
	os.Exit(retVal)
}

func TestAddProcess(t *testing.T) {
	monitor, err := NewMonitor()
	if err != nil {
		t.Fatal(err)
	}
	// hack
	buf := &bytes.Buffer{}
	monitor.(*monitorImpl).logger = log.New((*testBufWriter)(buf), "", 0)

	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	monitor.Start(ctx)
	monitor.AddProcess(&Request{
		pid:      1,
		duration: time.Hour,
		onFinish: nil,
		onError: func(pid uint, err error) {
			assert.FailNow(t, err.Error())
		},
	})
	<-time.After(100 * time.Millisecond) // 运行一会
	monitor.ShutDownNow()

	out := buf.String()
	assert.NotEqual(t, -1, strings.Index(buf.String(), "正在启动"))
	assert.NotEqual(t, -1, strings.Index(out, "接收到监控进程1的请求"))
	assert.NotEqual(t, -1, strings.Index(out, "正在退出"))
}

func TestStart(t *testing.T) {
	monitor, err := NewMonitor()
	if err != nil {
		t.Fatal(err)
	}
	// hack
	buf := &bytes.Buffer{}
	monitor.(*monitorImpl).logger = log.New((*testBufWriter)(buf), "", 0)

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Millisecond)
	monitor.Start(ctx)

	monitor.ShutDownNow()
	assert.NotEqual(t, -1, strings.Index(buf.String(), "正在退出"))
}

func TestMonitorProcess(t *testing.T) {
	monitor, err := NewMonitor()
	if err != nil {
		t.Fatal(err)
	}
	// hack
	buf := &bytes.Buffer{}
	monitor.(*monitorImpl).logger = log.New((*testBufWriter)(buf), "", 0)

	ctx, cancel := context.WithCancel(context.Background())
	monitor.Start(ctx)
	onFinishCalled := false
	monitor.AddProcess(&Request{
		pid:      1,
		duration: 5 * time.Second,
		onFinish: func(pid uint, file string) {
			onFinishCalled = true
		},
		onError: func(pid uint, err error) {
			assert.FailNow(t, err.Error())
		},
	})

	<-time.After(7 * time.Second)
	assert.True(t, onFinishCalled)
	cancel()
	monitor.ShutDownNow()

	// 检查log是否正确
	runLog := buf.String()
	assert.NotEqual(t, -1, strings.Index(runLog, "正在退出"))
	assert.NotEqual(t, -1, strings.Index(runLog, "进程1监控时间结束"))
	assert.NotEqual(t, -1, strings.Index(runLog, "监控队列第一个更新为进程1"))
	assert.NotEqual(t, -1, strings.Index(runLog, "接收到监控进程1的请求"))
}

func TestNonExistProcess(t *testing.T) {
	monitor, err := NewMonitor()
	if err != nil {
		t.Fatal(err)
	}
	monitor.Start(context.Background())
	errored := false
	monitor.AddProcess(&Request{
		pid:      10000000,
		duration: time.Second,
		onFinish: nil,
		onError: func(pid uint, err error) {
			errored = true
		},
	})
	<-time.After(100 * time.Millisecond) // 异步，需要等待一会
	assert.True(t, errored)

	monitor.ShutDownNow()
}

func TestAddProcessExceedRMID(t *testing.T) {
	monitor, err := NewMonitor()
	if err != nil {
		t.Fatal(err)
	}

	finished := 0
	onFinish := func(pid uint, file string) {
		_ = os.Remove(file)
		finished++
	}
	errored := false
	onError := func(pid uint, err error) {
		fmt.Printf("%v\n", err)
		errored = true
	}

	monitor.Start(context.Background())
	// hack
	monitor.(*monitorImpl).maxRmid = 10

	const numPid = 20
	currPid := 1
	for i := 0; i < numPid; i++ {
		for syscall.Kill(currPid, 0) != nil {
			currPid++
		}
		monitor.AddProcess(&Request{
			pid:      uint(currPid),
			duration: time.Second * 3,
			onFinish: onFinish,
			onError:  onError,
		})
		currPid++
	}
	if errored {
		assert.FailNow(t, "添加失败")
	}

	<-time.After(10 * time.Second)
	monitor.ShutDownNow()

	assert.Equal(t, numPid, finished)
}
