package eventloop

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
	"weak"
)

func TestCloseCleansPublishedJSHandleRegistries(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)

	if _, err := js.SetTimeout(func() {}, int(time.Hour/time.Millisecond)); err != nil {
		t.Fatalf("SetTimeout: %v", err)
	}
	if _, err := js.SetInterval(func() {}, int(time.Hour/time.Millisecond)); err != nil {
		t.Fatalf("SetInterval: %v", err)
	}
	immediateID, err := js.SetImmediate(func() {})
	if err != nil {
		t.Fatalf("SetImmediate: %v", err)
	}
	js.setImmediateMu.RLock()
	immediate := js.setImmediateMap[immediateID]
	js.setImmediateMu.RUnlock()
	if immediate == nil {
		t.Fatal("SetImmediate did not publish its adapter handle")
	}

	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertCloseSignals(t, loop)
	assertJSHandleRegistriesEmpty(t, js)
	if !immediate.cleared.Load() {
		t.Fatal("Close did not invalidate the published immediate handle")
	}
}

func TestCloseRejectsJSTimerPublicationAfterCleanup(t *testing.T) {
	type timerResult struct {
		id  uint64
		err error
	}
	tests := []struct {
		name     string
		setHook  func(*Loop, chan struct{}, <-chan struct{})
		schedule func(*JS) (uint64, error)
	}{
		{
			name: "timeout",
			setHook: func(loop *Loop, reached chan struct{}, release <-chan struct{}) {
				loop.testHooks = &loopTestHooks{
					BeforeJSTimeoutRegistryPublish: func(uint64) {
						close(reached)
						<-release
					},
				}
			},
			schedule: func(js *JS) (uint64, error) {
				return js.SetTimeout(func() {}, int(time.Hour/time.Millisecond))
			},
		},
		{
			name: "interval",
			setHook: func(loop *Loop, reached chan struct{}, release <-chan struct{}) {
				loop.testHooks = &loopTestHooks{
					BeforeJSIntervalTimerIDPublish: func(uint64, *intervalState, TimerID) {
						close(reached)
						<-release
					},
				}
			},
			schedule: func(js *JS) (uint64, error) {
				return js.SetInterval(func() {}, int(time.Hour/time.Millisecond))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New()
			registerLoopCleanupT(t, loop)
			js := NewJS(loop)

			publicationReached := make(chan struct{})
			releasePublication := make(chan struct{})
			releasePublicationFn := releaseSignalT(t, releasePublication)
			test.setHook(loop, publicationReached, releasePublication)
			result := make(chan timerResult, 1)
			promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
				id, err := test.schedule(js)
				result <- timerResult{id: id, err: err}
				return nil, err
			})
			select {
			case <-publicationReached:
			case <-time.After(5 * time.Second):
				t.Fatal("JS timer did not reach adapter publication")
			}

			if err := loop.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			assertCloseSignals(t, loop)
			assertPromiseRejected(t, promise, ErrLoopTerminated)
			assertJSHandleRegistriesEmpty(t, js)
			releasePublicationFn()
			select {
			case got := <-result:
				if got.id != 0 || !errors.Is(got.err, ErrLoopTerminated) {
					t.Fatalf("timer publication result = (%d, %v), want (0, ErrLoopTerminated)", got.id, got.err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("JS timer publication did not return after terminal cleanup")
			}
			waitPromisifyWorkersT(t, loop)
			assertJSHandleRegistriesEmpty(t, js)
			if got := len(loop.timerMap); got != 0 {
				t.Fatalf("native timers after rejected JS publication = %d, want 0", got)
			}
			if got := loop.refedTimerCount.Load(); got != 0 {
				t.Fatalf("refed timer count after rejected JS publication = %d, want 0", got)
			}
			assertPromiseRejected(t, promise, ErrLoopTerminated)
		})
	}
}

func TestNewJSRegistrationConvergesAfterCollectedAdapters(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	const adapterCount = 32
	pointers := make([]weak.Pointer[JS], adapterCount)
	for i := range pointers {
		pointers[i] = newWeakJSAdapterT(t, loop)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		allCollected := true
		for _, pointer := range pointers {
			if pointer.Value() != nil {
				allCollected = false
			}
		}
		if allCollected {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("JS adapters were not collected")
		}
		runtime.Gosched()
	}

	survivor := NewJS(loop)
	deadline = time.Now().Add(5 * time.Second)
	for {
		loop.livenessMu.Lock()
		registered := len(loop.jsAdapters)
		loop.livenessMu.Unlock()
		if registered == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("registered JS adapters after cleanup convergence = %d, want 1", registered)
		}
		runtime.KeepAlive(survivor)
		runtime.GC()
		runtime.Gosched()
	}
	runtime.KeepAlive(survivor)
}

