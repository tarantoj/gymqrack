package server

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rateLimiter is a per-key token-bucket limiter built on golang.org/x/time/rate.
// Each key gets its own bucket of perMin tokens that refills continuously at
// perMin tokens per minute, so a burst of perMin requests is allowed, then one
// token every window/perMin. Idle buckets are pruned so memory stays bounded.
type rateLimiter struct {
	mu        sync.Mutex
	perMin    int
	window    time.Duration
	entries   map[string]*rate.Limiter
	lastSweep time.Time
}

const (
	// window is the refill period for a bucket (perMin tokens refill over it).
	window = time.Minute
	// sweepInterval bounds how often idle buckets are pruned.
	sweepInterval = time.Minute
)

func newRateLimiter(perMin int) *rateLimiter {
	if perMin <= 0 {
		perMin = 1
	}
	return &rateLimiter{
		perMin:  perMin,
		window:  window,
		entries: make(map[string]*rate.Limiter),
	}
}

// allow reports whether key may proceed, consuming one token. Expired (full)
// buckets are pruned at most once per sweepInterval.
func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.sweepLocked(time.Now())
	l, ok := rl.entries[key]
	if !ok {
		l = rate.NewLimiter(rate.Every(rl.window/time.Duration(rl.perMin)), rl.perMin)
		rl.entries[key] = l
	}
	return l.Allow()
}

// sweepLocked prunes buckets that have refilled to full (i.e. been idle) since
// the last sweep. Callers must hold mu. No-op within sweepInterval of the last
// sweep.
func (rl *rateLimiter) sweepLocked(now time.Time) {
	if now.Sub(rl.lastSweep) < sweepInterval {
		return
	}
	for k, l := range rl.entries {
		if l.Tokens() >= float64(rl.perMin) {
			delete(rl.entries, k)
		}
	}
	rl.lastSweep = now
}

// reset clears the bucket for key (used after a successful login).
func (rl *rateLimiter) reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.entries, key)
}
