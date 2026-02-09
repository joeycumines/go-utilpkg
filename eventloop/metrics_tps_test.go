package eventloop

import (
	"strings"
	"testing"
	"time"
)

// containsSubstring checks if haystack contains needle (case-insensitive for error messages).
func containsSubstring(haystack, needle string) bool {
	return strings.Contains(
		strings.ToLower(haystack),
		strings.ToLower(needle),
	)
}

// Test_newTPSCounter validates newTPSCounter parameter validation and configuration.
// Verifies R111 fix: TPS Counter documentation and input validation.
func Test_newTPSCounter(t *testing.T) {
	tests := []struct {
		name       string
		windowSize time.Duration
		bucketSize time.Duration
		wantPanic  string
	}{
		{
			name:       "valid production config",
			windowSize: 10 * time.Second,
			bucketSize: 100 * time.Millisecond,
			wantPanic:  "",
		},
		{
			name:       "valid high-frequency config",
			windowSize: 5 * time.Second,
			bucketSize: 50 * time.Millisecond,
			wantPanic:  "",
		},
		{
			name:       "valid long-term analysis config",
			windowSize: 60 * time.Second,
			bucketSize: 500 * time.Millisecond,
			wantPanic:  "",
		},
		{
			name:       "zero windowSize should panic",
			windowSize: 0,
			bucketSize: 100 * time.Millisecond,
			wantPanic:  "eventloop: windowSize must be positive (use > 0 duration)",
		},
		{
			name:       "negative windowSize should panic",
			windowSize: -1 * time.Second,
			bucketSize: 100 * time.Millisecond,
			wantPanic:  "eventloop: windowSize must be positive (use > 0 duration)",
		},
		{
			name:       "zero bucketSize should panic",
			windowSize: 10 * time.Second,
			bucketSize: 0,
			wantPanic:  "eventloop: bucketSize must be positive (use > 0 duration)",
		},
		{
			name:       "negative bucketSize should panic",
			windowSize: 10 * time.Second,
			bucketSize: -1 * time.Millisecond,
			wantPanic:  "eventloop: bucketSize must be positive (use > 0 duration)",
		},
		{
			name:       "bucketSize larger than windowSize should panic",
			windowSize: 5 * time.Second,
			bucketSize: 10 * time.Second,
			wantPanic:  "eventloop: bucketSize cannot exceed windowSize (use <= windowSize)",
		},
		{
			name:       "bucketSize equal to windowSize is valid (single bucket)",
			windowSize: 10 * time.Second,
			bucketSize: 10 * time.Second,
			wantPanic:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic != "" {
				// Expect panic
				defer func() {
					r := recover()
					if r == nil {
						t.Errorf("Expected panic with: %s", tt.wantPanic)
					}
					errMsg, ok := r.(string)
					if !ok {
						t.Errorf("Expected string panic message, got %v (type %T)", r, r)
					} else if len(tt.wantPanic) > 0 && !containsSubstring(errMsg, tt.wantPanic) {
						t.Errorf("Expected panic message to contain '%s', got '%v'", tt.wantPanic, r)
					}
				}()
			}

			// This should panic if validation fails
			counter := newTPSCounter(tt.windowSize, tt.bucketSize)

			if tt.wantPanic == "" {
				// Valid config - verify counter is initialized
				if counter == nil {
					t.Fatal("newTPSCounter should return non-nil counter")
				}
				if counter.lastRotation.Load().(time.Time).IsZero() {
					t.Error("lastRotation should be initialized")
				}
				if counter.buckets == nil {
					t.Error("buckets should be initialized")
				}
				if len(counter.buckets) < 1 {
					t.Errorf("Expected at least 1 bucket, got %d", len(counter.buckets))
				}
			}
		})
	}
}

