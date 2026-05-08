package eventloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

func TestChainedPromise_UnhandledCallbackRunsOnLoopGoroutineDuringCheckpoint(t *testing.T) {
	affinity := make(chan bool, 1)
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop,
		WithUnhandledRejection(func(any) {
			affinity <- loop.isLoopThread()
		}),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)

	if err := loop.Submit(func() {
		_, _, reject := js.NewChainedPromise()
		reject("loop-affine")
	}); err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	select {
	case onLoop := <-affinity:
		if !onLoop {
			t.Fatal("in-loop unhandled rejection callback did not run on the loop goroutine")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("unhandled rejection callback did not run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := loop.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}
}

func TestChainedPromise_UnhandledFallbackAfterTerminationIsIsolatedOffLoop(t *testing.T) {
	affinity := make(chan bool, 1)
	loop := New()

	js := NewJS(loop,
		WithUnhandledRejection(func(any) { affinity <- loop.isLoopThread() }),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	_, _, reject := js.NewChainedPromise()
	reject("post-termination")

	select {
	case onLoop := <-affinity:
		if onLoop {
			t.Fatal("post-termination fallback unexpectedly claimed loop-goroutine affinity")
		}
	case <-time.After(time.Second):
		t.Fatal("post-termination fallback did not report rejection")
	}
}

func TestChainedPromise_TerminalDrainFallbackRetainsOwner(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	type callbackResult struct {
		ownsLocalQueues bool
		shutdownErr     error
	}
	callbackDone := make(chan callbackResult, 1)
	js := NewJS(loop,
		WithUnhandledRejection(func(any) {
			callbackDone <- callbackResult{
				ownsLocalQueues: loop.ownsLocalQueues(),
				shutdownErr:     loop.Shutdown(context.Background()),
			}
		}),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)

	var rejectOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeTerminateState: func() {
			rejectOnce.Do(func() {
				rejected := make(chan struct{})
				go func() {
					js.Reject("terminal-drain")
					close(rejected)
				}()
				<-rejected
			})
		},
	}

	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	result := waitContractValue(t, callbackDone, "terminal-drain rejection callback")
	if !result.ownsLocalQueues {
		t.Fatal("terminal-drain rejection fallback lost logical drain ownership")
	}
	if result.shutdownErr != nil {
		t.Fatalf("Shutdown from terminal-drain rejection fallback = %v, want nil acknowledgement", result.shutdownErr)
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, js)
}

func TestChainedPromise_TerminalDrainFallbackRetainsReentrantRejection(t *testing.T) {
	for _, test := range []struct {
		name string
		mode UnhandledRejectionFallbackMode
	}{
		{name: "isolated", mode: UnhandledRejectionFallbackIsolated},
		{name: "disabled", mode: UnhandledRejectionFallbackDisabled},
	} {
		t.Run(test.name, func(t *testing.T) {
			reported := make(chan any, 3)
			var js *JS
			record := func(reason any) {
				reported <- reason
				if reason == "first" {
					js.Reject("second")
				}
			}

			var logger *logiface.Logger[logiface.Event]
			if test.mode == UnhandledRejectionFallbackDisabled {
				logger = logiface.New[*testEvent](
					logiface.WithEventFactory[*testEvent](&testEventFactory{}),
					logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(event *testEvent) error {
						record(event.fields["reason"])
						return nil
					})),
				).Logger()
			}
			loop := New(WithLogger(logger))
			registerLoopCleanupT(t, loop)
			js = NewJS(loop,
				WithUnhandledRejection(record),
				WithUnhandledRejectionFallback(test.mode),
			)

			loop.testHooks = &loopTestHooks{
				AfterTerminateStateBeforeDrain: func() {
					rejected := make(chan struct{})
					go func() {
						js.Reject("first")
						close(rejected)
					}()
					<-rejected
				},
			}
			if err := loop.Shutdown(context.Background()); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}

			for _, want := range []any{"first", "second"} {
				select {
				case got := <-reported:
					if got != want {
						t.Fatalf("reported rejection = %#v, want %#v", got, want)
					}
				case <-time.After(2 * time.Second):
					t.Fatalf("terminal drain did not report reentrant rejection %#v", want)
				}
			}
			select {
			case extra := <-reported:
				t.Fatalf("duplicate terminal rejection report: %#v", extra)
			default:
			}
			assertUnhandledRejectionTrackingDrained(t, js)
		})
	}
}

