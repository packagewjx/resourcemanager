package rthsample

import "context"

type RthSampler interface {
	SampleCommand(ctx context.Context, cmd string, args []string, maxTime int) chan *Result
	SampleProcess(ctx context.Context, pid int, maxTime int) chan *Result
}

type Result struct {
	Rth   map[int][]int
	Error error
}
