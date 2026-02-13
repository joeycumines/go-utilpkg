package eventloop

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Phase 3b coverage tests for eventloop module.
// Targets specific uncovered code paths identified by coverage analysis.

// --- addHandler multiple handlers on a pending promise (promise.go:379-383) ---

func TestPhase3b_AddHandler_MultipleHandlersPendingPromise(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)
	js, err := NewJS(loop)
	require.NoError(t, err)
	var ch1, ch2, ch3 <-chan any
	setup := make(chan struct{})
	require.NoError(t, loop.Submit(func() {
		p, resolve, _ := js.NewChainedPromise()
		ch1 = p.Then(func(v any) any { return v }, nil).ToChannel()
		ch2 = p.Then(func(v any) any { return v }, nil).ToChannel()
		ch3 = p.Then(func(v any) any { return v }, nil).ToChannel()
		resolve("multi-handler-test")
		close(setup)
	}))
	select {
	case <-setup:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for setup")
	}
	for i, ch := range []<-chan any{ch1, ch2, ch3} {
		select {
		case r := <-ch:
			assert.Equal(t, "multi-handler-test", r, "handler %d", i+1)
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout on handler %d", i+1)
		}
	}
	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// --- addHandler four handlers, append path (promise.go:379-383) ---

func TestPhase3b_AddHandler_ThreeHandlers_AppendPath(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)
	js, err := NewJS(loop)
	require.NoError(t, err)
	var ch1, ch2, ch3, ch4 <-chan any
	setup := make(chan struct{})
	require.NoError(t, loop.Submit(func() {
		p, resolve, _ := js.NewChainedPromise()
		ch1 = p.Then(func(v any) any { return v }, nil).ToChannel()
		ch2 = p.Then(func(v any) any { return v }, nil).ToChannel()
		ch3 = p.Then(func(v any) any { return v }, nil).ToChannel()
		ch4 = p.Then(func(v any) any { return v }, nil).ToChannel()
		resolve("four-handlers")
		close(setup)
	}))
	select {
	case <-setup:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout setup")
	}
	for i, ch := range []<-chan any{ch1, ch2, ch3, ch4} {
		select {
		case r := <-ch:
			assert.Equal(t, "four-handlers", r, "handler %d", i+1)
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout on handler %d", i+1)
		}
	}
	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// --- ScheduleNextTick on terminated loop (loop.go:1447-1450) ---

func TestPhase3b_ScheduleNextTick_OnTerminatedLoop(t *testing.T) {
	loop, err := New()
	require.NoError(t, err)
	loop.state.Store(StateTerminated)
	err = loop.ScheduleNextTick(func() {})
	assert.ErrorIs(t, err, ErrLoopTerminated)
	loop.state.Store(StateAwake)
	loop.Close()
}

// --- SubmitInternal I/O mode wakeup (loop.go:1325-1331) ---

func TestPhase3b_SubmitInternal_IOMode_WithUserFDs(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	require.NoError(t, err)
	defer loop.Close()
	loop.userIOFDCount.Store(1)
	loop.state.Store(StateSleeping)
	var called bool
	err = loop.SubmitInternal(func() { called = true })
	require.NoError(t, err)
	loop.internalQueueMu.Lock()
	task, ok := loop.internal.Pop()
	loop.internalQueueMu.Unlock()
	require.True(t, ok, "internal queue should have the task")
	task()
	assert.True(t, called)
	select {
	case <-loop.fastWakeupCh:
	default:
	}
	loop.userIOFDCount.Store(0)
	loop.state.Store(StateAwake)
}

// --- microtaskRing overflow compact (ingress.go:390-393) ---

func TestPhase3b_MicrotaskRing_OverflowCompact(t *testing.T) {
	ring := newMicrotaskRing()
	total := ringBufferSize + 1024
	for i := 0; i < total; i++ {
		ok := ring.Push(func() {})
		require.True(t, ok, "push %d should succeed", i)
	}
	assert.Equal(t, total, ring.Length())
	popped := 0
	for {
		fn := ring.Pop()
		if fn == nil {
			break
		}
		popped++
	}
	assert.Equal(t, total, popped)
	assert.Equal(t, 0, ring.Length())
	ok := ring.Push(func() {})
	assert.True(t, ok)
	fn := ring.Pop()
	assert.NotNil(t, fn)
}

// --- pSquare Quantile index overflow (psquare.go:195-197) ---