func TestChainedPromise_UnhandledFallbackDisabledDoesNotInvokeCallback(t *testing.T) {
	reported := make(chan any, 1)
	loop := New()

	js := NewJS(loop,
		WithUnhandledRejection(func(reason any) { reported <- reason }),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled),
	)
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	_, _, reject := js.NewChainedPromise()
	reject("disabled fallback")
	waitTerminalUnhandledRejectionTrackingDrained(t, js)

	select {
	case reason := <-reported:
		t.Fatalf("disabled post-termination fallback invoked handler with %v", reason)
	default:
	}
}

func TestChainedPromise_UnhandledFallbackZeroValueMatchesDefault(t *testing.T) {
	var zeroMode UnhandledRejectionFallbackMode
	for _, test := range []struct {
		name    string
		options []JSOption
	}{
		{name: "omitted"},
		{name: "explicit zero", options: []JSOption{WithUnhandledRejectionFallback(zeroMode)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			reported := make(chan any, 1)
			loop := New(WithLogger(nil))
			options := append([]JSOption{
				WithUnhandledRejection(func(reason any) { reported <- reason }),
			}, test.options...)
			js := NewJS(loop, options...)
			if err := loop.Close(); err != nil {
				t.Fatal(err)
			}

			js.Reject("post-terminal")
			waitTerminalUnhandledRejectionTrackingDrained(t, js)

			select {
			case reason := <-reported:
				t.Fatalf("zero/default post-terminal fallback invoked handler with %v", reason)
			default:
			}
			if js.unhandledFallback != UnhandledRejectionFallbackDisabled {
				t.Fatalf("zero/default fallback mode = %v, want Disabled", js.unhandledFallback)
			}
		})
	}
}

func TestChainedPromise_UnhandledFallbackConcurrentLateRejectionsNoDuplicateReports(t *testing.T) {
	const count = 16
	reported := make(chan any, count*2)
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop,
		WithUnhandledRejection(func(reason any) { reported <- reason }),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	startNow := contractRelease(t, start)
	ready := make(chan struct{}, count)
	var workers sync.WaitGroup
	for i := range count {
		_, _, reject := js.NewChainedPromise()
		workers.Go(func() {
			ready <- struct{}{}
			<-start
			reject(i)
		})
	}
	for range count {
		waitContractSignal(t, ready, "late rejection worker readiness")
	}
	startNow()
	workersDone := make(chan struct{})
	go func() {
		workers.Wait()
		close(workersDone)
	}()
	waitContractSignal(t, workersDone, "concurrent late rejection settlement")

	seen := make(map[int]bool, count)
	deadline := time.After(5 * time.Second)
	for len(seen) < count {
		select {
		case reason := <-reported:
			idx, ok := reason.(int)
			if !ok {
				t.Fatalf("reported reason type = %T, want int", reason)
			}
			if seen[idx] {
				t.Fatalf("duplicate fallback report for reason %d", idx)
			}
			seen[idx] = true
		case <-deadline:
			t.Fatalf("fallback reported %d/%d late rejections", len(seen), count)
		}
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, js)
	select {
	case extra := <-reported:
		t.Fatalf("extra fallback report after all late rejections drained: %v", extra)
	default:
	}
}

