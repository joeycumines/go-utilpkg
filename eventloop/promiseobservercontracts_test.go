package eventloop

import (
	"errors"
	"testing"
)

func Test_ChainedPromise_ToChannel_Standalone(t *testing.T) {
	promise := &Promise{}
	promise.state.Store(int32(Pending))
	result := promise.ToChannel()
	promise.resolve("hello")
	if value, open := <-result; !open || value != "hello" {
		t.Fatalf("result = (%#v, open=%v), want (hello, open=true)", value, open)
	}
	if value, open := <-result; open {
		t.Fatalf("result channel remained open with %#v", value)
	}
}

func Test_ChainedPromise_ToChannel_StandaloneSettled(t *testing.T) {
	promise := &Promise{}
	promise.state.Store(int32(Pending))
	promise.resolve("already-done")
	result := promise.ToChannel()
	select {
	case value, open := <-result:
		if !open || value != "already-done" {
			t.Fatalf("result = (%#v, open=%v), want (already-done, open=true)", value, open)
		}
	default:
		t.Fatal("settled standalone Promise did not buffer its result")
	}
	if value, open := <-result; open {
		t.Fatalf("result channel remained open with %#v", value)
	}
}

func Test_promise_RejectAlreadyResolved(t *testing.T) {
	promise := &futureValue{state: Pending}
	promise.resolve("done")
	promise.reject(errors.New("late error"))
	if promise.state != Fulfilled || promise.result != "done" {
		t.Fatalf("promise = (%v, %#v), want (Fulfilled, done)", promise.state, promise.result)
	}
}

func Test_promise_DoubleReject(t *testing.T) {
	promise := &futureValue{state: Pending}
	first := errors.New("first")
	promise.reject(first)
	promise.reject(errors.New("second"))
	if promise.state != Rejected || promise.result != first {
		t.Fatalf("promise = (%v, %v), want (Rejected, %v)", promise.state, promise.result, first)
	}
}

func Test_ChainedPromise_FinallyStandalone_Fulfilled(t *testing.T) {
	parent := &Promise{}
	parent.state.Store(int32(Pending))
	called := false
	child := parent.Finally(func() { called = true })
	parent.resolve("value")
	if !called || child.State() != Fulfilled || child.Value() != "value" {
		t.Fatalf("Finally result = (called=%v, %v, %#v), want (true, Fulfilled, value)", called, child.State(), child.Value())
	}
}

func Test_ChainedPromise_FinallyStandalone_Rejected(t *testing.T) {
	parent := &Promise{}
	parent.state.Store(int32(Pending))
	called := false
	child := parent.Finally(func() { called = true })
	parent.reject("reason")
	if !called || child.State() != Rejected || child.Reason() != "reason" {
		t.Fatalf("Finally result = (called=%v, %v, %#v), want (true, Rejected, reason)", called, child.State(), child.Reason())
	}
}

func Test_ChainedPromise_FinallyNilCallback(t *testing.T) {
	parent := &Promise{}
	parent.state.Store(int32(Pending))
	child := parent.Finally(nil)
	parent.resolve("value")
	if child.State() != Fulfilled || child.Value() != "value" {
		t.Fatalf("Finally result = (%v, %#v), want (Fulfilled, value)", child.State(), child.Value())
	}
}

func Test_ChainedPromise_FinallyPanic(t *testing.T) {
	parent := &Promise{}
	parent.state.Store(int32(Pending))
	child := parent.Finally(func() { panic("cleanup panic") })
	parent.resolve("original")
	if child.State() != Fulfilled || child.Value() != "original" {
		t.Fatalf("Finally result = (%v, %#v), want (Fulfilled, original)", child.State(), child.Value())
	}
}

func Test_pSquareQuantile_P99_FewObs(t *testing.T) {
	quantile := newPSquareQuantile(0.99)
	quantile.Update(1)
	quantile.Update(100)
	quantile.Update(50)
	if got := quantile.Quantile(); got != 50 {
		t.Fatalf("p99 before initialization = %v, want 50", got)
	}
}

