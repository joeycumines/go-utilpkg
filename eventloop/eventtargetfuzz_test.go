package eventloop

import (
	"reflect"
	"slices"
	"testing"
)

type fuzzEventListenerModel struct {
	eventType string
	id        ListenerID
	behavior  byte
	once      bool
}

func FuzzEventTargetDispatchModel(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("click\x00load\x01abort\x02remove"))
	f.Add([]byte{255, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := newFuzzReader(data)
		target := NewEventTarget()
		var actualCalls []ListenerID
		listeners := make(map[ListenerID]fuzzEventListenerModel)
		byType := make(map[string][]ListenerID)
		types := []string{"", "abort", "click", "message", "load"}
		ops := 1 + min(len(data)*3, 1024)
		for range ops {
			eventType := types[r.intn(len(types))]
			if r.bool() {
				eventType = r.smallString(8)
			}

			switch r.byte() % 7 {
			case 0, 1:
				behavior := r.byte()
				once := r.bool()
				var id ListenerID
				recursionAttempt := false
				listener := func(e *Event) {
					actualCalls = append(actualCalls, id)
					if e == nil {
						t.Fatalf("listener received nil event")
					}
					if e.Target != target {
						t.Fatalf("event target = %p, want %p", e.Target, target)
					}
					if behavior&8 != 0 && !recursionAttempt {
						recursionAttempt = true
						saved := *e
						*e = Event{Type: saved.Type}
						panicValue := abortEventCapturePanic(func() { target.DispatchEvent(e) })
						*e = saved
						recursionAttempt = false
						if panicValue == nil {
							t.Fatal("whole-value overwrite bypassed same-pointer recursion rejection")
						}
					}
					if behavior&1 != 0 {
						e.PreventDefault()
					}
					if behavior&2 != 0 {
						e.StopPropagation()
					}
					if behavior&4 != 0 {
						e.StopImmediatePropagation()
					}
				}
				if once {
					id = target.AddEventListenerOnce(eventType, listener)
				} else {
					id = target.AddEventListener(eventType, listener)
				}
				if id == 0 {
					t.Fatalf("non-nil listener returned id 0")
				}
				listeners[id] = fuzzEventListenerModel{eventType: eventType, id: id, behavior: behavior, once: once}
				byType[eventType] = append(byType[eventType], id)

			case 2:
				if len(listeners) == 0 || r.bool() {
					if got := target.AddEventListener(eventType, nil); got != 0 {
						t.Fatalf("AddEventListener(nil) returned %d, want 0", got)
					}
					break
				}
				ids := listenerIDs(listeners)
				id := ids[r.intn(len(ids))]
				lm := listeners[id]
				removeType := lm.eventType
				if r.byte()%4 == 0 {
					removeType += "-wrong"
				}
				removed := target.RemoveEventListenerByID(removeType, id)
				wantRemoved := removeType == lm.eventType
				if removed != wantRemoved {
					t.Fatalf("RemoveEventListenerByID(%q, %d) = %v, want %v", removeType, id, removed, wantRemoved)
				}
				if wantRemoved {
					delete(listeners, id)
					byType[lm.eventType] = removeListenerID(byType[lm.eventType], id)
				}

			case 3:
				if r.bool() || eventType == "" {
					target.RemoveAllEventListeners("")
					listeners = make(map[ListenerID]fuzzEventListenerModel)
					byType = make(map[string][]ListenerID)
				} else {
					target.RemoveAllEventListeners(eventType)
					for _, id := range byType[eventType] {
						delete(listeners, id)
					}
					delete(byType, eventType)
				}

			default:
				cancelable := r.bool()
				bubbles := r.bool()
				event := NewEventWithOptions(eventType, bubbles, cancelable)
				if r.byte()%5 == 0 {
					custom := NewCustomEventWithOptions(eventType, r.smallString(8), bubbles, cancelable)
					event = custom.EventPtr()
					if custom.EventPtr() != &custom.Event {
						t.Fatalf("CustomEvent.EventPtr did not return embedded event")
					}
				}

				expectedCalls := make([]ListenerID, 0, len(byType[eventType]))
				actualCalls = make([]ListenerID, 0, len(expectedCalls))
				expectedDefaultPrevented := false
				expectedPropagationStopped := false
				expectedImmediateStopped := false
				for _, id := range append([]ListenerID(nil), byType[eventType]...) {
					lm, ok := listeners[id]
					if !ok {
						continue
					}
					if expectedImmediateStopped {
						break
					}
					expectedCalls = append(expectedCalls, id)
					if lm.behavior&1 != 0 && cancelable {
						expectedDefaultPrevented = true
					}
					if lm.behavior&2 != 0 {
						expectedPropagationStopped = true
					}
					if lm.behavior&4 != 0 {
						expectedPropagationStopped = true
						expectedImmediateStopped = true
					}
					if lm.once {
						delete(listeners, id)
						byType[eventType] = removeListenerID(byType[eventType], id)
					}
				}

				got := target.DispatchEvent(event)
				if !reflect.DeepEqual(actualCalls, expectedCalls) {
					t.Fatalf("listener calls for %q = %v, want %v", eventType, actualCalls, expectedCalls)
				}
				want := !cancelable || !expectedDefaultPrevented
				if got != want {
					t.Fatalf("DispatchEvent return = %v, want %v", got, want)
				}
				if event.Target != target {
					t.Fatalf("event.Target = %p, want %p", event.Target, target)
				}
				if event.DefaultPrevented != expectedDefaultPrevented {
					t.Fatalf("DefaultPrevented = %v, want %v", event.DefaultPrevented, expectedDefaultPrevented)
				}
				if event.PropagationStopped() != expectedPropagationStopped {
					t.Fatalf("PropagationStopped = %v, want %v", event.PropagationStopped(), expectedPropagationStopped)
				}
				if event.ImmediatePropagationStopped() != expectedImmediateStopped {
					t.Fatalf("ImmediatePropagationStopped = %v, want %v", event.ImmediatePropagationStopped(), expectedImmediateStopped)
				}
			}

			assertEventTargetCounts(t, target, byType)
		}

		if got := target.DispatchEvent(nil); !got {
			t.Fatalf("DispatchEvent(nil) = false, want true")
		}
	})
}

func listenerIDs(m map[ListenerID]fuzzEventListenerModel) []ListenerID {
	ids := make([]ListenerID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func removeListenerID(ids []ListenerID, id ListenerID) []ListenerID {
	for i, candidate := range ids {
		if candidate == id {
			return append(ids[:i], ids[i+1:]...)
		}
	}
	return ids
}

func assertEventTargetCounts(t *testing.T, target *EventTarget, byType map[string][]ListenerID) {
	t.Helper()
	for eventType, ids := range byType {
		if got, want := target.ListenerCount(eventType), len(ids); got != want {
			t.Fatalf("ListenerCount(%q) = %d, want %d", eventType, got, want)
		}
		if got, want := target.HasEventListeners(eventType), len(ids) > 0; got != want {
			t.Fatalf("HasEventListeners(%q) = %v, want %v", eventType, got, want)
		}
	}
}
