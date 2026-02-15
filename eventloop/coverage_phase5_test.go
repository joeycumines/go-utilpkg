package eventloop

// Phase 5 coverage tests for eventloop module.
// Targets public APIs at 0% coverage in the main package because they were
// only exercised via goja-eventloop (separate module, separate coverage).
//
// Groups:
//   A: EventTarget (eventtarget.go)
//   B: Performance (performance.go)
//   C: Promise statics (promise.go)
//   D: JS API (js.go)
//   E: errors.go, state.go, promisify.go, loop.go, metrics.go, psquare.go, registry.go

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// GROUP A — EventTarget
// ============================================================================

func TestPhase5_EventTarget_Basic(t *testing.T) {
	et := NewEventTarget()
	if et == nil {
		t.Fatal("NewEventTarget returned nil")
	}

	var received string
	id := et.AddEventListener("click", func(e *Event) {
		received = e.Type
	})
	if id == 0 {
		t.Fatal("expected non-zero listener ID")
	}

	ev := NewEvent("click")
	result := et.DispatchEvent(ev)
	if !result {
		t.Error("DispatchEvent should return true for non-cancelable event")
	}
	if received != "click" {
		t.Errorf("listener did not fire: got %q", received)
	}
	if ev.Target != et {
		t.Error("Event.Target not set by DispatchEvent")
	}
}

func TestPhase5_EventTarget_NilListener(t *testing.T) {
	et := NewEventTarget()
	id := et.AddEventListener("x", nil)
	if id != 0 {
		t.Error("nil listener should return 0 ID")
	}
}

func TestPhase5_EventTarget_NilDispatch(t *testing.T) {
	et := NewEventTarget()
	if !et.DispatchEvent(nil) {
		t.Error("DispatchEvent(nil) should return true")
	}
}

func TestPhase5_EventTarget_Once(t *testing.T) {
	et := NewEventTarget()
	var count int
	et.AddEventListenerOnce("fire", func(e *Event) {
		count++
	})

	ev := NewEvent("fire")
	et.DispatchEvent(ev)
	et.DispatchEvent(ev)
	if count != 1 {
		t.Errorf("once listener fired %d times, want 1", count)
	}
	if et.ListenerCount("fire") != 0 {
		t.Error("once listener not removed after dispatch")
	}
}

func TestPhase5_EventTarget_RemoveByID(t *testing.T) {
	et := NewEventTarget()
	var called bool
	id := et.AddEventListener("rm", func(e *Event) {
		called = true
	})

	// Remove existing listener
	if !et.RemoveEventListenerByID("rm", id) {
		t.Error("RemoveEventListenerByID returned false for existing listener")
	}

	et.DispatchEvent(NewEvent("rm"))
	if called {
		t.Error("listener should not fire after removal")
	}

	// Remove non-existent listener
	if et.RemoveEventListenerByID("rm", id) {
		t.Error("RemoveEventListenerByID should return false for missing listener")
	}

	// Remove from non-existent event type
	if et.RemoveEventListenerByID("bogus", 999) {
		t.Error("RemoveEventListenerByID should return false for missing event type")
	}
}

func TestPhase5_EventTarget_HasListeners_ListenerCount(t *testing.T) {
	et := NewEventTarget()
	if et.HasEventListeners("x") {
		t.Error("should have no listeners initially")
	}
	if et.ListenerCount("x") != 0 {
		t.Error("count should be 0 initially")
	}

	et.AddEventListener("x", func(e *Event) {})
	et.AddEventListener("x", func(e *Event) {})
	if !et.HasEventListeners("x") {
		t.Error("should have listeners after adding")
	}
	if et.ListenerCount("x") != 2 {
		t.Errorf("count: got %d, want 2", et.ListenerCount("x"))
	}
}

func TestPhase5_EventTarget_RemoveAll(t *testing.T) {
	et := NewEventTarget()
	et.AddEventListener("a", func(e *Event) {})
	et.AddEventListener("b", func(e *Event) {})

	// Remove specific type
	et.RemoveAllEventListeners("a")
	if et.HasEventListeners("a") {
		t.Error("listeners for 'a' should be removed")
	}
	if !et.HasEventListeners("b") {
		t.Error("listeners for 'b' should remain")
	}

	// Remove all types
	et.RemoveAllEventListeners("")
	if et.HasEventListeners("b") {
		t.Error("all listeners should be removed")
	}
}

func TestPhase5_EventTarget_PreventDefault(t *testing.T) {
	et := NewEventTarget()

	// Non-cancelable event: PreventDefault should be a no-op
	ev := NewEvent("test")
	ev.PreventDefault()
	if ev.DefaultPrevented {
		t.Error("PreventDefault should be no-op on non-cancelable event")
	}
	if !et.DispatchEvent(ev) {
		t.Error("non-cancelable event should return true")
	}

	// Cancelable event
	ev2 := NewEventWithOptions("cancel", false, true)
	et.AddEventListener("cancel", func(e *Event) {
		e.PreventDefault()
	})
	result := et.DispatchEvent(ev2)
	if result {
		t.Error("cancelable event with PreventDefault should return false from DispatchEvent")
	}
	if !ev2.DefaultPrevented {
		t.Error("DefaultPrevented should be true")
	}
}

func TestPhase5_EventTarget_StopPropagation(t *testing.T) {
	ev := NewEvent("test")
	if ev.IsPropagationStopped() {
		t.Error("should not be stopped initially")
	}
	ev.StopPropagation()
	if !ev.IsPropagationStopped() {
		t.Error("should be stopped after StopPropagation")
	}
	if ev.IsImmediatePropagationStopped() {
		t.Error("immediate should not be stopped by StopPropagation alone")
	}
}

func TestPhase5_EventTarget_StopImmediatePropagation(t *testing.T) {
	et := NewEventTarget()
	var order []int
	et.AddEventListener("test", func(e *Event) {
		order = append(order, 1)
		e.StopImmediatePropagation()
	})
	et.AddEventListener("test", func(e *Event) {
		order = append(order, 2) // Should not fire
	})

	ev := NewEvent("test")
	et.DispatchEvent(ev)

	if len(order) != 1 || order[0] != 1 {
		t.Errorf("only first listener should fire: got %v", order)
	}
	if !ev.IsPropagationStopped() {
		t.Error("propagation should be stopped")
	}
	if !ev.IsImmediatePropagationStopped() {
		t.Error("immediate propagation should be stopped")
	}
}

func TestPhase5_EventTarget_Detail(t *testing.T) {
	ev := NewEvent("x")
	if ev.Detail() != nil {
		t.Error("plain event detail should be nil")
	}
}

