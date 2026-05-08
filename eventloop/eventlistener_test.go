package eventloop

import (
	"errors"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
	"weak"
)

func TestEventTargetRemovalSuppressesUnclaimedListener(t *testing.T) {
	target := NewEventTarget()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseFirstNow := abortContractRelease(t, releaseFirst)
	secondCalled := atomic.Bool{}
	target.AddEventListener("event", func(*Event) {
		close(firstStarted)
		<-releaseFirst
	})
	secondID := target.AddEventListener("event", func(*Event) {
		secondCalled.Store(true)
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		target.DispatchEvent(NewEvent("event"))
	}()
	waitAbortContractSignal(t, firstStarted, "first listener start before removal")
	if !target.RemoveEventListenerByID("event", secondID) {
		t.Fatal("RemoveEventListenerByID did not remove pending listener")
	}
	releaseFirstNow()
	waitAbortContractSignal(t, done, "dispatch after pending-listener removal")
	if secondCalled.Load() {
		t.Fatal("removed listener started after removal returned")
	}
}

func TestEventTargetRemoveAllSuppressesUnclaimedListeners(t *testing.T) {
	target := NewEventTarget()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseFirstNow := abortContractRelease(t, releaseFirst)
	secondCalled := atomic.Bool{}
	target.AddEventListener("event", func(*Event) {
		close(firstStarted)
		<-releaseFirst
	})
	target.AddEventListener("event", func(*Event) {
		secondCalled.Store(true)
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		target.DispatchEvent(NewEvent("event"))
	}()
	waitAbortContractSignal(t, firstStarted, "first listener start before remove-all")
	target.RemoveAllEventListeners("event")
	releaseFirstNow()
	waitAbortContractSignal(t, done, "dispatch after remove-all")
	if secondCalled.Load() {
		t.Fatal("remove-all listener started after removal returned")
	}
}

func TestEventTargetRemovalAfterClaimAllowsCurrentCallbackOnly(t *testing.T) {
	target := NewEventTarget()
	started := make(chan struct{})
	release := make(chan struct{})
	releaseNow := abortContractRelease(t, release)
	done := make(chan struct{})
	var calls atomic.Int32
	id := target.AddEventListener("event", func(*Event) {
		calls.Add(1)
		close(started)
		<-release
	})
	go func() {
		defer close(done)
		target.DispatchEvent(NewEvent("event"))
	}()
	waitAbortContractSignal(t, started, "claimed listener start")
	if !target.RemoveEventListenerByID("event", id) {
		t.Fatal("RemoveEventListenerByID did not remove claimed listener")
	}
	releaseNow()
	waitAbortContractSignal(t, done, "claimed-listener dispatch completion")
	target.DispatchEvent(NewEvent("event"))
	if got := calls.Load(); got != 1 {
		t.Fatalf("listener calls = %d, want current claimed callback only", got)
	}
}

func TestEventTargetOnceClaimIsConcurrent(t *testing.T) {
	target := NewEventTarget()
	invoked := make(chan struct{}, 2)
	release := make(chan struct{})
	releaseNow := abortContractRelease(t, release)
	var calls atomic.Int32
	target.AddEventListenerOnce("event", func(*Event) {
		calls.Add(1)
		invoked <- struct{}{}
		<-release
	})

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		target.DispatchEvent(NewEvent("event"))
	}()
	waitAbortContractSignal(t, invoked, "first once-listener invocation")

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		target.DispatchEvent(NewEvent("event"))
	}()

	duplicate := false
	select {
	case <-invoked:
		duplicate = true
	case <-secondDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for concurrent once-listener claim")
	}
	releaseNow()
	waitAbortContractSignal(t, firstDone, "first once dispatch completion")
	waitAbortContractSignal(t, secondDone, "second once dispatch completion")
	if duplicate || calls.Load() != 1 {
		t.Fatalf("once listener calls = %d, want 1", calls.Load())
	}
}

func TestEventTargetOnceClaimPrecedesRecursiveDispatch(t *testing.T) {
	target := NewEventTarget()
	calls := 0
	target.AddEventListenerOnce("event", func(*Event) {
		calls++
		if calls == 1 {
			target.DispatchEvent(NewEvent("event"))
		}
	})
	target.DispatchEvent(NewEvent("event"))
	if calls != 1 {
		t.Fatalf("once listener calls = %d, want 1", calls)
	}
}

