//go:build linux || darwin

package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// EventTarget Tests (EXPAND-027)
// ============================================================================

func TestEventTarget_NewEventTarget(t *testing.T) {
	target := NewEventTarget()
	if target == nil {
		t.Fatal("NewEventTarget returned nil")
	}
	if target.listeners == nil {
		t.Error("listeners map should be initialized")
	}
	if target.nextListenerID != 1 {
		t.Errorf("nextListenerID should be 1, got %d", target.nextListenerID)
	}
}

func TestEventTarget_AddEventListener_Basic(t *testing.T) {
	target := NewEventTarget()
	called := false

	id := target.AddEventListener("click", func(e *Event) {
		called = true
	})

	if id == 0 {
		t.Error("AddEventListener should return non-zero ID")
	}

	event := &Event{Type: "click"}
	target.DispatchEvent(event)

	if !called {
		t.Error("Listener was not called")
	}
}

func TestEventTarget_AddEventListener_NilListener(t *testing.T) {
	target := NewEventTarget()
	id := target.AddEventListener("click", nil)

	if id != 0 {
		t.Error("AddEventListener with nil should return 0")
	}
}

func TestEventTarget_AddEventListener_MultipleListeners(t *testing.T) {
	target := NewEventTarget()
	var order []int

	target.AddEventListener("test", func(e *Event) {
		order = append(order, 1)
	})
	target.AddEventListener("test", func(e *Event) {
		order = append(order, 2)
	})
	target.AddEventListener("test", func(e *Event) {
		order = append(order, 3)
	})

	target.DispatchEvent(&Event{Type: "test"})

	if len(order) != 3 {
		t.Fatalf("Expected 3 calls, got %d", len(order))
	}
	for i, v := range order {
		if v != i+1 {
			t.Errorf("Expected order[%d]=%d, got %d", i, i+1, v)
		}
	}
}

func TestEventTarget_AddEventListener_DifferentTypes(t *testing.T) {
	target := NewEventTarget()
	clicks := 0
	hovers := 0

	target.AddEventListener("click", func(e *Event) {
		clicks++
	})
	target.AddEventListener("hover", func(e *Event) {
		hovers++
	})

	target.DispatchEvent(&Event{Type: "click"})
	target.DispatchEvent(&Event{Type: "click"})
	target.DispatchEvent(&Event{Type: "hover"})

	if clicks != 2 {
		t.Errorf("Expected 2 clicks, got %d", clicks)
	}
	if hovers != 1 {
		t.Errorf("Expected 1 hover, got %d", hovers)
	}
}

func TestEventTarget_RemoveEventListenerByID(t *testing.T) {
	target := NewEventTarget()
	called := false

	id := target.AddEventListener("click", func(e *Event) {
		called = true
	})

	removed := target.RemoveEventListenerByID("click", id)
	if !removed {
		t.Error("RemoveEventListenerByID should return true")
	}

	target.DispatchEvent(&Event{Type: "click"})

	if called {
		t.Error("Listener should not be called after removal")
	}
}

func TestEventTarget_RemoveEventListenerByID_WrongType(t *testing.T) {
	target := NewEventTarget()

	id := target.AddEventListener("click", func(e *Event) {})

	// Try to remove from wrong event type
	removed := target.RemoveEventListenerByID("hover", id)
	if removed {
		t.Error("RemoveEventListenerByID should return false for wrong type")
	}
}

func TestEventTarget_RemoveEventListenerByID_InvalidID(t *testing.T) {
	target := NewEventTarget()

	target.AddEventListener("click", func(e *Event) {})

	// Try to remove non-existent ID
	removed := target.RemoveEventListenerByID("click", 9999)
	if removed {
		t.Error("RemoveEventListenerByID should return false for invalid ID")
	}
}

func TestEventTarget_DispatchEvent_NilEvent(t *testing.T) {
	target := NewEventTarget()
	called := false

	target.AddEventListener("click", func(e *Event) {
		called = true
	})

	result := target.DispatchEvent(nil)
	if !result {
		t.Error("DispatchEvent(nil) should return true")
	}
	if called {
		t.Error("Listener should not be called for nil event")
	}
}

func TestEventTarget_DispatchEvent_SetsTarget(t *testing.T) {
	target := NewEventTarget()
	var receivedTarget *EventTarget

	target.AddEventListener("test", func(e *Event) {
		receivedTarget = e.Target
	})

	event := &Event{Type: "test"}
	target.DispatchEvent(event)

	if receivedTarget != target {
		t.Error("Event target should be set to dispatching EventTarget")
	}
	if event.Target != target {
		t.Error("Event.Target should be set")
	}
}