type testErrorJSOption struct {
	err error
}

func (option *testErrorJSOption) applyJSOption(_ *jsConfig) error {
	return option.err
}

var _ JSOption = (*testErrorJSOption)(nil)

func TestResolveJSOptionsErrorIdentity(t *testing.T) {
	sentinel := errors.New("option error")
	_, err := resolveJSOptions([]JSOption{&testErrorJSOption{err: sentinel}})
	if !errors.Is(err, sentinel) {
		t.Fatalf("resolveJSOptions error = %#v, want wrapped sentinel", err)
	}
}

func Test_CancelTimer_Terminated(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
	if err := loop.CancelTimer(TimerID(1)); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("CancelTimer error = %v, want ErrLoopTerminated", err)
	}
}

func Test_CancelTimers_Terminated(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
	for index, err := range loop.CancelTimers(1, 2) {
		if !errors.Is(err, ErrLoopTerminated) {
			t.Fatalf("CancelTimers error[%d] = %v, want ErrLoopTerminated", index, err)
		}
	}
}

// A nil callback must be rejected before callback admission, so it can never
// be observed as an executed user callback. Substituting a placeholder and
// executing it would fire BeforeCallbackAdmission and record a callback
// sample in user-visible metrics, both of which are real behavioral changes.
func assertNilCallbackNotAdmitted(t *testing.T, call func(loop *Loop, fn func())) {
	t.Helper()
	loop, err := New(WithMetrics(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	admissions := 0
	loop.testHooks = &loopTestHooks{
		BeforeCallbackAdmission: func() { admissions++ },
	}

	before := loop.Metrics()
	call(loop, nil)
	after := loop.Metrics()

	if admissions != 0 {
		t.Fatalf("nil callback reached callback admission %d times, want 0", admissions)
	}
	if after.Latency.Count != before.Latency.Count {
		t.Fatalf("nil callback recorded %d user callbacks, want 0", after.Latency.Count-before.Latency.Count)
	}
	if after.TPS != before.TPS {
		t.Fatalf("nil callback moved TPS from %v to %v, want no throughput sample", before.TPS, after.TPS)
	}
}

func Test_safeExecuteFn_Nil(t *testing.T) {
	assertNilCallbackNotAdmitted(t, func(loop *Loop, fn func()) { loop.safeExecuteFn(fn) })
}

func Test_safeExecute_Nil(t *testing.T) {
	assertNilCallbackNotAdmitted(t, func(loop *Loop, fn func()) { loop.safeExecute(fn) })
}

func Test_promise_ToChannel_AlreadySettled(t *testing.T) {
	promise := &futureValue{state: Pending}
	promise.resolve("done")
	result := promise.ToChannel()
	if value, open := <-result; !open || value != "done" {
		t.Fatalf("result = (%#v, open=%v), want (done, open=true)", value, open)
	}
	if value, open := <-result; open {
		t.Fatalf("result channel remained open with %#v", value)
	}
}

func Test_ChainedPromise_ResolveWithPromise(t *testing.T) {
	parent := &Promise{}
	parent.state.Store(int32(Pending))
	source := &Promise{}
	source.state.Store(int32(Pending))
	parent.resolve(source)
	source.resolve("adopted-value")
	if parent.State() != Fulfilled || parent.Value() != "adopted-value" {
		t.Fatalf("adopter = (%v, %#v), want (Fulfilled, adopted-value)", parent.State(), parent.Value())
	}
}

func Test_ChainedPromise_ResolveWithSelf(t *testing.T) {
	promise := &Promise{}
	promise.state.Store(int32(Pending))
	promise.resolve(promise)
	if promise.State() != Rejected {
		t.Fatalf("state = %v, want Rejected", promise.State())
	}
	reason, ok := promise.Reason().(error)
	if !ok || !errors.Is(reason, ErrPromiseSelfResolution) {
		t.Fatalf("reason = %v, want ErrPromiseSelfResolution", promise.Reason())
	}
}
