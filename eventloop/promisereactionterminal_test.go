package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func pendingPromiseReactionCount(loop *Loop) int {
	loop.pendingReactionsMu.Lock()
	defer loop.pendingReactionsMu.Unlock()
	count := len(loop.pendingReactionOverflow)
	if loop.pendingReactionTarget != nil {
		count++
	}
	return count
}

func TestPromiseReactionDequeuedImmediateCloseCleanerWinsClaim(t *testing.T) {
	reported := make(chan any, 2)
	loop := New(WithLogger(nil))
	registerLoopCleanupT(t, loop)
	js := NewJS(loop,
		WithUnhandledRejection(func(reason any) { reported <- reason }),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)

	dequeued := make(chan struct{})
	releaseClaim := make(chan struct{})
	releaseClaimFn := releaseSignalT(t, releaseClaim)
	closeTransitioned := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		BeforePromiseReactionClaim: func(*ChainedPromise) {
			close(dequeued)
			<-releaseClaim
		},
		AfterCloseStateTerminating: func() { close(closeTransitioned) },
	}

	source, resolveSource, _ := js.NewChainedPromise()
	var handlerCalls atomic.Int32
	child := source.Then(func(value any) any {
		handlerCalls.Add(1)
		return value
	}, nil)
	childResult := child.ToChannel()
	resolveSource("accepted before Close")
	if got := pendingPromiseReactionCount(loop); got != 1 {
		t.Fatalf("pending reactions before Run = %d, want 1", got)
	}
	var trailingCalls atomic.Int32
	if err := js.QueueMicrotask(func() { trailingCalls.Add(1) }); err != nil {
		t.Fatalf("QueueMicrotask trailing sentinel: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, dequeued, "dequeued Promise reaction before ownership claim")
	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, closeTransitioned, "immediate Close transition")
	waitContractSignal(t, loop.terminalDependencyDone, "terminal reaction cleaner")
	if got := pendingPromiseReactionCount(loop); got != 0 {
		t.Fatalf("pending reactions after terminal cleaner = %d, want 0", got)
	}
	assertTerminalCombinatorRejection(t, child, childResult)
	releaseClaimFn()

	if err := waitContractValue(t, closeDone, "Close after cleaner-owned reaction"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run after cleaner-owned reaction"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls := handlerCalls.Load(); calls != 0 {
		t.Fatalf("terminally disposed handler calls = %d, want 0", calls)
	}
	if calls := trailingCalls.Load(); calls != 0 {
		t.Fatalf("trailing microtask calls = %d, want 0", calls)
	}
	reason := waitContractValue(t, reported, "cleaner-owned child diagnostic")
	if err, ok := reason.(error); !ok || !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("reported rejection = %T %v, want ErrLoopTerminated", reason, reason)
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, js)
	select {
	case duplicate := <-reported:
		t.Fatalf("cleaner-owned child reported more than once: %v", duplicate)
	default:
	}
	if loop.ownerMicroCount.Load() != 0 || loop.ingressMicroCount.Load() != 0 {
		t.Fatalf("terminal microtask counts = owner %d ingress %d, want zero", loop.ownerMicroCount.Load(), loop.ingressMicroCount.Load())
	}
}

