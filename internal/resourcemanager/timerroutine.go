package resourcemanager

import (
	"context"
	"time"
)

// 确保短时间内不会多次重复执行的工具类
type timedRoutine struct {
	funcCoolDown    time.Duration
	requestCoolDown time.Duration // 将时间相近的请求合并为一次，以最后一个时间为准
	requestCh       chan struct{}
	f               func()
}

func newTimerRoutine(funcCoolDown, requestCoolDown time.Duration, f func()) *timedRoutine {
	return &timedRoutine{
		funcCoolDown:    funcCoolDown,
		requestCoolDown: requestCoolDown,
		requestCh:       make(chan struct{}, 100),
		f:               f,
	}
}

func (r *timedRoutine) requestRun() {
	r.requestCh <- struct{}{}
}

func (r *timedRoutine) start(ctx context.Context) {
	go func() {
		var lastRun = time.Unix(0, 0)
		var lastRequest = time.Unix(0, 0)
		var coolDownTimer = time.NewTimer(0)
		shouldRun := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-r.requestCh:
				if time.Now().Sub(lastRequest) > r.requestCoolDown {
					shouldRun = true
					if time.Now().Sub(lastRun) > r.funcCoolDown {
						coolDownTimer.Reset(r.requestCoolDown) // 触发执行，将一段时间内的请求合并到一起执行
					}
					lastRequest = time.Now()
				}
			case <-coolDownTimer.C:
				if shouldRun {
					lastRun = time.Now()
					coolDownTimer.Reset(r.funcCoolDown)
					shouldRun = false
					r.f()
				}
			}
		}
	}()
}