func TestEventTarget_DispatchEvent_NoListeners(t *testing.T) {
	target := NewEventTarget()
	event := &Event{Type: "unknown"}
	result := target.DispatchEvent(event)

	if !result {
		t.Error("DispatchEvent with no listeners should return true")
	}
}

func TestEventTarget_HasEventListeners(t *testing.T) {
	target := NewEventTarget()

	if target.HasEventListeners("click") {
		t.Error("Should not have listeners initially")
	}

	id := target.AddEventListener("click", func(e *Event) {})

	if !target.HasEventListeners("click") {
		t.Error("Should have listeners after adding")
	}

	target.RemoveEventListenerByID("click", id)

	if target.HasEventListeners("click") {
		t.Error("Should not have listeners after removal")
	}
}

func TestEventTarget_ListenerCount(t *testing.T) {
	target := NewEventTarget()

	if target.ListenerCount("click") != 0 {
		t.Error("Count should be 0 initially")
	}

	id1 := target.AddEventListener("click", func(e *Event) {})
	if target.ListenerCount("click") != 1 {
		t.Error("Count should be 1")
	}

	id2 := target.AddEventListener("click", func(e *Event) {})
	if target.ListenerCount("click") != 2 {
		t.Error("Count should be 2")
	}

	target.RemoveEventListenerByID("click", id1)
	if target.ListenerCount("click") != 1 {
		t.Error("Count should be 1 after removal")
	}

	target.RemoveEventListenerByID("click", id2)
	if target.ListenerCount("click") != 0 {
		t.Error("Count should be 0 after removing all")
	}
}

func TestEventTarget_RemoveAllEventListeners_SingleType(t *testing.T) {
	target := NewEventTarget()

	target.AddEventListener("click", func(e *Event) {})
	target.AddEventListener("click", func(e *Event) {})
	target.AddEventListener("hover", func(e *Event) {})

	target.RemoveAllEventListeners("click")

	if target.HasEventListeners("click") {
		t.Error("Should not have click listeners after removal")
	}
	if !target.HasEventListeners("hover") {
		t.Error("Should still have hover listeners")
	}
}

func TestEventTarget_RemoveAllEventListeners_AllTypes(t *testing.T) {
	target := NewEventTarget()

	target.AddEventListener("click", func(e *Event) {})
	target.AddEventListener("hover", func(e *Event) {})
	target.AddEventListener("keypress", func(e *Event) {})

	target.RemoveAllEventListeners("")

	if target.HasEventListeners("click") ||
		target.HasEventListeners("hover") ||
		target.HasEventListeners("keypress") {
		t.Error("Should not have any listeners after RemoveAllEventListeners")
	}
}

func TestEventTarget_AddEventListenerOnce(t *testing.T) {
	target := NewEventTarget()
	callCount := 0

	target.AddEventListenerOnce("click", func(e *Event) {
		callCount++
	})

	target.DispatchEvent(&Event{Type: "click"})
	target.DispatchEvent(&Event{Type: "click"})
	target.DispatchEvent(&Event{Type: "click"})

	if callCount != 1 {
		t.Errorf("Once listener should be called exactly once, got %d", callCount)
	}

	if target.HasEventListeners("click") {
		t.Error("Once listener should be removed after dispatch")
	}
}

func TestEventTarget_ConcurrentAccess(t *testing.T) {
	target := NewEventTarget()
	var wg sync.WaitGroup
	var count atomic.Int32

	// Add listeners concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			target.AddEventListener("test", func(e *Event) {
				count.Add(1)
			})
		}()
	}
	wg.Wait()

	if target.ListenerCount("test") != 10 {
		t.Errorf("Expected 10 listeners, got %d", target.ListenerCount("test"))
	}

	// Dispatch event
	target.DispatchEvent(&Event{Type: "test"})

	if count.Load() != 10 {
		t.Errorf("Expected all 10 listeners called, got %d", count.Load())
	}
}

// ============================================================================
// Event Tests (EXPAND-027)
// ============================================================================

func TestEvent_NewEvent(t *testing.T) {
	event := NewEvent("click")
	if event.Type != "click" {
		t.Errorf("Expected type 'click', got '%s'", event.Type)
	}
	if event.Bubbles {
		t.Error("Bubbles should be false by default")
	}
	if event.Cancelable {
		t.Error("Cancelable should be false by default")
	}
}