func TestPromiseReactionTerminalCleanerWinsBeforeSchedule(t *testing.T) {
	for _, aggregate := range []bool{false, true} {
		name := "ordinary Then"
		if aggregate {
			name = "combinator observer"
		}
		t.Run(name, func(t *testing.T) {
			reported := make(chan any, 2)
			loop := New(WithLogger(nil))
			registerLoopCleanupT(t, loop)
			js := NewJS(loop,
				WithUnhandledRejection(func(reason any) { reported <- reason }),
				WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
			)

			registered := make(chan struct{})
			releaseSchedule := make(chan struct{})
			releaseScheduleFn := releaseSignalT(t, releaseSchedule)
			closeTransitioned := make(chan struct{})
			loop.testHooks = &loopTestHooks{
				AfterPromiseReactionRegister: func() {
					close(registered)
					<-releaseSchedule
				},
				AfterCloseStateTerminating: func() { close(closeTransitioned) },
			}

			source, resolveSource, _ := js.NewChainedPromise()
			var handlerCalls atomic.Int32
			var result *ChainedPromise
			if aggregate {
				result = js.All([]*ChainedPromise{source})
			} else {
				result = source.Then(func(value any) any {
					handlerCalls.Add(1)
					return value
				}, nil)
			}
			resultChannel := result.ToChannel()
			resolveDone := make(chan struct{})
			go func() {
				resolveSource("registered before terminal scheduling check")
				close(resolveDone)
			}()
			waitContractSignal(t, registered, "registered Promise reaction")
			if got := pendingPromiseReactionCount(loop); got != 1 {
				t.Fatalf("pending reactions before Close = %d, want 1", got)
			}

			closeDone := make(chan error, 1)
			go func() { closeDone <- loop.Close() }()
			waitContractSignal(t, closeTransitioned, "Close before reaction scheduling check")
			waitContractSignal(t, loop.terminalDependencyDone, "terminal cleaner before reaction scheduling check")
			if got := pendingPromiseReactionCount(loop); got != 0 {
				t.Fatalf("pending reactions after cleaner = %d, want 0", got)
			}
			assertTerminalCombinatorRejection(t, result, resultChannel)
			releaseScheduleFn()
			waitContractSignal(t, resolveDone, "source settlement after terminal scheduling check")
			if err := waitContractValue(t, closeDone, "Close after terminal scheduling check"); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if calls := handlerCalls.Load(); calls != 0 {
				t.Fatalf("terminally disposed handler calls = %d, want 0", calls)
			}
			reason := waitContractValue(t, reported, "pre-schedule cleaner diagnostic")
			if err, ok := reason.(error); !ok || !errors.Is(err, ErrLoopTerminated) {
				t.Fatalf("reported rejection = %T %v, want ErrLoopTerminated", reason, reason)
			}
			waitTerminalUnhandledRejectionTrackingDrained(t, js)
			select {
			case duplicate := <-reported:
				t.Fatalf("pre-schedule cleaner reported duplicate/private rejection: %v", duplicate)
			default:
			}
		})
	}
}

func TestPromiseReactionAcceptedNotDequeuedImmediateClose(t *testing.T) {
	reported := make(chan any, 2)
	loop := New(WithLogger(nil))
	registerLoopCleanupT(t, loop)
	js := NewJS(loop,
		WithUnhandledRejection(func(reason any) { reported <- reason }),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)
	source, resolveSource, _ := js.NewChainedPromise()
	var handlerCalls atomic.Int32
	child := source.Then(func(value any) any {
		handlerCalls.Add(1)
		return value
	}, nil)
	childResult := child.ToChannel()
	resolveSource("accepted before Close")
	if got := pendingPromiseReactionCount(loop); got != 1 {
		t.Fatalf("pending reactions before Close = %d, want 1", got)
	}
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
	if calls := handlerCalls.Load(); calls != 0 {
		t.Fatalf("terminally disposed handler calls = %d, want 0", calls)
	}
	assertTerminalCombinatorRejection(t, child, childResult)
	reason := waitContractValue(t, reported, "accepted-not-dequeued child diagnostic")
	if err, ok := reason.(error); !ok || !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("reported rejection = %T %v, want ErrLoopTerminated", reason, reason)
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, js)
	if got := pendingPromiseReactionCount(loop); got != 0 {
		t.Fatalf("pending reactions after Close = %d, want 0", got)
	}
	select {
	case duplicate := <-reported:
		t.Fatalf("accepted-not-dequeued child reported more than once: %v", duplicate)
	default:
	}
}

func TestPromiseReactionGracefulShutdownDrainsAccepted(t *testing.T) {
	loop := New(WithLogger(nil))
	registerLoopCleanupT(t, loop)
	reported := make(chan any, 1)
	js := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))
	source, resolveSource, _ := js.NewChainedPromise()
	var handlerCalls atomic.Int32
	child := source.Then(func(value any) any {
		handlerCalls.Add(1)
		return value
	}, nil)
	childResult := child.ToChannel()
	resolveSource("graceful value")
	if got := pendingPromiseReactionCount(loop); got != 1 {
		t.Fatalf("pending reactions before Shutdown = %d, want 1", got)
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if loop.immediateClose.Load() {
		t.Fatal("graceful Shutdown published immediate-Close mode")
	}
	if calls := handlerCalls.Load(); calls != 1 {
		t.Fatalf("graceful handler calls = %d, want 1", calls)
	}
	assertSinglePromiseChannelValue(t, childResult, "graceful value")
	if state := child.State(); state != Fulfilled || child.Value() != "graceful value" {
		t.Fatalf("graceful child = (%v, %v), want (Fulfilled, graceful value)", state, child.Value())
	}
	if got := pendingPromiseReactionCount(loop); got != 0 {
		t.Fatalf("pending reactions after Shutdown = %d, want 0", got)
	}
	select {
	case reason := <-reported:
		t.Fatalf("graceful reaction reported rejection: %v", reason)
	default:
	}
}

