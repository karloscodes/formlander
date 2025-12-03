package ratelimit

import (
	"sync"
	"time"
)

// Limiter provides a lightweight, in-memory rate limiter with per-key windows.
type Limiter struct {
	mu        sync.Mutex
	buckets   map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	count      int
	windowEnds time.Time
}

// NewLimiter constructs a limiter with zeroed state.
func NewLimiter() *Limiter {
	return &Limiter{
		buckets:   make(map[string]*bucket),
		lastSweep: time.Now(),
	}
}

// Allow records an event for the provided key within the supplied window.
// When the number of events exceeds limit, Allow returns false.
func (l *Limiter) Allow(key string, limit int, window time.Duration) bool {
	if limit <= 0 || window <= 0 || key == "" {
		return true
	}

	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.buckets == nil {
		l.buckets = make(map[string]*bucket)
	}

	if b, ok := l.buckets[key]; ok {
		if now.After(b.windowEnds) {
			b.count = 1
			b.windowEnds = now.Add(window)
			return true
		}
		if b.count >= limit {
			return false
		}
		b.count++
		return true
	}

	l.buckets[key] = &bucket{
		count:      1,
		windowEnds: now.Add(window),
	}

	l.pruneLocked(now)
	return true
}

// Reset clears all tracked state.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buckets = make(map[string]*bucket)
	l.lastSweep = time.Now()
}

func (l *Limiter) pruneLocked(now time.Time) {
	if len(l.buckets) == 0 {
		return
	}

	if now.Sub(l.lastSweep) < time.Minute {
		return
	}

	for key, b := range l.buckets {
		if now.After(b.windowEnds) {
			delete(l.buckets, key)
		}
	}

	l.lastSweep = now
}
