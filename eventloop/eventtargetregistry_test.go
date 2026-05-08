package eventloop

import (
	"sync"
	"testing"
)

func TestEndEventDispatchRejectsInactiveEvent(t *testing.T) {
	event := NewEvent("event")
	got := abortEventCapturePanic(func() { endEventDispatch(event) })
	if got != "eventloop: inactive Event dispatch" {
		t.Fatalf("inactive end panic = %#v, want %q", got, "eventloop: inactive Event dispatch")
	}

	// The invariant failure must release the package registry lock and leave the
	// event reusable for a subsequent well-formed begin/end pair.
	beginEventDispatch(event)
	endEventDispatch(event)
}

func TestEventDispatchInlineHoleReuseAndActiveLookup(t *testing.T) {
	first := NewEvent("first")
	second := NewEvent("second")
	replacement := NewEvent("replacement")
	firstActive := false
	secondActive := false
	replacementActive := false
	defer func() {
		if replacementActive {
			endEventDispatch(replacement)
		}
		if secondActive {
			endEventDispatch(second)
		}
		if firstActive {
			endEventDispatch(first)
		}
	}()

	firstRef := beginEventDispatch(first)
	firstActive = true
	secondRef := beginEventDispatch(second)
	secondActive = true
	switch inlineActiveEventDispatchCapacity {
	case 0:
		if firstRef.inlineIndex != -1 || secondRef.inlineIndex != -1 {
			t.Fatalf("map-only registry refs = (%d, %d), want (-1, -1)", firstRef.inlineIndex, secondRef.inlineIndex)
		}
	case 1:
		if firstRef.inlineIndex != 0 || secondRef.inlineIndex != -1 {
			t.Fatalf("single-slot registry refs = (%d, %d), want (0, -1)", firstRef.inlineIndex, secondRef.inlineIndex)
		}
	default:
		if firstRef.inlineIndex < 0 || secondRef.inlineIndex < 0 || firstRef.inlineIndex == secondRef.inlineIndex {
			t.Fatalf("initial inline refs = (%d, %d), want distinct inline slots", firstRef.inlineIndex, secondRef.inlineIndex)
		}
	}

	endEventDispatch(first)
	firstActive = false
	replacementRef := beginEventDispatch(replacement)
	replacementActive = true
	if replacementRef.inlineIndex != firstRef.inlineIndex {
		t.Fatalf("replacement registry index = %d, want released index %d", replacementRef.inlineIndex, firstRef.inlineIndex)
	}

	replacement.StopPropagation()
	replacement.propagationStopped = false
	if !replacement.PropagationStopped() {
		t.Fatal("active registry lookup did not preserve StopPropagation outside the Event value")
	}
	replacement.StopImmediatePropagation()
	replacement.propagationStopped = false
	replacement.immediatePropagationStopped = false
	if !replacement.ImmediatePropagationStopped() {
		t.Fatal("active registry lookup did not preserve StopImmediatePropagation outside the Event value")
	}
	if !replacement.PropagationStopped() {
		t.Fatal("active registry lookup did not preserve the propagation effect of StopImmediatePropagation")
	}
}