func TestCollectedJSAdapterRegistrationsRetireWithoutNewAdapter(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	const adapterCount = 32
	pointers := make([]weak.Pointer[JS], adapterCount)
	for index := range pointers {
		pointers[index] = newWeakJSAdapterT(t, loop)
	}
	if collected := waitWeakJSAdapters(pointers, 5*time.Second); collected != adapterCount {
		t.Fatalf("collected JS adapters = %d/%d", collected, adapterCount)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		loop.livenessMu.Lock()
		registered := len(loop.jsAdapters)
		loop.livenessMu.Unlock()
		if registered == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("dead JS adapter registrations = %d, want 0 without another NewJS call", registered)
		}
		runtime.GC()
		runtime.Gosched()
	}
}

func TestNewJSSweepsDelayedCollectedAdapterRegistrations(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	cleanupEntered := make(chan struct{})
	releaseCleanup := make(chan struct{})
	var enteredOnce sync.Once
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseCleanup) }) })
	loop.testHooks = &loopTestHooks{
		BeforeJSAdapterCleanup: func() {
			enteredOnce.Do(func() { close(cleanupEntered) })
			<-releaseCleanup
		},
	}

	pointers := make([]weak.Pointer[JS], retainedRegistryHighWater)
	for index := range pointers {
		pointers[index] = newWeakJSAdapterT(t, loop)
	}
	if collected := waitWeakJSAdapters(pointers, 5*time.Second); collected != len(pointers) {
		t.Fatalf("collected delayed-cleanup JS adapters = %d/%d", collected, len(pointers))
	}
	waitContractSignal(t, cleanupEntered, "delayed JS adapter cleanup")

	loop.livenessMu.Lock()
	registeredBefore := len(loop.jsAdapters)
	loop.livenessMu.Unlock()
	if registeredBefore != len(pointers) {
		t.Fatalf("registrations before fallback sweep = %d, want %d", registeredBefore, len(pointers))
	}

	survivor := NewJS(loop)
	loop.livenessMu.Lock()
	registeredAfter := len(loop.jsAdapters)
	loop.livenessMu.Unlock()
	if registeredAfter != 1 {
		t.Fatalf("registrations after fallback sweep = %d, want 1", registeredAfter)
	}
	releaseOnce.Do(func() { close(releaseCleanup) })
	runtime.KeepAlive(survivor)
}

func TestJSAdapterRegistrationsRebuildAfterHighWater(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	const survivorCount = 8
	adapters := make([]*JS, retainedRegistryHighWater+1)
	for index := range adapters {
		adapters[index] = NewJS(loop)
	}
	loop.livenessMu.Lock()
	oversized := loop.jsAdaptersRetention.oversized
	registered := len(loop.jsAdapters)
	loop.livenessMu.Unlock()
	if !oversized || registered != len(adapters) {
		t.Fatalf("adapter high-water state = (oversized %v, registered %d), want (true, %d)", oversized, registered, len(adapters))
	}

	survivors := append([]*JS(nil), adapters[:survivorCount]...)
	pointers := make([]weak.Pointer[JS], len(adapters)-survivorCount)
	for index, js := range adapters[survivorCount:] {
		pointers[index] = weak.Make(js)
	}
	clear(adapters)
	adapters = nil
	if collected := waitWeakJSAdapters(pointers, 5*time.Second); collected != len(pointers) {
		t.Fatalf("collected high-water JS adapters = %d/%d", collected, len(pointers))
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		loop.livenessMu.Lock()
		registered = len(loop.jsAdapters)
		oversized = loop.jsAdaptersRetention.oversized
		loop.livenessMu.Unlock()
		if registered == survivorCount && !oversized {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("adapter low-water state = (oversized %v, registered %d), want (false, %d)", oversized, registered, survivorCount)
		}
		runtime.GC()
		runtime.Gosched()
	}
	runtime.KeepAlive(survivors)
}

func TestTerminalJSAdapterRegistryRemainsDiscardedAfterCollection(t *testing.T) {
	const (
		iterations   = 20
		adapterCount = 8
	)
	for range iterations {
		loop := New()
		pointers := make([]weak.Pointer[JS], adapterCount)
		for index := range pointers {
			pointers[index] = newWeakJSAdapterT(t, loop)
		}
		runtime.GC()
		if err := loop.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if collected := waitWeakJSAdapters(pointers, 5*time.Second); collected != adapterCount {
			t.Fatalf("collected racing JS adapters = %d/%d", collected, adapterCount)
		}
		loop.livenessMu.Lock()
		registrations := loop.jsAdapters
		oversized := loop.jsAdaptersRetention.oversized
		loop.livenessMu.Unlock()
		if registrations != nil || oversized {
			t.Fatalf("terminal adapter registry = (nil %v, oversized %v), want (true, false)", registrations == nil, oversized)
		}
	}
}