func TestEvent_NewEventWithOptions(t *testing.T) {
	event := NewEventWithOptions("submit", true, true)
	if event.Type != "submit" {
		t.Errorf("Expected type 'submit', got '%s'", event.Type)
	}
	if !event.Bubbles {
		t.Error("Bubbles should be true")
	}
	if !event.Cancelable {
		t.Error("Cancelable should be true")
	}
}

func TestEvent_PreventDefault_Cancelable(t *testing.T) {
	event := NewEventWithOptions("submit", false, true)
	if event.DefaultPrevented {
		t.Error("DefaultPrevented should be false initially")
	}

	event.PreventDefault()

	if !event.DefaultPrevented {
		t.Error("DefaultPrevented should be true after PreventDefault")
	}
}

func TestEvent_PreventDefault_NotCancelable(t *testing.T) {
	event := NewEvent("load")
	event.PreventDefault()

	if event.DefaultPrevented {
		t.Error("PreventDefault should have no effect on non-cancelable event")
	}
}

func TestEvent_StopPropagation(t *testing.T) {
	event := NewEvent("click")
	if event.IsPropagationStopped() {
		t.Error("Propagation should not be stopped initially")
	}

	event.StopPropagation()

	if !event.IsPropagationStopped() {
		t.Error("Propagation should be stopped after StopPropagation")
	}
	if event.IsImmediatePropagationStopped() {
		t.Error("Immediate propagation should not be stopped by StopPropagation")
	}
}

func TestEvent_StopImmediatePropagation(t *testing.T) {
	event := NewEvent("click")

	event.StopImmediatePropagation()

	if !event.IsPropagationStopped() {
		t.Error("Propagation should be stopped by StopImmediatePropagation")
	}
	if !event.IsImmediatePropagationStopped() {
		t.Error("Immediate propagation should be stopped")
	}
}

func TestEvent_StopImmediatePropagation_InDispatch(t *testing.T) {
	target := NewEventTarget()
	order := []int{}

	target.AddEventListener("test", func(e *Event) {
		order = append(order, 1)
		e.StopImmediatePropagation()
	})
	target.AddEventListener("test", func(e *Event) {
		order = append(order, 2) // Should not be called
	})
	target.AddEventListener("test", func(e *Event) {
		order = append(order, 3) // Should not be called
	})

	target.DispatchEvent(&Event{Type: "test"})

	if len(order) != 1 || order[0] != 1 {
		t.Errorf("Only first listener should be called, got order: %v", order)
	}
}

func TestEvent_DispatchEvent_ReturnValue_Cancelable(t *testing.T) {
	target := NewEventTarget()

	target.AddEventListener("submit", func(e *Event) {
		e.PreventDefault()
	})

	event := NewEventWithOptions("submit", false, true)
	result := target.DispatchEvent(event)

	if result {
		t.Error("DispatchEvent should return false when DefaultPrevented on cancelable event")
	}
}

func TestEvent_DispatchEvent_ReturnValue_NotCancelable(t *testing.T) {
	target := NewEventTarget()

	target.AddEventListener("load", func(e *Event) {
		e.PreventDefault() // Should have no effect
	})

	event := NewEvent("load")
	result := target.DispatchEvent(event)

	if !result {
		t.Error("DispatchEvent should return true for non-cancelable event")
	}
}

// ============================================================================
// CustomEvent Tests (EXPAND-028)
// ============================================================================

func TestCustomEvent_NewCustomEvent(t *testing.T) {
	detail := map[string]any{"key": "value", "count": 42}
	event := NewCustomEvent("custom", detail)

	if event.Type != "custom" {
		t.Errorf("Expected type 'custom', got '%s'", event.Type)
	}
	// Map comparison - compare individual values
	receivedDetail, ok := event.Detail().(map[string]any)
	if !ok {
		t.Error("Detail not set correctly - wrong type")
	} else if receivedDetail["key"] != "value" || receivedDetail["count"] != 42 {
		t.Error("Detail not set correctly")
	}
	if event.Bubbles {
		t.Error("Bubbles should be false by default")
	}
	if event.Cancelable {
		t.Error("Cancelable should be false by default")
	}
}

func TestCustomEvent_NewCustomEventWithOptions(t *testing.T) {
	detail := "test data"
	event := NewCustomEventWithOptions("custom", detail, true, true)

	if event.Type != "custom" {
		t.Errorf("Expected type 'custom', got '%s'", event.Type)
	}
	if event.Detail() != detail {
		t.Error("Detail not set correctly")
	}
	if !event.Bubbles {
		t.Error("Bubbles should be true")
	}
	if !event.Cancelable {
		t.Error("Cancelable should be true")
	}
}

