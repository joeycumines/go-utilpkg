package gojaeventloop

import (
	"context"
	goruntime "runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"weak"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func newRetentionCleanupAdapter(t *testing.T) *Adapter {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	return adapter
}

func bindRetainedAbortTestSurface(t *testing.T, adapter *Adapter) {
	t.Helper()
	bindRetainedEventTestSurface(t, adapter)
	if err := adapter.runtime.Set("AbortController", adapter.abortControllerConstructor); err != nil {
		t.Fatalf("install AbortController: %v", err)
	}
	if err := adapter.runtime.Set("AbortSignal", adapter.abortSignalConstructor); err != nil {
		t.Fatalf("install AbortSignal: %v", err)
	}
	abortControllerConstructor := adapter.runtime.Get("AbortController").ToObject(adapter.runtime)
	abortSignalConstructor := adapter.runtime.Get("AbortSignal").ToObject(adapter.runtime)
	if err := adapter.bindAbortControllerPrototype(abortControllerConstructor); err != nil {
		t.Fatalf("bind AbortController: %v", err)
	}
	if err := adapter.bindAbortSignalPrototype(abortSignalConstructor); err != nil {
		t.Fatalf("bind AbortSignal: %v", err)
	}
	abortPrototype := constructorPrototype(adapter.runtime, "AbortSignal")
	if abortPrototype == nil {
		t.Fatal("AbortSignal prototype unavailable")
	}
	if err := abortPrototype.SetPrototype(constructorPrototype(adapter.runtime, "EventTarget")); err != nil {
		t.Fatalf("set AbortSignal inheritance: %v", err)
	}
	iteratorValue, err := adapter.runtime.RunString(`((iteratorSymbol) => (object) => object[iteratorSymbol])(Symbol.iterator)`)
	if err != nil {
		t.Fatalf("compile iterator helper: %v", err)
	}
	iterator, ok := goja.AssertFunction(iteratorValue)
	if !ok {
		t.Fatal("iterator helper is not callable")
	}
	adapter.getIterator = iterator
	if err := adapter.bindAbortSignalStatics(abortSignalConstructor); err != nil {
		t.Fatalf("bind AbortSignal statics: %v", err)
	}
}

func TestTrackAbortSignalCleanupWinsBeforeCallbackClaim(t *testing.T) {
	adapter := newRetentionCleanupAdapter(t)
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	signal, err := adapter.runtime.RunString(`(globalThis.__cleanupController = new AbortController()).signal`)
	if err != nil {
		t.Fatalf("create signal: %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	if cleanup, aborted, ok := adapter.TrackAbortSignal(signal, func() {
		close(started)
		<-release
	}); !ok || aborted || cleanup == nil {
		t.Fatalf("register blocking algorithm = cleanup:%v aborted:%v ok:%v", cleanup != nil, aborted, ok)
	}
	var calls atomic.Int32
	cleanup, aborted, ok := adapter.TrackAbortSignal(signal, func() { calls.Add(1) })
	if !ok || aborted || cleanup == nil {
		t.Fatalf("TrackAbortSignal = cleanup:%v aborted:%v ok:%v, want active signal", cleanup != nil, aborted, ok)
	}
	abortDone := make(chan struct{})
	go func() {
		defer close(abortDone)
		adapter.abortSignalState(adapter.abortSignalValue(signal), goja.Undefined())
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocking algorithm did not start")
	}
	cleanup()
	close(release)
	select {
	case <-abortDone:
	case <-time.After(time.Second):
		t.Fatal("abort did not complete")
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("callback calls = %d, want 0 after cleanup won claim", got)
	}
}

func TestTrackAbortSignalCleanupLosesAfterCallbackClaim(t *testing.T) {
	adapter := newRetentionCleanupAdapter(t)
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	signal, err := adapter.runtime.RunString(`(globalThis.__claimedController = new AbortController()).signal`)
	if err != nil {
		t.Fatalf("create signal: %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	cleanup, aborted, ok := adapter.TrackAbortSignal(signal, func() {
		calls.Add(1)
		close(started)
		<-release
	})
	if !ok || aborted || cleanup == nil {
		t.Fatalf("TrackAbortSignal = cleanup:%v aborted:%v ok:%v, want active signal", cleanup != nil, aborted, ok)
	}
	abortDone := make(chan struct{})
	go func() {
		defer close(abortDone)
		adapter.abortSignalState(adapter.abortSignalValue(signal), goja.Undefined())
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("claimed callback did not start")
	}
	cleanup()
	close(release)
	select {
	case <-abortDone:
	case <-time.After(time.Second):
		t.Fatal("abort did not complete")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("callback calls = %d, want exactly 1 after callback claim", got)
	}
}

