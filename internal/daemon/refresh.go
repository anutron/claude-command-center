package daemon

import (
	"log"
	"sync"
	"time"
)

type refreshLoop struct {
	fn       func() error
	interval time.Duration
	mu       sync.Mutex
	running  bool
	stopCh   chan struct{}
}

func newRefreshLoop(fn func() error, interval time.Duration) *refreshLoop {
	return &refreshLoop{fn: fn, interval: interval, stopCh: make(chan struct{})}
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
				r.run()
			case <-r.stopCh:
				return
			}
		}
	}()
}

func (r *refreshLoop) stop() {
	close(r.stopCh)
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
	}
	return err
}
