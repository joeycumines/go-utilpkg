package eventloop

import (
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
	"weak"
)

func TestEventTargetDispatchReusesEventWithFreshState(t *testing.T) {
	target := NewEventTarget()
	regularCalls := 0
	target.AddEventListenerOnce("event", func(event *Event) {
		event.PreventDefault()
		event.StopImmediatePropagation()
	})
	target.AddEventListener("event", func(*Event) {
		regularCalls++
	})
	event := NewEventWithOptions("event", false, true)

	if target.DispatchEvent(event) {
		t.Fatal("first dispatch = true, want canceled")
	}
	if regularCalls != 0 {
		t.Fatalf("regular listener calls after first dispatch = %d, want 0", regularCalls)
	}
	if !target.DispatchEvent(event) {
		t.Fatal("second dispatch = false, want fresh uncanceled state")
	}
	if regularCalls != 1 {
		t.Fatalf("regular listener calls after reuse = %d, want 1", regularCalls)
	}
	if event.DefaultPrevented || event.PropagationStopped() || event.ImmediatePropagationStopped() {
		t.Fatalf("event state was not reset: %+v", event)
	}
}

func TestEventTargetRejectsRecursiveSameEventAndRestoresState(t *testing.T) {
	target := NewEventTarget()
	event := NewEvent("event")
	calls := 0
	id := target.AddEventListener("event", func(got *Event) {
		calls++
		if calls == 1 {
			target.DispatchEvent(got)
		}
	})

	if got := abortEventCapturePanic(func() { target.DispatchEvent(event) }); got == nil {
		t.Fatal("recursive dispatch of the same Event did not panic")
	}
	if !target.RemoveEventListenerByID("event", id) {
		t.Fatal("failed to remove recursive listener")
	}
	if !target.DispatchEvent(event) {
		t.Fatal("event remained stuck in dispatching state after panic")
	}
}

func TestEventTargetAllowsCopiedEventDuringDispatch(t *testing.T) {
	target := NewEventTarget()
	calls := 0
	target.AddEventListener("event", func(event *Event) {
		calls++
		if calls == 1 {
			copied := *event
			if !target.DispatchEvent(&copied) {
				t.Fatal("copied Event dispatch was canceled")
			}
		}
	})
	if !target.DispatchEvent(NewEvent("event")) {
		t.Fatal("original Event dispatch was canceled")
	}
	if calls != 2 {
		t.Fatalf("listener calls = %d, want 2", calls)
	}
}

func TestEventTargetCompletedDispatchCopyEstablishesIndependentIdentity(t *testing.T) {
	target := NewEventTarget()
	original := NewEvent("event")
	if !target.DispatchEvent(original) {
		t.Fatal("original Event dispatch was canceled")
	}
	copied := *original
	calls := 0
	target.AddEventListener("event", func(got *Event) {
		calls++
		if got != &copied {
			t.Fatalf("listener Event = %p, want copied value %p", got, &copied)
		}
	})
	if !target.DispatchEvent(&copied) {
		t.Fatal("completed Event copy dispatch was canceled")
	}
	if calls != 1 {
		t.Fatalf("completed Event copy calls = %d, want 1", calls)
	}
}

func TestEventTargetActiveCopyRejectsRecursiveSameCopy(t *testing.T) {
	target := NewEventTarget()
	var recursivePanic any
	calls := 0
	target.AddEventListener("event", func(event *Event) {
		calls++
		if calls == 1 {
			copied := *event
			target.DispatchEvent(&copied)
			return
		}
		recursivePanic = abortEventCapturePanic(func() { target.DispatchEvent(event) })
	})
	if !target.DispatchEvent(NewEvent("event")) {
		t.Fatal("outer Event dispatch was canceled")
	}
	if calls != 2 {
		t.Fatalf("listener calls = %d, want original and copied Event", calls)
	}
	if recursivePanic == nil {
		t.Fatal("recursive dispatch of the active copied Event did not panic")
	}
}

func TestEventTargetCompletedCopyDispatchIdentityDoesNotRetainSource(t *testing.T) {
	copy, original, laterTarget := newCompletedDispatchCopyPointers()
	assertEventCopyDispatchIdentityReleasesSource(t, copy, original, laterTarget)
}