func TestPromiseReactionOverflowHighWaterIsBounded(t *testing.T) {
	loop := New(WithLogger(nil))
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)
	children := make([]*ChainedPromise, pendingReactionOverflowRetainLimit+2)
	for index := range children {
		source, resolveSource, _ := js.NewChainedPromise()
		children[index] = source.Then(func(value any) any { return value }, nil)
		resolveSource(index)
	}
	if got := pendingPromiseReactionCount(loop); got != len(children) {
		t.Fatalf("pending reaction burst = %d, want %d", got, len(children))
	}
	loop.tick()
	for index, child := range children {
		if state := child.State(); state != Fulfilled || child.Value() != index {
			t.Fatalf("child %d = (%v, %v), want (Fulfilled, %d)", index, state, child.Value(), index)
		}
	}
	loop.pendingReactionsMu.Lock()
	overflow := loop.pendingReactionOverflow
	peak := loop.pendingReactionOverflowPeak
	loop.pendingReactionsMu.Unlock()
	if overflow != nil || peak != 0 {
		t.Fatalf("large drained reaction burst retained overflow: map=%v peak=%d", overflow != nil, peak)
	}
}

func TestPendingPromiseReactionTerminalSnapshotRegistrationOrder(t *testing.T) {
	loop := New(WithLogger(nil))
	registerLoopCleanupT(t, loop)
	const count = 8
	sources := make([]*ChainedPromise, count)
	for index := range count {
		sources[index] = newStandalonePromiseTestValue()
		target := newStandalonePromiseTestValue()
		loop.registerPendingPromiseReaction(target, sources[index], handlerScheduleFailure{
			target: target,
			result: index,
		})
	}
	reactions := loop.takePendingPromiseReactions()
	if len(reactions) != count {
		t.Fatalf("terminal reaction snapshot length = %d, want %d", len(reactions), count)
	}
	for index, reaction := range reactions {
		if reaction.source != sources[index] || reaction.failure.result != index {
			t.Fatalf("terminal reaction snapshot[%d] = (source %p, result %v), want (%p, %d)", index, reaction.source, reaction.failure.result, sources[index], index)
		}
		if index != 0 && reaction.seq <= reactions[index-1].seq {
			t.Fatalf("terminal reaction sequence[%d] = %d after %d", index, reaction.seq, reactions[index-1].seq)
		}
	}
	loop.pendingReactionsMu.Lock()
	target := loop.pendingReactionTarget
	overflow := loop.pendingReactionOverflow
	peak := loop.pendingReactionOverflowPeak
	loop.pendingReactionsMu.Unlock()
	if target != nil || overflow != nil || peak != 0 {
		t.Fatalf("terminal snapshot retained ownership: target=%p overflow=%v peak=%d", target, overflow != nil, peak)
	}
}

func TestPromiseReactionAcceptedBeforeCloseRejectsChild(t *testing.T) {
	reported := make(chan any, 2)
	loop := New(WithLogger(nil))
	registerLoopCleanupT(t, loop)
	js := NewJS(loop,
		WithUnhandledRejection(func(reason any) { reported <- reason }),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)

	admissionEntered := make(chan struct{})
	releaseAdmission := make(chan struct{})
	releaseAdmissionFn := releaseSignalT(t, releaseAdmission)
	closeTransitioned := make(chan struct{})
	var admissionOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeCallbackAdmission: func() {
			admissionOnce.Do(func() {
				close(admissionEntered)
				<-releaseAdmission
			})
		},
		AfterCloseStateTerminating: func() { close(closeTransitioned) },
	}

	source, resolveSource, _ := js.NewChainedPromise()
	var handlerCalls atomic.Int32
	child := source.Then(func(value any) any {
		handlerCalls.Add(1)
		return value
	}, nil)
	childResult := child.ToChannel()
	resolveSource("accepted before Close")

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, admissionEntered, "dequeued Then reaction admission")
	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, closeTransitioned, "Close callback gate transition")
	releaseAdmissionFn()

	if err := waitContractValue(t, closeDone, "Close after dequeued Then reaction"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run after dequeued Then reaction"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls := handlerCalls.Load(); calls != 0 {
		t.Fatalf("terminally discarded Then handler calls = %d, want 0", calls)
	}
	assertTerminalCombinatorRejection(t, child, childResult)
	reason := waitContractValue(t, reported, "public terminal Then-child diagnostic")
	if err, ok := reason.(error); !ok || !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("reported rejection = %T %v, want ErrLoopTerminated", reason, reason)
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, js)
	select {
	case duplicate := <-reported:
		t.Fatalf("terminal Then child reported more than once: %v", duplicate)
	default:
	}
}
