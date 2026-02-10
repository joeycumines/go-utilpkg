package eventloop

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

// testEvent is a minimal logiface.Event implementation for testing the
// structured logging paths (logCritical, logError).
type testEvent struct {
	logiface.UnimplementedEvent
	level logiface.Level
}

func (e *testEvent) Level() logiface.Level        { return e.level }
func (e *testEvent) AddField(key string, val any) {}

// testEventFactory creates testEvent instances.
type testEventFactory struct {
	onNew func(logiface.Level) // callback when NewEvent is called
}

func (f *testEventFactory) NewEvent(level logiface.Level) *testEvent {
	if f.onNew != nil {
		f.onNew(level)
	}
	return &testEvent{level: level}
}

// testEventWriter writes testEvent instances.
type testEventWriter struct {
	onWrite func(*testEvent) error
}

func (w *testEventWriter) Write(event *testEvent) error {
	if w.onWrite != nil {
		return w.onWrite(event)
	}
	return nil
}

// --- RemoveEventListener coverage (no-op stubs, 0% → 100%) ---

func TestAbortSignal_RemoveEventListener_NoOp(t *testing.T) {
	ac := NewAbortController()
	signal := ac.Signal()
	// RemoveEventListener is a documented no-op (Go funcs can't be compared).
	// Calling it should not panic.
	signal.RemoveEventListener("abort", func(reason any) {})
	signal.RemoveEventListener("abort", nil)
	signal.RemoveEventListener("", func(reason any) {})
}

func TestEventTarget_RemoveEventListener_NoOp(t *testing.T) {
	et := NewEventTarget()
	// RemoveEventListener is a documented no-op.
	// Calling it should not panic.
	et.RemoveEventListener("click", nil)
	et.RemoveEventListener("click", func(e *Event) {})
	et.RemoveEventListener("", nil)
}

// --- logCritical coverage (20% → higher) ---

func TestLogCritical_WithEnabledLogger(t *testing.T) {
	var logged bool

	writer := &testEventWriter{
		onWrite: func(event *testEvent) error {
			logged = true
			return nil
		},
	}
	factory := &testEventFactory{}

	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](factory),
		logiface.WithWriter[*testEvent](writer),
	)

	// Convert to the generic Logger[Event] that Loop requires
	genericLogger := typedLogger.Logger()

	loop, err := New(WithLogger(genericLogger))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	loop.logCritical("test critical message", errors.New("test error"))

	if !logged {
		t.Error("Expected logger to receive the critical message")
	}
}

func TestLogCritical_WithPanickingLogger(t *testing.T) {
	writer := &testEventWriter{
		onWrite: func(event *testEvent) error {
			panic("logger panic")
		},
	}
	factory := &testEventFactory{}

	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](factory),
		logiface.WithWriter[*testEvent](writer),
	)
	genericLogger := typedLogger.Logger()

	loop, err := New(WithLogger(genericLogger))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	// Should not panic — falls back to log.Printf
	loop.logCritical("test critical with panic", errors.New("test error"))
}

func TestLogError_WithEnabledLogger(t *testing.T) {
	var logged bool

	writer := &testEventWriter{
		onWrite: func(event *testEvent) error {
			logged = true
			return nil
		},
	}
	factory := &testEventFactory{}

	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](factory),
		logiface.WithWriter[*testEvent](writer),
	)
	genericLogger := typedLogger.Logger()

	loop, err := New(WithLogger(genericLogger))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	loop.logError("test error message", "some panic value")

	if !logged {
		t.Error("Expected logger to receive the error message")
	}
}

func TestLogError_WithPanickingLogger(t *testing.T) {
	writer := &testEventWriter{
		onWrite: func(event *testEvent) error {
			panic("logger panic")
		},
	}
	factory := &testEventFactory{}

	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](factory),
		logiface.WithWriter[*testEvent](writer),
	)
	genericLogger := typedLogger.Logger()

	loop, err := New(WithLogger(genericLogger))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	// Should not panic — falls back to log.Printf
	loop.logError("test error with panic", "test panic value")
}

