package server

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToBudget(t *testing.T) {
	rl := newRateLimiter(3)
	for i := 0; i < 3; i++ {
		if !rl.allow("ip") {
			t.Fatalf("allow %d should succeed", i)
		}
	}
	if rl.allow("ip") {
		t.Fatal("4th allow should be denied")
	}
}

func TestRateLimiterKeysIndependent(t *testing.T) {
	rl := newRateLimiter(1)
	if !rl.allow("a") {
		t.Fatal("first allow for a should succeed")
	}
	if rl.allow("a") {
		t.Fatal("second allow for a should be denied")
	}
	if !rl.allow("b") {
		t.Fatal("different key should be allowed")
	}
}

func TestRateLimiterReset(t *testing.T) {
	rl := newRateLimiter(2)
	if !rl.allow("ip") {
		t.Fatal("first allow should succeed")
	}
	if !rl.allow("ip") {
		t.Fatal("second allow should succeed")
	}
	if rl.allow("ip") {
		t.Fatal("third allow should be denied")
	}
	rl.reset("ip")
	if !rl.allow("ip") {
		t.Fatal("allow after reset should succeed")
	}
}

func TestRateLimiterRefills(t *testing.T) {
	rl := newRateLimiter(1)
	rl.window = 10 * time.Millisecond
	if !rl.allow("ip") {
		t.Fatal("first allow should succeed")
	}
	if rl.allow("ip") {
		t.Fatal("second allow should be denied")
	}
	time.Sleep(25 * time.Millisecond)
	if !rl.allow("ip") {
		t.Fatal("bucket should have refilled after the window")
	}
}

func TestRateLimiterSweepPrunesFullBuckets(t *testing.T) {
	rl := newRateLimiter(1)
	rl.window = time.Millisecond
	if !rl.allow("ip") {
		t.Fatal("first allow should succeed")
	}
	time.Sleep(5 * time.Millisecond) // let the bucket refill to full (idle)
	rl.mu.Lock()
	rl.lastSweep = time.Time{} // force the sweep to run (normally once/min)
	rl.sweepLocked(time.Now())
	pruned := len(rl.entries) == 0
	rl.mu.Unlock()
	if !pruned {
		t.Fatal("full bucket should have been pruned by the sweep")
	}
}

func TestNewRateLimiterClampsZeroBudget(t *testing.T) {
	rl := newRateLimiter(0)
	if rl.perMin != 1 {
		t.Fatalf("perMin = %d, want 1", rl.perMin)
	}
}