func TestPendingRejectionOrderClearDropsRetainedPromises(t *testing.T) {
	runtime := goja.New()
	first, _, _ := runtime.NewPromise()
	second, _, _ := runtime.NewPromise()
	order := make([]*goja.Promise, 0, 4)
	order = append(order, first, second)

	cleared := clearPendingRejectionOrder(order)
	if len(cleared) != 0 {
		t.Fatalf("cleared order len = %d, want 0", len(cleared))
	}
	if cap(cleared) != cap(order) {
		t.Fatalf("cleared order cap = %d, want retained cap %d", cap(cleared), cap(order))
	}
	for i, promise := range cleared[:cap(cleared)] {
		if promise != nil {
			t.Fatalf("cleared backing slot %d retained promise %p", i, promise)
		}
	}
}

func TestEventTargetListenerRemovalClearsBackingStorage(t *testing.T) {
	adapter := newRetentionCleanupAdapter(t)
	callbackA := adapter.runtime.ToValue(func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	callbackB := adapter.runtime.ToValue(func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	wrapper := adapter.initEventTargetObject(adapter.runtime.NewObject())

	adapter.eventTargetAddConfigured(wrapper, "tick", callbackA, false, false, false, false, false, nil)
	adapter.eventTargetAddConfigured(wrapper, "tick", callbackB, false, false, false, false, false, nil)
	infos := wrapper.listeners["tick"]
	if len(infos) != 2 {
		t.Fatalf("listener count = %d, want 2", len(infos))
	}
	removedInfo := infos[0]
	if !adapter.eventTargetRemove(wrapper, "tick", callbackA, false) {
		t.Fatal("removeEventListener returned false")
	}
	for i, info := range infos[:cap(infos)] {
		if info == removedInfo {
			t.Fatalf("listener backing slot %d retained removed listener", i)
		}
	}
	if got := wrapper.listeners["tick"]; len(got) != 1 || got[0].callback != callbackB {
		t.Fatalf("remaining listeners = %#v, want only callbackB", got)
	}
}

func TestEventTargetSignalCleanupRemovedWithListener(t *testing.T) {
	adapter := newRetentionCleanupAdapter(t)
	callback := adapter.runtime.ToValue(func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	target := adapter.initEventTargetObject(adapter.runtime.NewObject())
	signal, _ := adapter.newAbortSignal()

	adapter.eventTargetAddConfigured(target, "tick", callback, false, false, false, false, false, signal)
	infos := target.listeners["tick"]
	if len(infos) != 1 {
		t.Fatalf("target listener count = %d, want 1", len(infos))
	}
	if infos[0].signalCleanup == nil {
		t.Fatal("target listener did not record its signal cleanup")
	}
	signal.mu.Lock()
	algorithms := signal.algorithms
	signal.mu.Unlock()
	if len(algorithms) != 1 {
		t.Fatalf("signal cleanup algorithm count = %d, want 1", len(algorithms))
	}

	if !adapter.eventTargetRemove(target, "tick", callback, false) {
		t.Fatal("removeEventListener returned false")
	}
	signal.mu.Lock()
	remaining := signal.algorithms
	signal.mu.Unlock()
	if len(remaining) != 0 {
		t.Fatalf("signal cleanup algorithms after manual removal = %d, want 0", len(remaining))
	}
	for i, algorithm := range algorithms[:cap(algorithms)] {
		if algorithm != nil {
			t.Fatalf("signal cleanup backing slot %d retained algorithm", i)
		}
	}
}

type abortCallbackSentinel struct {
	calls atomic.Int32
}

func newAbortCallbackSentinel() (func(), weak.Pointer[abortCallbackSentinel]) {
	sentinel := new(abortCallbackSentinel)
	return func() { sentinel.calls.Add(1) }, weak.Make(sentinel)
}

func newJSTimerCallbackSentinel(runtime *goja.Runtime) (goja.Value, weak.Pointer[abortCallbackSentinel]) {
	sentinel := new(abortCallbackSentinel)
	return runtime.ToValue(func(goja.FunctionCall) goja.Value {
		sentinel.calls.Add(1)
		return goja.Undefined()
	}), weak.Make(sentinel)
}

func TestAbortAlgorithmCleanupHandleReleasesCallbackCapture(t *testing.T) {
	adapter := newRetentionCleanupAdapter(t)
	state, _ := adapter.newAbortSignal()
	callback, ref := newAbortCallbackSentinel()
	cleanup, aborted := adapter.addAbortAlgorithm(state, callback)
	if cleanup == nil || aborted {
		t.Fatal("register abort algorithm")
	}
	callback = nil
	cleanup()
	waitCollectedAbortCallbackSentinel(t, ref, "cleaned abort callback")
	goruntime.KeepAlive(cleanup)
}

func TestAbortAlgorithmClaimReleasesCallbackCapture(t *testing.T) {
	adapter := newRetentionCleanupAdapter(t)
	bindRetainedAbortTestSurface(t, adapter)
	state, _ := adapter.newAbortSignal()
	callback, ref := newAbortCallbackSentinel()
	cleanup, aborted := adapter.addAbortAlgorithm(state, callback)
	if cleanup == nil || aborted {
		t.Fatal("register abort algorithm")
	}
	callback = nil
	adapter.abortSignalState(state, goja.Undefined())
	waitCollectedAbortCallbackSentinel(t, ref, "claimed abort callback")
	goruntime.KeepAlive(cleanup)
}

func waitCollectedAbortCallbackSentinel(t *testing.T, ref weak.Pointer[abortCallbackSentinel], label string) {
	t.Helper()
	for range 100 {
		goruntime.GC()
		goruntime.Gosched()
		if ref.Value() == nil {
			return
		}
	}
	t.Fatalf("%s remained strongly retained", label)
}

func TestAbortSignalConcurrentObserverRetentionMatchesFinalState(t *testing.T) {
	adapter := newRetentionCleanupAdapter(t)
	source, _ := adapter.newAbortSignal()
	dependent, _ := adapter.newAbortSignal()
	dependent.dependent = true
	adapter.linkAbortSignal(source, dependent)

	const workers = 8
	const iterations = 250
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for range iterations {
				changeAbortSignalObservers(dependent, 1)
				goruntime.Gosched()
				changeAbortSignalObservers(dependent, -1)
			}
		}()
	}
	group.Wait()

	dependent.mu.Lock()
	observers := dependent.observers
	links := append([]*abortSignalLink(nil), dependent.sourceLinks...)
	dependent.mu.Unlock()
	source.mu.Lock()
	dependentObservers := source.dependentObservers
	source.mu.Unlock()
	if observers != 0 || len(links) != 1 || links[0].retained.Load() != nil || dependentObservers != 0 {
		t.Fatalf("final retention = observers:%d links:%d retained:%v source-observers:%d, want 0/1/false/0",
			observers, len(links), len(links) == 1 && links[0].retained.Load() != nil, dependentObservers)
	}

	changeAbortSignalObservers(dependent, 1)
	source.mu.Lock()
	dependentObservers = source.dependentObservers
	source.mu.Unlock()
	if links[0].retained.Load() != dependent || dependentObservers != 1 {
		t.Fatalf("observed retention = retained:%v source-observers:%d, want true/1", links[0].retained.Load() == dependent, dependentObservers)
	}
	changeAbortSignalObservers(dependent, -1)
}

func TestAbortSignalAnyDetachesSourceLinksAfterCompositeAbort(t *testing.T) {
	adapter := newRetentionCleanupAdapter(t)
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if _, err := adapter.runtime.RunString(`
		globalThis.__sourceOne = new AbortController();
		globalThis.__sourceTwo = new AbortController();
		globalThis.__composite = AbortSignal.any([__sourceOne.signal, __sourceTwo.signal]);
	`); err != nil {
		t.Fatalf("RunString setup: %v", err)
	}

	sourceOne := adapter.abortSignalValue(adapter.runtime.Get("__sourceOne").ToObject(adapter.runtime).Get("signal"))
	sourceTwo := adapter.abortSignalValue(adapter.runtime.Get("__sourceTwo").ToObject(adapter.runtime).Get("signal"))
	composite := adapter.abortSignalValue(adapter.runtime.Get("__composite"))
	if sourceOne == nil || sourceTwo == nil || composite == nil {
		t.Fatal("failed to resolve AbortSignal states")
	}
	sourceOne.mu.Lock()
	oneLinks := sourceOne.dependentLinks
	sourceOne.mu.Unlock()
	sourceTwo.mu.Lock()
	twoLinks := sourceTwo.dependentLinks
	sourceTwo.mu.Unlock()
	composite.mu.Lock()
	compositeLinks := composite.sourceLinks
	composite.mu.Unlock()
	if len(oneLinks) != 1 || len(twoLinks) != 1 || len(compositeLinks) != 2 {
		t.Fatalf("source/dependent link counts = %d/%d/%d, want 1/1/2", len(oneLinks), len(twoLinks), len(compositeLinks))
	}
	if !oneLinks[0].active.Load() || !twoLinks[0].active.Load() ||
		(oneLinks[0] != compositeLinks[0] && oneLinks[0] != compositeLinks[1]) ||
		(twoLinks[0] != compositeLinks[0] && twoLinks[0] != compositeLinks[1]) {
		t.Fatal("AbortSignal.any links are not active reciprocal source/dependent records")
	}
	oneLink := oneLinks[0]
	twoLink := twoLinks[0]

	if _, err := adapter.runtime.RunString(`__sourceOne.abort("done")`); err != nil {
		t.Fatalf("RunString abort: %v", err)
	}
	if !composite.aborted || composite.reason.String() != "done" {
		t.Fatalf("composite aborted=%v reason=%v, want true/done", composite.aborted, composite.reason)
	}
	sourceOne.mu.Lock()
	oneRemaining := sourceOne.dependentLinks
	sourceOne.mu.Unlock()
	if len(oneRemaining) != 0 {
		t.Fatalf("sourceOne dependent links after composite abort = %d, want 0", len(oneRemaining))
	}
	sourceTwo.mu.Lock()
	twoRemaining := sourceTwo.dependentLinks
	sourceTwo.mu.Unlock()
	if len(twoRemaining) != 0 {
		t.Fatalf("sourceTwo dependent links after composite abort = %d, want 0", len(twoRemaining))
	}
	composite.mu.Lock()
	compositeRemaining := composite.sourceLinks
	composite.mu.Unlock()
	if len(compositeRemaining) != 0 {
		t.Fatalf("composite source links after abort = %d, want 0", len(compositeRemaining))
	}
	for index, link := range oneLinks[:cap(oneLinks)] {
		if link != nil {
			t.Fatalf("sourceOne backing slot %d retained link", index)
		}
	}
	for index, link := range twoLinks[:cap(twoLinks)] {
		if link != nil {
			t.Fatalf("sourceTwo backing slot %d retained link", index)
		}
	}
	for index, link := range compositeLinks[:cap(compositeLinks)] {
		if link != nil {
			t.Fatalf("composite backing slot %d retained link", index)
		}
	}
	for index, link := range []*abortSignalLink{oneLink, twoLink} {
		link.mu.Lock()
		cleanupSet := link.cleanupSet
		link.mu.Unlock()
		if cleanupSet {
			t.Fatalf("detached source link %d retained runtime cleanup", index)
		}
	}
}

// TestAbortSignalTimeoutRetentionRegistration is a deterministic structural
// assertion (review finding 9): the abortTimeoutRef.retained pin must be set
// exactly while the timeout signal has observers and is not aborted, and
// unset once the observers are removed or the signal aborts. It uses no
// runtime.GC() — the GC stress tests are secondary evidence.
func TestAbortSignalTimeoutRetentionRegistration(t *testing.T) {
	adapter := newRetentionCleanupAdapter(t)
	state, obj := adapter.newAbortSignal()
	stateRef := &abortTimeoutRef{adapter: weak.Make(adapter), signal: weak.Make(state)}
	state.mu.Lock()
	state.timeout = stateRef
	state.mu.Unlock()

	// Without observers the timeout retention is not registered.
	refreshAbortTimeoutRetention(state)
	if stateRef.retained.Load() != nil {
		t.Fatal("timeout retention set without observers")
	}

	// Adding an observer registers the retention pin.
	changeAbortSignalObservers(state, 1)
	if stateRef.retained.Load() != state {
		t.Fatal("timeout retention not set while the signal has an observer")
	}

	// Removing the observer releases the pin.
	changeAbortSignalObservers(state, -1)
	if stateRef.retained.Load() != nil {
		t.Fatal("timeout retention not unset after the observer was removed")
	}

	// Re-adding an observer re-registers; aborting releases it.
	changeAbortSignalObservers(state, 1)
	if stateRef.retained.Load() != state {
		t.Fatal("timeout retention not re-set while the signal has an observer")
	}
	beginAbortSignal(state, adapter.timeoutReason())
	if stateRef.retained.Load() != nil {
		t.Fatal("timeout retention not unset after abort")
	}
	state.mu.Lock()
	timeout := state.timeout
	state.mu.Unlock()
	if timeout != nil {
		t.Fatal("aborted timeout signal retained its timeout reference")
	}
	_ = obj
}

func TestAbortSignalTimeoutStopsRuntimeCleanupAfterAbort(t *testing.T) {
	adapter := newRetentionCleanupAdapter(t)
	bindRetainedAbortTestSurface(t, adapter)
	value, err := adapter.runtime.RunString(`AbortSignal.timeout(60000)`)
	if err != nil {
		t.Fatalf("create timeout signal: %v", err)
	}
	state := adapter.abortSignalValue(value)
	timeout := state.timeout
	if timeout == nil {
		t.Fatal("timeout state has no timer reference")
	}
	timeout.mu.Lock()
	cleanupSet := timeout.cleanupSet
	timeout.mu.Unlock()
	if !cleanupSet {
		t.Fatal("timeout runtime cleanup was not registered")
	}
	adapter.abortSignalState(state, adapter.runtime.ToValue("manual-timeout-test"))
	timeout.mu.Lock()
	cleanupSet = timeout.cleanupSet
	timeout.mu.Unlock()
	if cleanupSet {
		t.Fatal("aborted timeout retained obsolete runtime cleanup")
	}
	adapter.clearTimer(timeout.timerID)
}

func TestAbortSignalDependentAndTimeoutStateCanBeCollected(t *testing.T) {
	adapter := newRetentionCleanupAdapter(t)
	bindRetainedAbortTestSurface(t, adapter)

	value, err := adapter.runtime.RunString(`
		globalThis.__retainedSource = new AbortController();
		AbortSignal.any([__retainedSource.signal]);
	`)
	if err != nil {
		t.Fatalf("create dependent signal: %v", err)
	}
	dependent := adapter.abortSignalValue(value)
	if dependent == nil {
		t.Fatal("resolve dependent AbortSignal state")
	}
	dependentRef := weak.Make(dependent)
	value = nil
	dependent = nil
	if _, err := adapter.runtime.RunString(`void 0`); err != nil {
		t.Fatalf("clear dependent result: %v", err)
	}
	waitCollectedAbortDependent(t, dependentRef, adapter.abortSignalValue(adapter.runtime.Get("__retainedSource").ToObject(adapter.runtime).Get("signal")))

	value, err = adapter.runtime.RunString(`AbortSignal.timeout(60000)`)
	if err != nil {
		t.Fatalf("create timeout signal: %v", err)
	}
	timeoutState := adapter.abortSignalValue(value)
	if timeoutState == nil {
		t.Fatal("resolve timeout AbortSignal state")
	}
	timeoutRef := weak.Make(timeoutState)
	timerID := timeoutState.timeout.timerID
	value = nil
	timeoutState = nil
	if _, err := adapter.runtime.RunString(`void 0`); err != nil {
		t.Fatalf("clear timeout result: %v", err)
	}
	waitCollectedAbortState(t, timeoutRef, "AbortSignal.timeout state")
	for range 10 {
		goruntime.GC()
		goruntime.Gosched()
	}
	adapter.clearTimer(timerID)
	adapter.timersMu.Lock()
	_, retained := adapter.timers[timerID]
	adapter.timersMu.Unlock()
	if retained {
		t.Fatal("AbortSignal.timeout timer remained after owner cleanup")
	}
}

func waitCollectedAbortDependent(t *testing.T, ref weak.Pointer[abortSignalState], source *abortSignalState) {
	t.Helper()
	for range 100 {
		goruntime.GC()
		goruntime.Gosched()
		source.mu.Lock()
		linksCleared := len(source.dependentLinks) == 0 && cap(source.dependentLinks) == 0
		source.mu.Unlock()
		if ref.Value() == nil && linksCleared {
			return
		}
	}
	t.Fatal("AbortSignal.any dependent or its source-link storage remained retained")
}

func waitCollectedAbortState(t *testing.T, ref weak.Pointer[abortSignalState], label string) {
	t.Helper()
	for range 100 {
		goruntime.GC()
		goruntime.Gosched()
		if ref.Value() == nil {
			return
		}
	}
	t.Fatalf("%s remained strongly retained", label)
}

func TestAbortSignalTimeoutDoesNotKeepLoopAliveButFiresWhileLoopIsLive(t *testing.T) {
	if got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		globalThis.__unrefTimeoutSignal = AbortSignal.timeout(60000);
		process.on("exit", function() { events.push("exit"); });
	`); got != "exit" {
		t.Fatalf("unreferenced AbortSignal.timeout lifecycle = %q, want %q", got, "exit")
	}

	if got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const signal = AbortSignal.timeout(5);
		signal.addEventListener("abort", function() { events.push("abort:" + signal.reason.name); });
		setTimeout(function() { events.push("keepalive"); }, 20);
	`); got != "abort:TimeoutError,keepalive" {
		t.Fatalf("live-loop AbortSignal.timeout lifecycle = %q, want %q", got, "abort:TimeoutError,keepalive")
	}
}