func TestEventDispatchStateRefsRejectInactiveEvents(t *testing.T) {
	event := NewEvent("inactive")
	activeEventDispatches.Lock()
	clean := activeEventDispatches.inlineCount == 0 && activeEventDispatches.overflowCount == 0 &&
		!activeEventDispatches.overflowLarge && len(activeEventDispatches.overflow) == 0
	for i := range activeEventDispatches.inline {
		clean = clean && activeEventDispatches.inline[i] == (eventDispatchState{})
	}
	savedInline := activeEventDispatches.inline
	savedInlineCount := activeEventDispatches.inlineCount
	savedOverflow := activeEventDispatches.overflow
	savedOverflowCount := activeEventDispatches.overflowCount
	savedOverflowLarge := activeEventDispatches.overflowLarge
	activeEventDispatches.Unlock()
	if !clean {
		t.Fatal("active Event dispatch registry was not clean before inactive-ref probes")
	}
	restore := func() {
		activeEventDispatches.Lock()
		delete(savedOverflow, event)
		activeEventDispatches.inline = savedInline
		activeEventDispatches.inlineCount = savedInlineCount
		activeEventDispatches.overflow = savedOverflow
		activeEventDispatches.overflowCount = savedOverflowCount
		activeEventDispatches.overflowLarge = savedOverflowLarge
		activeEventDispatches.Unlock()
	}
	t.Cleanup(restore)

	overflowRef := eventDispatchStateRef{event: event, inlineIndex: -1}
	type probe struct {
		name string
		call func()
	}
	tests := make([]probe, 0, 4)
	if inlineActiveEventDispatchCapacity > 0 {
		inlineRef := eventDispatchStateRef{event: event, inlineIndex: 0}
		tests = append(tests, probe{name: "inline snapshot", call: func() { inlineRef.snapshot() }})
	}
	tests = append(tests, probe{name: "overflow snapshot", call: func() { overflowRef.snapshot() }})
	if inlineActiveEventDispatchCapacity > 0 {
		inlineRef := eventDispatchStateRef{event: event, inlineIndex: 0}
		tests = append(tests, probe{name: "inline merge", call: func() { inlineRef.merge(event) }})
	}
	tests = append(tests, probe{name: "overflow merge", call: func() { overflowRef.merge(event) }})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := abortEventCapturePanic(test.call)
			lockRetained := !activeEventDispatches.TryLock()
			// TryLock owns the idle mutex on success; on failure, the synchronous
			// call retained it. Unlock either path before any assertion can exit.
			activeEventDispatches.Unlock()
			restore()
			if got != "eventloop: inactive Event dispatch" {
				t.Fatalf("inactive state-ref panic = %#v, want exact invariant marker", got)
			}
			if lockRetained {
				t.Fatal("inactive state-ref panic retained the registry lock")
			}
		})
	}
}

func TestBeginEventDispatchRejectsInconsistentInlineRegistry(t *testing.T) {
	probe := NewEvent("inconsistent")
	activeEventDispatches.Lock()
	clean := activeEventDispatches.inlineCount == 0 && activeEventDispatches.overflowCount == 0 &&
		!activeEventDispatches.overflowLarge && len(activeEventDispatches.overflow) == 0
	for i := range activeEventDispatches.inline {
		clean = clean && activeEventDispatches.inline[i] == (eventDispatchState{})
	}
	if !clean {
		activeEventDispatches.Unlock()
		t.Fatal("active Event dispatch registry was not clean before invariant probe")
	}
	savedInline := activeEventDispatches.inline
	savedInlineCount := activeEventDispatches.inlineCount
	savedOverflow := activeEventDispatches.overflow
	savedOverflowCount := activeEventDispatches.overflowCount
	savedOverflowLarge := activeEventDispatches.overflowLarge

	var restoreOnce sync.Once
	restore := func() {
		restoreOnce.Do(func() {
			activeEventDispatches.Lock()
			delete(savedOverflow, probe)
			activeEventDispatches.inline = savedInline
			activeEventDispatches.inlineCount = savedInlineCount
			activeEventDispatches.overflow = savedOverflow
			activeEventDispatches.overflowCount = savedOverflowCount
			activeEventDispatches.overflowLarge = savedOverflowLarge
			activeEventDispatches.Unlock()
		})
	}
	t.Cleanup(restore)

	for i := range activeEventDispatches.inline {
		activeEventDispatches.inline[i] = eventDispatchState{event: NewEvent("occupied")}
	}
	activeEventDispatches.inlineCount = len(activeEventDispatches.inline) - 1
	activeEventDispatches.Unlock()

	accepted := false
	got := abortEventCapturePanic(func() {
		beginEventDispatch(probe)
		accepted = true
	})
	lockRetained := !activeEventDispatches.TryLock()
	// TryLock owns the isolated mutex on success; on failure, begin retained it.
	// Unlock either path before restore attempts to reacquire the mutex.
	activeEventDispatches.Unlock()
	restore()
	if accepted {
		t.Fatal("inconsistent registry unexpectedly accepted a new dispatch")
	}
	if got != "eventloop: active Event dispatch registry is inconsistent" {
		t.Fatalf("inconsistent registry panic = %#v, want exact invariant marker", got)
	}
	if lockRetained {
		t.Fatal("inconsistent registry panic retained the registry lock")
	}

	event := NewEvent("reusable")
	beginEventDispatch(event)
	endEventDispatch(event)
}

