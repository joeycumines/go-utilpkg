package eventloop_test

import (
	"testing"

	"github.com/joeycumines/go-eventloop"
)

func TestEventTargetWholeValueOverwriteRejectsSameTargetRecursion(t *testing.T) {
	target := eventloop.NewEventTarget()
	event := eventloop.NewEvent("event")
	calls := 0
	var recursivePanic any
	target.AddEventListener("event", func(got *eventloop.Event) {
		calls++
		if calls != 1 {
			return
		}
		*got = eventloop.Event{Type: "event"}
		recursivePanic = captureEventTargetPanic(func() {
			target.DispatchEvent(got)
		})
	})

	target.DispatchEvent(event)

	if recursivePanic == nil {
		t.Fatal("whole-value overwrite bypassed same-pointer recursive dispatch rejection")
	}
	if calls != 1 {
		t.Fatalf("listener calls = %d, want one outer dispatch", calls)
	}
}

func TestEventTargetWholeValueOverwriteRejectsCrossTargetRecursion(t *testing.T) {
	outer := eventloop.NewEventTarget()
	inner := eventloop.NewEventTarget()
	event := eventloop.NewEvent("event")
	innerCalls := 0
	inner.AddEventListener("event", func(*eventloop.Event) {
		innerCalls++
	})
	var recursivePanic any
	outer.AddEventListener("event", func(got *eventloop.Event) {
		*got = eventloop.Event{Type: "event"}
		recursivePanic = captureEventTargetPanic(func() {
			inner.DispatchEvent(got)
		})
	})

	outer.DispatchEvent(event)

	if recursivePanic == nil {
		t.Fatal("whole-value overwrite bypassed cross-target same-pointer dispatch rejection")
	}
	if innerCalls != 0 {
		t.Fatalf("inner listener calls = %d, want 0", innerCalls)
	}
}

func TestEventTargetWholeValueOverwritePreservesDispatchOutcome(t *testing.T) {
	target := eventloop.NewEventTarget()
	event := &eventloop.Event{Type: "event", Cancelable: true}
	laterCalls := 0
	target.AddEventListener("event", func(got *eventloop.Event) {
		got.StopImmediatePropagation()
		got.PreventDefault()
		*got = eventloop.Event{Type: "replacement", Cancelable: true}
	})
	target.AddEventListener("event", func(*eventloop.Event) {
		laterCalls++
	})

	if target.DispatchEvent(event) {
		t.Fatal("whole-value overwrite cleared the canceled dispatch outcome")
	}
	if laterCalls != 0 {
		t.Fatalf("later listener calls = %d, want 0 after StopImmediatePropagation", laterCalls)
	}
	if !event.DefaultPrevented {
		t.Fatal("whole-value overwrite cleared DefaultPrevented")
	}
	if !event.PropagationStopped() || !event.ImmediatePropagationStopped() {
		t.Fatalf("propagation state = (%v, %v), want true/true", event.PropagationStopped(), event.ImmediatePropagationStopped())
	}
	if event.Type != "event" || event.Target != target {
		t.Fatalf("dispatch identity = (%q, %p), want (%q, %p)", event.Type, event.Target, "event", target)
	}
}

func TestEventCopyRetainsOrdinaryFields(t *testing.T) {
	target := eventloop.NewEventTarget()
	payload := &struct{ value string }{value: "detail"}
	custom := eventloop.NewCustomEvent("event", payload)
	target.DispatchEvent(custom.EventPtr())
	copied := custom.Event

	if copied.Target != target {
		t.Fatalf("copied Target = %p, want original target %p", copied.Target, target)
	}
	if copied.Detail() != payload {
		t.Fatalf("copied Detail = %#v, want original payload %#v", copied.Detail(), payload)
	}
}

func captureEventTargetPanic(fn func()) (value any) {
	defer func() {
		value = recover()
	}()
	fn()
	return nil
}
