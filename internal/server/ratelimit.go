package server

import (
	"sync"
	"time"
)

// rateLimiter is a simple in-memory fixed-window limiter keyed by IP.
type rateLimiter struct {
	mu      sync.Mutex
	perMin  int
	entries map[string]rateEntry
}

type rateEntry struct {
	count   int
	resetAt time.Time
}

func newRateLimiter(perMin int) *rateLimiter {
	return &rateLimiter{
		perMin:  perMin,
		entries: make(map[string]rateEntry),
	}
}

// allow reports whether key may proceed, incrementing its count.
func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	entry, ok := rl.entries[key]
	if !ok || entry.resetAt.Before(now) {
		rl.entries[key] = rateEntry{count: 1, resetAt: now.Add(time.Minute)}
		return true
	}
	entry.count++
	rl.entries[key] = entry
	return entry.count <= rl.perMin
}

// reset clears the count for key (used after a successful login).
func (rl *rateLimiter) reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.entries, key)
}
