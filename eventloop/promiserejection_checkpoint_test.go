package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestChainedPromise_UnhandledCheckScheduleErrorReportsSynchronously(t *testing.T) {
	reported := make(chan any, 1)
	loop := New()

	js := NewJS(loop,
		WithUnhandledRejection(func(reason any) { reported <- reason }),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)

	_, _, reject := js.NewChainedPromise()
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	reject("boom")

	select {
	case reason := <-reported:
		if reason != "boom" {
			t.Fatalf("reported reason = %v, want boom", reason)
		}
	case <-time.After(time.Second):
		t.Fatal("unhandled rejection was not reported after ScheduleMicrotask failure")
	}
}

func TestChainedPromise_UnhandledCheckScheduleErrorRecoversCallbackPanic(t *testing.T) {
	loop := New()

	callbackStarted := make(chan struct{})
	js := NewJS(loop,
		WithUnhandledRejection(func(any) {
			close(callbackStarted)
			panic("boom")
		}),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)

	_, _, reject := js.NewChainedPromise()
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("unhandled rejection callback panic escaped fallback path: %v", r)
			}
		}()
		reject("boom")
	}()

	waitContractSignal(t, callbackStarted, "panicking isolated rejection callback entry")
	waitTerminalUnhandledRejectionTrackingDrained(t, js)
}

func TestChainedPromise_UnhandledCheckUsesMicrotaskCheckpointNotSleep(t *testing.T) {
	reported := make(chan any, 1)
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	_, _, reject := js.NewChainedPromise()
	reject("boom")

	loop.tick()

	select {
	case reason := <-reported:
		if reason != "boom" {
			t.Fatalf("reported reason = %v, want boom", reason)
		}
	default:
		t.Fatal("unhandled rejection was not reported in the microtask checkpoint")
	}
}

func TestChainedPromise_UnhandledCheckWaitsForLaterMicrotasksInCheckpoint(t *testing.T) {
	reported := make(chan any, 1)
	handled := make(chan struct{}, 1)
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop,
		WithUnhandledRejection(func(reason any) { reported <- reason }),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)

	p, _, reject := js.NewChainedPromise()
	reject("boom")
	if err := js.QueueMicrotask(func() {
		p.Catch(func(reason any) any {
			if reason != "boom" {
				t.Fatalf("catch reason = %v, want boom", reason)
			}
			handled <- struct{}{}
			return nil
		})
	}); err != nil {
		t.Fatalf("QueueMicrotask failed: %v", err)
	}

	loop.tick()

	select {
	case reason := <-reported:
		t.Fatalf("reported handled rejection from same checkpoint: %v", reason)
	default:
	}
	select {
	case <-handled:
	default:
		t.Fatal("same-checkpoint catch handler did not run")
	}
}

func TestChainedPromise_UnhandledCheckpointDoesNotStarveNextTick(t *testing.T) {
	reported := make(chan any, 1)
	nextTickRan := make(chan struct{}, 1)
	scheduleErr := make(chan error, 1)
	loop := New()

	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	_, _, reject := js.NewChainedPromise()
	reject("boom")
	if err := js.QueueMicrotask(func() {
		scheduleErr <- loop.ScheduleNextTick(func() { nextTickRan <- struct{}{} })
	}); err != nil {
		t.Fatalf("QueueMicrotask failed: %v", err)
	}

	drained := make(chan struct{})
	go func() {
		loop.tick()
		close(drained)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := loop.Shutdown(ctx); err != nil {
			t.Errorf("cleanup Shutdown: %v", err)
		}
		waitContractSignal(t, drained, "manual tick cleanup")
	})

	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("microtask checkpoint did not drain; unhandled check may be starving nextTick")
	}

	select {
	case err := <-scheduleErr:
		if err != nil {
			t.Fatalf("ScheduleNextTick from microtask failed: %v", err)
		}
	default:
		t.Fatal("microtask that schedules nextTick did not run")
	}
	select {
	case <-nextTickRan:
	default:
		t.Fatal("nextTick scheduled by microtask did not run before checkpoint diagnostic")
	}
	select {
	case reason := <-reported:
		if reason != "boom" {
			t.Fatalf("reported reason = %v, want boom", reason)
		}
	default:
		t.Fatal("unhandled rejection was not reported after nextTick drained")
	}
}

func TestChainedPromise_UnhandledCheckpointWaitsForMicrotasksScheduledByDiagnostic(t *testing.T) {
	reported := make(chan any, 2)
	handledSecond := make(chan struct{}, 1)
	scheduleErr := make(chan error, 1)
	loop := New()
	registerLoopCleanupT(t, loop)

	var js *JS
	js = NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
		if reason != "first" {
			return
		}
		second, _, rejectSecond := js.NewChainedPromise()
		rejectSecond("second")
		if err := js.QueueMicrotask(func() {
			second.Catch(func(reason any) any {
				if reason == "second" {
					handledSecond <- struct{}{}
				}
				return nil
			})
		}); err != nil {
			scheduleErr <- err
		}
	}))

	_, _, rejectFirst := js.NewChainedPromise()
	rejectFirst("first")
	loop.tick()

	select {
	case err := <-scheduleErr:
		t.Fatalf("QueueMicrotask from diagnostic failed: %v", err)
	default:
	}
	select {
	case reason := <-reported:
		if reason != "first" {
			t.Fatalf("first reported reason = %v, want first", reason)
		}
	default:
		t.Fatal("first unhandled rejection was not reported")
	}
	select {
	case <-handledSecond:
	default:
		t.Fatal("second rejection handler did not run")
	}
	select {
	case reason := <-reported:
		t.Fatalf("second rejection was falsely reported before diagnostic-scheduled microtask ran: %v", reason)
	default:
	}
}

