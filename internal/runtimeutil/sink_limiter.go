package runtimeutil

import "sync"

type SinkLimiter struct {
	mu       sync.Mutex
	inflight map[string]int
}

func NewSinkLimiter() *SinkLimiter {
	return &SinkLimiter{inflight: map[string]int{}}
}

func EffectiveSinkConcurrency(sinkLimit int, defaultSinkConcurrency int) int {
	if sinkLimit > 0 {
		return sinkLimit
	}
	return EffectiveConcurrency(defaultSinkConcurrency)
}

func (l *SinkLimiter) TryAcquire(sinkID string, limit int) bool {
	limit = EffectiveConcurrency(limit)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[sinkID] >= limit {
		return false
	}
	l.inflight[sinkID]++
	return true
}

func (l *SinkLimiter) Release(sinkID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[sinkID] <= 1 {
		delete(l.inflight, sinkID)
		return
	}
	l.inflight[sinkID]--
}