func TestAdapterTerminalCleanupScrubsDetachedImmediateBatch(t *testing.T) {
	ctx, loop, runtime, _ := newAutoExitAdapter(t)
	if _, err := runtime.RunString(`
		setImmediate(function() { process.exit(7); });
		globalThis.__detachedTerminalImmediate =
			setImmediate(function retainedDetachedImmediate() {});
	`); err != nil {
		t.Fatalf("install detached Immediate batch: %v", err)
	}
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	handle := runtime.Get("__detachedTerminalImmediate").ToObject(runtime)
	if !goja.IsNull(handle.Get("_onImmediate")) ||
		!goja.IsNull(handle.Get("_argv")) ||
		!goja.IsNull(handle.Get("_idlePrev")) ||
		!goja.IsNull(handle.Get("_idleNext")) ||
		!handle.Get("_destroyed").ToBoolean() {
		t.Fatal("terminal cleanup retained a detached Immediate callback, arguments, or links")
	}
}

func TestAdapterTerminalCleanupClearsRootsAfterThrowingTimerLink(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	callback, ref := newJSTimerCallbackSentinel(runtime)
	if err := runtime.Set("__terminalThrowingCallback", callback); err != nil {
		t.Fatalf("install timer callback: %v", err)
	}
	if _, err := runtime.RunString(`
		(function() {
			const handle = setTimeout(__terminalThrowingCallback, 60000).unref();
			Object.defineProperty(handle, "_idleNext", {
				configurable: true,
				get: function() { throw new Error("terminal link"); },
			});
		})();
		delete globalThis.__terminalThrowingCallback;
	`); err != nil {
		t.Fatalf("install throwing timer link: %v", err)
	}
	callback = nil

	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	snapshot, err := adapter.timerSnapshot(goja.Undefined())
	if err != nil {
		t.Fatalf("snapshot terminal timers: %v", err)
	}
	object := snapshot.ToObject(runtime)
	if refCount := object.Get("timeoutRefCount").ToInteger(); refCount != 0 {
		t.Fatalf("terminal timeout ref count = %d, want 0", refCount)
	}
	if listCount := object.Get("listCount").ToInteger(); listCount != 0 {
		t.Fatalf("terminal timer list count = %d, want 0", listCount)
	}
	waitCollectedAbortCallbackSentinel(t, ref, "throwing-link timer callback")
}