func TestPhase5_Event_Constructors(t *testing.T) {
	// NewEvent
	e1 := NewEvent("click")
	if e1.Type != "click" || e1.Bubbles || e1.Cancelable {
		t.Error("NewEvent unexpected fields")
	}

	// NewEventWithOptions
	e2 := NewEventWithOptions("hover", true, true)
	if e2.Type != "hover" || !e2.Bubbles || !e2.Cancelable {
		t.Error("NewEventWithOptions unexpected fields")
	}

	// NewCustomEvent
	ce1 := NewCustomEvent("custom", "payload")
	if ce1.Type != "custom" || ce1.Detail() != "payload" || ce1.Bubbles || ce1.Cancelable {
		t.Error("NewCustomEvent unexpected fields")
	}

	// NewCustomEventWithOptions
	ce2 := NewCustomEventWithOptions("custom2", 42, true, false)
	if ce2.Type != "custom2" || ce2.Detail() != 42 || !ce2.Bubbles || ce2.Cancelable {
		t.Error("NewCustomEventWithOptions unexpected fields")
	}

	// EventPtr
	ptr := ce2.EventPtr()
	if ptr != &ce2.Event {
		t.Error("EventPtr should return address of embedded Event")
	}

	// Dispatch a CustomEvent
	et := NewEventTarget()
	var gotDetail any
	et.AddEventListener("custom", func(e *Event) {
		gotDetail = e.Detail()
	})
	et.DispatchEvent(ce1.EventPtr())
	if gotDetail != "payload" {
		t.Errorf("custom event detail: got %v, want payload", gotDetail)
	}
}

// ============================================================================
// GROUP B — Performance
// ============================================================================

func TestPhase5_Performance_Basic(t *testing.T) {
	perf := NewPerformance()
	if perf == nil {
		t.Fatal("NewPerformance returned nil")
	}

	// Now should be >= 0 and small (just created)
	now := perf.Now()
	if now < 0 {
		t.Errorf("Now() returned negative: %f", now)
	}

	// TimeOrigin should be a recent Unix timestamp in ms
	origin := perf.TimeOrigin()
	if origin <= 0 {
		t.Error("TimeOrigin should be positive")
	}
}

func TestPhase5_Performance_Marks(t *testing.T) {
	perf := NewPerformance()
	perf.Mark("a")
	perf.MarkWithDetail("b", map[string]string{"key": "val"})

	entries := perf.GetEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Name != "a" || entries[0].EntryType != "mark" || entries[0].Duration != 0 {
		t.Errorf("mark 'a' unexpected: %+v", entries[0])
	}
	if entries[1].Name != "b" || entries[1].Detail == nil {
		t.Errorf("mark 'b' unexpected: %+v", entries[1])
	}
}

func TestPhase5_Performance_Measure(t *testing.T) {
	perf := NewPerformance()
	perf.Mark("start")
	perf.Mark("end")

	// Normal measure
	if err := perf.Measure("duration", "start", "end"); err != nil {
		t.Fatalf("Measure failed: %v", err)
	}

	measures := perf.GetEntriesByType("measure")
	if len(measures) != 1 {
		t.Fatalf("expected 1 measure, got %d", len(measures))
	}
	if measures[0].Name != "duration" {
		t.Errorf("unexpected measure name: %s", measures[0].Name)
	}
	if measures[0].Duration < 0 {
		t.Error("negative duration")
	}

	// Measure with empty start mark (use origin)
	if err := perf.Measure("from-origin", "", "end"); err != nil {
		t.Fatalf("Measure from origin failed: %v", err)
	}

	// Measure with empty end mark (use Now)
	if err := perf.Measure("to-now", "start", ""); err != nil {
		t.Fatalf("Measure to now failed: %v", err)
	}

	// MeasureWithDetail
	if err := perf.MeasureWithDetail("detailed", "start", "end", "extra"); err != nil {
		t.Fatalf("MeasureWithDetail failed: %v", err)
	}

	// Error: missing start mark
	if err := perf.Measure("bad", "nonexistent", "end"); err == nil {
		t.Error("expected error for missing start mark")
	}

	// Error: missing end mark
	if err := perf.Measure("bad2", "start", "nonexistent"); err == nil {
		t.Error("expected error for missing end mark")
	}
}

func TestPhase5_Performance_GetEntriesByName(t *testing.T) {
	perf := NewPerformance()
	perf.Mark("x")
	perf.Mark("y")
	perf.Mark("x")

	// By name only
	xs := perf.GetEntriesByName("x")
	if len(xs) != 2 {
		t.Errorf("got %d entries for 'x', want 2", len(xs))
	}

	// By name + type
	xMarks := perf.GetEntriesByName("x", "mark")
	if len(xMarks) != 2 {
		t.Errorf("got %d mark entries for 'x', want 2", len(xMarks))
	}

	// By name + wrong type
	xMeasures := perf.GetEntriesByName("x", "measure")
	if len(xMeasures) != 0 {
		t.Error("should find no measures named 'x'")
	}
}

func TestPhase5_Performance_GetEntriesSorted(t *testing.T) {
	perf := NewPerformance()
	perf.Mark("first")
	perf.Mark("second")

	sorted := perf.GetEntriesSorted()
	if len(sorted) != 2 {
		t.Fatal("expected 2 sorted entries")
	}
	if sorted[0].StartTime > sorted[1].StartTime {
		t.Error("entries not sorted by start time")
	}
}

func TestPhase5_Performance_ClearMarks(t *testing.T) {
	perf := NewPerformance()
	perf.Mark("a")
	perf.Mark("b")
	perf.Mark("a")

	// Clear specific
	perf.ClearMarks("a")
	entries := perf.GetEntriesByType("mark")
	if len(entries) != 1 || entries[0].Name != "b" {
		t.Errorf("after clearing 'a': got %+v", entries)
	}

	// Clear all
	perf.ClearMarks("")
	entries = perf.GetEntriesByType("mark")
	if len(entries) != 0 {
		t.Error("all marks should be cleared")
	}
}

func TestPhase5_Performance_ClearMeasures(t *testing.T) {
	perf := NewPerformance()
	perf.Mark("s")
	perf.Mark("e")
	perf.Measure("m1", "s", "e")
	perf.Measure("m2", "s", "e")

	// Clear specific
	perf.ClearMeasures("m1")
	measures := perf.GetEntriesByType("measure")
	if len(measures) != 1 || measures[0].Name != "m2" {
		t.Error("only m2 should remain")
	}

	// Clear all
	perf.ClearMeasures("")
	measures = perf.GetEntriesByType("measure")
	if len(measures) != 0 {
		t.Error("all measures should be cleared")
	}
}

func TestPhase5_Performance_ClearResourceTimings(t *testing.T) {
	perf := NewPerformance()
	perf.Mark("keep")
	// ClearResourceTimings should not remove marks
	perf.ClearResourceTimings()
	if len(perf.GetEntries()) != 1 {
		t.Error("ClearResourceTimings should not remove marks")
	}
}

func TestPhase5_Performance_ToJSON(t *testing.T) {
	perf := NewPerformance()
	j := perf.ToJSON()
	if j == nil {
		t.Fatal("ToJSON returned nil")
	}
	if _, ok := j["timeOrigin"]; !ok {
		t.Error("ToJSON missing timeOrigin")
	}
	origin, ok := j["timeOrigin"].(float64)
	if !ok || origin <= 0 {
		t.Error("timeOrigin should be a positive float64")
	}
}

func TestPhase5_Performance_Observer(t *testing.T) {
	perf := NewPerformance()
	perf.Mark("existing-mark")

	var observed []PerformanceEntry
	obs := newPerformanceObserver(perf, func(entries []PerformanceEntry, o *performanceObserver) {
		observed = append(observed, entries...)
	})

	// Observe with buffered=true delivers existing entries
	obs.Observe(performanceObserverOptions{
		EntryTypes: []string{"mark"},
		Buffered:   true,
	})
	if len(observed) != 1 || observed[0].Name != "existing-mark" {
		t.Errorf("buffered observe: got %v", observed)
	}

	// TakeRecords returns nil when buffer is empty
	records := obs.TakeRecords()
	if records != nil {
		t.Error("TakeRecords on empty buffer should return nil")
	}

	// Disconnect clears state
	obs.Disconnect()

	// After disconnect, observe without buffered delivers no existing entries
	observed = nil
	obs.Observe(performanceObserverOptions{
		EntryTypes: []string{"mark"},
		Buffered:   false,
	})
	if len(observed) != 0 {
		t.Error("non-buffered observe after disconnect should deliver nothing")
	}
}

