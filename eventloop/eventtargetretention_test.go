package eventloop

import (
	"errors"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"weak"
)

func TestEventTargetDispatchRegistryReleasesCompletedEvents(t *testing.T) {
	for _, completion := range []string{"return", "panic", "goexit"} {
		t.Run(completion, func(t *testing.T) {
			target, pointer := newCompletedDispatchPointer(t, completion)
			waitContractCollected(t, pointer, target)
		})
	}
}

func TestEventTargetDispatchRegistryOverflowReleasesCompletedEvents(t *testing.T) {
	target, pointers := newCompletedOverflowDispatchPointers(t, inlineActiveEventDispatchCapacity+1)
	activeEventDispatches.Lock()
	if activeEventDispatches.inlineCount != 0 || activeEventDispatches.overflowCount != 0 || activeEventDispatches.overflowLarge || len(activeEventDispatches.overflow) != 0 {
		activeEventDispatches.Unlock()
		t.Fatal("completed overflow dispatch retained active identities")
	}
	activeEventDispatches.Unlock()
	for i, pointer := range pointers {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			waitContractCollected(t, pointer, target)
		})
	}
}

func TestEventTargetDispatchRegistryReleasesLargeHighWaterMap(t *testing.T) {
	const eventCount = inlineActiveEventDispatchCapacity + retainedOverflowEventDispatchCapacity + 1
	target, _ := newCompletedOverflowDispatchPointers(t, eventCount)
	activeEventDispatches.Lock()
	if activeEventDispatches.inlineCount != 0 || activeEventDispatches.overflowCount != 0 || activeEventDispatches.overflowLarge || activeEventDispatches.overflow != nil {
		activeEventDispatches.Unlock()
		t.Fatal("completed large overflow dispatch retained identities or high-water map capacity")
	}
	activeEventDispatches.Unlock()
	runtime.KeepAlive(target)
}

func TestEventTargetDispatchRegistryOverflowAbnormalExitReleasesEvent(t *testing.T) {
	for _, completion := range []string{"panic", "goexit"} {
		t.Run(completion, func(t *testing.T) {
			target, pointer := newCompletedOverflowAbnormalDispatchPointer(t, completion)
			waitContractCollected(t, pointer, target)
		})
	}
}

func newCompletedDispatchPointer(t *testing.T, completion string) (*EventTarget, weak.Pointer[Event]) {
	t.Helper()
	target := NewEventTarget()
	switch completion {
	case "panic":
		target.AddEventListener("event", func(*Event) { panic("listener panic") })
	case "goexit":
		target.AddEventListener("event", func(*Event) { runtime.Goexit() })
	}
	event := NewEvent("event")
	pointer := weak.Make(event)
	switch completion {
	case "return":
		target.DispatchEvent(event)
	case "panic":
		abortEventCapturePanic(func() { target.DispatchEvent(event) })
	case "goexit":
		done := make(chan struct{})
		go func() {
			defer close(done)
			target.DispatchEvent(event)
		}()
		waitAbortContractSignal(t, done, "EventTarget Goexit completion")
	default:
		panic("unknown dispatch completion")
	}
	runtime.KeepAlive(event)
	return target, pointer
}

func newCompletedOverflowDispatchPointers(t *testing.T, eventCount int) (*EventTarget, []weak.Pointer[Event]) {
	t.Helper()
	target := NewEventTarget()
	started := make(chan struct{}, eventCount)
	release := make(chan struct{})
	releaseDispatches := abortContractRelease(t, release)
	target.AddEventListener("event", func(*Event) {
		started <- struct{}{}
		<-release
	})
	events := make([]*Event, eventCount)
	pointers := make([]weak.Pointer[Event], eventCount)
	var wait sync.WaitGroup
	wait.Add(eventCount)
	for i := range events {
		events[i] = NewEvent("event")
		pointers[i] = weak.Make(events[i])
		go func(event *Event) {
			defer wait.Done()
			target.DispatchEvent(event)
		}(events[i])
	}
	for range eventCount {
		waitAbortContractSignal(t, started, "EventTarget overflow dispatch")
	}
	releaseDispatches()
	done := make(chan struct{})
	go func() {
		wait.Wait()
		close(done)
	}()
	waitAbortContractSignal(t, done, "EventTarget overflow completion")
	runtime.KeepAlive(events)
	return target, pointers
}

