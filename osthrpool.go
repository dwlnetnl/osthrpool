// Package osthrpool provides a pool of locked OS threads.
package osthrpool

import (
	"container/heap"
	"runtime"
	"sync"
	"time"
)

// Task represents a task that requires to be run on a locked OS thread.
type Task func()

// Pool represents a pool of locked OS threads.
type Pool struct {
	maxSize int
	timeout time.Duration
	workers queue
	mu      sync.Mutex // protects workers
	exit    chan *worker

	// InitFn is called by a worker just before it starts the work loop. The
	// goroutine is locked to an OS thread when InitFn is called.
	InitFn func()

	// ExitFn is called by a worker just before it exits. The goroutine is still
	// locked to an OS thread when ExitFn is called.
	ExitFn func()
}

// New returns a pool of locked OS threads that grows to it's maximum size and
// shrinks automatically depending on the load. A thread automatically unlocks
// itself after a timeout if there are no more tasks to process.
func New(maxSize int, timeout time.Duration) *Pool {
	return &Pool{
		maxSize: maxSize,
		timeout: timeout,
		workers: make(queue, 0, maxSize),
		exit:    make(chan *worker, maxSize),
	}
}

// Execute executes the given task on a locked OS thread.
func (p *Pool) Execute(t Task) {
	p.mu.Lock()
	p.startWorker()
	w := p.getWorker()
	p.mu.Unlock()

	w.tasks <- t // send task for execution
	<-w.done     // wait on result of task

	p.putWorker(w)
}

// startWorker starts a worker if the pool has not reached it's maximum number
// of workers.
func (p *Pool) startWorker() {
	if p.workers.Len() < int(p.maxSize) {
		if p.workers.Len() == 0 {
			// Start background process that collects terminated workers and removes
			// them from the pool.
			go p.collectWorkers()
		}

		w := newWorker(p.exit, p.timeout, p.InitFn, p.ExitFn)
		heap.Push(&p.workers, w)
		go w.work()
	}
}

// collectWorkers is a background process that remove terminated workers from
// the worker pool. This stops itself if there are no more active workers left.
func (p *Pool) collectWorkers() {
	for {
		select {
		case w := <-p.exit:
			p.mu.Lock()

			heap.Remove(&p.workers, w.index)

			if p.workers.Len() == 0 {
				p.mu.Unlock()
				return
			}

			p.mu.Unlock()
		}
	}
}

// getWorker returns the lightest loaded worker from the worker pool.
func (p *Pool) getWorker() *worker {
	w := heap.Pop(&p.workers).(*worker)
	w.pending++
	// Store worker back into queue to adjust the load of the worker.
	heap.Push(&p.workers, w)
	return w
}

// putWorker puts a worker back in the worker pool.
func (p *Pool) putWorker(w *worker) {
	p.mu.Lock()
	defer p.mu.Unlock()
	w.pending--
	// Reorder the queue based on the load of the workers.
	heap.Fix(&p.workers, w.index)
}

type worker struct {
	timeout time.Duration
	exit    chan *worker
	tasks   chan Task
	done    chan struct{}
	index   int
	pending int
	initFn  func()
	exitFn  func()
}

func newWorker(exit chan *worker, timeout time.Duration, initFn, exitFn func()) *worker {
	return &worker{
		exit:    exit,
		timeout: timeout,
		tasks:   make(chan Task),
		done:    make(chan struct{}),
		initFn:  initFn,
		exitFn:  exitFn,
	}
}

func (w *worker) work() {
	runtime.LockOSThread()

	if w.initFn != nil {
		w.initFn()
	}

	for {
		select {
		case task := <-w.tasks:
			task()
			w.done <- struct{}{}
		case <-time.After(w.timeout):
			if w.exitFn != nil {
				w.exitFn()
			}

			runtime.UnlockOSThread()
			w.exit <- w
			return
		}
	}
}

// queue is a priority queue of workers with the lightest loaded worker as head
// of the queue.
type queue []*worker

func (q queue) Len() int           { return len(q) }
func (q queue) Less(i, j int) bool { return q[i].pending < q[j].pending }

func (q queue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}

func (q *queue) Push(item interface{}) {
	s := *q
	n := len(s)
	s = s[0 : n+1]
	w := item.(*worker)
	s[n] = w
	w.index = n
	*q = s
}

func (q *queue) Pop() interface{} {
	s := *q
	*q = s[0 : len(s)-1]
	w := s[len(s)-1]
	w.index = -1 // for safety
	return w
}
