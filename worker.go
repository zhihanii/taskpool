package taskpool

import "time"

type worker struct {
	pool *Pool

	task chan *task

	lastUsed time.Time
}

func (w *worker) run() {
	w.pool.addRunning(1)
	go func() {
		defer func() {
			w.pool.addRunning(-1)
			w.pool.freeWorkers.Put(w)
			w.pool.cond.Signal()
		}()

		for t := range w.task {
			if t == nil { //结束
				return
			}
			t.f()
			if ok := w.pool.putWorker(w); !ok {
				return
			}
		}
	}()
}

func (w *worker) put(t *task) {
	w.task <- t
}

func (w *worker) finish() {
	w.task <- nil
}