func TestEventTargetActiveCopyDispatchIdentityDoesNotRetainSource(t *testing.T) {
	copy, original, laterTarget := newActiveDispatchCopyPointers()
	assertEventCopyDispatchIdentityReleasesSource(t, copy, original, laterTarget)
}

func assertEventCopyDispatchIdentityReleasesSource(
	t *testing.T,
	copy Event,
	original weak.Pointer[Event],
	laterTarget weak.Pointer[EventTarget],
) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for original.Value() != nil || laterTarget.Value() != nil {
		runtime.GC()
		runtime.KeepAlive(copy)
		if time.Now().After(deadline) {
			t.Fatalf("event copy dispatch identity retained source: original=%v laterTarget=%v", original.Value() != nil, laterTarget.Value() != nil)
		}
		runtime.Gosched()
	}
	runtime.KeepAlive(copy)
}

func TestEventTargetDispatchKeepsInitialTypeStable(t *testing.T) {
	target := NewEventTarget()
	observed := ""
	target.AddEventListener("event", func(event *Event) {
		event.Type = "mutated"
	})
	target.AddEventListener("event", func(event *Event) {
		observed = event.Type
	})
	event := NewEvent("event")
	target.DispatchEvent(event)
	if observed != "event" {
		t.Fatalf("later listener observed Type %q, want %q", observed, "event")
	}
	if event.Type != "event" {
		t.Fatalf("event Type after dispatch = %q, want %q", event.Type, "event")
	}
}

func TestEventTargetWholeValueOverwriteRestoresOutcomeAcrossExitPaths(t *testing.T) {
	for _, overflow := range []bool{false, true} {
		registryName := "unheld"
		if overflow {
			registryName = "inline-saturated"
		}
		for _, completion := range []string{"return", "panic", "goexit"} {
			t.Run(registryName+"/"+completion, func(t *testing.T) {
				releaseInline := func() {}
				if overflow {
					releaseInline = holdInlineEventDispatches(t)
				}
				defer releaseInline()

				target := NewEventTarget()
				event := &Event{Type: "event", Cancelable: true}
				marker := errors.New("listener panic")
				var location atomic.Int32
				var laterCalls atomic.Int32
				target.AddEventListener("event", func(got *Event) {
					got.PreventDefault()
					got.StopImmediatePropagation()
					*got = Event{Type: "replacement", Cancelable: true}
					location.Store(activeEventDispatchTestLocation(got))
					switch completion {
					case "return":
						return
					case "panic":
						panic(marker)
					case "goexit":
						runtime.Goexit()
					default:
						panic("unknown completion")
					}
				})
				target.AddEventListener("event", func(*Event) { laterCalls.Add(1) })

				switch completion {
				case "return":
					if target.DispatchEvent(event) {
						t.Fatal("whole-value overwrite cleared cancellation")
					}
				case "panic":
					if got := abortEventCapturePanic(func() { target.DispatchEvent(event) }); got != marker {
						t.Fatalf("listener panic = %#v, want %#v", got, marker)
					}
				case "goexit":
					done := make(chan struct{})
					var returned atomic.Bool
					go func() {
						defer close(done)
						target.DispatchEvent(event)
						returned.Store(true)
					}()
					waitAbortContractSignal(t, done, "whole-overwrite listener Goexit")
					if returned.Load() {
						t.Fatal("DispatchEvent returned after listener Goexit")
					}
				}

				wantLocation := int32(1)
				if overflow || inlineActiveEventDispatchCapacity == 0 {
					wantLocation = 2
				}
				if got := location.Load(); got != wantLocation {
					t.Fatalf("active registry location = %d, want %d", got, wantLocation)
				}
				if got := laterCalls.Load(); got != 0 {
					t.Fatalf("later listener calls = %d, want 0", got)
				}
				if event.Type != "event" || event.Target != target || !event.DefaultPrevented || !event.PropagationStopped() || !event.ImmediatePropagationStopped() {
					t.Fatalf("restored event = type %q target %p default %v propagation %v immediate %v", event.Type, event.Target, event.DefaultPrevented, event.PropagationStopped(), event.ImmediatePropagationStopped())
				}
			})
		}
	}
}
