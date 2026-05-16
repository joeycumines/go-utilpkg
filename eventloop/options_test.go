package eventloop

import "testing"

// captureLoopOptionPanic captures a panic value from fn. It is retained for
// tests that exercise SetFastPathMode, which still panics on static contract
// violations per ADR-003.
func captureLoopOptionPanic(fn func()) (value any) {
	defer func() { value = recover() }()
	fn()
	return nil
}

// Test 1.2.6: Test default options
func TestDefaultOptions(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	registerLoopCleanupT(t, l)

	// FastPathMode should be Auto (0) by default
	mode := FastPathMode(l.fastPathMode.Load())
	if mode != FastPathAuto {
		t.Errorf("Default FastPathMode should be Auto (%d), got %d", FastPathAuto, mode)
	}
}

// Test 1.2.7: Test custom options
func TestCustomOptions(t *testing.T) {
	l, err := New(WithFastPathMode(FastPathForced))
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	registerLoopCleanupT(t, l)

	mode := FastPathMode(l.fastPathMode.Load())
	if mode != FastPathForced {
		t.Errorf("FastPathMode should be Forced (%d), got %d", FastPathForced, mode)
	}
}

// Test: Multiple options in any order
func TestMultipleOptions(t *testing.T) {
	l1, err := New(
		WithFastPathMode(FastPathDisabled),
		WithMetrics(true),
	)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	registerLoopCleanupT(t, l1)

	mode := FastPathMode(l1.fastPathMode.Load())
	if mode != FastPathDisabled {
		t.Errorf("Option order 1: FastPathMode should be Disabled (%d), got %d", FastPathDisabled, mode)
	}

	l2, err := New(
		WithMetrics(false),
		WithFastPathMode(FastPathForced),
	)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	registerLoopCleanupT(t, l2)

	mode = FastPathMode(l2.fastPathMode.Load())
	if mode != FastPathForced {
		t.Errorf("Option order 2: FastPathMode should be Forced (%d), got %d", FastPathForced, mode)
	}
}

// Test: Nil option handling
func TestNilOption(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("New accepted a nil LoopOption")
	}
}

func TestTypedNilBuiltInOptionsError(t *testing.T) {
	tests := []struct {
		name   string
		option LoopOption
	}{
		{name: "fast path mode", option: (*FastPathModeOption)(nil)},
		{name: "metrics", option: (*MetricsOption)(nil)},
		{name: "logger", option: (*LoggerOption)(nil)},
		{name: "debug mode", option: (*DebugModeOption)(nil)},
		{name: "auto exit", option: (*AutoExitOption)(nil)},
		{name: "queue pressure handler", option: (*QueuePressureHandlerOption)(nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.option); err == nil {
				t.Fatal("New accepted a typed-nil built-in option")
			}
		})
	}
}

func TestNewJSStaticContracts(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	registerLoopCleanupT(t, loop)

	options := []struct {
		name   string
		option JSOption
	}{
		{name: "nil", option: nil},
		{name: "typed nil rejection handler", option: (*UnhandledRejectionOption)(nil)},
		{name: "typed nil fallback", option: (*UnhandledRejectionFallbackOption)(nil)},
		{name: "nil rejection handler", option: WithUnhandledRejection(nil)},
		{name: "invalid fallback", option: WithUnhandledRejectionFallback(UnhandledRejectionFallbackMode(99))},
	}
	for _, test := range options {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateJSOptions(test.option); err == nil {
				t.Fatal("ValidateJSOptions accepted an invalid option")
			}
			if _, err := NewJS(loop, test.option); err == nil {
				t.Fatal("NewJS accepted an invalid option")
			}
		})
	}
	if err := ValidateJSOptions(
		WithUnhandledRejection(func(any) {}),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled),
	); err != nil {
		t.Fatalf("ValidateJSOptions rejected valid options: %v", err)
	}
	for _, test := range []struct {
		name string
		loop *Loop
	}{
		{name: "nil loop"},
		{name: "zero loop", loop: &Loop{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewJS(test.loop); err == nil {
				t.Fatal("NewJS accepted a Loop not created by New")
			}
		})
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS error: %v", err)
	}
	if js.Loop() != loop {
		t.Fatal("NewJS did not retain the configured Loop")
	}
	if js.unhandledFallback != UnhandledRejectionFallbackDisabled {
		t.Fatalf("default unhandled fallback = %v, want disabled", js.unhandledFallback)
	}
}

func TestWithQueuePressureHandlerRejectsNil(t *testing.T) {
	if _, err := New(WithQueuePressureHandler(nil)); err == nil {
		t.Fatal("New accepted a nil queue-pressure handler")
	}
}

func TestFastPathModeRejectsInvalidValues(t *testing.T) {
	invalid := FastPathMode(99)
	if _, err := New(WithFastPathMode(invalid)); err == nil {
		t.Fatal("New accepted an invalid fast-path mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	registerLoopCleanupT(t, loop)
	if got := captureLoopOptionPanic(func() { _ = loop.SetFastPathMode(invalid) }); got == nil {
		t.Fatal("SetFastPathMode accepted an invalid fast-path mode")
	}
	if got := FastPathMode(loop.fastPathMode.Load()); got != FastPathAuto {
		t.Fatalf("mode after rejected update = %v, want %v", got, FastPathAuto)
	}
}

func TestSetFastPathModeTransitions(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	registerLoopCleanupT(t, loop)

	for _, mode := range []FastPathMode{FastPathDisabled, FastPathForced, FastPathAuto} {
		if err := loop.SetFastPathMode(mode); err != nil {
			t.Fatalf("SetFastPathMode(%v): %v", mode, err)
		}
		if got := FastPathMode(loop.fastPathMode.Load()); got != mode {
			t.Fatalf("mode after SetFastPathMode(%v) = %v, want %v", mode, got, mode)
		}
	}
}
