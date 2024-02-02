package gopool

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

//tasks

var (
	taskChanCap = func() int {
		if runtime.GOMAXPROCS(0) == 1 {
			return 0
		}
		return 1
	}()

	defaultPool, _ = NewPool(DefaultPoolCapacity, WithMaxBlockingTasks(10), WithNonBlocking())
)

const (
	DefaultPoolCapacity    = 10000
	DefaultExpiredDuration = time.Second

	nowTimerInterval = 500 * time.Millisecond
)

const (
	opened = iota
	closed
)

// Submit 提交task
func Submit(ctx context.Context, task func()) error {
	return defaultPool.Submit(ctx, task)
}

type Pool struct {
	o *Options

	capacity int32
	running  int32 //正在执行task的worker数量

	mux sync.Mutex

	workers *workerPool //正在执行task的worker

	state int32

	cond *sync.Cond

	freeWorkers sync.Pool //空闲的worker

	waiting int32

	now atomic.Value //当前时间

	purgeDone int32
	stopPurge context.CancelFunc

	timerDone int32
	stopTimer context.CancelFunc
}

func NewPool(capacity int, opts ...Option) (*Pool, error) {
	if capacity <= 0 {
		capacity = -1
	}

	o := NewOptions()
	o.Apply(opts...)

	p := &Pool{
		o:        o,
		capacity: int32(capacity),
	}
	p.freeWorkers.New = func() any {
		return &worker{
			pool: p,
			task: make(chan *task, taskChanCap),
		}
	}
	p.workers = newWorkerPool(capacity)
	p.cond = sync.NewCond(&p.mux)

	p.startPurge()
	p.startTimer()

	return p, nil
}

func (p *Pool) startPurge() {
	var ctx context.Context
	ctx, p.stopPurge = context.WithCancel(context.Background())
	go p.purgeExpiredWorkers(ctx)
}

func (p *Pool) startTimer() {
	var ctx context.Context
	ctx, p.stopTimer = context.WithCancel(context.Background())
	go p.timer(ctx)
}

func (p *Pool) Submit(ctx context.Context, f func()) error {
	t := taskPool.Get().(*task)
	t.set(ctx, f)
	if w := p.getWorker(); w != nil {
		w.put(t)
		return nil
	}
	return ErrPoolIsOverload
}

func (p *Pool) IsClosed() bool {
	return atomic.LoadInt32(&p.state) == closed
}

func (p *Pool) Release() {
	if !atomic.CompareAndSwapInt32(&p.state, opened, closed) {
		return
	}
	p.mux.Lock()
	p.workers.reset()
	p.mux.Unlock()
	p.cond.Broadcast()
}

func (p *Pool) Cap() int {
	return int(atomic.LoadInt32(&p.capacity))
}

func (p *Pool) Running() int {
	return int(atomic.LoadInt32(&p.running))
}

func (p *Pool) Waiting() int {
	return int(atomic.LoadInt32(&p.waiting))
}

func (p *Pool) addRunning(delta int) {
	atomic.AddInt32(&p.running, int32(delta))
}

func (p *Pool) addWaiting(delta int) {
	atomic.AddInt32(&p.waiting, int32(delta))
}

func (p *Pool) nowTime() time.Time {
	return p.now.Load().(time.Time)
}

func (p *Pool) getWorker() (w *worker) {
	spawnWorker := func() {
		w = p.freeWorkers.Get().(*worker)
		w.run()
	}

	p.mux.Lock()
	w = p.workers.get()
	if w != nil {
		p.mux.Unlock()
	} else if capacity := p.Cap(); capacity == -1 || capacity > p.Running() {
		p.mux.Unlock()
		spawnWorker()
	} else {
		if p.o.Nonblocking {
			p.mux.Unlock()
			return
		}
		//等待获取worker
	retry:
		if p.o.MaxBlockingTasks != 0 && p.Waiting() >= p.o.MaxBlockingTasks {
			p.mux.Unlock()
			return
		}

		p.addWaiting(1)
		p.cond.Wait()
		p.addWaiting(-1)

		if p.IsClosed() {
			p.mux.Unlock()
			return
		}

		var n int
		if n = p.Running(); n == 0 {
			p.mux.Unlock()
			spawnWorker()
			return
		}
		if w = p.workers.get(); w == nil {
			if n < p.Cap() {
				p.mux.Unlock()
				spawnWorker()
				return
			}
			goto retry
		}
		p.mux.Unlock()
	}
	return
}

func (p *Pool) putWorker(w *worker) bool {
	if capacity := p.Cap(); (capacity > 0 && p.Running() > capacity) || p.IsClosed() {
		p.cond.Broadcast()
		return false
	}
	w.lastUsed = p.nowTime()
	p.mux.Lock()
	if p.IsClosed() {
		p.mux.Unlock()
		return false
	}
	if err := p.workers.put(w); err != nil {
		p.mux.Unlock()
		return false
	}
	p.cond.Signal()
	p.mux.Unlock()
	return true
}

// 清理长时间空闲的worker
func (p *Pool) purgeExpiredWorkers(ctx context.Context) {
	ticker := time.NewTicker(p.o.ExpiredDuration)

	defer func() {
		ticker.Stop()
		atomic.StoreInt32(&p.purgeDone, 1)
	}()

	var expiredWorkers []*worker

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		p.mux.Lock()
		//获取过期的worker
		expiredWorkers = p.workers.expiredWorkers(p.o.ExpiredDuration)
		p.mux.Unlock()

		for i := range expiredWorkers {
			expiredWorkers[i].finish() //结束
			expiredWorkers[i] = nil
		}
	}
}

func (p *Pool) timer(ctx context.Context) {
	ticker := time.NewTicker(nowTimerInterval)

	defer func() {
		ticker.Stop()
		atomic.StoreInt32(&p.timerDone, 1)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		p.now.Store(time.Now())
	}
}