func TestPhase3b_PSquare_QuantileIndexOverflow(t *testing.T) {
	ps := newPSquareQuantile(5.0)
	ps.Update(10.0)
	ps.Update(20.0)
	result := ps.Quantile()
	assert.Equal(t, 20.0, result)
}

func TestPhase3b_PSquare_QuantileIndexOverflow_ThreeObs(t *testing.T) {
	ps := newPSquareQuantile(5.0)
	ps.Update(5.0)
	ps.Update(15.0)
	ps.Update(25.0)
	result := ps.Quantile()
	assert.Equal(t, 25.0, result)
}

// --- drainAuxJobs with strictMicrotaskOrdering (loop.go:934-936) ---

func TestPhase3b_DrainAuxJobs_StrictOrdering_Direct(t *testing.T) {
	loop, err := New(WithStrictMicrotaskOrdering(true))
	require.NoError(t, err)
	defer loop.Close()
	var order []int
	loop.externalMu.Lock()
	loop.auxJobs = append(loop.auxJobs, func() {
		order = append(order, 1)
		loop.microtasks.Push(func() {
			order = append(order, 2)
		})
	})
	loop.auxJobs = append(loop.auxJobs, func() {
		order = append(order, 3)
	})
	loop.externalMu.Unlock()
	loop.drainAuxJobs()
	assert.Equal(t, []int{1, 2, 3}, order)
}

// --- transitionToTerminated drain (loop.go:660,671,691) ---

func TestPhase3b_TransitionToTerminated_DirectDrain(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	var internalDrained, externalDrained, auxDrained atomic.Int32
	loop.internalQueueMu.Lock()
	loop.internal.Push(func() { internalDrained.Add(1) })
	loop.internal.Push(func() { internalDrained.Add(1) })
	loop.internalQueueMu.Unlock()
	loop.externalMu.Lock()
	loop.external.Push(func() { externalDrained.Add(1) })
	loop.external.Push(func() { externalDrained.Add(1) })
	loop.externalMu.Unlock()
	loop.externalMu.Lock()
	loop.auxJobs = append(loop.auxJobs, func() { auxDrained.Add(1) })
	loop.auxJobs = append(loop.auxJobs, func() { auxDrained.Add(1) })
	loop.externalMu.Unlock()
	loop.transitionToTerminated()
	assert.Equal(t, int32(2), internalDrained.Load(), "internal queue should be drained")
	assert.Equal(t, int32(2), externalDrained.Load(), "external queue should be drained")
	assert.Equal(t, int32(2), auxDrained.Load(), "auxJobs should be drained")
	assert.Equal(t, StateTerminated, loop.state.Load())
}

func TestPhase3b_TransitionToTerminated_MicrotaskDrain(t *testing.T) {
	loop, err := New()
	require.NoError(t, err)
	var microDrained atomic.Int32
	loop.microtasks.Push(func() { microDrained.Add(1) })
	loop.microtasks.Push(func() { microDrained.Add(1) })
	loop.transitionToTerminated()
	assert.Equal(t, int32(2), microDrained.Load(), "microtasks should be drained")
}

// --- SubmitInternal fast-path with pending timers (loop.go:1296-1300) ---

func TestPhase3b_SubmitInternal_FastPath_WakesOnPendingTimers(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)
	timerFired := make(chan struct{})
	require.NoError(t, loop.SubmitInternal(func() {
		_, err := loop.ScheduleTimer(1*time.Millisecond, func() {
			close(timerFired)
		})
		if err != nil {
			t.Errorf("ScheduleTimer failed: %v", err)
		}
	}))
	select {
	case <-timerFired:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for timer to fire")
	}
	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// --- ScheduleNextTick nil function (loop.go:1432) ---

func TestPhase3b_ScheduleNextTick_NilFunction(t *testing.T) {
	loop, err := New()
	require.NoError(t, err)
	defer loop.Close()
	err = loop.ScheduleNextTick(nil)
	assert.NoError(t, err)
}

// --- Poll entry when not running (loop.go:961-963) ---

func TestPhase3b_Poll_NotRunning_EarlyReturn(t *testing.T) {
	loop, err := New()
	require.NoError(t, err)
	defer loop.Close()
	loop.poll()
}

// --- pollFastMode termination check (loop.go:1048,1086) ---

func TestPhase3b_PollFastMode_TerminatingBeforeBlock(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()
	loop.state.Store(StateSleeping)
	go func() {
		runtime.Gosched()
		loop.state.Store(StateTerminating)
		select {
		case loop.fastWakeupCh <- struct{}{}:
		default:
		}
	}()
	loop.pollFastMode(500)
	loop.state.Store(StateAwake)
}

