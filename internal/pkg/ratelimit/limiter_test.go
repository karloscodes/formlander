package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterAllowResetsWindow(t *testing.T) {
	lim := NewLimiter()
	window := 50 * time.Millisecond

	if !lim.Allow("key", 2, window) {
		t.Fatalf("expected first attempt to be allowed")
	}
	if !lim.Allow("key", 2, window) {
		t.Fatalf("expected second attempt to be allowed")
	}
	if lim.Allow("key", 2, window) {
		t.Fatalf("expected third attempt to be rejected")
	}

	time.Sleep(window + 10*time.Millisecond)

	if !lim.Allow("key", 2, window) {
		t.Fatalf("expected window reset to allow again")
	}
}

func TestLimiterResetClearsCounters(t *testing.T) {
	lim := NewLimiter()
	window := time.Minute

	if !lim.Allow("key", 1, window) {
		t.Fatalf("expected initial allow")
	}
	if lim.Allow("key", 1, window) {
		t.Fatalf("expected second call to be rejected before reset")
	}

	lim.Reset()

	if !lim.Allow("key", 1, window) {
		t.Fatalf("expected reset to clear counts")
	}
}
