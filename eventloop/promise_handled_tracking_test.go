package eventloop

import "testing"

func TestThenWithoutRejectionHandlerDoesNotMarkSourceHandled(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	p, _, _ := js.NewChainedPromise()

	_ = p.Then(func(v any) any {
		return v
	}, nil)

	if p.rejectionHandled.Load() {
		t.Fatal("Then without a rejection handler marked the source promise handled")
	}
}

func TestThenWithRejectionHandlerMarksSourceHandled(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	p, _, _ := js.NewChainedPromise()

	_ = p.Then(nil, func(r any) any {
		return r
	})

	if !p.rejectionHandled.Load() {
		t.Fatal("Then with a rejection handler did not mark the source promise handled")
	}
}

func TestThenWithoutRejectionHandlerReportsPropagatedChildOnce(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	var reasons []any
	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reasons = append(reasons, reason)
	}))

	p, _, reject := js.NewChainedPromise()
	child := p.Then(func(v any) any { return v }, nil)
	reject("boom")

	loop.tick()
	loop.tick()

	if child.State() != Rejected {
		t.Fatalf("expected propagated child rejection, got %v", child.State())
	}
	if child.Reason() != "boom" {
		t.Fatalf("expected child reason boom, got %v", child.Reason())
	}
	if len(reasons) != 1 || reasons[0] != "boom" {
		t.Fatalf("expected exactly one propagated unhandled rejection for child, got %#v", reasons)
	}
}

func TestThenWithoutRejectionHandlerChildCatchSuppressesPropagatedUnhandled(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	var reasons []any
	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reasons = append(reasons, reason)
	}))

	p, _, reject := js.NewChainedPromise()
	child := p.Then(func(v any) any { return v }, nil)
	caught := false
	child.Catch(func(r any) any {
		caught = true
		if r != "boom" {
			t.Fatalf("expected propagated reason boom, got %v", r)
		}
		return "handled"
	})
	reject("boom")

	loop.tick()
	loop.tick()

	if !caught {
		t.Fatal("expected child catch to observe propagated rejection")
	}
	if len(reasons) != 0 {
		t.Fatalf("expected handled propagated child to suppress unhandled diagnostics, got %#v", reasons)
	}
}

func TestUnhandledRejectionHandledBranchCleansRecord(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		t.Fatalf("handled rejection was reported as unhandled: %v", reason)
	}))

	p := &ChainedPromise{js: js}
	p.state.Store(int32(Rejected))
	p.result = "boom"
	p.rejectionHandled.Store(true)

	js.rejectionsMu.Lock()
	js.unhandledRejections[p] = &rejectionInfo{
		promise: p,
		reason:  "boom",
	}
	js.rejectionsMu.Unlock()

	js.checkUnhandledRejections()

	js.rejectionsMu.RLock()
	_, unhandledExists := js.unhandledRejections[p]
	js.rejectionsMu.RUnlock()
	if unhandledExists {
		t.Fatal("handled rejection remained in unhandledRejections")
	}
}