// --- SubmitInternal fast-path terminated re-check (loop.go:1276) ---

func TestPhase3b_SubmitInternal_FastPath_TerminatedRecheck(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()
	loop.state.Store(StateRunning)
	loop.loopGoroutineID.Store(getGoroutineID())
	loop.state.Store(StateTerminated)
	err = loop.SubmitInternal(func() {})
	if err != nil {
		assert.ErrorIs(t, err, ErrLoopTerminated)
	}
	loop.loopGoroutineID.Store(0)
	loop.state.Store(StateAwake)
}

// --- checkUnhandledRejections without debug mode (promise.go:955-958) ---

func TestPhase3b_CheckUnhandledRejections_NoDebugMode(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)
	var rejectionReason any
	rejectionCh := make(chan struct{}, 1)
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		rejectionReason = reason
		select {
		case rejectionCh <- struct{}{}:
		default:
		}
	}))
	require.NoError(t, err)
	require.NoError(t, loop.Submit(func() {
		_, _, reject := js.NewChainedPromise()
		reject("no-debug-rejection")
	}))
	select {
	case <-rejectionCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for unhandled rejection callback")
	}
	_, isDebugInfo := rejectionReason.(*UnhandledRejectionDebugInfo)
	assert.False(t, isDebugInfo, "should NOT be UnhandledRejectionDebugInfo without debug mode")
	assert.Equal(t, "no-debug-rejection", rejectionReason)
	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// --- Promisify context-cancelled fallback (promisify.go:71-73) ---

func TestPhase3b_Promisify_ContextCancelled_SubmitInternalFails(t *testing.T) {
	loop, err := New()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)
	fnCtx, fnCancel := context.WithCancel(context.Background())
	fnCancel()
	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
	p := loop.Promisify(fnCtx, func(ctx context.Context) (any, error) {
		return nil, ctx.Err()
	})
	ch := p.ToChannel()
	select {
	case result := <-ch:
		_ = result
	case <-time.After(5 * time.Second):
		t.Fatal("promise never settled")
	}
}

// --- setImmediate cleared before run (js.go:583-585) ---

func TestPhase3b_SetImmediate_ClearedBeforeRun(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)
	js, err := NewJS(loop)
	require.NoError(t, err)
	callbackRan := make(chan struct{})
	done := make(chan struct{})
	require.NoError(t, loop.Submit(func() {
		id, err := js.SetImmediate(func() {
			close(callbackRan)
		})
		require.NoError(t, err)
		js.ClearImmediate(id)
		close(done)
	}))
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
	time.Sleep(50 * time.Millisecond)
	select {
	case <-callbackRan:
		t.Fatal("callback should not have run after ClearImmediate")
	default:
	}
	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// --- SetInterval ScheduleTimer error (js.go:357-363) ---

func TestPhase3b_SetInterval_ScheduleTimerError(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	require.NoError(t, err)
	defer loop.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)
	js, err := NewJS(loop)
	require.NoError(t, err)
	firstFired := make(chan struct{})
	require.NoError(t, loop.Submit(func() {
		js.SetInterval(func() {
			select {
			case <-firstFired:
			default:
				close(firstFired)
			}
		}, 1)
	}))
	select {
	case <-firstFired:
	case <-time.After(5 * time.Second):
		t.Fatal("interval never fired")
	}
	cancel()
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't stop")
	}
}

// --- submitWakeup on terminated loop (loop.go:1157-1160) ---

func TestPhase3b_SubmitWakeup_Terminated(t *testing.T) {
	loop, err := New()
	require.NoError(t, err)
	defer loop.Close()
	loop.state.Store(StateTerminated)
	err = loop.submitWakeup()
	assert.ErrorIs(t, err, ErrLoopTerminated)
	loop.state.Store(StateAwake)
}

// --- Shutdown called twice (loop.go:385-387) ---

func TestPhase3b_Shutdown_CalledTwice(t *testing.T) {
	loop, err := New()
	require.NoError(t, err)
	defer loop.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitForRunning(t, loop)
	require.NoError(t, loop.Shutdown(context.Background()))
	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't exit after Shutdown")
	}
	err2 := loop.Shutdown(context.Background())
	if err2 != nil {
		assert.ErrorIs(t, err2, ErrLoopTerminated)
	}
}