func TestEventTargetActiveRegistryOverflowRejectsEverySamePointer(t *testing.T) {
	const activeCount = inlineActiveEventDispatchCapacity + 1
	target := NewEventTarget()
	started := make(chan struct{}, activeCount)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseDispatches := func() { releaseOnce.Do(func() { close(release) }) }
	target.AddEventListener("event", func(*Event) {
		started <- struct{}{}
		<-release
	})

	events := make([]*Event, activeCount)
	var wait sync.WaitGroup
	wait.Add(activeCount)
	for i := range events {
		events[i] = NewEvent("event")
		go func(event *Event) {
			defer wait.Done()
			target.DispatchEvent(event)
		}(events[i])
	}
	done := make(chan struct{})
	go func() {
		wait.Wait()
		close(done)
	}()
	cleanup := func() {
		releaseDispatches()
		waitAbortContractSignal(t, done, "overflow dispatch completion")
	}
	t.Cleanup(cleanup)
	for range activeCount {
		waitAbortContractSignal(t, started, "active overflow dispatch")
	}

	activeEventDispatches.Lock()
	inlineCount := activeEventDispatches.inlineCount
	overflowCount := activeEventDispatches.overflowCount
	activeEventDispatches.Unlock()
	if inlineCount != inlineActiveEventDispatchCapacity || overflowCount != 1 {
		t.Fatalf("active registry counts = inline %d overflow %d, want %d/1", inlineCount, overflowCount, inlineActiveEventDispatchCapacity)
	}
	probeTarget := NewEventTarget()
	for i, event := range events {
		if got := abortEventCapturePanic(func() { probeTarget.DispatchEvent(event) }); got == nil {
			t.Fatalf("active Event %d was accepted recursively", i)
		}
	}

	cleanup()
	for i, event := range events {
		if !target.DispatchEvent(event) {
			t.Fatalf("completed Event %d could not be reused", i)
		}
	}
}

func holdInlineEventDispatches(t *testing.T) func() {
	t.Helper()
	target := NewEventTarget()
	started := make(chan struct{}, inlineActiveEventDispatchCapacity)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseDispatches := func() { releaseOnce.Do(func() { close(release) }) }
	target.AddEventListener("inline", func(*Event) {
		started <- struct{}{}
		<-release
	})

	var wait sync.WaitGroup
	wait.Add(inlineActiveEventDispatchCapacity)
	for range inlineActiveEventDispatchCapacity {
		go func() {
			defer wait.Done()
			target.DispatchEvent(NewEvent("inline"))
		}()
	}
	done := make(chan struct{})
	go func() {
		wait.Wait()
		close(done)
	}()
	cleanup := func() {
		releaseDispatches()
		waitAbortContractSignal(t, done, "inline dispatch completion")
	}
	t.Cleanup(cleanup)
	for range inlineActiveEventDispatchCapacity {
		waitAbortContractSignal(t, started, "inline dispatch admission")
	}
	return cleanup
}

// activeEventDispatchTestLocation reports 1 for inline, 2 for overflow, and 0
// for inactive. It exists only to keep overflow proofs coupled to the current
// implementation capacity instead of a duplicated numeric assumption.
func activeEventDispatchTestLocation(event *Event) int32 {
	activeEventDispatches.Lock()
	defer activeEventDispatches.Unlock()
	for i := range activeEventDispatches.inline {
		if activeEventDispatches.inline[i].event == event {
			return 1
		}
	}
	if _, ok := activeEventDispatches.overflow[event]; ok {
		return 2
	}
	return 0
}