func TestChainedPromise_UnhandledCheckpointYieldsBetweenSnapshotDiagnostics(t *testing.T) {
	reported := make(chan any, 2)
	handledOther := make(chan any, 1)
	scheduleErr := make(chan error, 1)
	loop := New()
	registerLoopCleanupT(t, loop)

	var js *JS
	var first, second *ChainedPromise
	var scheduled atomic.Bool
	js = NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
		if !scheduled.CompareAndSwap(false, true) {
			return
		}

		other := second
		if reason == "second" {
			other = first
		}
		if err := js.QueueMicrotask(func() {
			other.Catch(func(reason any) any {
				handledOther <- reason
				return nil
			})
		}); err != nil {
			scheduleErr <- err
		}
	}))

	var rejectFirst, rejectSecond RejectFunc
	first, _, rejectFirst = js.NewChainedPromise()
	second, _, rejectSecond = js.NewChainedPromise()
	rejectFirst("first")
	rejectSecond("second")
	loop.tick()

	select {
	case err := <-scheduleErr:
		t.Fatalf("QueueMicrotask from diagnostic failed: %v", err)
	default:
	}
	select {
	case <-reported:
	default:
		t.Fatal("first diagnostic did not run")
	}
	select {
	case <-handledOther:
	default:
		t.Fatal("diagnostic-scheduled handler for the other rejection did not run")
	}
	select {
	case reason := <-reported:
		t.Fatalf("other rejection was falsely reported before diagnostic-scheduled microtask ran: %v", reason)
	default:
	}
}

func TestChainedPromise_UnhandledCheckpointDiscardedByShutdownDoesNotSuppressFallback(t *testing.T) {
	reported := make(chan any, 2)
	loop := New()

	js := NewJS(loop,
		WithUnhandledRejection(func(reason any) { reported <- reason }),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)

	_, _, rejectBeforeShutdown := js.NewChainedPromise()
	rejectBeforeShutdown("before-shutdown")
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, _, rejectAfterShutdown := js.NewChainedPromise()
	rejectAfterShutdown("after-shutdown")

	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	for {
		select {
		case reason := <-reported:
			if reason == "after-shutdown" {
				waitTerminalUnhandledRejectionTrackingDrained(t, js)
				return
			}
		case <-timeout.C:
			t.Fatal("after-shutdown rejection was suppressed by stale scheduled checkpoint flag")
		}
	}
}

func TestChainedPromise_UnhandledCheckpointDiscardedByNeverRunShutdownReportsOriginal(t *testing.T) {
	reported := make(chan any, 2)
	loop := New()

	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	_, _, rejectBeforeShutdown := js.NewChainedPromise()
	rejectBeforeShutdown("before-shutdown")
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	select {
	case reason := <-reported:
		if reason != "before-shutdown" {
			t.Fatalf("reported reason = %v, want before-shutdown", reason)
		}
	case <-time.After(time.Second):
		t.Fatal("original rejection was not reported after never-run Shutdown discarded its checkpoint")
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, js)
}

func TestChainedPromise_TerminatedFallbackSingleOwnerReportsEachRejectionOnce(t *testing.T) {
	reported := make(chan any, 4)
	firstCallbackEntered := make(chan struct{})
	releaseFirstCallback := make(chan struct{})
	var firstCallbackOnce atomic.Bool

	loop := New()

	js := NewJS(loop,
		WithUnhandledRejection(func(reason any) {
			reported <- reason
			if reason == "first" && firstCallbackOnce.CompareAndSwap(false, true) {
				close(firstCallbackEntered)
				<-releaseFirstCallback
			}
		}),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	_, _, rejectFirst := js.NewChainedPromise()
	firstDone := make(chan struct{})
	go func() {
		rejectFirst("first")
		close(firstDone)
	}()

	select {
	case <-firstCallbackEntered:
	case <-time.After(time.Second):
		t.Fatal("first fallback did not enter unhandled callback")
	}

	_, _, rejectSecond := js.NewChainedPromise()
	rejectSecond("second")
	close(releaseFirstCallback)

	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first fallback did not finish after release")
	}

	counts := map[any]int{}
	deadline := time.After(time.Second)
	for counts["first"] == 0 || counts["second"] == 0 {
		select {
		case reason := <-reported:
			counts[reason]++
		case <-deadline:
			t.Fatalf("reports before timeout = %#v, want first and second exactly once", counts)
		}
	}
	for {
		select {
		case reason := <-reported:
			counts[reason]++
		default:
			if counts["first"] != 1 || counts["second"] != 1 {
				t.Fatalf("report counts = %#v, want first=1 second=1", counts)
			}
			waitTerminalUnhandledRejectionTrackingDrained(t, js)
			return
		}
	}
}

