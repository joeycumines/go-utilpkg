package eventloop

import (
	"errors"
	"math"
	"testing"
	"time"
)

type terminalTestError string

func (e terminalTestError) Error() string { return string(e) }

func TestTerminalErrorStoresHeterogeneousErrorTypes(t *testing.T) {
	loop := New()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("storeTerminalError panicked for heterogeneous error types: %v", r)
		}
	}()

	first := errors.New("first terminal error")
	second := terminalTestError("second terminal error")
	loop.storeTerminalError(first)
	loop.storeTerminalError(second)

	if got := loop.terminalError(); !errors.Is(got, second) {
		t.Fatalf("terminalError = %v, want %v", got, second)
	}
}

func TestCalculateTimeoutCapsLongFiniteDeadlines(t *testing.T) {
	loop := New()

	pushTestTimer(loop, &timer{when: time.Now().Add(time.Duration(maxFinitePollTimeoutMs+60_000) * time.Millisecond)})
	if got := loop.calculateTimeout(); got != maxFinitePollTimeoutMs {
		t.Fatalf("calculateTimeout long finite deadline = %d, want cap %d", got, maxFinitePollTimeoutMs)
	}
}

func TestCalculateTimeoutCapsBeforeIntConversion(t *testing.T) {
	loop := New()

	pushTestTimer(loop, &timer{when: time.Now().Add(time.Duration(math.MaxInt32+1) * time.Millisecond)})
	if got := loop.calculateTimeout(); got != maxFinitePollTimeoutMs {
		t.Fatalf("calculateTimeout overflow-sized finite deadline = %d, want cap %d", got, maxFinitePollTimeoutMs)
	}
}

func TestBoundedPhysicalPollTimeout(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{input: -1, want: maxPhysicalPollWaitMs},
		{input: 0, want: 0},
		{input: 1, want: 1},
		{input: maxPhysicalPollWaitMs - 1, want: maxPhysicalPollWaitMs - 1},
		{input: maxPhysicalPollWaitMs, want: maxPhysicalPollWaitMs},
		{input: maxPhysicalPollWaitMs + 1, want: maxPhysicalPollWaitMs},
		{input: maxFinitePollTimeoutMs, want: maxPhysicalPollWaitMs},
	}
	for _, test := range tests {
		if got := boundedPhysicalPollTimeout(test.input); got != test.want {
			t.Errorf("boundedPhysicalPollTimeout(%d) = %d, want %d", test.input, got, test.want)
		}
	}
}

func TestProcessExternalSkipsQueuePressureAfterHardAbort(t *testing.T) {
	loop := New(
		WithFastPathMode(FastPathDisabled),
		WithQueuePressureHandler(func() {
			t.Fatal("queue-pressure handler ran after hard abort")
		}),
	)

	loop.pushOwnerExternal(func() {
		loop.state.Store(StateTerminated)
	})
	loop.pushOwnerExternal(func() {
		t.Fatal("second external callback ran after hard abort")
	})
	loop.processExternal()
}
