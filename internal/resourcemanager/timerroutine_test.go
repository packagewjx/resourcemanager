package resourcemanager

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestTimerRoutine(t *testing.T) {
	setTime := make([]time.Time, 0, 10)
	f := func() {
		setTime = append(setTime, time.Now())
	}

	r := newTimerRoutine(time.Second, 50*time.Millisecond, f)
	r.start(context.Background())
	r.requestRun() // 第1次，3次合并，只会执行1次
	r.requestRun()
	r.requestRun()
	<-time.After(1100 * time.Millisecond)
	r.requestRun() // 第二次，立即执行
	<-time.After(500 * time.Millisecond)
	r.requestRun() // 第三次，不会被压缩请求，500ms后才执行

	<-time.After(600 * time.Millisecond)
	assert.Equal(t, 3, len(setTime))
	assert.Greater(t, int64(setTime[1].Sub(setTime[0])), int64(1100*time.Millisecond))
	assert.Greater(t, int64(setTime[2].Sub(setTime[1])), int64(time.Second))
}
