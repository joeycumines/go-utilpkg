package eventloop

import (
	"errors"
	"testing"
)

func TestTrackRejection_DuplicateMicrotaskPrevention(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	const count = 10
	reported := make(chan any, count)
	js := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))
	want := make(map[any]int, count)
	for i := range count {
		_, _, reject := js.NewChainedPromise()
		reject(i)
		want[i] = 1
	}
	if !js.checkRejectionScheduled.Load() {
		t.Fatal("multiple pending rejections did not publish one scheduled checkpoint")
	}
	loop.tick()

	got := make(map[any]int, count)
	for range count {
		got[waitContractValue(t, reported, "coalesced unhandled-rejection report")]++
	}
	select {
	case extra := <-reported:
		t.Fatalf("extra unhandled-rejection report: %#v", extra)
	default:
	}
	for reason, wantCount := range want {
		if got[reason] != wantCount {
			t.Fatalf("report counts = %#v, want each reason exactly once", got)
		}
	}
	assertUnhandledRejectionTrackingDrained(t, js)
}

func TestTrackRejection_HandlerReadyChannelSignaling(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	reported := make(chan any, 1)
	js := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))

	promise, _, reject := js.NewChainedPromise()
	reject("handled")
	js.handlerReadyMu.Lock()
	ready, exists := js.handlerReadyChans[promise]
	js.handlerReadyMu.Unlock()
	if !exists {
		t.Fatal("rejection did not publish its handler-ready channel")
	}
	child := promise.Catch(func(reason any) any { return "caught: " + reason.(string) })
	select {
	case <-ready:
	default:
		t.Fatal("Catch did not synchronously signal handler readiness")
	}
	loop.tick()

	if child.State() != Fulfilled || child.Value() != "caught: handled" {
		t.Fatalf("Catch child = (%v, %#v), want (Fulfilled, caught: handled)", child.State(), child.Value())
	}
	select {
	case reason := <-reported:
		t.Fatalf("handled rejection was reported: %#v", reason)
	default:
	}
	assertUnhandledRejectionTrackingDrained(t, js)
}

func TestTrackRejection_HandleAfterCheck(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	reported := make(chan any, 1)
	js := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))

	reason := errors.New("late handler")
	promise, _, reject := js.NewChainedPromise()
	reject(reason)
	loop.tick()
	if got := waitContractValue(t, reported, "pre-handler unhandled-rejection report"); got != reason {
		t.Fatalf("reported reason = %v, want %v", got, reason)
	}

	child := promise.Catch(func(got any) any {
		if got != reason {
			t.Fatalf("Catch reason = %v, want %v", got, reason)
		}
		return "handled"
	})
	loop.tick()
	if child.State() != Fulfilled || child.Value() != "handled" {
		t.Fatalf("late Catch child = (%v, %#v), want (Fulfilled, handled)", child.State(), child.Value())
	}
	select {
	case extra := <-reported:
		t.Fatalf("late handler caused duplicate report: %#v", extra)
	default:
	}
}

func TestTrackRejection_CheckRejectionScheduledReset(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	reported := make(chan any, 2)
	js := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))

	for _, reason := range []string{"first", "second"} {
		_, _, reject := js.NewChainedPromise()
		reject(reason)
		if !js.checkRejectionScheduled.Load() {
			t.Fatalf("%s rejection did not schedule a checkpoint", reason)
		}
		loop.tick()
		if got := waitContractValue(t, reported, reason+" rejection report"); got != reason {
			t.Fatalf("reported reason = %#v, want %q", got, reason)
		}
		if js.checkRejectionScheduled.Load() {
			t.Fatalf("scheduled flag remained set after %s checkpoint", reason)
		}
	}
	assertUnhandledRejectionTrackingDrained(t, js)
}

func TestTrackRejection_RejectionInfoStorage(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop, WithUnhandledRejection(func(any) {}))

	promise, _, reject := js.NewChainedPromise()
	reason := errors.New("specific error")
	reject(reason)
	js.rejectionsMu.RLock()
	info := js.unhandledRejections[promise]
	js.rejectionsMu.RUnlock()
	if info == nil {
		t.Fatal("rejection info was not stored")
	}
	if info.promise != promise || info.reason != reason || info.timestamp <= 0 {
		t.Fatalf("rejection info = %#v, want promise=%p reason=%v positive timestamp", info, promise, reason)
	}
	loop.tick()
	assertUnhandledRejectionTrackingDrained(t, js)
}

func TestTrackRejection_NilCallback(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)

	_, _, reject := js.NewChainedPromise()
	reject("no callback")
	loop.tick()
	assertUnhandledRejectionTrackingDrained(t, js)
}