// --- pSquareQuantile.Max coverage (33% → 100%) ---

func TestPSquareMax_EmptyReturnsZero(t *testing.T) {
	ps := newPSquareQuantile(0.5)
	if max := ps.Max(); max != 0 {
		t.Errorf("Max() with 0 samples should be 0, got %f", max)
	}
}

func TestPSquareMax_FewSamples(t *testing.T) {
	ps := newPSquareQuantile(0.5)

	// Test with 1 sample
	ps.Update(42)
	if max := ps.Max(); max != 42 {
		t.Errorf("Max() with 1 sample should be 42, got %f", max)
	}

	// Test with 2 samples
	ps.Update(10)
	if max := ps.Max(); max != 42 {
		t.Errorf("Max() with 2 samples should be 42 (max of {42, 10}), got %f", max)
	}

	// Test with 3 samples — max at beginning
	ps.Update(5)
	if max := ps.Max(); max != 42 {
		t.Errorf("Max() with 3 samples should be 42 (max of {42, 10, 5}), got %f", max)
	}

	// Test with 4 samples — new max at end
	ps.Update(100)
	if max := ps.Max(); max != 100 {
		t.Errorf("Max() with 4 samples should be 100 (max of {42, 10, 5, 100}), got %f", max)
	}
}

func TestPSquareMax_AfterInit(t *testing.T) {
	ps := newPSquareQuantile(0.5)
	// Add 5+ samples to trigger full P-Square initialization
	for i := 1; i <= 10; i++ {
		ps.Update(float64(i))
	}
	if max := ps.Max(); max != 10 {
		t.Errorf("Max() after 10 samples should be 10 (q[4]), got %f", max)
	}
}

// --- ScheduleNextTick coverage: verifies nextTick functionality ---

func TestScheduleNextTick_ExecutesBeforeMicrotasks(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	var order []int
	result := make(chan []int, 1)

	loop.Submit(func() {
		loop.ScheduleNextTick(func() {
			order = append(order, 1)
		})
		loop.ScheduleMicrotask(func() {
			order = append(order, 2)
			result <- order
		})
	})

	select {
	case got := <-result:
		if len(got) != 2 || got[0] != 1 || got[1] != 2 {
			t.Errorf("Expected nextTick (1) before microtask (2), got %v", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Timed out waiting for result")
	}

	loop.Shutdown(context.Background())
	<-done
}

// --- ToChannel coverage: ChainedPromise.ToChannel ---

func TestChainedPromise_ToChannel_Resolved(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	result := make(chan any, 1)

	loop.Submit(func() {
		js, jsErr := NewJS(loop)
		if jsErr != nil {
			t.Errorf("NewJS failed: %v", jsErr)
			return
		}
		p, resolve, _ := js.NewChainedPromise()
		ch := p.ToChannel()
		resolve(42)
		go func() {
			r := <-ch
			result <- r
		}()
	})

	select {
	case v := <-result:
		if v != 42 {
			t.Errorf("Expected 42, got %v", v)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Timed out")
	}

	loop.Shutdown(context.Background())
	<-done
}

func TestChainedPromise_ToChannel_AlreadySettled(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	result := make(chan any, 1)

	loop.Submit(func() {
		js, jsErr := NewJS(loop)
		if jsErr != nil {
			t.Errorf("NewJS failed: %v", jsErr)
			return
		}
		p, _, reject := js.NewChainedPromise()
		reject(errors.New("test rejection"))
		// Call ToChannel AFTER settlement to test the pre-filled path
		ch := p.ToChannel()
		go func() {
			r := <-ch
			result <- r
		}()
	})

	select {
	case v := <-result:
		if v == nil {
			t.Error("Expected non-nil result from rejected promise")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Timed out")
	}

	loop.Shutdown(context.Background())
	<-done
}
