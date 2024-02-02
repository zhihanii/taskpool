package taskpool

import "time"

// 基于循环数组的协程池
type workerPool struct {
	items        []*worker //正常执行task的worker
	expiredItems []*worker //过期的worker
	head         int
	tail         int
	size         int
	isFull       bool
}

func newWorkerPool(size int) *workerPool {
	return &workerPool{
		items: make([]*worker, size),
		size:  size,
	}
}

func (p *workerPool) IsEmpty() bool {
	return p.head == p.tail && !p.isFull
}

func (p *workerPool) Len() int {
	if p.size == 0 {
		return 0
	}
	if p.head == p.tail {
		if p.isFull {
			return p.size
		}
		return 0
	}
	if p.tail > p.head {
		return p.tail - p.head
	}
	return p.size - p.head + p.tail
}

func (p *workerPool) put(w *worker) error {
	if p.size == 0 {
		return ErrPoolIsReleased
	}
	if p.isFull {
		return ErrPoolIsFull
	}
	p.items[p.tail] = w
	p.tail++
	if p.tail == p.size {
		p.tail = 0
	}
	if p.tail == p.head {
		p.isFull = true
	}
	return nil
}

func (p *workerPool) get() *worker {
	if p.IsEmpty() {
		return nil
	}
	w := p.items[p.head]
	p.items[p.head] = nil
	p.head++
	if p.head == p.size {
		p.head = 0
	}
	p.isFull = false
	return w
}

// 返回过期(长时间空闲)的worker列表
// duration是最大的空闲时间
func (p *workerPool) expiredWorkers(duration time.Duration) []*worker {
	//过期时间
	expiredTime := time.Now().Add(-duration)
	//过期worker的索引
	idx := p.search(expiredTime)
	if idx == -1 { //没有过期的worker
		return nil
	}
	p.expiredItems = p.expiredItems[:0]
	if p.head <= idx {
		//head到idx的worker都是过期的
		p.expiredItems = append(p.expiredItems, p.items[p.head:idx+1]...)
		for i := p.head; i < idx+1; i++ {
			p.items[i] = nil
		}
	} else {
		//0到idx及head到数组末尾的worker都是过期的
		p.expiredItems = append(p.expiredItems, p.items[0:idx+1]...)
		p.expiredItems = append(p.expiredItems, p.items[p.head:]...)
		for i := 0; i < idx+1; i++ {
			p.items[i] = nil
		}
		for i := p.head; i < p.size; i++ {
			p.items[i] = nil
		}
	}
	//更新head
	p.head = (idx + 1) % p.size
	if len(p.expiredItems) > 0 {
		p.isFull = false
	}
	return p.expiredItems
}

// 在循环数组中执行二分查找
// 返回最后一个过期的worker的索引
func (p *workerPool) search(expiredTime time.Time) int {
	n := len(p.items)
	//没有过期的worker
	if p.IsEmpty() || expiredTime.Before(p.items[p.head].lastUsed) {
		return -1
	}
	baseL := p.head
	l, r := 0, (p.tail-1-baseL+n)%n
	var mid, tmid int
	for l <= r {
		mid = l + (r-l)/2
		tmid = (mid + baseL + n) % n                    //实际的mid
		if expiredTime.Before(p.items[tmid].lastUsed) { //tmid指向的worker未过期
			r = mid - 1
		} else {
			l = mid + 1
		}
	}
	//返回最后一个过期的worker的索引
	return (r + baseL + n) % n
}

func (p *workerPool) reset() {
	if p.IsEmpty() {
		return
	}

retry:
	if w := p.get(); w != nil {
		w.finish()
		goto retry
	}
	p.items = p.items[:0]
	p.size = 0
	p.head = 0
	p.tail = 0
}