func TestCloseWinsBlockedJSAdapterCleanup(t *testing.T) {
	loop := New()
	cleanupEntered := make(chan struct{})
	cleanupRetired := make(chan struct{})
	releaseCleanup := make(chan struct{})
	var enteredOnce sync.Once
	var retiredOnce sync.Once
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseCleanup) }) })
	loop.testHooks = &loopTestHooks{
		BeforeJSAdapterCleanup: func() {
			enteredOnce.Do(func() { close(cleanupEntered) })
			<-releaseCleanup
		},
		AfterJSAdapterCleanupLock: func() {
			retiredOnce.Do(func() { close(cleanupRetired) })
		},
	}

	pointer := newWeakJSAdapterT(t, loop)
	if collected := waitWeakJSAdapters([]weak.Pointer[JS]{pointer}, 5*time.Second); collected != 1 {
		t.Fatal("JS adapter was not collected")
	}
	waitContractSignal(t, cleanupEntered, "blocked JS adapter cleanup")
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	releaseOnce.Do(func() { close(releaseCleanup) })
	waitContractSignal(t, cleanupRetired, "late JS adapter cleanup")
	assertTerminalJSAdapterRegistryDiscarded(t, loop)
}

func TestJSAdapterCleanupWinsClose(t *testing.T) {
	loop := New()
	cleanupLocked := make(chan struct{})
	releaseCleanup := make(chan struct{})
	var lockedOnce sync.Once
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseCleanup) }) })
	loop.testHooks = &loopTestHooks{
		AfterJSAdapterCleanupLock: func() {
			lockedOnce.Do(func() { close(cleanupLocked) })
			<-releaseCleanup
		},
	}

	pointer := newWeakJSAdapterT(t, loop)
	if collected := waitWeakJSAdapters([]weak.Pointer[JS]{pointer}, 5*time.Second); collected != 1 {
		t.Fatal("JS adapter was not collected")
	}
	waitContractSignal(t, cleanupLocked, "locked JS adapter cleanup")
	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	releaseOnce.Do(func() { close(releaseCleanup) })
	if err := waitContractValue(t, closeDone, "Close after JS adapter cleanup"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertTerminalJSAdapterRegistryDiscarded(t, loop)
}

func assertTerminalJSAdapterRegistryDiscarded(t *testing.T, loop *Loop) {
	t.Helper()
	loop.livenessMu.Lock()
	registrations := loop.jsAdapters
	state := loop.jsAdaptersRetention
	sweepAt := loop.jsAdapterSweepAt
	loop.livenessMu.Unlock()
	if registrations != nil || state != (retainedMapState{}) || sweepAt != 0 {
		t.Fatalf("terminal adapter registry = (nil %v, state %+v, sweep %d), want (true, zero, 0)", registrations == nil, state, sweepAt)
	}
}

func TestNewJSRejectionCheckpointDoesNotRetainAdapters(t *testing.T) {
	loop := New()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	registerActiveLoopCleanupT(t, loop, runDone)
	waitLoopOwnerTurnT(t, loop)

	const adapterCount = 16
	pointers := make([]weak.Pointer[JS], adapterCount)
	for index := range pointers {
		pointers[index] = newReportedWeakJSAdapterT(t, loop, index)
	}
	// Every user callback has returned before this barrier is admitted. Waiting
	// for the next owner turn proves the corresponding rejection checks have
	// finished rather than mistaking their ordinary in-flight ownership for a
	// retention leak.
	waitLoopOwnerTurnT(t, loop)

	collected := waitWeakJSAdapters(pointers, 2*time.Second)
	if collected == adapterCount {
		return
	}

	// Terminal cleanup is the causal control: if active-checkpoint ownership did
	// not quiesce normally, cleanup must still collect and release it.
	if err := loop.Close(); err != nil {
		t.Fatalf("Close causal control: %v", err)
	}
	afterClose := waitWeakJSAdapters(pointers, 5*time.Second)
	if afterClose != adapterCount {
		t.Fatalf("JS adapters collected while loop active = %d/%d; after Close = %d/%d, causal collection control failed", collected, adapterCount, afterClose, adapterCount)
	}
	t.Fatalf("JS adapters collected while loop active = %d/%d; all collected only after loopDone closed", collected, adapterCount)
}

