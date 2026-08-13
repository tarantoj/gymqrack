package server

import (
	"sync"
	"time"
)

// rateLimiter is a simple in-memory fixed-window limiter keyed by IP.
type rateLimiter struct {
	mu        sync.Mutex
	perMin    int
	entries   map[string]rateEntry
	lastSweep time.Time
}

type rateEntry struct {
	count   int
	resetAt time.Time
}

const (
	// window is the rate-limiting window.
	window = time.Minute
	// sweepInterval bounds how often expired entries are pruned so the map
	// cannot grow without bound.
	sweepInterval = time.Minute
)

func newRateLimiter(perMin int) *rateLimiter {
	return &rateLimiter{
		perMin:  perMin,
		entries: make(map[string]rateEntry),
	}
}

// allow reports whether key may proceed, incrementing its count. Expired
// entries are pruned at most once per sweepInterval.
func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if now.Sub(rl.lastSweep) >= sweepInterval {
		for k, e := range rl.entries {
			if !e.resetAt.After(now) {
				delete(rl.entries, k)
			}
		}
		rl.lastSweep = now
	}
	entry, ok := rl.entries[key]
	if !ok || entry.resetAt.Before(now) {
		rl.entries[key] = rateEntry{count: 1, resetAt: now.Add(window)}
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
