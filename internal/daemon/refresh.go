package daemon

import (
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type refreshLoop struct {
	fn       func() error
	interval time.Duration
	notify   func() // called after each successful refresh
	mu       sync.Mutex
	running  bool
	paused   atomic.Bool // when true, ticker skips refresh runs
	stopCh   chan struct{}
	stopOnce sync.Once
}

func newRefreshLoop(fn func() error, interval time.Duration, notify func()) *refreshLoop {
	return &refreshLoop{fn: fn, interval: interval, notify: notify, stopCh: make(chan struct{})}
}

func (r *refreshLoop) start() {
	if r.interval <= 0 || r.fn == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if !r.paused.Load() {
					r.run()
				}
			case <-r.stopCh:
				return
			}
		}
	}()
}

func (r *refreshLoop) stop() {
	r.stopOnce.Do(func() {
		close(r.stopCh)
	})
}

func (r *refreshLoop) run() error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return nil
	}
	r.running = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()
	if r.fn == nil {
		return nil
	}
	err := r.fn()
	if err != nil {
		log.Printf("refresh error: %v", err)
	} else if r.notify != nil {
		r.notify()
	}
	return err
}