func TestRejectionCheckRetentionSingleSlotAllocatesNoMap(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)

	allocations := testing.AllocsPerRun(1000, func() {
		loop.retainRejectionCheckAdapter(js)
		loop.releaseRejectionCheckAdapter(js)
	})
	if allocations != 0 {
		t.Fatalf("single-adapter retention allocations = %v, want 0", allocations)
	}
	loop.rejectionCheckMu.Lock()
	primary := loop.rejectionCheckAdapter
	overflow := loop.rejectionCheckAdapters
	loop.rejectionCheckMu.Unlock()
	runtime.KeepAlive(js)
	if primary != nil || overflow != nil {
		t.Fatalf("single-adapter retention storage did not collapse: primary=%p overflow=%v", primary, overflow)
	}
}

func TestRejectionCheckRetentionOverflowCollapses(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	adapters := []*JS{NewJS(loop), NewJS(loop), NewJS(loop)}
	for _, js := range adapters {
		loop.retainRejectionCheckAdapter(js)
	}

	loop.rejectionCheckMu.Lock()
	primary := loop.rejectionCheckAdapter
	overflowCount := len(loop.rejectionCheckAdapters)
	loop.rejectionCheckMu.Unlock()
	if primary != adapters[0] || overflowCount != len(adapters)-1 {
		t.Fatalf("retention high-water storage = (primary %p, overflow %d), want (%p, %d)", primary, overflowCount, adapters[0], len(adapters)-1)
	}

	loop.releaseRejectionCheckAdapter(adapters[0])
	loop.rejectionCheckMu.Lock()
	primary = loop.rejectionCheckAdapter
	overflowCount = len(loop.rejectionCheckAdapters)
	loop.rejectionCheckMu.Unlock()
	if primary == nil || primary == adapters[0] || overflowCount != 1 {
		t.Fatalf("retention promotion = (primary %p, overflow %d), want a promoted adapter and one overflow", primary, overflowCount)
	}

	loop.releaseRejectionCheckAdapter(adapters[1])
	loop.releaseRejectionCheckAdapter(adapters[2])
	loop.rejectionCheckMu.Lock()
	primary = loop.rejectionCheckAdapter
	overflow := loop.rejectionCheckAdapters
	loop.rejectionCheckMu.Unlock()
	runtime.KeepAlive(adapters)
	if primary != nil || overflow != nil {
		t.Fatalf("retention storage after final release = (primary %p, overflow %v), want empty", primary, overflow)
	}
}

func TestCloseCollectsDequeuedRejectionCheckpointOwner(t *testing.T) {
	loop := New(WithLogger(nil))
	registerLoopCleanupT(t, loop)

	callbackReached := make(chan struct{})
	releaseCallback := make(chan struct{})
	releaseCallbackFn := releaseSignalT(t, releaseCallback)
	closeTransitioned := make(chan struct{})
	cleanupRetention := make(chan bool, 1)
	releaseCleanup := make(chan struct{})
	releaseCleanupFn := releaseSignalT(t, releaseCleanup)
	reported := make(chan any, 2)
	var pointer weak.Pointer[JS]
	var callbackOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeCallbackAdmission: func() {
			callbackOnce.Do(func() {
				close(callbackReached)
				<-releaseCallback
			})
		},
		AfterCloseStateTerminating: func() { close(closeTransitioned) },
		BeforeJSTerminalCleanupCollect: func() {
			runtime.GC()
			cleanupRetention <- pointer.Value() != nil
			<-releaseCleanup
		},
	}

	pointer = func() weak.Pointer[JS] {
		js := NewJS(loop,
			WithUnhandledRejection(func(reason any) { reported <- reason }),
			WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
		)
		_, _, reject := js.NewChainedPromise()
		reject("dequeued checkpoint")
		result := weak.Make(js)
		runtime.KeepAlive(js)
		return result
	}()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, callbackReached, "rejection checkpoint callback admission")

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, closeTransitioned, "Close StateTerminating publication")
	releaseCallbackFn()

	if retained := waitContractValue(t, cleanupRetention, "pre-cleanup rejection owner retention"); !retained {
		releaseCleanupFn()
		t.Fatal("dequeued rejection adapter was collected before terminal cleanup could recover its fallback")
	}
	releaseCleanupFn()
	if err := waitContractValue(t, closeDone, "Close completion"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason := waitContractValue(t, reported, "terminal fallback report"); reason != "dequeued checkpoint" {
		t.Fatalf("terminal fallback reason = %v, want dequeued checkpoint", reason)
	}

	waitRejectionCheckRetentionEmpty(t, loop)
	select {
	case reason := <-reported:
		t.Fatalf("duplicate terminal fallback report: %v", reason)
	default:
	}
	if collected := waitWeakJSAdapters([]weak.Pointer[JS]{pointer}, 5*time.Second); collected != 1 {
		t.Fatal("rejection adapter remained reachable after terminal fallback quiesced")
	}
}

