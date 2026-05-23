package gojaeventloop

import (
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goja"
)

func TestTrackAbortSignalRejectsNilCallback(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	signal, err := adapter.runtime.RunString(`new AbortController().signal`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if got := recover(); got != "goja-eventloop: TrackAbortSignal callback must not be nil" {
			t.Fatalf("nil callback panic = %#v", got)
		}
	}()
	adapter.TrackAbortSignal(signal, nil)
}

func TestAbortSignalHostPanicPreservesCompleteDelivery(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	events := make([]string, 0, 4)
	if err := adapter.runtime.Set("__recordAbort", func(value string) { events = append(events, value) }); err != nil {
		t.Fatalf("set recorder: %v", err)
	}
	signal, err := adapter.runtime.RunString(`
		globalThis.__panicSource = new AbortController();
		globalThis.__panicDependent = AbortSignal.any([__panicSource.signal]);
		__panicSource.signal.addEventListener("abort", () => __recordAbort("source"));
		__panicDependent.addEventListener("abort", () => __recordAbort("dependent"));
		__panicSource.signal;
	`)
	if err != nil {
		t.Fatalf("create abort graph: %v", err)
	}
	wantPanic := &struct{ label string }{label: "first"}
	if cleanup, aborted, ok := adapter.TrackAbortSignal(signal, func() {
		events = append(events, "algorithm-one")
		panic(wantPanic)
	}); cleanup == nil || aborted || !ok {
		t.Fatalf("register first algorithm = cleanup:%v aborted:%v ok:%v", cleanup != nil, aborted, ok)
	}
	if cleanup, aborted, ok := adapter.TrackAbortSignal(signal, func() {
		events = append(events, "algorithm-two")
	}); cleanup == nil || aborted || !ok {
		t.Fatalf("register second algorithm = cleanup:%v aborted:%v ok:%v", cleanup != nil, aborted, ok)
	}

	var panicValue any
	func() {
		defer func() { panicValue = recover() }()
		adapter.abortSignalState(adapter.abortSignalValue(signal), adapter.runtime.ToValue("reason"))
	}()
	if panicValue != wantPanic {
		t.Fatalf("abort panic = %#v, want exact first panic", panicValue)
	}
	if got, want := strings.Join(events, ","), "algorithm-one,algorithm-two,source,dependent"; got != want {
		t.Fatalf("abort delivery = %q, want %q", got, want)
	}
}

