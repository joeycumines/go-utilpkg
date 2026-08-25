package gojaeventloop

import (
	"context"
	"errors"
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
	runtime := goja.New()
	adapter, err := New(loop, runtime)
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

// awaitTrack runs script inside an async IIFE on the loop and fails the test
// with the rejection message when the IIFE throws.
func awaitTrack(t *testing.T, adapter *Adapter, script string) goja.Value {
	t.Helper()
	type result struct {
		v   goja.Value
		err error
	}
	ch := make(chan result, 1)
	if err := adapter.Submit(func(_ *goja.Runtime) {
		_ = adapter.runtime.Set("__trackDone", func(v goja.Value) { ch <- result{v: v} })
		_ = adapter.runtime.Set("__trackFail", func(msg string) { ch <- result{err: errors.New(msg)} })
		wrapped := `(async function() { ` + script + ` })()
		.then(function(v) { __trackDone(v); }, function(e) { __trackFail(e && e.message ? e.message : String(e)); });`
		if _, runErr := adapter.runtime.RunString(wrapped); runErr != nil {
			ch <- result{err: runErr}
		}
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("script: %v", r.err)
		}
		return r.v
	case <-time.After(10 * time.Second):
		t.Fatal("awaitTrack timed out")
		return nil
	}
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
	runtime := goja.New()
	adapter, err := New(loop, runtime)
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

func aUndefined(rt *goja.Runtime) goja.Value { return goja.Undefined() }

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
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
