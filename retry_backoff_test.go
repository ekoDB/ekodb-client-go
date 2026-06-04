package ekodb

import (
	"testing"
	"time"
)

// TestRetryBackoffBase verifies the deterministic capped exponential schedule
// and that it never overflows (regression for #36).
func TestRetryBackoffBase(t *testing.T) {
	cases := map[int]time.Duration{
		0: 200 * time.Millisecond,
		1: 400 * time.Millisecond,
		2: 800 * time.Millisecond,
		3: 1600 * time.Millisecond,
		4: 3200 * time.Millisecond,
		5: 5 * time.Second, // capped
	}
	for attempt, want := range cases {
		if got := retryBackoffBase(attempt); got != want {
			t.Errorf("retryBackoffBase(%d) = %v, want %v", attempt, got, want)
		}
	}
	// Large attempt counts must stay capped and never overflow/panic.
	if got := retryBackoffBase(1_000_000); got != 5*time.Second {
		t.Errorf("retryBackoffBase(1e6) = %v, want 5s (capped)", got)
	}
}

// TestRetryBackoffJitterBounds verifies full-jitter stays within [base/2, base].
func TestRetryBackoffJitterBounds(t *testing.T) {
	for attempt := 0; attempt < 8; attempt++ {
		base := retryBackoffBase(attempt)
		for i := 0; i < 200; i++ {
			d := retryBackoff(attempt)
			if d < base/2 || d > base {
				t.Fatalf("retryBackoff(%d) = %v, out of bounds [%v, %v]", attempt, d, base/2, base)
			}
		}
	}
}
