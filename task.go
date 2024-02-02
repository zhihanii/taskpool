package gopool

import (
	"context"
	"sync"
)

// 任务池
var taskPool sync.Pool

func init() {
	taskPool.New = func() any {
		return &task{}
	}
}

type task struct {
	ctx context.Context
	f   func()
}

func (t *task) set(ctx context.Context, f func()) {
	t.ctx = ctx
	t.f = f
}

func (t *task) zero() {
	t.ctx = nil
	t.f = nil
}

func (t *task) recycle() {
	t.zero()
	taskPool.Put(t)
}