func TestChainedPromise_RepeatedUnhandledRejectionChecksReportOnce(t *testing.T) {
	reported := make(chan any, 32)
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))

	for i := range 16 {
		_, _, reject := js.NewChainedPromise()
		reject(i)
		loop.tick()
		select {
		case reason := <-reported:
			if reason != i {
				t.Fatalf("reported reason = %v, want %d", reason, i)
			}
		default:
			t.Fatalf("rejection %d was not reported by checkpoint", i)
		}
	}

	assertUnhandledRejectionTrackingDrained(t, js)
}

func TestChainedPromise_CloseDiscardedUnhandledRejectionCheckpointReportsFallback(t *testing.T) {
	reported := make(chan any, 1)
	loop := New()

	js := NewJS(loop,
		WithUnhandledRejection(func(reason any) { reported <- reason }),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)

	_, _, reject := js.NewChainedPromise()
	reject("close-discarded")

	if err := loop.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	select {
	case reason := <-reported:
		if reason != "close-discarded" {
			t.Fatalf("reported reason = %v, want close-discarded", reason)
		}
	case <-time.After(time.Second):
		t.Fatal("unhandled rejection scheduled before Close was never reported")
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, js)
}

func TestChainedPromise_TerminalDrainFallbackWatcherIsShared(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop, WithUnhandledRejection(func(any) {}))

	const rejectionCount = 32
	var wg sync.WaitGroup
	wg.Add(rejectionCount)
	rejectionsStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	release := releaseSignalT(t, releaseCallback)
	if err := loop.Submit(func() {
		for i := range rejectionCount {
			reason := i
			_, _, reject := js.NewChainedPromise()
			go func() {
				defer wg.Done()
				reject(reason)
			}()
		}
		close(rejectionsStarted)
		<-releaseCallback
	}); err != nil {
		t.Fatalf("Submit terminal-drain work: %v", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(shutdownCancel)
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(shutdownCtx) }()
	waitContractSignal(t, rejectionsStarted, "terminal-drain rejection fanout start")
	rejectionsDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(rejectionsDone)
	}()
	waitContractSignal(t, rejectionsDone, "terminal-drain rejection fanout completion")
	release()
	if err := waitContractValue(t, shutdownDone, "terminal-drain watcher Shutdown completion"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if got := js.checkRejectionTerminalWatchers.Load(); got != 1 {
		t.Fatalf("terminal-drain fallback watchers = %d, want exactly one shared watcher", got)
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, js)
}

func TestConcurrentRejectionCheckScheduleClaimRetainsOneGeneration(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	reported := make(chan any, 2)
	js := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))

	claimReached := make(chan struct{}, 2)
	releaseClaims := make(chan struct{})
	releaseClaimsFn := releaseSignalT(t, releaseClaims)
	loop.testHooks = &loopTestHooks{
		BeforeRejectionCheckScheduleClaim: func() {
			claimReached <- struct{}{}
			<-releaseClaims
		},
	}

	var rejects sync.WaitGroup
	rejects.Add(2)
	for _, reason := range []string{"first", "second"} {
		go func() {
			defer rejects.Done()
			js.Reject(reason)
		}()
	}
	waitContractSignal(t, claimReached, "first rejection schedule claimant")
	waitContractSignal(t, claimReached, "second rejection schedule claimant")
	releaseClaimsFn()
	rejects.Wait()

	if !js.checkRejectionScheduled.Load() {
		t.Fatal("concurrent rejection claims did not publish a scheduled generation")
	}
	if count := loop.ingressMicroCount.Load(); count != 1 {
		t.Fatalf("scheduled rejection-check generations = %d, want 1", count)
	}
	js.checkRejectionRunMu.Lock()
	runDone := js.checkRejectionRunDone
	js.checkRejectionRunMu.Unlock()
	if runDone == nil {
		t.Fatal("scheduled rejection-check generation has no completion token")
	}
	loop.rejectionCheckMu.Lock()
	retained := loop.rejectionCheckAdapter == js
	if !retained {
		_, retained = loop.rejectionCheckAdapters[js]
	}
	loop.rejectionCheckMu.Unlock()
	if !retained {
		t.Fatal("scheduled rejection-check generation has no strong adapter owner")
	}

	loop.tick()
	got := map[any]bool{
		waitContractValue(t, reported, "first concurrent rejection report"):  true,
		waitContractValue(t, reported, "second concurrent rejection report"): true,
	}
	if !got["first"] || !got["second"] || len(got) != 2 {
		t.Fatalf("concurrent rejection reports = %v, want first and second exactly once", got)
	}
	waitUnhandledRejectionCheckOwnershipReleased(t, js)
	assertUnhandledRejectionTrackingDrained(t, js)
}