func TestAdapterTerminalCleanupReleasesDynamicState(t *testing.T) {
	for _, test := range []struct {
		name      string
		terminate func(*goeventloop.Loop) error
	}{
		{name: "Close", terminate: func(loop *goeventloop.Loop) error { return loop.Close() }},
		{name: "auto-exit", terminate: func(loop *goeventloop.Loop) error { return loop.Run(context.Background()) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop, err := goeventloop.New(goeventloop.WithAutoExit(true))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = loop.Close() })
			runtime := goja.New()
			adapter, err := New(loop, runtime)
			if err != nil {
				t.Fatal(err)
			}
			if err := adapter.Bind(); err != nil {
				t.Fatalf("Bind: %v", err)
			}
			if _, err := runtime.RunString(`
				globalThis.__terminalTimeout = setTimeout(function retainedTimeout() {}, 60000).unref();
				globalThis.__terminalInterval = setInterval(function retainedInterval() {}, 60000).unref();
				globalThis.__terminalImmediate = setImmediate(function retainedImmediate() {}).unref();
				process.on("terminal-retention", function retainedProcessListener() {});
			`); err != nil {
				t.Fatalf("install dynamic state: %v", err)
			}
			if test.name == "Close" {
				if _, err := runtime.RunString(`
					globalThis.__terminalLatent = new __terminalTimeout.constructor(
						function retainedLatentTimeout() {}, 60000, undefined, false, true
					);
					globalThis.__terminalAbortSignal = AbortSignal.timeout(60000);
					globalThis.__terminalGenericImmediate = {};
					const immediateRefed = Object.getOwnPropertySymbols(__terminalImmediate)
						.find(function(symbol) { return String(symbol) === "Symbol(refed)"; });
					__terminalGenericImmediate[immediateRefed] = false;
					__terminalImmediate.constructor.prototype.ref.call(__terminalGenericImmediate);
				`); err != nil {
					t.Fatalf("install terminal liveness state: %v", err)
				}
				snapshot, err := adapter.timerSnapshot(goja.Undefined())
				if err != nil {
					t.Fatalf("snapshot terminal timers: %v", err)
				}
				refCount := snapshot.ToObject(runtime).Get("timeoutRefCount").ToInteger()
				adapter.timersMu.Lock()
				abortCount := 0
				for _, timer := range adapter.timers {
					if timer.kind == adapterTimerAbort {
						abortCount++
					}
				}
				backendPresent := adapter.timerBackendWake != nil
				backendRefed := adapter.timeoutBackendRefed
				genericRefs := adapter.genericImmediateRefs
				genericID := adapter.genericImmediateRefID
				adapter.timersMu.Unlock()
				if refCount != 1 || abortCount != 1 || !backendPresent || !backendRefed || genericRefs != 1 || genericID == 0 {
					t.Fatalf("installed terminal liveness state = refs:%d abort:%d backend:%t/%t immediate:%d/%d, want 1/1/true/true/1/nonzero",
						refCount, abortCount, backendPresent, backendRefed, genericRefs, genericID)
				}
			}

			adapter.timersMu.Lock()
			var timerStates []*adapterTimer
			for _, timer := range adapter.timers {
				timerStates = append(timerStates, timer)
			}
			adapter.timersMu.Unlock()
			adapter.immediatesMu.Lock()
			var immediateStates []*adapterImmediate
			for _, immediate := range adapter.immediates {
				immediateStates = append(immediateStates, immediate)
			}
			adapter.immediatesMu.Unlock()
			wantTimerStates := 2
			if test.name == "Close" {
				wantTimerStates = 3
			}
			if len(timerStates) != wantTimerStates || len(immediateStates) != 1 {
				t.Fatalf("dynamic handle state = %d timers/%d immediates, want %d/1",
					len(timerStates), len(immediateStates), wantTimerStates)
			}

			promise, _, _ := runtime.NewPromise()
			adapter.processMu.Lock()
			adapter.pendingRejections[promise] = runtime.ToValue("retained-rejection")
			adapter.pendingRejectionOrder = append(adapter.pendingRejectionOrder, promise)
			adapter.rejectionCheckScheduled = true
			adapter.processMu.Unlock()
			adapter.dispatchJSEvents.Store(new(goeventloop.Event), runtime.NewObject())

			if err := test.terminate(loop); err != nil {
				t.Fatalf("terminate: %v", err)
			}
			if !adapter.exiting.Load() {
				t.Fatal("terminal cleanup did not close adapter admission")
			}
			adapter.timersMu.Lock()
			timerCount := len(adapter.timers)
			registryCount := len(adapter.timerRegistry.states)
			backendPresent := adapter.timerBackendWake != nil
			backendRefed := adapter.timeoutBackendRefed
			genericRefs := adapter.genericImmediateRefs
			genericID := adapter.genericImmediateRefID
			adapter.timersMu.Unlock()
			adapter.immediatesMu.Lock()
			immediateCount := len(adapter.immediates)
			adapter.immediatesMu.Unlock()
			adapter.processMu.Lock()
			rejectionCount := len(adapter.pendingRejections)
			rejectionOrderLength := len(adapter.pendingRejectionOrder)
			rejectionScheduled := adapter.rejectionCheckScheduled
			adapter.processMu.Unlock()
			processCoreRetained := adapter.processEmitterCore != nil
			snapshot, err := adapter.timerSnapshot(goja.Undefined())
			if err != nil {
				t.Fatalf("snapshot terminal timer state: %v", err)
			}
			refCount := snapshot.ToObject(runtime).Get("timeoutRefCount").ToInteger()
			listCount := snapshot.ToObject(runtime).Get("listCount").ToInteger()
			if timerCount != 0 || registryCount != 0 || refCount != 0 || listCount != 0 ||
				backendPresent || backendRefed || genericRefs != 0 || genericID != 0 ||
				immediateCount != 0 || processCoreRetained ||
				rejectionCount != 0 || rejectionOrderLength != 0 || rejectionScheduled {
				t.Fatalf("terminal dynamic state = timers:%d registry:%d refs:%d lists:%d backend:%t/%t immediate-carrier:%d/%d immediates:%d process-core:%t rejections:%d order:%d scheduled:%t, want empty",
					timerCount, registryCount, refCount, listCount, backendPresent, backendRefed, genericRefs, genericID,
					immediateCount, processCoreRetained, rejectionCount, rejectionOrderLength, rejectionScheduled)
			}
			for index, timer := range timerStates {
				if !timer.canceled.Load() || timer.active.Load() || timer.payload.Load() != nil {
					t.Fatalf("terminal timer %d retained state", index)
				}
			}
			for index, immediate := range immediateStates {
				if !immediate.canceled.Load() || immediate.object != nil || immediate.callback != nil || immediate.args != nil {
					t.Fatalf("terminal immediate %d retained state", index)
				}
			}
			terminalImmediate := runtime.Get("__terminalImmediate").ToObject(runtime)
			if !goja.IsNull(terminalImmediate.Get("_onImmediate")) ||
				!goja.IsNull(terminalImmediate.Get("_argv")) ||
				!goja.IsNull(terminalImmediate.Get("_idlePrev")) ||
				!goja.IsNull(terminalImmediate.Get("_idleNext")) ||
				!terminalImmediate.Get("_destroyed").ToBoolean() {
				t.Fatal("terminal cleanup retained Immediate callback, arguments, or linked-list chain")
			}
			timerNames := []string{"__terminalTimeout", "__terminalInterval"}
			if test.name == "Close" {
				timerNames = append(timerNames, "__terminalLatent")
			}
			for _, name := range timerNames {
				handle := runtime.Get(name).ToObject(runtime)
				if !goja.IsNull(handle.Get("_onTimeout")) ||
					!goja.IsNull(handle.Get("_timerArgs")) ||
					!goja.IsNull(handle.Get("_repeat")) ||
					!goja.IsNull(handle.Get("_idlePrev")) ||
					!goja.IsNull(handle.Get("_idleNext")) ||
					handle.Get("_idleTimeout").ToInteger() != -1 ||
					!handle.Get("_destroyed").ToBoolean() {
					t.Fatalf("terminal cleanup retained %s callback, arguments, or linked-list state", name)
				}
			}
			dispatchRetained := false
			adapter.dispatchJSEvents.Range(func(_, _ any) bool {
				dispatchRetained = true
				return false
			})
			if dispatchRetained {
				t.Fatal("terminal cleanup retained dispatch JS values")
			}
			adapter.terminateCleanup()
			adapter.timersMu.Lock()
			idempotentTimers := len(adapter.timers)
			idempotentWake := adapter.timerBackendWake
			adapter.timersMu.Unlock()
			if idempotentTimers != 0 || idempotentWake != nil {
				t.Fatalf("repeated terminal cleanup restored timer state = %d/%#v", idempotentTimers, idempotentWake)
			}
		})
	}
}
