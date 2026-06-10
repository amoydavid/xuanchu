package runtimeutil

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSinkLimiterAcquireRelease(t *testing.T) {
	limiter := NewSinkLimiter()
	if !limiter.TryAcquire("sink-a", 1) {
		t.Fatal("first acquire failed")
	}
	if limiter.TryAcquire("sink-a", 1) {
		t.Fatal("second acquire succeeded, want limited")
	}
	limiter.Release("sink-a")
	if !limiter.TryAcquire("sink-a", 1) {
		t.Fatal("acquire after release failed")
	}
}

func TestEffectiveSinkConcurrencyUsesDefaultWhenZero(t *testing.T) {
	if got := EffectiveSinkConcurrency(0, 4); got != 4 {
		t.Fatalf("EffectiveSinkConcurrency(0,4) = %d, want 4", got)
	}
	if got := EffectiveSinkConcurrency(2, 4); got != 2 {
		t.Fatalf("EffectiveSinkConcurrency(2,4) = %d, want 2", got)
	}
}

func TestSinkLimiterBoundsConcurrentAcquire(t *testing.T) {
	limiter := NewSinkLimiter()
	start := make(chan struct{})
	var inflight int32
	var maxInflight int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if limiter.TryAcquire("sink-a", 3) {
				current := atomic.AddInt32(&inflight, 1)
				for {
					seen := atomic.LoadInt32(&maxInflight)
					if current <= seen || atomic.CompareAndSwapInt32(&maxInflight, seen, current) {
						break
					}
				}
				time.Sleep(time.Millisecond)
				atomic.AddInt32(&inflight, -1)
				limiter.Release("sink-a")
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := atomic.LoadInt32(&maxInflight); got > 3 {
		t.Fatalf("max inflight = %d, want <= 3", got)
	}
}