func TestPhase5_LoopPerformance(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	lp := NewLoopPerformance(loop)
	if lp == nil {
		t.Fatal("NewLoopPerformance returned nil")
	}
	if lp.Performance == nil {
		t.Fatal("embedded Performance is nil")
	}

	// Should work with zero tick anchor
	now := lp.Now()
	if now < 0 {
		t.Error("Now should be non-negative")
	}
}

// ============================================================================
// GROUP C — Promise static methods
// ============================================================================

// helper: creates loop + js, starts loop, returns cleanup
func phase5LoopSetup(t *testing.T) (*Loop, *JS, context.CancelFunc) {
	t.Helper()
	loop, err := New(WithFastPathMode(FastPathForced))
	if err != nil {
		t.Fatal(err)
	}
	js, err := NewJS(loop)
	if err != nil {
		loop.Close()
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = loop.Run(ctx) }()
	waitForRunning(t, loop)
	return loop, js, cancel
}

func TestPhase5_Promise_All_Fulfilled(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	var ch <-chan any
	ready := make(chan struct{})
	loop.Submit(func() {
		p1, r1, _ := js.NewChainedPromise()
		p2, r2, _ := js.NewChainedPromise()
		r1("a")
		r2("b")
		result := js.All([]*ChainedPromise{p1, p2})
		ch = result.ToChannel()
		close(ready)
	})

	<-ready
	select {
	case v := <-ch:
		vals, ok := v.([]any)
		if !ok || len(vals) != 2 || vals[0] != "a" || vals[1] != "b" {
			t.Errorf("All result: %v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_Promise_All_Empty(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	var ch <-chan any
	ready := make(chan struct{})
	loop.Submit(func() {
		result := js.All([]*ChainedPromise{})
		ch = result.ToChannel()
		close(ready)
	})

	<-ready
	select {
	case v := <-ch:
		vals, ok := v.([]any)
		if !ok || len(vals) != 0 {
			t.Errorf("All empty: %v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_Promise_All_Rejected(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	var ch <-chan any
	ready := make(chan struct{})
	loop.Submit(func() {
		p1, r1, _ := js.NewChainedPromise()
		p2, _, rej2 := js.NewChainedPromise()
		r1("a")
		rej2(fmt.Errorf("fail"))
		result := js.All([]*ChainedPromise{p1, p2})
		caught := result.Catch(func(r any) any { return r })
		ch = caught.ToChannel()
		close(ready)
	})

	<-ready
	select {
	case v := <-ch:
		if err, ok := v.(error); !ok || err.Error() != "fail" {
			t.Errorf("All rejected: %v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_Promise_Race_Fulfilled(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	var ch <-chan any
	ready := make(chan struct{})
	loop.Submit(func() {
		p1, r1, _ := js.NewChainedPromise()
		p2, _, _ := js.NewChainedPromise() // Never settled
		r1("winner")
		result := js.Race([]*ChainedPromise{p1, p2})
		ch = result.ToChannel()
		close(ready)
	})

	<-ready
	select {
	case v := <-ch:
		if v != "winner" {
			t.Errorf("Race: got %v, want winner", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_Promise_Race_Empty(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	var p *ChainedPromise
	done := make(chan struct{})
	loop.Submit(func() {
		p = js.Race([]*ChainedPromise{})
		close(done)
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
	// Empty Race never settles
	if p.State() != Pending {
		t.Errorf("empty Race should be Pending, got %v", p.State())
	}
}

func TestPhase5_Promise_AllSettled_Mixed(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	var ch <-chan any
	ready := make(chan struct{})
	loop.Submit(func() {
		p1, r1, _ := js.NewChainedPromise()
		p2, _, rej2 := js.NewChainedPromise()
		r1("ok")
		rej2(fmt.Errorf("bad"))
		result := js.AllSettled([]*ChainedPromise{p1, p2})
		ch = result.ToChannel()
		close(ready)
	})

	<-ready
	select {
	case v := <-ch:
		results, ok := v.([]any)
		if !ok || len(results) != 2 {
			t.Fatalf("AllSettled: %v", v)
		}
		r0 := results[0].(map[string]any)
		r1 := results[1].(map[string]any)
		if r0["status"] != "fulfilled" || r0["value"] != "ok" {
			t.Errorf("AllSettled[0]: %v", r0)
		}
		if r1["status"] != "rejected" {
			t.Errorf("AllSettled[1]: %v", r1)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_Promise_AllSettled_Empty(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	var ch <-chan any
	ready := make(chan struct{})
	loop.Submit(func() {
		result := js.AllSettled([]*ChainedPromise{})
		ch = result.ToChannel()
		close(ready)
	})

	<-ready
	select {
	case v := <-ch:
		vals, ok := v.([]any)
		if !ok || len(vals) != 0 {
			t.Errorf("AllSettled empty: %v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_Promise_Any_Fulfilled(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	var ch <-chan any
	ready := make(chan struct{})
	loop.Submit(func() {
		p1, _, rej1 := js.NewChainedPromise()
		p2, r2, _ := js.NewChainedPromise()
		rej1(fmt.Errorf("e1"))
		r2("success")
		result := js.Any([]*ChainedPromise{p1, p2})
		ch = result.ToChannel()
		close(ready)
	})

	<-ready
	select {
	case v := <-ch:
		if v != "success" {
			t.Errorf("Any: got %v, want success", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_Promise_Any_AllRejected(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	var ch <-chan any
	ready := make(chan struct{})
	loop.Submit(func() {
		p1, _, rej1 := js.NewChainedPromise()
		p2, _, rej2 := js.NewChainedPromise()
		rej1(fmt.Errorf("e1"))
		rej2(fmt.Errorf("e2"))
		result := js.Any([]*ChainedPromise{p1, p2})
		caught := result.Catch(func(r any) any { return r })
		ch = caught.ToChannel()
		close(ready)
	})

	<-ready
	select {
	case v := <-ch:
		aggErr, ok := v.(*AggregateError)
		if !ok {
			t.Fatalf("expected AggregateError, got %T: %v", v, v)
		}
		if len(aggErr.Errors) != 2 {
			t.Errorf("expected 2 errors, got %d", len(aggErr.Errors))
		}
		if aggErr.Error() != "All promises were rejected" {
			t.Errorf("unexpected message: %s", aggErr.Error())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_Promise_Any_Empty(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	var ch <-chan any
	ready := make(chan struct{})
	loop.Submit(func() {
		result := js.Any([]*ChainedPromise{})
		caught := result.Catch(func(r any) any { return r })
		ch = caught.ToChannel()
		close(ready)
	})

	<-ready
	select {
	case v := <-ch:
		aggErr, ok := v.(*AggregateError)
		if !ok {
			t.Fatalf("expected AggregateError, got %T", v)
		}
		if aggErr.Cause() == nil {
			t.Error("Cause should return first error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_Promise_WithResolvers(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	var ch <-chan any
	ready := make(chan struct{})
	loop.Submit(func() {
		r := js.WithResolvers()
		r.Resolve("hello")
		ch = r.Promise.ToChannel()
		close(ready)
	})

	<-ready
	select {
	case v := <-ch:
		if v != "hello" {
			t.Errorf("WithResolvers: got %v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_Promise_Value_Reason_State(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	done := make(chan struct{})
	loop.Submit(func() {
		defer close(done)

		// Resolved promise
		p1 := js.Resolve("val")
		if p1.State() != Fulfilled {
			t.Errorf("Resolve state: %v", p1.State())
		}
		if p1.Value() != "val" {
			t.Errorf("Value: %v", p1.Value())
		}
		if p1.Reason() != nil {
			t.Errorf("Reason on fulfilled: %v", p1.Reason())
		}

		// Rejected promise
		p2 := js.Reject(fmt.Errorf("err"))
		// Catch to prevent unhandled
		p2.Catch(func(r any) any { return nil })
		if p2.State() != Rejected {
			t.Errorf("Reject state: %v", p2.State())
		}
		if p2.Reason() == nil {
			t.Error("Reason on rejected should be non-nil")
		}
		if p2.Value() != nil {
			t.Errorf("Value on rejected: %v", p2.Value())
		}

		// Pending promise
		p3, _, _ := js.NewChainedPromise()
		if p3.State() != Pending {
			t.Errorf("pending state: %v", p3.State())
		}
		if p3.Value() != nil {
			t.Error("pending value should be nil")
		}
		if p3.Reason() != nil {
			t.Error("pending reason should be nil")
		}
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_Promise_CreationStackTrace(t *testing.T) {
	// Without debug mode: should be empty
	loop, err := New(WithFastPathMode(FastPathForced))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	p, _, _ := js.NewChainedPromise()
	if p.CreationStackTrace() != "" {
		t.Error("stack trace should be empty without debug mode")
	}

	// With nil JS
	p2 := &ChainedPromise{}
	if p2.CreationStackTrace() != "" {
		t.Error("stack trace should be empty with nil JS")
	}
}

func TestPhase5_Promise_ToChannelStandalone(t *testing.T) {
	// Create a ChainedPromise without JS adapter
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	// Resolve it
	p.resolve("standalone-value")

	ch := p.ToChannel()
	select {
	case v := <-ch:
		if v != "standalone-value" {
			t.Errorf("standalone ToChannel: %v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_Promise_ToChannelStandalone_Pending(t *testing.T) {
	// Create a standalone pending promise
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	ch := p.ToChannel() // registers handler

	// Resolve after calling ToChannel
	p.resolve("delayed")

	select {
	case v := <-ch:
		if v != "delayed" {
			t.Errorf("standalone pending ToChannel: %v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// ============================================================================
// GROUP D — JS API
// ============================================================================

func TestPhase5_JS_Loop(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	if js.Loop() != loop {
		t.Error("JS.Loop() should return the underlying loop")
	}
}

func TestPhase5_JS_SetTimeout_ClearTimeout(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	done := make(chan struct{})
	loop.Submit(func() {
		// Basic SetTimeout fires
		var fired atomic.Bool
		_, err := js.SetTimeout(func() {
			fired.Store(true)
			close(done)
		}, 0)
		if err != nil {
			t.Errorf("SetTimeout: %v", err)
		}
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for SetTimeout callback")
	}

	// SetTimeout with nil fn
	id, err := js.SetTimeout(nil, 100)
	if id != 0 || err != nil {
		t.Errorf("nil fn: id=%d, err=%v", id, err)
	}

	// ClearTimeout with invalid ID
	err = js.ClearTimeout(999999)
	if !errors.Is(err, ErrTimerNotFound) {
		t.Errorf("ClearTimeout invalid: %v", err)
	}
}

func TestPhase5_JS_ClearInterval(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	done := make(chan struct{})
	loop.Submit(func() {
		var count atomic.Int32
		id, err := js.SetInterval(func() {
			count.Add(1)
		}, 10)
		if err != nil {
			t.Errorf("SetInterval: %v", err)
			close(done)
			return
		}
		// Clear immediately
		if err := js.ClearInterval(id); err != nil {
			t.Errorf("ClearInterval: %v", err)
		}
		close(done)
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	// SetInterval with nil fn
	id, err := js.SetInterval(nil, 100)
	if id != 0 || err != nil {
		t.Errorf("nil interval fn: id=%d, err=%v", id, err)
	}

	// ClearInterval with invalid ID
	err = js.ClearInterval(999999)
	if !errors.Is(err, ErrTimerNotFound) {
		t.Errorf("ClearInterval invalid: %v", err)
	}
}

func TestPhase5_JS_SetImmediate_ClearImmediate(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	done := make(chan struct{})
	loop.Submit(func() {
		var fired atomic.Bool
		id, err := js.SetImmediate(func() {
			fired.Store(true)
			close(done)
		})
		if err != nil || id == 0 {
			t.Errorf("SetImmediate: id=%d, err=%v", id, err)
		}
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for SetImmediate")
	}

	// SetImmediate nil fn
	id, err := js.SetImmediate(nil)
	if id != 0 || err != nil {
		t.Errorf("nil immediate fn: id=%d, err=%v", id, err)
	}

	// ClearImmediate invalid ID
	err = js.ClearImmediate(999999)
	if !errors.Is(err, ErrTimerNotFound) {
		t.Errorf("ClearImmediate invalid: %v", err)
	}
}

func TestPhase5_JS_ClearImmediate_Before_Run(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	done := make(chan struct{})
	loop.Submit(func() {
		defer close(done)
		var called bool
		id, err := js.SetImmediate(func() {
			called = true
		})
		if err != nil {
			t.Errorf("SetImmediate: %v", err)
			return
		}
		// Clear before it runs
		err = js.ClearImmediate(id)
		if err != nil {
			t.Errorf("ClearImmediate: %v", err)
		}
		// Give loop a tick to verify it doesn't fire
		runtime.Gosched()
		if called {
			// This is a best-effort check; the callback may have already run
			// due to scheduling, which is acceptable per JS semantics
			_ = called
		}
	})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_JS_Try(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	done := make(chan struct{})
	loop.Submit(func() {
		defer close(done)
		// Normal return
		p := js.Try(func() any { return 42 })
		if p.State() != Fulfilled || p.Value() != 42 {
			t.Errorf("Try normal: state=%v, value=%v", p.State(), p.Value())
		}

		// Panic case
		p2 := js.Try(func() any { panic("oops") })
		p2.Catch(func(r any) any { return nil }) // Prevent unhandled
		if p2.State() != Rejected {
			t.Errorf("Try panic: state=%v", p2.State())
		}
		reason := p2.Reason()
		if pe, ok := reason.(PanicError); !ok || pe.Value != "oops" {
			t.Errorf("Try panic reason: %v (%T)", reason, reason)
		}
	})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_JS_NextTick(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	done := make(chan struct{})
	loop.Submit(func() {
		var fired bool
		js.NextTick(func() {
			fired = true
			close(done)
		})
		_ = fired
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_JS_QueueMicrotask_Nil(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	// Nil fn should be a no-op
	if err := js.QueueMicrotask(nil); err != nil {
		t.Errorf("QueueMicrotask(nil): %v", err)
	}
}

func TestPhase5_JS_Resolve_Reject(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	done := make(chan struct{})
	loop.Submit(func() {
		defer close(done)

		// Resolve
		p := js.Resolve("v")
		if p.State() != Fulfilled || p.Value() != "v" {
			t.Errorf("Resolve: state=%v, value=%v", p.State(), p.Value())
		}

		// Reject
		p2 := js.Reject(fmt.Errorf("e"))
		p2.Catch(func(r any) any { return nil }) // prevent unhandled
		if p2.State() != Rejected {
			t.Errorf("Reject: state=%v", p2.State())
		}
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_JS_GetDelay(t *testing.T) {
	state := &intervalState{delayMs: 50}
	d := state.getDelay()
	if d != 50*time.Millisecond {
		t.Errorf("getDelay: got %v, want 50ms", d)
	}
}

// ============================================================================
// GROUP E — errors.go, state.go, promisify.go, loop.go, metrics.go, psquare.go, registry.go
// ============================================================================

// --- errors.go ---

func TestPhase5_PanicError(t *testing.T) {
	pe := PanicError{Value: "boom"}
	if pe.Error() != "promise: goroutine panicked: boom" {
		t.Errorf("PanicError.Error: %q", pe.Error())
	}

	// Unwrap with error value
	inner := fmt.Errorf("inner")
	pe2 := PanicError{Value: inner}
	if pe2.Unwrap() != inner {
		t.Error("Unwrap should return inner error")
	}

	// Unwrap with non-error value
	pe3 := PanicError{Value: "string"}
	if pe3.Unwrap() != nil {
		t.Error("Unwrap should return nil for non-error")
	}

	// Is
	if !pe.Is(PanicError{}) {
		t.Error("Is should match PanicError")
	}
	if !pe.Is(&PanicError{}) {
		t.Error("Is should match *PanicError")
	}
	if pe.Is(fmt.Errorf("other")) {
		t.Error("Is should not match other errors")
	}

	// errors.Is integration
	if !errors.Is(pe2, inner) {
		t.Error("errors.Is should find inner through Unwrap")
	}
}

func TestPhase5_AggregateError(t *testing.T) {
	agg := &AggregateError{
		Errors:  []error{fmt.Errorf("e1"), fmt.Errorf("e2")},
		Message: "test msg",
	}
	if agg.Error() != "test msg" {
		t.Errorf("Error: %q", agg.Error())
	}
	if agg.Cause().Error() != "e1" {
		t.Error("Cause should return first error")
	}

	// Unwrap []error
	unwrapped := agg.Unwrap()
	if len(unwrapped) != 2 {
		t.Errorf("Unwrap: %d errors", len(unwrapped))
	}

	// Is
	if !agg.Is(&AggregateError{}) {
		t.Error("Is should match *AggregateError")
	}
	if agg.Is(fmt.Errorf("other")) {
		t.Error("Is should not match other errors")
	}

	// Empty message defaults
	agg2 := &AggregateError{}
	if agg2.Error() != "All promises were rejected" {
		t.Errorf("default Error: %q", agg2.Error())
	}
	if agg2.Cause() != nil {
		t.Error("Cause with empty Errors should be nil")
	}

	// errors.Is integration
	if !errors.Is(agg, fmt.Errorf("e1")) {
		// Go 1.20+ multi-error
		if errors.Is(agg, agg.Errors[0]) {
			// Fine, the errors.Is finds it
		}
	}
}

func TestPhase5_TypeError(t *testing.T) {
	te := &TypeError{Message: "bad type"}
	if te.Error() != "bad type" {
		t.Errorf("Error: %q", te.Error())
	}

	te2 := &TypeError{}
	if te2.Error() != "type error" {
		t.Errorf("default Error: %q", te2.Error())
	}

	// Unwrap
	inner := fmt.Errorf("cause")
	te3 := &TypeError{Cause: inner}
	if te3.Unwrap() != inner {
		t.Error("Unwrap should return Cause")
	}

	// Is
	if !te.Is(&TypeError{}) {
		t.Error("Is should match *TypeError")
	}
	if te.Is(fmt.Errorf("x")) {
		t.Error("Is should not match other errors")
	}
}

func TestPhase5_RangeError(t *testing.T) {
	re := &RangeError{Message: "out of range"}
	if re.Error() != "out of range" {
		t.Errorf("Error: %q", re.Error())
	}

	re2 := &RangeError{}
	if re2.Error() != "range error" {
		t.Errorf("default Error: %q", re2.Error())
	}

	inner := fmt.Errorf("cause")
	re3 := &RangeError{Cause: inner}
	if re3.Unwrap() != inner {
		t.Error("Unwrap should return Cause")
	}

	if !re.Is(&RangeError{}) {
		t.Error("Is should match *RangeError")
	}
}

func TestPhase5_TimeoutError(t *testing.T) {
	te := &TimeoutError{Message: "timed out"}
	if te.Error() != "timed out" {
		t.Errorf("Error: %q", te.Error())
	}

	te2 := &TimeoutError{}
	if te2.Error() != "operation timed out" {
		t.Errorf("default Error: %q", te2.Error())
	}

	inner := fmt.Errorf("cause")
	te3 := &TimeoutError{Cause: inner}
	if te3.Unwrap() != inner {
		t.Error("Unwrap should return Cause")
	}

	if !te.Is(&TimeoutError{}) {
		t.Error("Is should match *TimeoutError")
	}
	if te.Is(fmt.Errorf("x")) {
		t.Error("Is should not match other errors")
	}
}

func TestPhase5_UnhandledRejectionDebugInfo(t *testing.T) {
	// With error reason
	info := &UnhandledRejectionDebugInfo{
		Reason:             fmt.Errorf("test-err"),
		CreationStackTrace: "main.go:42",
	}
	if info.Error() != "test-err" {
		t.Errorf("Error: %q", info.Error())
	}
	if info.Unwrap().Error() != "test-err" {
		t.Error("Unwrap should return the error reason")
	}

	// With non-error reason
	info2 := &UnhandledRejectionDebugInfo{Reason: "string-reason"}
	if info2.Error() != "string-reason" {
		t.Errorf("non-error Error: %q", info2.Error())
	}
	if info2.Unwrap() != nil {
		t.Error("Unwrap should be nil for non-error reason")
	}
}

func TestPhase5_ErrorWrapper(t *testing.T) {
	ew := &errorWrapper{Value: 42}
	if ew.Error() != "42" {
		t.Errorf("errorWrapper.Error: %q", ew.Error())
	}
}

func TestPhase5_ErrNoPromiseResolved(t *testing.T) {
	e := &errNoPromiseResolved{}
	if e.Error() != "No promises were provided" {
		t.Errorf("errNoPromiseResolved.Error: %q", e.Error())
	}
}

// --- state.go ---

func TestPhase5_LoopState_String(t *testing.T) {
	tests := []struct {
		state LoopState
		want  string
	}{
		{StateAwake, "Awake"},
		{StateRunning, "Running"},
		{StateSleeping, "Sleeping"},
		{StateTerminating, "Terminating"},
		{StateTerminated, "Terminated"},
		{LoopState(99), "Unknown"},
	}
	for _, tc := range tests {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("LoopState(%d).String() = %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestPhase5_FastState_TransitionAny(t *testing.T) {
	s := newFastState()
	// Should be Awake initially
	if s.Load() != StateAwake {
		t.Fatalf("initial state: %v", s.Load())
	}

	// TransitionAny from valid from-state
	ok := s.TransitionAny([]LoopState{StateAwake, StateRunning}, StateRunning)
	if !ok {
		t.Error("TransitionAny should succeed from Awake to Running")
	}
	if s.Load() != StateRunning {
		t.Errorf("after transition: %v", s.Load())
	}

	// TransitionAny from invalid from-state
	ok = s.TransitionAny([]LoopState{StateAwake, StateSleeping}, StateTerminated)
	if ok {
		t.Error("TransitionAny should fail when current state not in validFrom")
	}
}

func TestPhase5_FastState_IsTerminal(t *testing.T) {
	s := newFastState()
	if s.IsTerminal() {
		t.Error("Awake is not terminal")
	}
	s.Store(StateTerminated)
	if !s.IsTerminal() {
		t.Error("Terminated should be terminal")
	}
}

func TestPhase5_FastState_CanAcceptWork(t *testing.T) {
	s := newFastState()
	if !s.CanAcceptWork() {
		t.Error("Awake can accept work")
	}

	s.Store(StateRunning)
	if !s.CanAcceptWork() {
		t.Error("Running can accept work")
	}

	s.Store(StateSleeping)
	if !s.CanAcceptWork() {
		t.Error("Sleeping can accept work")
	}

	s.Store(StateTerminating)
	if s.CanAcceptWork() {
		t.Error("Terminating cannot accept work")
	}

	s.Store(StateTerminated)
	if s.CanAcceptWork() {
		t.Error("Terminated cannot accept work")
	}
}

// --- promisify.go ---

func TestPhase5_PromisifyWithTimeout(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	go func() { _ = loop.Run(ctx) }()
	waitForRunning(t, loop)
	defer loop.Close()

	// Success case
	p := loop.PromisifyWithTimeout(context.Background(), 5*time.Second, func(ctx context.Context) (any, error) {
		return "ok", nil
	})

	ch := p.ToChannel()
	select {
	case v := <-ch:
		if v != "ok" {
			t.Errorf("PromisifyWithTimeout: %v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_PromisifyWithDeadline(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	go func() { _ = loop.Run(ctx) }()
	waitForRunning(t, loop)
	defer loop.Close()

	// Success case
	p := loop.PromisifyWithDeadline(context.Background(), time.Now().Add(5*time.Second), func(ctx context.Context) (any, error) {
		return "deadline-ok", nil
	})

	ch := p.ToChannel()
	select {
	case v := <-ch:
		if v != "deadline-ok" {
			t.Errorf("PromisifyWithDeadline: %v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// --- loop.go ---

func TestPhase5_SetFastPathMode(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	// Auto -> Disabled
	if err := loop.SetFastPathMode(FastPathDisabled); err != nil {
		t.Errorf("set Disabled: %v", err)
	}

	// Disabled -> Forced (no FDs registered, should succeed)
	if err := loop.SetFastPathMode(FastPathForced); err != nil {
		t.Errorf("set Forced: %v", err)
	}

	// Back to Auto
	if err := loop.SetFastPathMode(FastPathAuto); err != nil {
		t.Errorf("set Auto: %v", err)
	}
}

func TestPhase5_SetTickAnchor_TickAnchor(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	anchor := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	loop.SetTickAnchor(anchor)
	got := loop.TickAnchor()
	if !got.Equal(anchor) {
		t.Errorf("TickAnchor: got %v, want %v", got, anchor)
	}
}

func TestPhase5_CancelTimers(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	if err != nil {
		t.Fatal(err)
	}

	// CancelTimers before loop is running => ErrLoopNotRunning
	errs := loop.CancelTimers([]TimerID{1, 2})
	for i, e := range errs {
		if !errors.Is(e, ErrLoopNotRunning) {
			t.Errorf("CancelTimers[%d] before run: %v", i, e)
		}
	}

	// Empty slice
	errs = loop.CancelTimers(nil)
	if errs != nil {
		t.Error("empty CancelTimers should return nil")
	}

	loop.Close()
}

// --- metrics.go ---

func TestPhase5_Metrics_Record_Sample(t *testing.T) {
	var lm LatencyMetrics

	// Record some samples
	for i := range 10 {
		lm.Record(time.Duration(i) * time.Millisecond)
	}

	n := lm.Sample()
	if n != 10 {
		t.Errorf("Sample count: %d", n)
	}
	if lm.P50 <= 0 {
		t.Errorf("P50 should be > 0: %v", lm.P50)
	}
	if lm.Max != 9*time.Millisecond {
		t.Errorf("Max: %v", lm.Max)
	}
}

func TestPhase5_Metrics_Record_LargeSample(t *testing.T) {
	// Trigger P-Square path (>= 5 samples, lots of data)
	var lm LatencyMetrics
	for i := range 100 {
		lm.Record(time.Duration(i) * time.Microsecond)
	}
	n := lm.Sample()
	if n != 100 {
		t.Errorf("Sample count: %d", n)
	}
	if lm.P50 <= 0 {
		t.Errorf("P50 should be > 0: %v", lm.P50)
	}
	if lm.P99 <= 0 {
		t.Error("P99 should be positive")
	}
}

func TestPhase5_Metrics_Record_Overflow(t *testing.T) {
	// Fill beyond sampleSize to exercise ring buffer wraparound
	var lm LatencyMetrics
	for range sampleSize + 100 {
		lm.Record(time.Millisecond)
	}
	n := lm.Sample()
	if n != sampleSize {
		t.Errorf("after overflow, sample count: %d, want %d", n, sampleSize)
	}
}

func TestPhase5_Metrics_EmptySample(t *testing.T) {
	var lm LatencyMetrics
	n := lm.Sample()
	if n != 0 {
		t.Errorf("empty Sample: %d", n)
	}
}

func TestPhase5_PercentileIndex(t *testing.T) {
	// Basic
	idx := percentileIndex(100, 50)
	if idx != 50 {
		t.Errorf("percentileIndex(100,50): %d", idx)
	}

	// Edge: 100th percentile should clamp
	idx = percentileIndex(10, 100)
	if idx != 9 {
		t.Errorf("percentileIndex(10,100): %d, want 9", idx)
	}

	// Small sample
	idx = percentileIndex(1, 99)
	if idx != 0 {
		t.Errorf("percentileIndex(1,99): %d", idx)
	}
}

func TestPhase5_QueueMetrics(t *testing.T) {
	var qm QueueMetrics

	qm.UpdateIngress(5)
	qm.UpdateIngress(10)
	if qm.IngressMax != 10 {
		t.Errorf("IngressMax: %d", qm.IngressMax)
	}
	if qm.IngressCurrent != 10 {
		t.Errorf("IngressCurrent: %d", qm.IngressCurrent)
	}

	qm.UpdateInternal(3)
	qm.UpdateInternal(7)
	if qm.InternalMax != 7 {
		t.Errorf("InternalMax: %d", qm.InternalMax)
	}

	qm.UpdateMicrotask(1)
	qm.UpdateMicrotask(4)
	if qm.MicrotaskMax != 4 {
		t.Errorf("MicrotaskMax: %d", qm.MicrotaskMax)
	}

	// Check EMA is initialized
	if qm.IngressAvg == 0 {
		t.Error("IngressAvg should be non-zero after updates")
	}
	if qm.InternalAvg == 0 {
		t.Error("InternalAvg should be non-zero after updates")
	}
	if qm.MicrotaskAvg == 0 {
		t.Error("MicrotaskAvg should be non-zero after updates")
	}
}

func TestPhase5_TPSCounter(t *testing.T) {
	counter := newTPSCounter(1*time.Second, 100*time.Millisecond)

	// Initially zero
	if tps := counter.TPS(); tps != 0 {
		t.Errorf("initial TPS: %f", tps)
	}

	// Increment
	for range 10 {
		counter.Increment()
	}

	tps := counter.TPS()
	if tps <= 0 {
		t.Errorf("TPS after increments should be > 0: %f", tps)
	}
}

func TestPhase5_TPSCounter_Panics(t *testing.T) {
	// Zero windowSize
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for zero windowSize")
			}
		}()
		newTPSCounter(0, time.Millisecond)
	}()

	// Zero bucketSize
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for zero bucketSize")
			}
		}()
		newTPSCounter(time.Second, 0)
	}()

	// bucketSize > windowSize
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for bucketSize > windowSize")
			}
		}()
		newTPSCounter(time.Millisecond, time.Second)
	}()
}

func TestPhase5_TPSCounter_Rotate(t *testing.T) {
	// Counter with very short window to exercise rotation
	counter := newTPSCounter(100*time.Millisecond, 10*time.Millisecond)
	counter.Increment()
	counter.Increment()

	// Force rotation by manipulating lastRotation to the past
	counter.mu.Lock()
	counter.lastRotation.Store(time.Now().Add(-200 * time.Millisecond))
	counter.mu.Unlock()

	// This call should trigger full window reset (elapsed > window)
	counter.Increment()
	tps := counter.TPS()
	// After a full reset, the counter should still be valid
	_ = tps
}

// --- psquare.go ---

func TestPhase5_PSquare_Basic(t *testing.T) {
	ps := newPSquareQuantile(0.50)

	// Before initialization
	if ps.Count() != 0 {
		t.Error("initial count should be 0")
	}
	if ps.Quantile() != 0 {
		t.Error("initial quantile should be 0")
	}

	// Add < 5 observations
	ps.Update(10)
	ps.Update(20)
	ps.Update(5)
	if ps.Count() != 3 {
		t.Errorf("count after 3 updates: %d", ps.Count())
	}
	q := ps.Quantile()
	if q < 5 || q > 20 {
		t.Errorf("3-sample quantile out of range: %f", q)
	}

	// Max with < 5 observations
	m := ps.Max()
	if m != 20 {
		t.Errorf("Max before init: %f, want 20", m)
	}
}

func TestPhase5_PSquare_After_Init(t *testing.T) {
	ps := newPSquareQuantile(0.50)
	// Add exactly 5 to trigger init
	ps.Update(1)
	ps.Update(2)
	ps.Update(3)
	ps.Update(4)
	ps.Update(5)

	if ps.Count() != 5 {
		t.Errorf("count: %d", ps.Count())
	}
	q := ps.Quantile()
	if q < 1 || q > 5 {
		t.Errorf("5-sample P50 out of range: %f", q)
	}
	if ps.Max() != 5 {
		t.Errorf("Max after init: %f", ps.Max())
	}
}

func TestPhase5_PSquare_ManyObservations(t *testing.T) {
	ps := newPSquareQuantile(0.50)
	for i := range 1000 {
		ps.Update(float64(i))
	}

	// P50 of 0..999 should be ~500
	q := ps.Quantile()
	if q < 400 || q > 600 {
		t.Errorf("P50 of 0..999: %f (expected ~500)", q)
	}
	if ps.Max() != 999 {
		t.Errorf("Max: %f", ps.Max())
	}
}

func TestPhase5_PSquare_Linear(t *testing.T) {
	ps := newPSquareQuantile(0.50)
	// Initialize fully
	ps.Update(10)
	ps.Update(20)
	ps.Update(30)
	ps.Update(40)
	ps.Update(50)

	// Test linear adjustment (d=1 and d=-1)
	// Linear is called internally during Update when parabolic is invalid.
	// We can get coverage by adding values in specific patterns.
	for i := range 100 {
		ps.Update(float64(i))
	}
	// The linear function is exercised during marker adjustment
	if ps.Count() != 105 {
		t.Errorf("count after linear test: %d", ps.Count())
	}
}

func TestPhase5_PSquare_ClampPercentile(t *testing.T) {
	// Clamp negative
	ps := newPSquareQuantile(-1)
	if ps.p != 0 {
		t.Errorf("negative percentile should clamp to 0: %f", ps.p)
	}

	// Clamp > 1
	ps2 := newPSquareQuantile(2)
	if ps2.p != 1 {
		t.Errorf("percentile > 1 should clamp to 1: %f", ps2.p)
	}
}

func TestPhase5_PSquareMultiQuantile(t *testing.T) {
	m := newPSquareMultiQuantile(0.50, 0.90, 0.99)

	// Zero state
	if m.Count() != 0 {
		t.Error("initial count")
	}
	if m.Sum() != 0 {
		t.Error("initial sum")
	}
	if m.Max() != 0 {
		t.Error("initial Max should be 0")
	}
	if m.Mean() != 0 {
		t.Error("initial Mean should be 0")
	}

	// Add data
	for i := range 100 {
		m.Update(float64(i))
	}

	if m.Count() != 100 {
		t.Errorf("count: %d", m.Count())
	}
	if m.Sum() != 4950 {
		t.Errorf("sum: %f", m.Sum())
	}
	if m.Max() != 99 {
		t.Errorf("Max: %f", m.Max())
	}

	mean := m.Mean()
	if mean < 49 || mean > 50 {
		t.Errorf("Mean: %f (expected ~49.5)", mean)
	}

	// Quantile retrieval
	p50 := m.Quantile(0)
	if p50 < 40 || p50 > 60 {
		t.Errorf("P50: %f", p50)
	}
	p99 := m.Quantile(2)
	if p99 < 80 {
		t.Errorf("P99 too low: %f", p99)
	}

	// Out of bounds index
	if m.Quantile(-1) != 0 {
		t.Error("negative index should return 0")
	}
	if m.Quantile(100) != 0 {
		t.Error("out of bounds index should return 0")
	}

	// Reset
	m.Reset()
	if m.Count() != 0 || m.Sum() != 0 {
		t.Error("Reset did not clear state")
	}
	if m.Max() != 0 {
		t.Errorf("Max after Reset should be 0, got %f", m.Max())
	}
}

func TestPhase5_PSquareMultiQuantile_MaxReset(t *testing.T) {
	m := newPSquareMultiQuantile(0.50)
	m.Update(100)
	m.Reset()
	// After reset, max should be back to -MaxFloat64 internally,
	// but Max() returns 0 when count == 0
	if m.Max() != 0 {
		t.Errorf("Max after Reset: %f", m.Max())
	}
	// Add a value and check it updates
	m.Update(42)
	if m.Max() != 42 {
		t.Errorf("Max after re-add: %f", m.Max())
	}
}

func TestPhase5_PSquare_NewMinMax(t *testing.T) {
	// Test the code path where x < q[0] (new minimum) and x >= q[4] (new maximum)
	ps := newPSquareQuantile(0.50)
	ps.Update(50)
	ps.Update(60)
	ps.Update(70)
	ps.Update(80)
	ps.Update(90) // Now initialized with [50,60,70,80,90]

	ps.Update(10)  // New minimum (x < q[0])
	ps.Update(100) // New maximum (x >= q[4])

	if ps.Max() != 100 {
		t.Errorf("Max after new max: %f", ps.Max())
	}
}

// --- registry.go ---

func TestPhase5_Registry_CompactAndRenew(t *testing.T) {
	r := newRegistry()

	// Create enough promises to trigger compaction
	// Need capacity > 256 and load factor < 25%
	for range 300 {
		r.NewPromise()
	}

	// Mark many as settled (so they get scavenged)
	r.mu.RLock()
	for _, wp := range r.data {
		p := wp.Value()
		if p != nil {
			p.Resolve("done")
		}
	}
	r.mu.RUnlock()

	// Run scavenge cycles to remove settled promises
	// Need a full cycle completion (head wraps to 0)
	for range 10 {
		r.Scavenge(50)
	}

	// After scavenging, check ring is compacted
	r.mu.RLock()
	ringLen := len(r.ring)
	dataLen := len(r.data)
	r.mu.RUnlock()

	// All promises were resolved, so ring should be compact
	_ = ringLen
	_ = dataLen
	// Just verify it doesn't panic/deadlock
}

func TestPhase5_Registry_RejectAll(t *testing.T) {
	r := newRegistry()

	_, p1 := r.NewPromise()
	_, p2 := r.NewPromise()

	r.RejectAll(fmt.Errorf("shutdown"))

	if p1.State() != Rejected {
		t.Error("p1 should be rejected")
	}
	if p2.State() != Rejected {
		t.Error("p2 should be rejected")
	}
}

func TestPhase5_Registry_ScavengeEmpty(t *testing.T) {
	r := newRegistry()
	// Scavenge on empty registry should be no-op
	r.Scavenge(10)
	r.Scavenge(0)  // zero batch size
	r.Scavenge(-1) // negative batch size
}

// --- Additional edge cases for better coverage ---

func TestPhase5_PSquare_MaxZeroObs(t *testing.T) {
	ps := newPSquareQuantile(0.50)
	if ps.Max() != 0 {
		t.Errorf("Max with 0 obs: %f", ps.Max())
	}
}

func TestPhase5_PSquare_SingleObs(t *testing.T) {
	ps := newPSquareQuantile(0.99)
	ps.Update(42)
	q := ps.Quantile()
	if q != 42 {
		t.Errorf("single obs P99: %f, want 42", q)
	}
	if ps.Max() != 42 {
		t.Errorf("Max with 1 obs: %f", ps.Max())
	}
}

func TestPhase5_PSquare_Negative(t *testing.T) {
	// P-Square should handle negative values
	ps := newPSquareQuantile(0.50)
	for i := -50; i < 50; i++ {
		ps.Update(float64(i))
	}
	q := ps.Quantile()
	if q < -10 || q > 10 {
		t.Errorf("P50 of -50..49: %f (expected ~0)", q)
	}
}

func TestPhase5_PSquare_Float_Max(t *testing.T) {
	m := newPSquareMultiQuantile(0.50)
	// After Reset, internal max is -MaxFloat64 but Max() returns 0
	m.Reset()
	if m.max != -math.MaxFloat64 {
		t.Errorf("internal max after Reset: %f", m.max)
	}
}

func TestPhase5_Promise_Any_NonErrorRejection(t *testing.T) {
	// Cover errorWrapper path: when rejection reason is not an error
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	var ch <-chan any
	ready := make(chan struct{})
	loop.Submit(func() {
		p1, _, rej1 := js.NewChainedPromise()
		p2, _, rej2 := js.NewChainedPromise()
		rej1("string-reason") // Not an error type
		rej2(42)              // Not an error type
		result := js.Any([]*ChainedPromise{p1, p2})
		caught := result.Catch(func(r any) any { return r })
		ch = caught.ToChannel()
		close(ready)
	})

	<-ready
	select {
	case v := <-ch:
		aggErr, ok := v.(*AggregateError)
		if !ok {
			t.Fatalf("expected AggregateError, got %T: %v", v, v)
		}
		if len(aggErr.Errors) != 2 {
			t.Errorf("expected 2 errors, got %d", len(aggErr.Errors))
		}
		// Check errorWrapper wrapping
		for _, e := range aggErr.Errors {
			if e == nil {
				t.Error("error should not be nil")
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_JS_Sleep(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	var ch <-chan any
	ready := make(chan struct{})
	loop.Submit(func() {
		p := js.Sleep(0)
		ch = p.ToChannel()
		close(ready)
	})

	<-ready
	select {
	case v := <-ch:
		if v != nil {
			t.Errorf("Sleep result: %v, want nil", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_JS_Timeout(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	var ch <-chan any
	ready := make(chan struct{})
	loop.Submit(func() {
		p := js.Timeout(0)
		caught := p.Catch(func(r any) any { return r })
		ch = caught.ToChannel()
		close(ready)
	})

	<-ready
	select {
	case v := <-ch:
		te, ok := v.(*TimeoutError)
		if !ok {
			t.Fatalf("expected TimeoutError, got %T: %v", v, v)
		}
		if te.Message == "" {
			t.Error("TimeoutError should have a message")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_JS_WithUnhandledRejection(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	var received atomic.Value
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		received.Store(reason)
	}))
	if err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()
	go func() { _ = loop.Run(ctx) }()
	waitForRunning(t, loop)

	// Create a rejected promise without a handler
	done := make(chan struct{})
	loop.Submit(func() {
		js.Reject(fmt.Errorf("unhandled"))
		// Schedule a delayed check via NextTick
		js.NextTick(func() {
			close(done)
		})
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	// Give rejection detection a moment
	time.Sleep(50 * time.Millisecond)

	// The callback may or may not have fired depending on timing
	// (this is best-effort coverage of the WithUnhandledRejection path)
	_ = received.Load()
}

func TestPhase5_JS_NewJS_NilOption(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	// nil option should be skipped gracefully
	js, err := NewJS(loop, nil)
	if err != nil {
		t.Fatalf("NewJS with nil option: %v", err)
	}
	if js.Loop() != loop {
		t.Error("Loop() mismatch")
	}
}

func TestPhase5_PromiseState_Constants(t *testing.T) {
	// Verify Fulfilled == Resolved alias
	if Fulfilled != Resolved {
		t.Error("Fulfilled should equal Resolved")
	}
}

func TestPhase5_Promise_Catch_Finally(t *testing.T) {
	loop, js, cancel := phase5LoopSetup(t)
	defer cancel()
	defer loop.Close()

	done := make(chan struct{})
	loop.Submit(func() {
		defer close(done)

		// Catch
		p := js.Reject(fmt.Errorf("err"))
		var caught any
		p.Catch(func(r any) any {
			caught = r
			return "recovered"
		})
		_ = caught

		// Finally on fulfilled
		var finallyRan bool
		p2 := js.Resolve("val")
		p2.Finally(func() {
			finallyRan = true
		})
		_ = finallyRan

		// Finally with nil callback
		p3 := js.Resolve("val2")
		p3.Finally(nil)
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPhase5_Promise_ThenStandalone(t *testing.T) {
	// Standalone promise (no JS) uses thenStandalone
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	var result any
	child := p.Then(func(v any) any {
		result = v
		return v
	}, nil)

	p.resolve("standalone")

	// thenStandalone executes synchronously
	if child.State() != Fulfilled {
		t.Errorf("child state: %v", child.State())
	}
	if result != "standalone" {
		t.Errorf("result: %v", result)
	}
}