func TestEventTargetOnceCleanupSurvivesPanic(t *testing.T) {
	target := NewEventTarget()
	marker := errors.New("listener panic")
	target.AddEventListenerOnce("event", func(*Event) {
		panic(marker)
	})

	if got := abortEventCapturePanic(func() { target.DispatchEvent(NewEvent("event")) }); got != marker {
		t.Fatalf("panic = %#v, want %#v", got, marker)
	}
	if got := target.ListenerCount("event"); got != 0 {
		t.Fatalf("listener count after panic = %d, want 0", got)
	}
	if got := abortEventCapturePanic(func() { target.DispatchEvent(NewEvent("event")) }); got != nil {
		t.Fatalf("second dispatch panic = %#v, want nil", got)
	}
}

func TestEventTargetOnceGoexitRestoresDispatchState(t *testing.T) {
	target := NewEventTarget()
	event := NewEvent("event")
	target.AddEventListenerOnce("event", func(*Event) {
		runtime.Goexit()
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		target.DispatchEvent(event)
	}()
	waitAbortContractSignal(t, done, "EventTarget once-listener Goexit completion")
	called := false
	target.AddEventListenerOnce("event", func(*Event) { called = true })
	if !target.DispatchEvent(event) {
		t.Fatal("reused event was canceled after Goexit")
	}
	if !called {
		t.Fatal("reused event did not reach a later listener after Goexit")
	}
}

func TestEventTargetRemovalReleasesListenerCapture(t *testing.T) {
	target, pointer := newRemovedListenerPayload()
	waitContractCollected(t, pointer, target)
}

func TestEventTargetOnceClaimReleasesListenerCapture(t *testing.T) {
	target, pointer := newOnceListenerPayload()
	waitContractCollected(t, pointer, target)
}

func TestEventTargetRemoveAllReleasesListenerCaptures(t *testing.T) {
	for _, eventType := range []string{"event", ""} {
		name := "typed"
		if eventType == "" {
			name = "global"
		}
		t.Run(name, func(t *testing.T) {
			target, pointer := newRemoveAllListenerPayload(eventType)
			waitContractCollected(t, pointer, target)
		})
	}
}

func TestEventTargetListenerIDWrapReusesReleasedID(t *testing.T) {
	for _, once := range []bool{false, true} {
		name := "ordinary"
		if once {
			name = "once"
		}
		t.Run(name, func(t *testing.T) {
			target := NewEventTarget()
			id := target.AddEventListener("event", func(*Event) {})
			if !target.RemoveEventListenerByID("event", id) {
				t.Fatal("failed to release initial listener ID")
			}
			target.nextListenerID = 0
			var got ListenerID
			if once {
				got = target.AddEventListenerOnce("event", func(*Event) {})
			} else {
				got = target.AddEventListener("event", func(*Event) {})
			}
			if got != id {
				t.Fatalf("listener ID after wrap = %d, want released ID %d", got, id)
			}
		})
	}
}

func TestEventTargetListenerIDWrapSkipsLiveIDsAcrossTypes(t *testing.T) {
	target := NewEventTarget()
	firstID := target.AddEventListener("first", func(*Event) {})
	secondID := target.AddEventListener("second", func(*Event) {})
	thirdID := target.AddEventListener("third", func(*Event) {})
	if !target.RemoveEventListenerByID("second", secondID) {
		t.Fatal("failed to release the second listener ID")
	}
	target.nextListenerID = 0
	reusedID := target.AddEventListener("fourth", func(*Event) {})
	if reusedID != secondID {
		t.Fatalf("wrapped listener ID = %d, want released ID %d", reusedID, secondID)
	}
	nextID := target.AddEventListener("fifth", func(*Event) {})
	if nextID == firstID || nextID == reusedID || nextID == thirdID {
		t.Fatalf("wrapped allocation collided with live IDs: first=%d reused=%d third=%d next=%d", firstID, reusedID, thirdID, nextID)
	}
}

func TestEventTargetListenerIDMaxUint64Transition(t *testing.T) {
	target := NewEventTarget()
	maxID := ^ListenerID(0)
	target.nextListenerID = maxID
	if got := target.AddEventListener("maximum", func(*Event) {}); got != maxID {
		t.Fatalf("maximum listener ID = %d, want %d", got, maxID)
	}
	if got := target.AddEventListener("wrapped", func(*Event) {}); got != 1 {
		t.Fatalf("listener ID after MaxUint64 = %d, want 1", got)
	}
}

func TestEventTargetWrappedListenerIDTerminalScanPanics(t *testing.T) {
	maxID := ^ListenerID(0)
	target := NewEventTarget()
	target.listeners = map[string][]*listenerEntry{
		"live": {{id: maxID, active: true}},
	}
	if got := abortEventCapturePanic(func() {
		target.allocateWrappedListenerIDLocked(maxID)
	}); got != "eventloop: EventTarget listener IDs exhausted" {
		t.Fatalf("exhaustion panic = %#v, want exact exhaustion marker", got)
	}
}

func TestEventTargetClaimListenerDefensiveInputs(t *testing.T) {
	target := NewEventTarget()
	if got := target.claimListener("event", nil); got != nil {
		t.Fatalf("nil listener claim = %p, want nil", got)
	}

	entry := &listenerEntry{
		id:       1,
		listener: func(*Event) {},
		once:     true,
		active:   true,
	}
	if got := target.claimListener("event", entry); got != nil {
		t.Fatalf("detached once-listener claim = %p, want nil", got)
	}
	if entry.active || entry.listener != nil {
		t.Fatal("detached once-listener fallback did not deactivate and release the callback")
	}
}

func TestEventTargetListenerIDWrapReusesOnceClaim(t *testing.T) {
	target := NewEventTarget()
	target.AddEventListener("live", func(*Event) {})
	onceID := target.AddEventListenerOnce("once", func(*Event) {})
	target.DispatchEvent(NewEvent("once"))
	target.nextListenerID = 0
	if got := target.AddEventListener("reused", func(*Event) {}); got != onceID {
		t.Fatalf("wrapped listener ID = %d, want claimed once ID %d", got, onceID)
	}
}

func TestEventTargetListenerIDsRemainDistinctAfterWrap(t *testing.T) {
	target := NewEventTarget()
	live := make(map[ListenerID]struct{}, 1000)
	for i := range 1000 {
		id := target.AddEventListener("live-"+strconv.Itoa(i%7), func(*Event) {})
		if _, exists := live[id]; exists {
			t.Fatalf("duplicate monotonic listener ID %d", id)
		}
		live[id] = struct{}{}
	}
	for id := ListenerID(2); id <= 1000; id += 2 {
		eventType := "live-" + strconv.Itoa((int(id)-1)%7)
		if !target.RemoveEventListenerByID(eventType, id) {
			t.Fatalf("failed to release listener ID %d", id)
		}
		delete(live, id)
	}
	target.nextListenerID = 0
	for range 500 {
		id := target.AddEventListener("wrapped", func(*Event) {})
		if _, exists := live[id]; exists {
			t.Fatalf("wrapped allocation reused live listener ID %d", id)
		}
		live[id] = struct{}{}
	}
	if len(live) != 1000 {
		t.Fatalf("simultaneously live listener IDs = %d, want 1000", len(live))
	}
}

func TestEventTargetZeroValueIsUsable(t *testing.T) {
	var target EventTarget
	target.RemoveAllEventListeners("")
	called := false
	id := target.AddEventListener("event", func(*Event) {
		called = true
	})
	if id == 0 {
		t.Fatal("zero-value EventTarget returned listener ID 0")
	}
	target.DispatchEvent(NewEvent("event"))
	if !called {
		t.Fatal("zero-value EventTarget did not dispatch")
	}
}

func newRemovedListenerPayload() (*EventTarget, weak.Pointer[contractRetentionPayload]) {
	payload := &contractRetentionPayload{value: 1}
	pointer := weak.Make(payload)
	target := NewEventTarget()
	id := target.AddEventListener("event", func(*Event) {
		payload.value++
	})
	target.RemoveEventListenerByID("event", id)
	runtime.KeepAlive(payload)
	return target, pointer
}

func newOnceListenerPayload() (*EventTarget, weak.Pointer[contractRetentionPayload]) {
	payload := &contractRetentionPayload{value: 1}
	pointer := weak.Make(payload)
	target := NewEventTarget()
	target.AddEventListenerOnce("event", func(*Event) {
		payload.value++
	})
	target.DispatchEvent(NewEvent("event"))
	runtime.KeepAlive(payload)
	return target, pointer
}

func newRemoveAllListenerPayload(eventType string) (*EventTarget, weak.Pointer[contractRetentionPayload]) {
	payload := &contractRetentionPayload{value: 1}
	pointer := weak.Make(payload)
	target := NewEventTarget()
	target.AddEventListener("event", func(*Event) {
		payload.value++
	})
	target.RemoveAllEventListeners(eventType)
	runtime.KeepAlive(payload)
	return target, pointer
}