func TestCloseDrainsRejectionCheckRetentionOverflow(t *testing.T) {
	loop := New(WithLogger(nil))
	registerLoopCleanupT(t, loop)

	const adapterCount = 3
	reported := make(chan any, adapterCount*2)
	pointers := make([]weak.Pointer[JS], adapterCount)
	for index := range pointers {
		pointers[index] = func() weak.Pointer[JS] {
			js := NewJS(loop,
				WithUnhandledRejection(func(reason any) { reported <- reason }),
				WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
			)
			_, _, reject := js.NewChainedPromise()
			reject(index)
			pointer := weak.Make(js)
			runtime.KeepAlive(js)
			return pointer
		}()
	}

	loop.rejectionCheckMu.Lock()
	primary := loop.rejectionCheckAdapter
	overflowCount := len(loop.rejectionCheckAdapters)
	loop.rejectionCheckMu.Unlock()
	if primary == nil || overflowCount != adapterCount-1 {
		t.Fatalf("end-to-end retention storage = (primary %p, overflow %d), want one primary and %d overflow", primary, overflowCount, adapterCount-1)
	}
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}

	seen := make(map[int]bool, adapterCount)
	for range adapterCount {
		reason := waitContractValue(t, reported, "overflow terminal fallback report")
		index, ok := reason.(int)
		if !ok || index < 0 || index >= adapterCount || seen[index] {
			t.Fatalf("overflow terminal fallback reason = %T %v, want a unique adapter index", reason, reason)
		}
		seen[index] = true
	}
	waitRejectionCheckRetentionEmpty(t, loop)
	select {
	case reason := <-reported:
		t.Fatalf("duplicate overflow terminal fallback report: %v", reason)
	default:
	}
	if collected := waitWeakJSAdapters(pointers, 5*time.Second); collected != adapterCount {
		t.Fatalf("overflow rejection adapters collected = %d/%d after fallback quiesced", collected, adapterCount)
	}
}

func waitRejectionCheckRetentionEmpty(t *testing.T, loop *Loop) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		loop.rejectionCheckMu.Lock()
		retained := loop.rejectionCheckAdapter != nil || len(loop.rejectionCheckAdapters) != 0
		loop.rejectionCheckMu.Unlock()
		if !retained {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("terminal fallback did not release rejection-check ownership")
		}
		runtime.Gosched()
	}
}

func newWeakJSAdapterT(t *testing.T, loop *Loop) weak.Pointer[JS] {
	t.Helper()
	js := NewJS(loop)
	return weak.Make(js)
}

func newReportedWeakJSAdapterT(t *testing.T, loop *Loop, reason int) weak.Pointer[JS] {
	t.Helper()
	reported := make(chan any, 1)
	js := NewJS(loop, WithUnhandledRejection(func(value any) { reported <- value }))
	_, _, reject := js.NewChainedPromise()
	reject(reason)
	if got := waitContractValue(t, reported, "transient adapter rejection report"); got != reason {
		t.Fatalf("unhandled rejection reason = %v, want %d", got, reason)
	}
	pointer := weak.Make(js)
	runtime.KeepAlive(js)
	return pointer
}

func waitWeakJSAdapters(pointers []weak.Pointer[JS], timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		runtime.GC()
		collected := 0
		for _, pointer := range pointers {
			if pointer.Value() == nil {
				collected++
			}
		}
		if collected == len(pointers) || time.Now().After(deadline) {
			return collected
		}
		runtime.Gosched()
	}
}

func assertJSHandleRegistriesEmpty(t *testing.T, js *JS) {
	t.Helper()
	js.timeoutsMu.RLock()
	timeoutCount := len(js.timeouts)
	js.timeoutsMu.RUnlock()
	js.intervalsMu.RLock()
	intervalCount := len(js.intervals)
	js.intervalsMu.RUnlock()
	js.setImmediateMu.RLock()
	immediateCount := len(js.setImmediateMap)
	js.setImmediateMu.RUnlock()
	if timeoutCount != 0 || intervalCount != 0 || immediateCount != 0 {
		t.Fatalf("JS handle registries after Close: timeouts=%d intervals=%d immediates=%d", timeoutCount, intervalCount, immediateCount)
	}
}
