package gojaeventloop

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// newTrackTestAdapter starts a bound adapter on a running loop.
func newTrackTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = loop.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = loop.Shutdown(context.Background())
	})
	return adapter
}

func TestTrackPromise_ResolveOnOwner(t *testing.T) {
	a := newTrackTestAdapter(t)
	done := make(chan string, 1)
	if err := a.Submit(func(_ *goja.Runtime) {
		promise := a.TrackPromise(context.Background(), func(ctx context.Context, settle TrackedSettlement) {
			settle.Settle(false, func(rt *goja.Runtime) any {
				return "ok"
			})
		})
		thenFn, _ := goja.AssertFunction(promise.ToObject(a.runtime).Get("then"))
		_, _ = thenFn(promise, a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			done <- call.Argument(0).String()
			return goja.Undefined()
		}))
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if !strings.Contains(got, "ok") {
			t.Fatalf("resolved value = %s", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPromisify_RejectsGoError(t *testing.T) {
	a := newTrackTestAdapter(t)
	val := make(chan string, 1)
	err := a.Submit(func(_ *goja.Runtime) {
		p := a.Promisify(context.Background(), func(ctx context.Context) (any, error) {
			return nil, errors.New("boom reason")
		})
		thenFn, _ := goja.AssertFunction(p.ToObject(a.runtime).Get("then"))
		_, _ = thenFn(p,
			a.runtime.ToValue(func(c goja.FunctionCall) goja.Value { return goja.Undefined() }),
			a.runtime.ToValue(func(c goja.FunctionCall) goja.Value {
				val <- c.Argument(0).String()
				return goja.Undefined()
			}))
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-val:
		if !strings.Contains(msg, "boom reason") {
			t.Fatalf("rejection = %s", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPromisify_NilResultUndefined(t *testing.T) {
	a := newTrackTestAdapter(t)
	got := make(chan goja.Value, 1)
	if err := a.Submit(func(_ *goja.Runtime) {
		p := a.Promisify(context.Background(), func(ctx context.Context) (any, error) {
			return nil, nil
		})
		thenFn, _ := goja.AssertFunction(p.ToObject(a.runtime).Get("then"))
		_, _ = thenFn(p, a.runtime.ToValue(func(c goja.FunctionCall) goja.Value {
			got <- c.Argument(0)
			return goja.Undefined()
		}))
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case v := <-got:
		if !goja.IsUndefined(v) {
			t.Fatalf("nil result should fulfill undefined, got %v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestTrackPromise_TerminalSweepRejectsPending(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	loopDone := make(chan struct{})
	go func() { _ = loop.Run(ctx); close(loopDone) }()

	var promiseVal goja.Value
	_ = adapter.Submit(func(_ *goja.Runtime) {
		promiseVal = adapter.TrackPromise(ctx, func(ctx context.Context, settle TrackedSettlement) {
			<-ctx.Done() // abandoned: returns without settling
		})
	})

	time.Sleep(100 * time.Millisecond)
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	<-loopDone

	if promiseVal == nil {
		t.Fatal("promise was never created")
	}
	native, ok := promiseVal.Export().(*goja.Promise)
	if !ok {
		t.Fatalf("exported %T, want *goja.Promise", promiseVal.Export())
	}
	if native.State() != goja.PromiseStateRejected {
		t.Fatalf("terminal state = %v, want rejected (sweep must dispose pending promises)", native.State())
	}
}

func TestTrackPromise_ExactlyOnceUnderRace(t *testing.T) {
	a := newTrackTestAdapter(t)
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		err := a.Submit(func(_ *goja.Runtime) {
			defer wg.Done()
			p := a.TrackPromise(context.Background(), func(ctx context.Context, settle TrackedSettlement) {
				var inner sync.WaitGroup
				inner.Add(2)
				go func() { defer inner.Done(); _ = settle.Settle(false, aUndefinedRT) }()
				go func() { defer inner.Done(); _ = settle.Settle(true, aErrRT) }()
				inner.Wait()
			})
			_ = p
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}

func aUndefinedRT(rt *goja.Runtime) any { return goja.Undefined() }
func aErrRT(rt *goja.Runtime) any       { return rt.NewGoError(errors.New("lost race")) }

func TestTrackPromise_NilRunPanics(t *testing.T) {
	a := newTrackTestAdapter(t)
	errCh := make(chan error, 1)
	if err := a.Submit(func(_ *goja.Runtime) {
		defer func() {
			if recover() == nil {
				errCh <- errors.New("expected panic for nil run")
			} else {
				errCh <- nil
			}
		}()
		_ = a.TrackPromise(context.Background(), nil)
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestTrackPromise_CtxCancellationPropagates(t *testing.T) {
	a := newTrackTestAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	sawCancel := make(chan error, 1)
	started := make(chan struct{})
	if err := a.Submit(func(_ *goja.Runtime) {
		_ = a.TrackPromise(ctx, func(ctx context.Context, settle TrackedSettlement) {
			close(started)
			select {
			case <-ctx.Done():
				sawCancel <- ctx.Err()
			case <-time.After(5 * time.Second):
				sawCancel <- errors.New("cancellation not observed")
			}
		})
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	select {
	case err := <-sawCancel:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ctx.Err() = %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("timeout")
	}
}

func TestTrackPromise_AutoExitWaitsForWork(t *testing.T) {
	loop, err := goeventloop.New(goeventloop.WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	loopDone := make(chan struct{})
	go func() { _ = loop.Run(ctx); close(loopDone) }()

	completed := make(chan struct{}, 1)
	_ = adapter.Submit(func(_ *goja.Runtime) {
		_ = adapter.TrackPromise(context.Background(), func(ctx context.Context, settle TrackedSettlement) {
			time.Sleep(300 * time.Millisecond)
			_ = settle.Settle(false, aUndefinedRT)
			completed <- struct{}{}
		})
	})

	select {
	case <-completed:
		// Work completed while loop had no other refs: proves promisifyCount
		// liveness kept auto-exit from committing mid-work.
	case <-time.After(5 * time.Second):
		t.Fatal("tracked work did not complete under auto-exit pressure")
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	_ = loop.Shutdown(shutdownCtx)
}

// TestTrackPromise_PreCanceledContextSettles pins the carrier-decoupling
// fast-path: if ctx is already canceled before admission, TrackPromise returns
// an immediately rejected promise without spawning a worker or bridge. This
// verifies the intent-explicit fast-path for already-canceled contexts.
func TestTrackPromise_PreCanceledContextSettles(t *testing.T) {
	a := newTrackTestAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled before admission — fast-path should reject without spawning worker

	done := make(chan string, 1)
	runCalled := make(chan struct{}, 1)
	if err := a.Submit(func(_ *goja.Runtime) {
		promiseVal := a.TrackPromise(ctx, func(ctx context.Context, settle TrackedSettlement) {
			runCalled <- struct{}{}
			_ = settle.Settle(true, func(rt *goja.Runtime) any {
				return rt.NewGoError(ctx.Err())
			})
		})
		thenFn, _ := goja.AssertFunction(promiseVal.ToObject(a.runtime).Get("then"))
		_, _ = thenFn(promiseVal,
			a.runtime.ToValue(func(c goja.FunctionCall) goja.Value {
				t.Error("promise fulfilled unexpectedly")
				return goja.Undefined()
			}),
			a.runtime.ToValue(func(c goja.FunctionCall) goja.Value {
				done <- c.Argument(0).String()
				return goja.Undefined()
			}))
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-done:
		if !strings.Contains(msg, "context canceled") {
			t.Fatalf("rejection reason = %s, want context canceled", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: pre-cancelled tracked promise stranded pending")
	}
	select {
	case <-runCalled:
		t.Fatal("run was called: fast-path should reject without spawning worker for already-canceled ctx")
	default:
	}
	a.bridgesMu.Lock()
	n := len(a.pendingBridges)
	a.bridgesMu.Unlock()
	if n != 0 {
		t.Fatalf("pendingBridges = %d, want 0 (bridge leak)", n)
	}
}

// TestPromisify_PreCanceledContextRejects verifies the fast-path via TrackPromise: pre-canceled ctx is rejected immediately without spawning a worker. The future-entry-gate path (ctx canceled after admission) is covered by the sugar's ctx.Err() check inside TrackPromise's run, not this pre-canceled test.
func TestPromisify_PreCanceledContextRejects(t *testing.T) {
	a := newTrackTestAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	done := make(chan string, 1)
	if err := a.Submit(func(_ *goja.Runtime) {
		promiseVal := a.Promisify(ctx, func(ctx context.Context) (any, error) {
			return "unexpected", nil
		})
		thenFn, _ := goja.AssertFunction(promiseVal.ToObject(a.runtime).Get("then"))
		_, _ = thenFn(promiseVal,
			a.runtime.ToValue(func(c goja.FunctionCall) goja.Value {
				t.Error("promise fulfilled unexpectedly")
				return goja.Undefined()
			}),
			a.runtime.ToValue(func(c goja.FunctionCall) goja.Value {
				done <- c.Argument(0).String()
				return goja.Undefined()
			}))
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-done:
		if !strings.Contains(msg, "context canceled") {
			t.Fatalf("rejection reason = %s, want context canceled", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: pre-cancelled promisified promise stranded pending")
	}
}

// TestTrackPromise_SweepAfterImmediateClose verifies terminal sweep disposes
// pending bridges under immediate Close: Close does not wait for claimed
// workers, but the sweep (on drain owner) disposes any bridge whose Submit
// was rejected (terminal). No off-owner handback is needed.
func TestTrackPromise_SweepAfterImmediateClose(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	loopDone := make(chan struct{})
	go func() { _ = loop.Run(ctx); close(loopDone) }()

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	var promiseVal goja.Value
	_ = adapter.Submit(func(_ *goja.Runtime) {
		promiseVal = adapter.TrackPromise(ctx, func(ctx context.Context, settle TrackedSettlement) {
			close(started)
			<-release
			_ = settle.Settle(false, func(rt *goja.Runtime) any {
				return "too late for submission"
			})
			close(done)
		})
	})
	<-started

	if err := loop.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	close(release)
	<-done
	<-loopDone

	if promiseVal == nil {
		t.Fatal("promise was never created")
	}
	native, ok := promiseVal.Export().(*goja.Promise)
	if !ok {
		t.Fatalf("exported %T, want *goja.Promise", promiseVal.Export())
	}
	if native.State() == goja.PromiseStatePending {
		t.Fatal("promise left pending: sweep after immediate-Close should have rejected it")
	}
	adapter.bridgesMu.Lock()
	n := len(adapter.pendingBridges)
	adapter.bridgesMu.Unlock()
	if n != 0 {
		t.Fatalf("pendingBridges = %d after sweep, want 0 (bridge leak)", n)
	}
}

// assertTrackedGoReason verifies a rejected tracked promise carries a real
// JavaScript Error (GoError): .name "GoError", .message text, and a String()
// form. A raw Go error leaked to rawReject converts to an opaque reflect host
// object whose message/name are undefined — exactly what this assertion
// rejects.
func assertTrackedGoReason(t *testing.T, promiseVal goja.Value, wantMessage string) {
	t.Helper()
	native, ok := promiseVal.Export().(*goja.Promise)
	if !ok {
		t.Fatalf("exported %T, want *goja.Promise", promiseVal.Export())
	}
	if native.State() != goja.PromiseStateRejected {
		t.Fatalf("state = %v, want rejected", native.State())
	}
	reason, ok := native.Result().(*goja.Object)
	if !ok {
		t.Fatalf("rejection reason %T (%v), want *goja.Object", native.Result(), native.Result())
	}
	name := reason.Get("name")
	if name == nil || goja.IsUndefined(name) || name.String() != "GoError" {
		t.Fatalf("rejection .name = %v, want GoError (reason must be a JS Error, not an opaque host object)", name)
	}
	message := reason.Get("message")
	if message == nil || goja.IsUndefined(message) || !strings.Contains(message.String(), wantMessage) {
		t.Fatalf("rejection .message = %v, want substring %q", message, wantMessage)
	}
	if !strings.Contains(reason.String(), wantMessage) {
		t.Fatalf("String(reason) = %s, want substring %q", reason.String(), wantMessage)
	}
}

// TestTrackPromise_TerminalSweepRejectsPendingWithGoError pins the JS-visible
// payload of the terminal sweep: the graceful-disposal rejection must be a
// real Error (message readable in catch blocks), matching the documented
// "ErrLoopTerminated GoError" — not an opaque host object.
func TestTrackPromise_TerminalSweepRejectsPendingWithGoError(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	loopDone := make(chan struct{})
	go func() { _ = loop.Run(ctx); close(loopDone) }()

	var promiseVal goja.Value
	_ = adapter.Submit(func(_ *goja.Runtime) {
		promiseVal = adapter.TrackPromise(ctx, func(ctx context.Context, settle TrackedSettlement) {
			<-ctx.Done() // abandoned: returns without settling
		})
	})

	time.Sleep(100 * time.Millisecond)
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	<-loopDone

	if promiseVal == nil {
		t.Fatal("promise was never created")
	}
	assertTrackedGoReason(t, promiseVal, goeventloop.ErrLoopTerminated.Error())

	adapter.bridgesMu.Lock()
	n := len(adapter.pendingBridges)
	adapter.bridgesMu.Unlock()
	if n != 0 {
		t.Fatalf("pendingBridges = %d after sweep, want 0 (bridge leak)", n)
	}
}

// TestTrackPromise_AdmissionRefusedRejectsWithGoError covers the synchronous
// admission-refusal path: once the loop is terminal, TrackPromise must reject
// immediately with a GoError-shaped reason (the future's actual refusal
// reason), dispose the bridge synchronously, and leave no leaks.
func TestTrackPromise_AdmissionRefusedRejectsWithGoError(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	loopDone := make(chan struct{})
	go func() { _ = loop.Run(ctx); close(loopDone) }()
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	<-loopDone

	// The loop is fully terminated: no callback can race this exclusive,
	// off-loop access (claim survives termination by contract).
	runCalled := false
	promiseVal := adapter.TrackPromise(context.Background(), func(_ context.Context, _ TrackedSettlement) {
		runCalled = true
	})
	if runCalled {
		t.Fatal("run must never execute when admission is refused")
	}
	assertTrackedGoReason(t, promiseVal, goeventloop.ErrLoopTerminated.Error())

	adapter.bridgesMu.Lock()
	n := len(adapter.pendingBridges)
	adapter.bridgesMu.Unlock()
	if n != 0 {
		t.Fatalf("pendingBridges = %d after refused admission, want 0 (bridge leak)", n)
	}
}

// TestPromisify_PanickingFnRejectsPromptly pins liveness for runtime failures:
// a panicking fn must reject the returned promise promptly with a GoError
// carrying the panic value — not strand pending until terminal cleanup.
func TestPromisify_PanickingFnRejectsPromptly(t *testing.T) {
	a := newTrackTestAdapter(t)
	msg := make(chan string, 1)
	if err := a.Submit(func(_ *goja.Runtime) {
		promiseVal := a.Promisify(context.Background(), func(ctx context.Context) (any, error) {
			panic("boom panic value")
		})
		thenFn, ok := goja.AssertFunction(promiseVal.ToObject(a.runtime).Get("then"))
		if !ok {
			t.Error("promise.then is not callable")
			return
		}
		if _, err := thenFn(promiseVal, goja.Undefined(), a.runtime.ToValue(func(c goja.FunctionCall) goja.Value {
			e := c.Argument(0)
			if m := e.ToObject(a.runtime).Get("message"); m != nil && !goja.IsUndefined(m) {
				msg <- m.String()
			} else {
				msg <- ""
			}
			return goja.Undefined()
		})); err != nil {
			t.Errorf("attach then: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	var got string
	select {
	case got = <-msg:
	case <-time.After(5 * time.Second):
		t.Fatal("rejection not observed while loop running: panicking fn stranded the promise pending")
	}
	if !strings.Contains(got, "promisify callback panicked") || !strings.Contains(got, "boom panic value") {
		t.Fatalf("rejection message = %q, want panic surfaced via GoError", got)
	}
	a.bridgesMu.Lock()
	n := len(a.pendingBridges)
	a.bridgesMu.Unlock()
	if n != 0 {
		t.Fatalf("pendingBridges = %d after prompt disposal, want 0 (bridge leak)", n)
	}
}

// TestTrackPromise_PanickingRunRejectsPromptly extends prompt-failure liveness
// to raw TrackPromise: a panicking run disposes its own promise with a
// GoError instead of waiting for the terminal sweep.
func TestTrackPromise_PanickingRunRejectsPromptly(t *testing.T) {
	a := newTrackTestAdapter(t)
	msg := make(chan string, 1)
	if err := a.Submit(func(_ *goja.Runtime) {
		promiseVal := a.TrackPromise(context.Background(), func(ctx context.Context, settle TrackedSettlement) {
			panic("run exploded")
		})
		thenFn, ok := goja.AssertFunction(promiseVal.ToObject(a.runtime).Get("then"))
		if !ok {
			t.Error("promise.then is not callable")
			return
		}
		if _, err := thenFn(promiseVal, goja.Undefined(), a.runtime.ToValue(func(c goja.FunctionCall) goja.Value {
			e := c.Argument(0)
			if m := e.ToObject(a.runtime).Get("message"); m != nil && !goja.IsUndefined(m) {
				msg <- m.String()
			} else {
				msg <- ""
			}
			return goja.Undefined()
		})); err != nil {
			t.Errorf("attach then: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	var got string
	select {
	case got = <-msg:
	case <-time.After(5 * time.Second):
		t.Fatal("rejection not observed while loop running: panicking run stranded the promise pending")
	}
	if !strings.Contains(got, "TrackPromise callback panicked") || !strings.Contains(got, "run exploded") {
		t.Fatalf("rejection message = %q, want panic surfaced via GoError", got)
	}
	a.bridgesMu.Lock()
	n := len(a.pendingBridges)
	a.bridgesMu.Unlock()
	if n != 0 {
		t.Fatalf("pendingBridges = %d after prompt disposal, want 0 (bridge leak)", n)
	}
}

// TestPromisify_GoexitFnRejectsPromptly mirrors the eventloop's ErrGoexit
// contract at the Goja boundary: a fn that exits via runtime.Goexit must
// reject promptly instead of stranding pending until shutdown.
func TestPromisify_GoexitFnRejectsPromptly(t *testing.T) {
	a := newTrackTestAdapter(t)
	msg := make(chan string, 1)
	if err := a.Submit(func(_ *goja.Runtime) {
		promiseVal := a.Promisify(context.Background(), func(ctx context.Context) (any, error) {
			runtime.Goexit()
			return nil, nil // unreachable
		})
		thenFn, ok := goja.AssertFunction(promiseVal.ToObject(a.runtime).Get("then"))
		if !ok {
			t.Error("promise.then is not callable")
			return
		}
		if _, err := thenFn(promiseVal, goja.Undefined(), a.runtime.ToValue(func(c goja.FunctionCall) goja.Value {
			e := c.Argument(0)
			if m := e.ToObject(a.runtime).Get("message"); m != nil && !goja.IsUndefined(m) {
				msg <- m.String()
			} else {
				msg <- ""
			}
			return goja.Undefined()
		})); err != nil {
			t.Errorf("attach then: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	var got string
	select {
	case got = <-msg:
	case <-time.After(5 * time.Second):
		t.Fatal("rejection not observed while loop running: Goexit stranded the promise pending")
	}
	if !strings.Contains(got, "runtime.Goexit") {
		t.Fatalf("rejection message = %q, want Goexit surfaced via GoError", got)
	}
	a.bridgesMu.Lock()
	n := len(a.pendingBridges)
	a.bridgesMu.Unlock()
	if n != 0 {
		t.Fatalf("pendingBridges = %d after prompt disposal, want 0 (bridge leak)", n)
	}
}

// TestTrackPromise_DirectNilFulfillmentUndefined pins the TrackedSettlement
// doc contract "(nil maps to undefined)" on the direct settlement path: a
// nil product must fulfill undefined, never Go null.
func TestTrackPromise_DirectNilFulfillmentUndefined(t *testing.T) {
	a := newTrackTestAdapter(t)
	got := make(chan goja.Value, 1)
	if err := a.Submit(func(_ *goja.Runtime) {
		p := a.TrackPromise(context.Background(), func(ctx context.Context, settle TrackedSettlement) {
			_ = settle.Settle(false, func(rt *goja.Runtime) any {
				return nil
			})
		})
		thenFn, _ := goja.AssertFunction(p.ToObject(a.runtime).Get("then"))
		_, _ = thenFn(p, a.runtime.ToValue(func(c goja.FunctionCall) goja.Value {
			got <- c.Argument(0)
			return goja.Undefined()
		}))
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case v := <-got:
		if !goja.IsUndefined(v) {
			t.Fatalf("nil fulfillment = %v (%s), want undefined", v, v.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: nil fulfillment stranded pending")
	}
}

// TestTrackPromise_DuplicateSettleReturnsTerminated pins the synchronous
// claim ticket: a second Settle on the same TrackedSettlement must fail fast
// with ErrLoopTerminated without enqueuing a second owner callback. This
// preserves the exactly-once return contract while keeping the bridge
// sweep-visible for hard-Close.
func TestTrackPromise_DuplicateSettleReturnsTerminated(t *testing.T) {
	a := newTrackTestAdapter(t)
	result := make(chan []error, 1)
	settled := make(chan string, 1)
	if err := a.Submit(func(_ *goja.Runtime) {
		p := a.TrackPromise(context.Background(), func(ctx context.Context, settle TrackedSettlement) {
			e1 := settle.Settle(false, func(rt *goja.Runtime) any { return "first" })
			e2 := settle.Settle(false, func(rt *goja.Runtime) any { return "second" })
			result <- []error{e1, e2}
		})
		thenFn, _ := goja.AssertFunction(p.ToObject(a.runtime).Get("then"))
		_, _ = thenFn(p,
			a.runtime.ToValue(func(c goja.FunctionCall) goja.Value {
				settled <- "fulfilled:" + c.Argument(0).String()
				return goja.Undefined()
			}),
			a.runtime.ToValue(func(c goja.FunctionCall) goja.Value {
				settled <- "rejected:" + c.Argument(0).String()
				return goja.Undefined()
			}),
		)
	}); err != nil {
		t.Fatal(err)
	}
	var errs []error
	select {
	case errs = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: duplicate settle result not observed")
	}
	if len(errs) != 2 {
		t.Fatalf("errors len = %d, want 2", len(errs))
	}
	if errs[0] != nil {
		t.Fatalf("first Settle error = %v, want nil", errs[0])
	}
	if !errors.Is(errs[1], goeventloop.ErrLoopTerminated) {
		t.Fatalf("second Settle error = %v, want ErrLoopTerminated", errs[1])
	}
	select {
	case v := <-settled:
		if v != "fulfilled:first" {
			t.Fatalf("settlement = %q, want fulfilled:first (first wins, second must not compete)", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: duplicate settlement promise not settled")
	}
	a.bridgesMu.Lock()
	n := len(a.pendingBridges)
	a.bridgesMu.Unlock()
	if n != 0 {
		t.Fatalf("pendingBridges = %d, want 0", n)
	}
}

// TestTrackPromise_SettleThenPanicKeepsFirstSettlement verifies that a run
// which settles successfully and then panics preserves the first settlement.
// The panic recovery's second settlement must fail with ErrLoopTerminated
// (synchronous claim) and not race the first via the owner queue.
func TestTrackPromise_SettleThenPanicKeepsFirstSettlement(t *testing.T) {
	a := newTrackTestAdapter(t)
	got := make(chan string, 1)
	if err := a.Submit(func(_ *goja.Runtime) {
		p := a.TrackPromise(context.Background(), func(ctx context.Context, settle TrackedSettlement) {
			_ = settle.Settle(false, func(rt *goja.Runtime) any { return "ok" })
			panic("boom after settle")
		})
		thenFn, _ := goja.AssertFunction(p.ToObject(a.runtime).Get("then"))
		_, _ = thenFn(p,
			a.runtime.ToValue(func(c goja.FunctionCall) goja.Value {
				got <- "fulfilled:" + c.Argument(0).String()
				return goja.Undefined()
			}),
			a.runtime.ToValue(func(c goja.FunctionCall) goja.Value {
				got <- "rejected:" + c.Argument(0).String()
				return goja.Undefined()
			}),
		)
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case v := <-got:
		if v != "fulfilled:ok" {
			t.Fatalf("settlement = %q, want fulfilled:ok (panic after settle must not override first)", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: settle-then-panic promise not settled")
	}
	a.bridgesMu.Lock()
	n := len(a.pendingBridges)
	a.bridgesMu.Unlock()
	if n != 0 {
		t.Fatalf("pendingBridges = %d after settle-then-panic, want 0", n)
	}
}

// TestTrackPromise_AdmittedSettlementSurvivesGracefulShutdown pins the
// claimed-bridge drain-before-sweep guarantee for graceful paths.
// A worker's successful Settle (Submit admitted, entry remains with
// claimed==true until owner callback) must survive a concurrent graceful
// Shutdown: the drain (drainTerminalQueuesStarted) must execute the admitted
// owner callback before sweepTrackedBridges, so the promise fulfills with the
// worker's value rather than being swept to ErrLoopTerminated.
//
// This is the exact race identified in review-07: claimed leaves entry
// sweep-visible, so correctness depends on Loop draining admitted external
// tasks before sweeping. The test forces the race by blocking the owner,
// enqueuing an admitted settlement behind the block, then triggering Shutdown
// while the settlement is still queued.
func TestTrackPromise_AdmittedSettlementSurvivesGracefulShutdown(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	loopDone := make(chan struct{})
	go func() { _ = loop.Run(ctx); close(loopDone) }()
	t.Cleanup(func() {
		cancel()
		_ = loop.Shutdown(context.Background())
		<-loopDone
	})

	blockOwner := make(chan struct{})
	promiseReady := make(chan goja.Value, 1)
	workerStarted := make(chan struct{})
	allowSettle := make(chan struct{})
	settlementErr := make(chan error, 1)

	// Submit a blocking owner task that will hold the logical owner
	// while we enqueue an admitted settlement behind it.
	if err := adapter.Submit(func(_ *goja.Runtime) {
		// Inside owner: create the tracked promise; its worker will signal
		// and wait for allowSettle before settling.
		p := adapter.TrackPromise(context.Background(), func(ctx context.Context, settle TrackedSettlement) {
			close(workerStarted)
			<-allowSettle
			err := settle.Settle(false, func(rt *goja.Runtime) any { return "admitted" })
			settlementErr <- err
		})
		promiseReady <- p
		// Block owner until test releases — this keeps the settlement's
		// owner callback (enqueued via Submit) from executing.
		<-blockOwner
	}); err != nil {
		t.Fatal(err)
	}

	// Wait for worker to be running and blocked on allowSettle
	select {
	case <-workerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: worker not started")
	}

	// Allow worker to settle — this does claim -> Submit (should succeed,
	// entry remains with claimed==true, callback queued behind blockOwner)
	close(allowSettle)
	var errSettle error
	select {
	case errSettle = <-settlementErr:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: settlement not attempted")
	}
	if errSettle != nil {
		t.Fatalf("Settle returned %v, want nil (admitted)", errSettle)
	}

	// At this point the settlement's owner callback is queued but not
	// executed because owner is still blocked on blockOwner.
	// Trigger graceful Shutdown concurrently — it will store StateTerminated
	// and then attempt to drain, but drain cannot complete until owner
	// unblocks. The settlement must be drained before sweep.
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- loop.Shutdown(context.Background())
	}()

	// Give Shutdown a moment to commit StateTerminating/StateTerminated
	// (it will be waiting for owner to unblock to finish drain/termination)
	time.Sleep(50 * time.Millisecond)

	// Unblock owner — this lets the blocking task return, then the owner
	// can drain the pending settlement before sweeping.
	close(blockOwner)

	// Wait for Shutdown to complete
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: Shutdown did not complete")
	}
	<-loopDone

	// Promise should have been created
	var promiseVal goja.Value
	select {
	case promiseVal = <-promiseReady:
	case <-time.After(1 * time.Second):
		t.Fatal("promise never created")
	}
	if promiseVal == nil {
		t.Fatal("promise was nil")
	}
	native, ok := promiseVal.Export().(*goja.Promise)
	if !ok {
		t.Fatalf("exported %T, want *goja.Promise", promiseVal.Export())
	}
	// Must be fulfilled with admitted value, not rejected with ErrLoopTerminated
	if native.State() != goja.PromiseStateFulfilled {
		// Surface rejection reason for diagnostics
		if native.State() == goja.PromiseStateRejected {
			reason := native.Result()
			if obj, ok := reason.(*goja.Object); ok {
				t.Fatalf("promise rejected (want fulfilled:admitted) — state rejected, reason name=%v message=%v string=%v", obj.Get("name"), obj.Get("message"), obj.String())
			}
			t.Fatalf("promise rejected (want fulfilled:admitted) — reason %v (%T)", reason, reason)
		}
		t.Fatalf("promise state = %v, want fulfilled", native.State())
	}
	if got := native.Result(); got == nil || got.String() != "admitted" {
		// Goja may wrap string as goja.Value; check via String()
		t.Fatalf("fulfilled value = %v (%T) string=%q, want admitted", native.Result(), native.Result(), fmt.Sprint(native.Result()))
	}
	adapter.bridgesMu.Lock()
	n := len(adapter.pendingBridges)
	adapter.bridgesMu.Unlock()
	if n != 0 {
		t.Fatalf("pendingBridges = %d after admitted graceful shutdown, want 0", n)
	}
}