func TestAbortSignalEarlierPanicOverridesLaterGoexitAndCleansGraph(t *testing.T) {
	adapter, source, dependent := newAbortControlGraph(t)
	wantPanic := &struct{ label string }{label: "first"}
	calls := make(chan string, 3)
	if cleanup, aborted := adapter.addAbortAlgorithm(source, func() {
		calls <- "panic"
		panic(wantPanic)
	}); cleanup == nil || aborted {
		t.Fatal("register panic algorithm")
	}
	if cleanup, aborted := adapter.addAbortAlgorithm(source, func() {
		calls <- "goexit"
		runtime.Goexit()
	}); cleanup == nil || aborted {
		t.Fatal("register Goexit algorithm")
	}
	cleanupLast, aborted := adapter.addAbortAlgorithm(source, func() { calls <- "late" })
	if cleanupLast == nil || aborted {
		t.Fatal("register final algorithm")
	}

	panicResult := make(chan any, 1)
	go func() {
		defer func() { panicResult <- recover() }()
		adapter.abortSignalState(source, goja.Undefined())
	}()
	select {
	case got := <-panicResult:
		if got != wantPanic {
			t.Fatalf("abort panic = %#v, want exact first panic", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("abort panic/Goexit path did not finish")
	}
	if got := drainAbortControlCalls(calls); got != "panic,goexit" {
		t.Fatalf("abort callback sequence = %q, want panic,goexit", got)
	}
	cleanupLast()
	assertAbortControlGraphSettled(t, source, dependent)
}

func TestAbortSignalBareGoexitAbandonsUndispatchedAlgorithms(t *testing.T) {
	adapter, source, dependent := newAbortControlGraph(t)
	calls := make(chan string, 2)
	if cleanup, aborted := adapter.addAbortAlgorithm(source, func() {
		calls <- "goexit"
		runtime.Goexit()
	}); cleanup == nil || aborted {
		t.Fatal("register Goexit algorithm")
	}
	cleanupLast, aborted := adapter.addAbortAlgorithm(source, func() { calls <- "late" })
	if cleanupLast == nil || aborted {
		t.Fatal("register final algorithm")
	}

	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		adapter.abortSignalState(source, goja.Undefined())
	}()
	select {
	case got := <-done:
		if got != nil {
			t.Fatalf("bare Goexit recovered panic = %#v, want nil", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bare Goexit path did not finish")
	}
	if got := drainAbortControlCalls(calls); got != "goexit" {
		t.Fatalf("abort callback sequence = %q, want goexit", got)
	}
	cleanupLast()
	assertAbortControlGraphSettled(t, source, dependent)
}

func TestAbortSignalGoexitCleanupRace(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	for iteration := range 200 {
		state, _ := adapter.newAbortSignal()
		started := make(chan struct{})
		release := make(chan struct{})
		if cleanup, aborted := adapter.addAbortAlgorithm(state, func() {
			close(started)
			<-release
			runtime.Goexit()
		}); cleanup == nil || aborted {
			t.Fatalf("iteration %d: register Goexit algorithm", iteration)
		}
		var calls atomic.Int32
		cleanup, aborted := adapter.addAbortAlgorithm(state, func() { calls.Add(1) })
		if cleanup == nil || aborted {
			t.Fatalf("iteration %d: register cleanup target", iteration)
		}
		abortDone := make(chan struct{})
		go func() {
			defer close(abortDone)
			adapter.abortSignalState(state, goja.Undefined())
		}()
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("iteration %d: Goexit algorithm did not start", iteration)
		}
		cleanupDone := make(chan struct{})
		go func() {
			<-release
			cleanup()
			close(cleanupDone)
		}()
		close(release)
		select {
		case <-abortDone:
		case <-time.After(time.Second):
			t.Fatalf("iteration %d: abort did not finish", iteration)
		}
		select {
		case <-cleanupDone:
		case <-time.After(time.Second):
			t.Fatalf("iteration %d: cleanup did not finish", iteration)
		}
		if got := calls.Load(); got != 0 {
			t.Fatalf("iteration %d: undispatched callback calls = %d, want 0", iteration, got)
		}
	}
}

func newAbortControlGraph(t *testing.T) (*Adapter, *abortSignalState, *abortSignalState) {
	t.Helper()
	adapter := newBoundAdapterForNode26Test(t)
	if _, err := adapter.runtime.RunString(`
		globalThis.__controlSource = new AbortController();
		globalThis.__controlDependent = AbortSignal.any([__controlSource.signal]);
	`); err != nil {
		t.Fatalf("create abort graph: %v", err)
	}
	source := adapter.abortSignalValue(adapter.runtime.Get("__controlSource").ToObject(adapter.runtime).Get("signal"))
	dependent := adapter.abortSignalValue(adapter.runtime.Get("__controlDependent"))
	return adapter, source, dependent
}

func drainAbortControlCalls(calls <-chan string) string {
	values := make([]string, 0, len(calls))
	for len(calls) != 0 {
		values = append(values, <-calls)
	}
	return strings.Join(values, ",")
}

func assertAbortControlGraphSettled(t *testing.T, source, dependent *abortSignalState) {
	t.Helper()
	source.mu.Lock()
	sourceAborted := source.aborted
	sourceAlgorithms := len(source.algorithms)
	sourceLinks := len(source.dependentLinks)
	source.mu.Unlock()
	dependent.mu.Lock()
	dependentAborted := dependent.aborted
	dependentAlgorithms := len(dependent.algorithms)
	dependentLinks := len(dependent.sourceLinks)
	dependent.mu.Unlock()
	if !sourceAborted || !dependentAborted || sourceAlgorithms != 0 || dependentAlgorithms != 0 || sourceLinks != 0 || dependentLinks != 0 {
		t.Fatalf("settled graph = source(aborted=%v algorithms=%d links=%d) dependent(aborted=%v algorithms=%d links=%d)",
			sourceAborted, sourceAlgorithms, sourceLinks, dependentAborted, dependentAlgorithms, dependentLinks)
	}
}
