package eventloop

import (
	"errors"
	"testing"
)

// testTrackLoopOption is a LoopOption that records application order and can optionally return an error.
type testTrackLoopOption struct {
	id    int
	err   error
	order *[]int
}

func (o *testTrackLoopOption) applyLoopOption(opts *loopConfig) error {
	if o.order != nil {
		*o.order = append(*o.order, o.id)
	}
	if o.err != nil {
		return o.err
	}
	return nil
}

var _ LoopOption = (*testTrackLoopOption)(nil)

// Tests for: New(), resolveLoopOptions(), lazy poller/wake initialization,
//            and poller.Init() cleanup behavior.

// TestNew_ResolveLoopOptions_ErrorPath tests that New returns an error when a
// LoopOption reports a configuration error.
func TestNew_ResolveLoopOptions_ErrorPath(t *testing.T) {
	expectedErr := errors.New("intentional option error for testing")
	badOption := &testTrackLoopOption{err: expectedErr}
	_, err := New(badOption)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("New error = %#v, want error wrapping %v", err, expectedErr)
	}
}

// TestNew_ResolveLoopOptions_NilOption tests that nil options return an error.
func TestNew_ResolveLoopOptions_NilOption(t *testing.T) {
	if _, err := New(WithMetrics(true), nil); err == nil {
		t.Fatal("New accepted a nil LoopOption")
	}
}

// TestNew_ResolveLoopOptions_EmptyOptions tests that New() works with no options.
func TestNew_ResolveLoopOptions_EmptyOptions(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	registerLoopCleanupT(t, loop)

	mode := FastPathMode(loop.fastPathMode.Load())
	if mode != FastPathAuto {
		t.Errorf("Default FastPathMode should be Auto (%d), got %d", FastPathAuto, mode)
	}
	if loop.pollerReady.Load() {
		t.Error("default task-only loop should not initialize poller resources")
	}
	if loop.wakePipe != -1 || loop.wakePipeWrite != -1 {
		t.Errorf("default task-only loop wake fds = (%d, %d), want (-1, -1)", loop.wakePipe, loop.wakePipeWrite)
	}
}

// TestNew_ResolveLoopOptions_ChainedOptions tests applying multiple options.
func TestNew_ResolveLoopOptions_ChainedOptions(t *testing.T) {
	// Test that all options are applied in order by tracking via shared slice
	var order []int

	opt1 := &testTrackLoopOption{id: 1, order: &order}
	opt2 := &testTrackLoopOption{id: 2, order: &order}
	opt3 := &testTrackLoopOption{id: 3, order: &order}

	loop, err := New(opt1, opt2, opt3)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	registerLoopCleanupT(t, loop)

	// Verify order of application (populated by applyLoop during New())
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("Options applied in wrong order: %v, expected [1 2 3]", order)
	}
}

// TestNew_ResolveLoopOptions_ErrorAtMiddle tests a static error in the middle
// of an option chain.
func TestNew_ResolveLoopOptions_ErrorAtMiddle(t *testing.T) {
	// Error in middle option should stop processing
	var applied []int
	expectedErr := errors.New("middle option error")

	opt1 := &testTrackLoopOption{id: 1, order: &applied}
	opt2 := &testTrackLoopOption{id: 2, err: expectedErr, order: &applied}
	opt3 := &testTrackLoopOption{id: 3, order: &applied}

	_, err := New(opt1, opt2, opt3)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("New error = %#v, want error wrapping %v", err, expectedErr)
	}

	// Only first two options should have been applied (opt2 errors, opt3 never reached)
	if len(applied) != 2 || applied[0] != 1 || applied[1] != 2 {
		t.Errorf("Wrong options applied: %v, expected [1 2]", applied)
	}
}

// TestNew_AllValidOptions tests that all supported options work correctly.
func TestNew_AllValidOptions(t *testing.T) {
	loop, err := New(
		WithFastPathMode(FastPathForced),
		WithMetrics(true),
		WithLogger(nil), // nil logger should be accepted
	)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	registerLoopCleanupT(t, loop)

	mode := FastPathMode(loop.fastPathMode.Load())
	if mode != FastPathForced {
		t.Errorf("FastPathMode should be Forced (%d), got %d", FastPathForced, mode)
	}

	if loop.metrics == nil {
		t.Error("Metrics should be enabled")
	}
}

// TestNew_WithFastPathModes tests all FastPathMode values.
func TestNew_WithFastPathModes(t *testing.T) {
	modes := []struct {
		mode FastPathMode
		name string
	}{
		{FastPathAuto, "Auto"},
		{FastPathForced, "Forced"},
		{FastPathDisabled, "Disabled"},
	}

	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			loop, err := New(WithFastPathMode(tc.mode))
			if err != nil {
				t.Fatalf("New error: %v", err)
			}
			registerLoopCleanupT(t, loop)

			actual := FastPathMode(loop.fastPathMode.Load())
			if actual != tc.mode {
				t.Errorf("FastPathMode = %d, want %d", actual, tc.mode)
			}
			if loop.pollerReady.Load() {
				t.Errorf("pollerReady = true, want lazy initialization for mode %s", tc.name)
			}
		})
	}
}

// TestNew_WithMetrics tests metrics initialization.
func TestNew_WithMetrics(t *testing.T) {
	// Without metrics
	loop1, err := New(WithMetrics(false))
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	registerLoopCleanupT(t, loop1)

	if loop1.metrics != nil {
		t.Error("Metrics should be nil when disabled")
	}

	if loop1.metrics != nil {
		t.Error("runtime metrics should be nil when metrics are disabled")
	}

	// With metrics
	loop2, err := New(WithMetrics(true))
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	registerLoopCleanupT(t, loop2)

	if loop2.metrics == nil {
		t.Error("Metrics should not be nil when enabled")
	}

	if loop2.metrics == nil || loop2.metrics.tps == nil {
		t.Error("runtime metrics and TPS counter should be initialized when metrics are enabled")
	}
}

// TestNew_LoopIDIncrement verifies unique loop IDs are generated.
func TestNew_LoopIDIncrement(t *testing.T) {
	loop1, err := New()
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	registerLoopCleanupT(t, loop1)

	loop2, err := New()
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	registerLoopCleanupT(t, loop2)

	if loop1.id >= loop2.id {
		t.Errorf("Loop IDs should be unique and incrementing: id1=%d, id2=%d", loop1.id, loop2.id)
	}
}
