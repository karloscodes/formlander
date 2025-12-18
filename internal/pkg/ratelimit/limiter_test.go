package ratelimit

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	t.Run("allows requests within limit and resets after window", func(t *testing.T) {
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
	})

	t.Run("reset clears counters", func(t *testing.T) {
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
	})
}