// Test_tpsCounterBasicFunctionality tests basic increment and TPS calculation.
func Test_tpsCounterBasicFunctionality(t *testing.T) {
	counter := newTPSCounter(1*time.Second, 100*time.Millisecond)

	// Initial TPS should be 0 (no events recorded yet)
	tps := counter.TPS()
	if tps != 0 {
		t.Errorf("Initial TPS should be 0, got %.2f", tps)
	}

	// Record some increments
	for i := 0; i < 10; i++ {
		counter.Increment()
	}

	// TPS immediately reflects 10 events in 1-second rolling window
	// This is the CORRECT behavior - no artificial warmup suppression
	tps = counter.TPS()
	if tps != 10 {
		t.Errorf("TPS should be 10.0 (10 events in 1s window), got %.2f", tps)
	}

	// Wait for window to fill
	time.Sleep(1 * time.Second)

	// After aging out, no events remain in window, TPS returns to 0
	tps = counter.TPS()
	if tps != 0 {
		t.Errorf("TPS should be 0.0 after events age out, got %.2f", tps)
	}
}

// Test_tpsCounterRotation tests bucket rotation behavior.
func Test_tpsCounterRotation(t *testing.T) {
	windowSize := 500 * time.Millisecond
	bucketSize := 100 * time.Millisecond
	counter := newTPSCounter(windowSize, bucketSize)

	// Record events in first bucket
	for i := 0; i < 5; i++ {
		counter.Increment()
	}

	tps := counter.TPS()
	if tps != 10 {
		// 5 events in 500ms = 10 TPS
		t.Errorf("Expected TPS of 10.0, got %.2f", tps)
	}

	// Wait for first bucket to expire and second bucket to fill
	time.Sleep(200 * time.Millisecond)

	// Record events in new buckets
	for i := 0; i < 10; i++ {
		counter.Increment()
	}

	tps = counter.TPS()
	// Now we have ~5 events in first part of window + 10 in second part
	// Total: 15 events in 500ms = 30 TPS (approximately)
	// The exact value depends on timing, so we just verify it's reasonable
	if tps < 10 || tps > 50 {
		t.Errorf("TPS %.2f is outside reasonable range [10, 50]", tps)
	}

	// Wait for window to completely rotate (old buckets fall out)
	time.Sleep(600 * time.Millisecond)

	// All events have aged out - TPS should be 0
	tps = counter.TPS()
	if tps != 0 {
		t.Errorf("After window rotation with all events aged out, TPS should be 0, got %.2f", tps)
	}
}

// Test_tpsCounterWindowSizing tests that different window sizes work correctly.
func Test_tpsCounterWindowSizing(t *testing.T) {
	testCases := []struct {
		name       string
		windowSize time.Duration
		bucketSize time.Duration
	}{
		{"small window", 500 * time.Millisecond, 50 * time.Millisecond},
		{"medium window", 2 * time.Second, 100 * time.Millisecond},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			counter := newTPSCounter(tc.windowSize, tc.bucketSize)

			// For a rolling window TPS counter, we need to account for events aging out
			// as time advances. We'll record events and verify they are tracked
			// within the same bucket (no aging).

			// First, verify initial state
			tps := counter.TPS()
			if tps != 0 {
				t.Errorf("Initial TPS should be 0, got %.2f", tps)
			}

			// Record events and check immediately (no time advancement)
			expectedTPS := 100.0
			// Use float64 division to correctly convert duration to seconds
			// tc.windowSize is in nanoseconds, so we divide by float64(time.Second)
			eventCount := int(expectedTPS * (float64(tc.windowSize) / float64(time.Second)))
			for i := 0; i < eventCount; i++ {
				counter.Increment()
			}

			// Events should be tracked immediately (in the current bucket)
			tps = counter.TPS()
			// Allow some tolerance for timing
			if tps < expectedTPS*0.8 {
				t.Errorf("TPS %.2f is below expected [%.2f], events not tracked correctly",
					tps, expectedTPS)
			}

			// Now wait for events to age out
			time.Sleep(tc.windowSize + tc.bucketSize)

			// TPS should return to 0 after all events age out
			tps = counter.TPS()
			if tps != 0 {
				t.Errorf("After events age out, TPS should be 0, got %.2f", tps)
			}
		})
	}
}
