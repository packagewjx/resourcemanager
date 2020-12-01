package resourcemonitor

import (
	"bytes"
	"context"
	"encoding/csv"
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

func mustCreateWithLog() (Monitor, *bytes.Buffer) {
	monitor, err := New(1000)
	if err != nil {
		panic(err)
	}
	// hack
	buf := &bytes.Buffer{}
	monitor.(*monitorImpl).logger = log.New((*testBufWriter)(buf), "", 0)
	return monitor, buf
}

func TestMain(m *testing.M) {
	if err := core.LibInit(); err != nil {
		fmt.Printf("%s\n", err.Error())
		os.Exit(1)
	}

	retVal := m.Run()
	_ = core.LibFinalize()
	os.Exit(retVal)
}

func TestAddProcess(t *testing.T) {
	monitor, buf := mustCreateWithLog()

	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	monitor.Start(ctx)
	monitor.AddProcess(&Request{
		requestId: "test",
		pidList:   []int{1},
		duration:  time.Hour,
		onFinish:  nil,
		onError: func(requestId string, pid []int, err error) {
			assert.FailNow(t, err.Error())
		},
	})
	<-time.After(100 * time.Millisecond) // 运行一会
	monitor.ShutDownNow()

	out := buf.String()
	assert.NotEqual(t, -1, strings.Index(buf.String(), "正在启动"))
	assert.NotEqual(t, -1, strings.Index(out, "正在将进程组test加入监控队列"))
	assert.NotEqual(t, -1, strings.Index(out, "正在退出"))
}

func TestStart(t *testing.T) {
	monitor, buf := mustCreateWithLog()

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Millisecond)
	monitor.Start(ctx)

	monitor.ShutDownNow()
	assert.NotEqual(t, -1, strings.Index(buf.String(), "正在退出"))
}

func TestMonitorProcess(t *testing.T) {
	println("这应该处于先的")
	monitor, buf := mustCreateWithLog()

	ctx, cancel := context.WithCancel(context.Background())
	monitor.Start(ctx)
	onFinishCalled := false
	monitor.AddProcess(&Request{
		requestId: "test",
		pidList:   []int{os.Getpid()},
		duration:  5 * time.Second,
		onFinish: func(requestId string, pid []int, pqosFile, rthFile string) {
			f, err := os.Open(pqosFile)
			assert.NoError(t, err)
			records, err := csv.NewReader(f).ReadAll()
			assert.NoError(t, err)
			assert.NotEqual(t, 0, len(records))
			onFinishCalled = true
			_ = os.Remove(pqosFile)
			_ = os.Remove(rthFile)
		},
		onError: func(requestId string, pid []int, err error) {
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
	assert.NotEqual(t, -1, strings.Index(runLog, "正在将进程组test加入监控队列"))
	assert.NotEqual(t, -1, strings.Index(runLog, "监控队列第一个更新为进程组test"))
}

func TestNonExistProcess(t *testing.T) {
	monitor, err := New(1000)
	if err != nil {
		t.Fatal(err)
	}
	monitor.Start(context.Background())
	errored := false
	monitor.AddProcess(&Request{
		requestId: "test",
		pidList:   []int{10000000},
		duration:  time.Second,
		onFinish:  nil,
		onError: func(requestId string, pid []int, err error) {
			errored = true
		},
	})
	<-time.After(100 * time.Millisecond) // 异步，需要等待一会
	assert.True(t, errored)

	monitor.ShutDownNow()
}

func TestAddProcessExceedRMID(t *testing.T) {
	monitor, err := New(1000)
	if err != nil {
		t.Fatal(err)
	}

	finished := 0
	onFinish := func(requestId string, pid []int, pqosFile, rthFile string) {
		f, err := os.Open(pqosFile)
		assert.NoError(t, err)
		records, err := csv.NewReader(f).ReadAll()
		assert.NoError(t, err)
		assert.NotEqual(t, 0, len(records))
		_ = os.Remove(pqosFile)
		_ = os.Remove(rthFile)
		finished++
	}
	errored := false
	onError := func(requestId string, pid []int, err error) {
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
			requestId: fmt.Sprintf("%d", i),
			pidList:   []int{currPid},
			duration:  time.Second * 3,
			onFinish:  onFinish,
			onError:   onError,
		})
		currPid++
	}
	assert.False(t, errored)

	<-time.After(10 * time.Second)
	monitor.ShutDownNow()

	assert.Equal(t, numPid, finished)
}

func TestRemoveProcess(t *testing.T) {
	monitor, err := New(1000)
	if err != nil {
		t.Fatal(err)
	}
	removed := false
	monitor.Start(context.Background())
	monitor.AddProcess(&Request{
		requestId: "func",
		pidList:   []int{1},
		duration:  time.Hour,
		onFinish:  nil,
		onError:   nil,
	})
	monitor.AddProcess(&Request{
		requestId: "test",
		pidList:   []int{os.Getpid()},
		duration:  time.Hour,
		onFinish:  nil,
		onError: func(requestId string, pid []int, err error) {
			removed = true
		},
	})

	<-time.After(200 * time.Millisecond)
	monitor.RemoveProcess("test")
	<-time.After(200 * time.Millisecond)
	monitor.ShutDownNow()
	assert.True(t, removed)
}