func TestCustomEvent_NilDetail(t *testing.T) {
	event := NewCustomEvent("test", nil)
	if event.Detail() != nil {
		t.Error("Detail should be nil")
	}
}

func TestCustomEvent_EventPtr(t *testing.T) {
	event := NewCustomEvent("custom", "data")
	ptr := event.EventPtr()

	if ptr != &event.Event {
		t.Error("EventPtr should return pointer to embedded Event")
	}
	if ptr.Type != "custom" {
		t.Error("EventPtr should reference correct event")
	}
}

func TestCustomEvent_DispatchWithEventPtr(t *testing.T) {
	target := NewEventTarget()
	var receivedDetail any

	target.AddEventListener("userAction", func(e *Event) {
		receivedDetail = e.Detail()
	})

	detail := map[string]any{"action": "login", "user": "alice"}
	customEvent := NewCustomEvent("userAction", detail)
	target.DispatchEvent(customEvent.EventPtr())

	if receivedDetail == nil {
		t.Error("Detail should be received by listener")
	}
	received, ok := receivedDetail.(map[string]any)
	if !ok {
		t.Fatal("Detail should be a map")
	}
	if received["action"] != "login" || received["user"] != "alice" {
		t.Error("Detail data mismatch")
	}
}

func TestCustomEvent_InheritsMethods(t *testing.T) {
	event := NewCustomEventWithOptions("cancel", nil, false, true)

	// Test inherited methods
	event.PreventDefault()
	if !event.DefaultPrevented {
		t.Error("CustomEvent should inherit PreventDefault")
	}

	event.StopImmediatePropagation()
	if !event.IsImmediatePropagationStopped() {
		t.Error("CustomEvent should inherit StopImmediatePropagation")
	}
}

func TestCustomEvent_ComplexDetail(t *testing.T) {
	type UserData struct {
		ID       int
		Username string
		Roles    []string
	}

	detail := UserData{
		ID:       123,
		Username: "bob",
		Roles:    []string{"admin", "user"},
	}

	event := NewCustomEvent("userUpdate", detail)
	retrieved, ok := event.Detail().(UserData)
	if !ok {
		t.Fatal("Detail should be UserData")
	}
	if retrieved.ID != 123 || retrieved.Username != "bob" {
		t.Error("Detail data mismatch")
	}
	if len(retrieved.Roles) != 2 {
		t.Error("Detail roles mismatch")
	}
}

func TestCustomEvent_AccessViaEventDetail(t *testing.T) {
	// Verify that Detail() works correctly when accessing through *Event
	target := NewEventTarget()
	var receivedEvent *Event

	target.AddEventListener("test", func(e *Event) {
		receivedEvent = e
	})

	customEvent := NewCustomEvent("test", "custom data")
	target.DispatchEvent(customEvent.EventPtr())

	if receivedEvent == nil {
		t.Fatal("Event not received")
	}
	if receivedEvent.Detail() != "custom data" {
		t.Error("Detail should be accessible via Event pointer")
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestEventTarget_WithLoop(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	target := NewEventTarget()
	done := make(chan bool, 1)
	var receivedType string

	target.AddEventListener("message", func(e *Event) {
		receivedType = e.Type
		done <- true
	})

	go func() {
		_ = loop.Run(ctx)
	}()
	defer loop.Shutdown(ctx)

	// Dispatch event from loop
	err = loop.Submit(func() {
		target.DispatchEvent(&Event{Type: "message"})
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for event with proper timeout
	select {
	case <-done:
		if receivedType != "message" {
			t.Errorf("Expected type 'message', got '%s'", receivedType)
		}
	case <-time.After(time.Second):
		t.Error("Event not received")
	}
}

func TestCustomEvent_WithLoop(t *testing.T) {
	ctx := context.Background()
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	target := NewEventTarget()
	done := make(chan bool, 1)
	var receivedDetail any

	target.AddEventListener("data", func(e *Event) {
		receivedDetail = e.Detail()
		done <- true
	})

	go func() {
		_ = loop.Run(ctx)
	}()
	defer loop.Shutdown(ctx)

	// Dispatch custom event from loop
	err = loop.Submit(func() {
		event := NewCustomEvent("data", map[string]int{"value": 42})
		target.DispatchEvent(event.EventPtr())
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for event with proper timeout
	select {
	case <-done:
		detail, ok := receivedDetail.(map[string]int)
		if !ok {
			t.Fatal("Detail should be map[string]int")
		}
		if detail["value"] != 42 {
			t.Errorf("Expected value 42, got %d", detail["value"])
		}
	case <-time.After(time.Second):
		t.Error("CustomEvent not received")
	}
}