func newCompletedOverflowAbnormalDispatchPointer(t *testing.T, completion string) (*EventTarget, weak.Pointer[Event]) {
	t.Helper()
	target := NewEventTarget()
	started := make(chan struct{}, inlineActiveEventDispatchCapacity)
	release := make(chan struct{})
	releaseDispatches := abortContractRelease(t, release)
	target.AddEventListener("block", func(*Event) {
		started <- struct{}{}
		<-release
	})
	var wait sync.WaitGroup
	wait.Add(inlineActiveEventDispatchCapacity)
	for range inlineActiveEventDispatchCapacity {
		go func() {
			defer wait.Done()
			target.DispatchEvent(NewEvent("block"))
		}()
	}
	for range inlineActiveEventDispatchCapacity {
		waitAbortContractSignal(t, started, "EventTarget inline dispatch")
	}

	marker := errors.New("overflow listener panic")
	switch completion {
	case "panic":
		target.AddEventListener("abnormal", func(*Event) { panic(marker) })
	case "goexit":
		target.AddEventListener("abnormal", func(*Event) { runtime.Goexit() })
	default:
		t.Fatalf("unknown completion %q", completion)
	}
	event := NewEvent("abnormal")
	pointer := weak.Make(event)
	if completion == "panic" {
		if got := abortEventCapturePanic(func() { target.DispatchEvent(event) }); got != marker {
			t.Fatalf("overflow listener panic = %#v, want %#v", got, marker)
		}
	} else {
		done := make(chan struct{})
		go func() {
			defer close(done)
			target.DispatchEvent(event)
		}()
		waitAbortContractSignal(t, done, "EventTarget overflow Goexit")
	}

	activeEventDispatches.Lock()
	_, retained := activeEventDispatches.overflow[event]
	stateOK := activeEventDispatches.inlineCount == inlineActiveEventDispatchCapacity &&
		activeEventDispatches.overflowCount == 0 && !retained
	activeEventDispatches.Unlock()
	if !stateOK {
		t.Fatal("abnormal overflow dispatch retained its active identity")
	}
	target.RemoveAllEventListeners("abnormal")
	if !target.DispatchEvent(event) {
		t.Fatal("event could not be reused after abnormal overflow dispatch")
	}

	releaseDispatches()
	done := make(chan struct{})
	go func() {
		wait.Wait()
		close(done)
	}()
	waitAbortContractSignal(t, done, "EventTarget inline dispatch completion")
	runtime.KeepAlive(event)
	return target, pointer
}

func newCompletedDispatchCopyPointers() (
	Event,
	weak.Pointer[Event],
	weak.Pointer[EventTarget],
) {
	original := NewEvent("event")
	firstTarget := NewEventTarget()
	firstTarget.DispatchEvent(original)
	copy := *original
	laterTarget := NewEventTarget()
	laterTarget.DispatchEvent(original)
	originalPointer := weak.Make(original)
	targetPointer := weak.Make(laterTarget)
	runtime.KeepAlive(original)
	runtime.KeepAlive(laterTarget)
	return copy, originalPointer, targetPointer
}

func newActiveDispatchCopyPointers() (
	Event,
	weak.Pointer[Event],
	weak.Pointer[EventTarget],
) {
	original := NewEvent("event")
	firstTarget := NewEventTarget()
	var copy Event
	firstTarget.AddEventListenerOnce("event", func(event *Event) {
		copy = *event
	})
	firstTarget.DispatchEvent(original)
	laterTarget := NewEventTarget()
	laterTarget.DispatchEvent(original)
	originalPointer := weak.Make(original)
	targetPointer := weak.Make(laterTarget)
	runtime.KeepAlive(original)
	runtime.KeepAlive(laterTarget)
	return copy, originalPointer, targetPointer
}