func TestChainedPromise_TerminalDrainNonOwnerRejectWaitsForActiveCheckpoint(t *testing.T) {
	reported := make(chan any, 3)
	handledOwner := make(chan struct{}, 1)
	scheduleErr := make(chan error, 1)
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	ownerPromise, _, rejectOwner := js.NewChainedPromise()
	_, _, rejectOther := js.NewChainedPromise()
	otherStart := make(chan struct{})
	otherStartNow := releaseSignalT(t, otherStart)
	otherDone := make(chan struct{})
	rejectJoinErr := make(chan error, 1)

	go func() {
		<-otherStart
		rejectOther("other")
		close(otherDone)
	}()

	if err := loop.Submit(func() {
		rejectOwner("owner")
		if err := js.QueueMicrotask(func() {
			ownerPromise.Catch(func(reason any) any {
				if reason == "owner" {
					handledOwner <- struct{}{}
				}
				return nil
			})
		}); err != nil {
			scheduleErr <- err
		}
		otherStartNow()
		select {
		case <-otherDone:
		case <-time.After(5 * time.Second):
			rejectJoinErr <- errors.New("non-owner rejection did not complete during active checkpoint")
		}
	}); err != nil {
		t.Fatalf("Submit terminal-drain work: %v", err)
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	select {
	case err := <-scheduleErr:
		t.Fatalf("QueueMicrotask from terminal drain callback failed: %v", err)
	default:
	}
	select {
	case err := <-rejectJoinErr:
		t.Fatal(err)
	default:
	}
	select {
	case <-handledOwner:
	default:
		t.Fatal("owner rejection handler did not run during terminal drain checkpoint")
	}

	for {
		select {
		case reason := <-reported:
			if reason == "owner" {
				t.Fatal("owner rejection was reported before terminal-drain microtask handler ran")
			}
		default:
			return
		}
	}
}

func TestChainedPromise_TerminalDrainNonOwnerFirstRejectWaitsForDrain(t *testing.T) {
	reported := make(chan any, 2)
	handled := make(chan struct{}, 1)
	scheduleErr := make(chan error, 1)
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	promise, _, reject := js.NewChainedPromise()
	rejectStart := make(chan struct{})
	rejectStartNow := releaseSignalT(t, rejectStart)
	rejectDone := make(chan struct{})
	rejectJoinErr := make(chan error, 1)

	go func() {
		<-rejectStart
		reject("first")
		close(rejectDone)
	}()

	if err := loop.Submit(func() {
		if err := js.QueueMicrotask(func() {
			promise.Catch(func(reason any) any {
				if reason == "first" {
					handled <- struct{}{}
				}
				return nil
			})
		}); err != nil {
			scheduleErr <- err
		}
		rejectStartNow()
		select {
		case <-rejectDone:
		case <-time.After(5 * time.Second):
			rejectJoinErr <- errors.New("first non-owner rejection did not complete during terminal drain")
		}
	}); err != nil {
		t.Fatalf("Submit terminal-drain work: %v", err)
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	select {
	case err := <-scheduleErr:
		t.Fatalf("QueueMicrotask from terminal drain callback failed: %v", err)
	default:
	}
	select {
	case err := <-rejectJoinErr:
		t.Fatal(err)
	default:
	}
	select {
	case <-handled:
	default:
		t.Fatal("rejection handler did not run during terminal drain before deferred fallback")
	}

	waitTerminalUnhandledRejectionTrackingDrained(t, js)

	select {
	case reason := <-reported:
		t.Fatalf("terminal-drain non-owner rejection was reported before handler was observed: %v", reason)
	default:
	}
}

func TestChainedPromise_TerminatedBeforeTerminalDrainNonOwnerRejectWaitsForDrain(t *testing.T) {
	reported := make(chan any, 2)
	handled := make(chan struct{}, 1)
	scheduleErr := make(chan error, 1)
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	promise, _, reject := js.NewChainedPromise()
	rejectStart := make(chan struct{})
	rejectStartNow := releaseSignalT(t, rejectStart)
	rejectDone := make(chan struct{})
	rejectJoinErr := make(chan error, 1)

	go func() {
		<-rejectStart
		reject("pre-drain")
		close(rejectDone)
	}()

	if err := loop.Submit(func() {
		if err := js.QueueMicrotask(func() {
			promise.Catch(func(reason any) any {
				if reason == "pre-drain" {
					handled <- struct{}{}
				}
				return nil
			})
		}); err != nil {
			scheduleErr <- err
		}
	}); err != nil {
		t.Fatalf("Submit terminal-drain work: %v", err)
	}
	loop.testHooks = &loopTestHooks{
		AfterTerminateStateBeforeDrain: func() {
			rejectStartNow()
			select {
			case <-rejectDone:
			case <-time.After(5 * time.Second):
				rejectJoinErr <- errors.New("pre-drain non-owner rejection did not complete")
			}
		},
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	select {
	case err := <-scheduleErr:
		t.Fatalf("QueueMicrotask from terminal drain callback failed: %v", err)
	default:
	}
	select {
	case err := <-rejectJoinErr:
		t.Fatal(err)
	default:
	}
	select {
	case <-handled:
	default:
		t.Fatal("pre-drain rejection handler did not run during terminal drain before deferred fallback")
	}

	waitTerminalUnhandledRejectionTrackingDrained(t, js)

	select {
	case reason := <-reported:
		t.Fatalf("pre-drain non-owner rejection was reported before handler was observed: %v", reason)
	default:
	}
}

func TestChainedPromise_TerminatingBeforePublicShutdownDrainNonOwnerRejectWaitsForDrain(t *testing.T) {
	reported := make(chan any, 2)
	handled := make(chan struct{}, 1)
	scheduleErr := make(chan error, 1)
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	taskStarted := make(chan struct{})
	releaseTask := make(chan struct{})
	releaseTaskNow := releaseSignalT(t, releaseTask)
	if err := loop.Submit(func() {
		close(taskStarted)
		<-releaseTask
	}); err != nil {
		t.Fatalf("blocking Submit: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	waitContractSignal(t, taskStarted, "blocking terminal-rejection task entry")

	promise, _, reject := js.NewChainedPromise()
	rejectStart := make(chan struct{})
	rejectStartNow := releaseSignalT(t, rejectStart)
	rejectDone := make(chan struct{})
	hookResult := make(chan error, 1)

	go func() {
		<-rejectStart
		reject("terminating")
		close(rejectDone)
	}()

	if err := loop.Submit(func() {
		if err := js.QueueMicrotask(func() {
			promise.Catch(func(reason any) any {
				if reason == "terminating" {
					handled <- struct{}{}
				}
				return nil
			})
		}); err != nil {
			scheduleErr <- err
		}
	}); err != nil {
		t.Fatalf("Submit terminal-drain work: %v", err)
	}
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			rejectStartNow()
			select {
			case <-rejectDone:
				hookResult <- nil
			case <-time.After(5 * time.Second):
				hookResult <- errors.New("StateTerminating non-owner rejection did not complete")
			}
		},
	}

	shutdownErr := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr <- loop.Shutdown(ctx)
	}()

	if err := waitContractValue(t, hookResult, "StateTerminating rejection hook"); err != nil {
		t.Fatal(err)
	}
	releaseTaskNow()

	if err := waitContractValue(t, runDone, "public Shutdown Run completion"); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if err := waitContractValue(t, shutdownErr, "public terminal-rejection Shutdown completion"); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	select {
	case err := <-scheduleErr:
		t.Fatalf("QueueMicrotask from public shutdown terminal drain callback failed: %v", err)
	default:
	}
	select {
	case <-handled:
	default:
		t.Fatal("StateTerminating rejection handler did not run during public shutdown drain before deferred fallback")
	}

	waitTerminalUnhandledRejectionTrackingDrained(t, js)

	select {
	case reason := <-reported:
		t.Fatalf("StateTerminating non-owner rejection was reported before handler was observed: %v", reason)
	default:
	}
}

func TestChainedPromise_ContextCancelBeforeTerminalDrainNonOwnerRejectWaitsForDrain(t *testing.T) {
	reported := make(chan any, 2)
	handlerAttached := make(chan struct{}, 1)
	scheduleErr := make(chan error, 1)
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	promise, _, reject := js.NewChainedPromise()
	rejectStart := make(chan struct{})
	rejectStartNow := releaseSignalT(t, rejectStart)
	rejectDone := make(chan struct{})
	hookResult := make(chan error, 1)
	loop.testHooks = &loopTestHooks{
		BeforeTerminateState: func() {
			rejectStartNow()
			select {
			case <-rejectDone:
				hookResult <- nil
			case <-time.After(5 * time.Second):
				hookResult <- errors.New("context-cancel non-owner rejection did not complete")
			}
		},
	}

	go func() {
		<-rejectStart
		reject("context-cancel")
		close(rejectDone)
	}()

	taskStarted := make(chan struct{})
	releaseTask := make(chan struct{})
	releaseTaskNow := releaseSignalT(t, releaseTask)
	if err := loop.Submit(func() {
		close(taskStarted)
		<-releaseTask
	}); err != nil {
		t.Fatalf("blocking Submit: %v", err)
	}

	if err := loop.Submit(func() {
		if err := js.QueueMicrotask(func() {
			promise.Catch(func(reason any) any {
				return nil
			})
			handlerAttached <- struct{}{}
		}); err != nil {
			scheduleErr <- err
		}
	}); err != nil {
		t.Fatalf("Submit terminal-drain work: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	waitContractSignal(t, taskStarted, "context-cancel blocking task entry")
	cancel()
	releaseTaskNow()

	if err := waitContractValue(t, hookResult, "context-cancel pre-terminal rejection hook"); err != nil {
		t.Fatal(err)
	}
	if err := waitContractValue(t, runDone, "context-canceled terminal-rejection Run completion"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned error = %v, want context.Canceled", err)
	}

	select {
	case err := <-scheduleErr:
		t.Fatalf("QueueMicrotask from context terminal drain callback failed: %v", err)
	default:
	}
	select {
	case <-handlerAttached:
	default:
		t.Fatal("context-cancel rejection handler was not attached during terminal drain before deferred fallback")
	}

	waitTerminalUnhandledRejectionTrackingDrained(t, js)

	for {
		select {
		case reason := <-reported:
			if reason == "context-cancel" {
				t.Fatalf("context-cancel non-owner rejection was reported before handler was observed: %v", reason)
			}
		default:
			return
		}
	}
}
