package eventloop

import (
	"sync"
)

// EventListenerFunc is a callback function for [EventTarget.AddEventListener].
// The callback receives the dispatched [Event] and can inspect/modify its state.
type EventListenerFunc func(event *Event)

// ListenerID uniquely identifies an event listener for removal purposes.
// In Go, functions cannot be reliably compared for equality, so we generate
// a unique ID for each registered listener.
type ListenerID uint64

// listenerEntry pairs a listener with its unique ID for removal.
type listenerEntry struct { //nolint:govet // betteralign:ignore
	id       ListenerID
	listener EventListenerFunc
	once     bool // if true, remove after first dispatch
}

// EventTarget provides DOM-style event dispatching.
//
// This implementation follows the W3C DOM EventTarget specification:
// https://dom.spec.whatwg.org/#interface-eventtarget
//
// Thread Safety:
// EventTarget is safe for concurrent use from multiple goroutines.
// All state mutations are protected by an internal mutex. However, for proper
// DOM-style semantics where events are dispatched synchronously, it is recommended
// to use EventTarget only from the event loop goroutine (via Submit/callbacks).
//
// Usage:
//
//	target := eventloop.NewEventTarget()
//
//	// Add a listener
//	id := target.AddEventListener("click", func(e *eventloop.Event) {
//	    fmt.Println("Clicked!", e.Type)
//	})
//
//	// Dispatch an event
//	event := &eventloop.Event{Type: "click"}
//	target.DispatchEvent(event)
//
//	// Remove the listener
//	target.RemoveEventListenerByID("click", id)
type EventTarget struct {
	listeners      map[string][]listenerEntry // eventType -> listeners
	nextListenerID ListenerID
	mu             sync.RWMutex
}

// Event represents an event dispatched by [EventTarget.DispatchEvent].
//
// This implementation follows the W3C DOM Event specification:
// https://dom.spec.whatwg.org/#interface-event
//
// Thread Safety:
// Event is NOT safe for concurrent access. An Event should only be used
// from the goroutine that calls DispatchEvent.
type Event struct { //nolint:govet // betteralign:ignore
	// Type is the name of the event (e.g., "click", "abort", "load").
	Type string

	// Target is the EventTarget on which the event was dispatched.
	Target *EventTarget

	// DefaultPrevented is true if PreventDefault() was called.
	DefaultPrevented bool

	// propagationStopped is true if StopPropagation() was called.
	propagationStopped bool

	// immediatePropagationStopped is true if StopImmediatePropagation() was called.
	immediatePropagationStopped bool

	// Bubbles indicates whether the event bubbles up through the DOM tree.
	// Default is false.
	Bubbles bool

	// Cancelable indicates whether the event can be canceled.
	// Default is false.
	Cancelable bool

	// detail holds custom event data (used by CustomEvent).
	detail any
}

// NewEventTarget creates a new EventTarget with an empty listener map.
func NewEventTarget() *EventTarget {
	return &EventTarget{
		listeners:      make(map[string][]listenerEntry),
		nextListenerID: 1,
	}
}

// AddEventListener registers a listener for events of the specified type.
//
// Parameters:
//   - eventType: The event type to listen for (e.g., "click", "abort")
//   - listener: The callback function to invoke when the event is dispatched
//
// Returns:
//   - ListenerID: A unique identifier that can be used to remove the listener
//
// Thread Safety: Safe to call concurrently.
//
// This follows the DOM addEventListener() API with some simplifications:
//   - No options object (capture, once, passive, signal)
//   - Returns an ID for reliable removal (Go functions can't be compared)
func (et *EventTarget) AddEventListener(eventType string, listener EventListenerFunc) ListenerID {
	return et.addListenerInternal(eventType, listener, false)
}

// AddEventListenerOnce registers a listener that will be removed after the first dispatch.
//
// This is equivalent to calling addEventListener with { once: true } in DOM.
//
// Thread Safety: Safe to call concurrently.
func (et *EventTarget) AddEventListenerOnce(eventType string, listener EventListenerFunc) ListenerID {
	return et.addListenerInternal(eventType, listener, true)
}

// addListenerInternal is the internal implementation for adding listeners.
func (et *EventTarget) addListenerInternal(eventType string, listener EventListenerFunc, once bool) ListenerID {
	if listener == nil {
		return 0
	}

	et.mu.Lock()
	defer et.mu.Unlock()

	id := et.nextListenerID
	et.nextListenerID++

	entry := listenerEntry{
		id:       id,
		listener: listener,
		once:     once,
	}

	et.listeners[eventType] = append(et.listeners[eventType], entry)
	return id
}

// RemoveEventListenerByID removes a listener by its ID.
//
// This is the recommended way to remove listeners in Go since function
// values cannot be reliably compared for equality.
//
// Parameters:
//   - eventType: The event type the listener was registered for
//   - id: The ListenerID returned by AddEventListener
//
// Returns:
//   - true if a listener was removed, false otherwise
//
// Thread Safety: Safe to call concurrently.
func (et *EventTarget) RemoveEventListenerByID(eventType string, id ListenerID) bool {
	et.mu.Lock()
	defer et.mu.Unlock()

	entries, ok := et.listeners[eventType]
	if !ok {
		return false
	}

	for i, entry := range entries {
		if entry.id == id {
			// Remove this entry
			et.listeners[eventType] = append(entries[:i], entries[i+1:]...)
			return true
		}
	}

	return false
}

// RemoveEventListener is provided for DOM API compatibility but is a no-op.
//
// In the DOM API, removeEventListener compares functions by reference.
// Go function values cannot be reliably compared for equality, so this
// method cannot work as expected. Use [EventTarget.RemoveEventListenerByID] instead.
//
// This method exists only to provide API compatibility with DOM patterns.
//
// Thread Safety: Safe to call concurrently (no-op).
func (et *EventTarget) RemoveEventListener(eventType string, listener EventListenerFunc) {
	// Cannot implement - Go functions cannot be compared
	// See RemoveEventListenerByID for the Go-friendly alternative
}

// DispatchEvent dispatches an event to all registered listeners.
//
// The event's Target is set to this EventTarget before listeners are called.
// Listeners are called in the order they were registered.
//
// Parameters:
//   - event: The event to dispatch
//
// Returns:
//   - true if the event was not canceled (DefaultPrevented is false),
//     or if the event is not cancelable
//
// Thread Safety: Safe to call concurrently, though listeners are called
// synchronously and should not block.
func (et *EventTarget) DispatchEvent(event *Event) bool {
	if event == nil {
		return true
	}

	// Set the target
	event.Target = et

	// Get a copy of listeners to avoid holding lock during dispatch
	et.mu.RLock()
	entries := make([]listenerEntry, len(et.listeners[event.Type]))
	copy(entries, et.listeners[event.Type])
	et.mu.RUnlock()

	// Track IDs to remove (for once listeners)
	var removeIDs []ListenerID

	// Dispatch to all listeners
	for _, entry := range entries {
		if event.immediatePropagationStopped {
			break
		}

		// Call the listener (panics propagate to caller)
		entry.listener(event)

		// Mark for removal if once
		if entry.once {
			removeIDs = append(removeIDs, entry.id)
		}
	}

	// Remove once listeners
	if len(removeIDs) > 0 {
		et.mu.Lock()
		for _, id := range removeIDs {
			entries := et.listeners[event.Type]
			for i, entry := range entries {
				if entry.id == id {
					et.listeners[event.Type] = append(entries[:i], entries[i+1:]...)
					break
				}
			}
		}
		et.mu.Unlock()
	}

	// Return false if default was prevented and event is cancelable
	return !event.Cancelable || !event.DefaultPrevented
}

// HasEventListeners returns true if there are any listeners for the event type.
//
// Thread Safety: Safe to call concurrently.
func (et *EventTarget) HasEventListeners(eventType string) bool {
	et.mu.RLock()
	defer et.mu.RUnlock()
	return len(et.listeners[eventType]) > 0
}

// ListenerCount returns the number of listeners for the event type.
//
// Thread Safety: Safe to call concurrently.
func (et *EventTarget) ListenerCount(eventType string) int {
	et.mu.RLock()
	defer et.mu.RUnlock()
	return len(et.listeners[eventType])
}

// RemoveAllEventListeners removes all listeners for the specified event type.
// If eventType is empty, removes all listeners for all event types.
//
// Thread Safety: Safe to call concurrently.
func (et *EventTarget) RemoveAllEventListeners(eventType string) {
	et.mu.Lock()
	defer et.mu.Unlock()

	if eventType == "" {
		et.listeners = make(map[string][]listenerEntry)
	} else {
		delete(et.listeners, eventType)
	}
}

// PreventDefault marks the event as having its default action canceled.
//
// This only has effect if the event's Cancelable property is true.
// After calling PreventDefault, the DefaultPrevented property returns true.
//
// This follows the DOM Event.preventDefault() method.
func (e *Event) PreventDefault() {
	if e.Cancelable {
		e.DefaultPrevented = true
	}
}

// StopPropagation prevents the event from propagating further.
//
// When called, remaining listeners for the current target will still be called,
// but if the event bubbles, parent targets will not receive it.
//
// This follows the DOM Event.stopPropagation() method.
func (e *Event) StopPropagation() {
	e.propagationStopped = true
}

// StopImmediatePropagation prevents any further listeners from being called.
//
// When called, remaining listeners for the current target will NOT be called,
// and if the event bubbles, parent targets will not receive it.
//
// This follows the DOM Event.stopImmediatePropagation() method.
func (e *Event) StopImmediatePropagation() {
	e.propagationStopped = true
	e.immediatePropagationStopped = true
}

// IsPropagationStopped returns true if StopPropagation or StopImmediatePropagation was called.
func (e *Event) IsPropagationStopped() bool {
	return e.propagationStopped
}

// IsImmediatePropagationStopped returns true if StopImmediatePropagation was called.
func (e *Event) IsImmediatePropagationStopped() bool {
	return e.immediatePropagationStopped
}

// Detail returns the custom detail data associated with the event.
// This is primarily used by [CustomEvent].
func (e *Event) Detail() any {
	return e.detail
}

// NewEvent creates a new Event with the specified type.
//
// Parameters:
//   - eventType: The type/name of the event
//
// Returns:
//   - A new Event with Bubbles=false and Cancelable=false
func NewEvent(eventType string) *Event {
	return &Event{
		Type: eventType,
	}
}

// NewEventWithOptions creates a new Event with specified options.
//
// Parameters:
//   - eventType: The type/name of the event
//   - bubbles: Whether the event bubbles up through the DOM tree
//   - cancelable: Whether the event can be canceled
//
// Returns:
//   - A new Event configured with the specified options
func NewEventWithOptions(eventType string, bubbles, cancelable bool) *Event {
	return &Event{
		Type:       eventType,
		Bubbles:    bubbles,
		Cancelable: cancelable,
	}
}

// ============================================================================
// EXPAND-028: CustomEvent Support
// ============================================================================

// CustomEvent is an Event that carries custom data in its Detail field.
//
// This implementation follows the W3C DOM CustomEvent specification:
// https://dom.spec.whatwg.org/#interface-customevent
//
// CustomEvent is typically used for application-defined events that need
// to pass data to their listeners.
//
// Usage:
//
//	target := eventloop.NewEventTarget()
//
//	// Create and dispatch a custom event with data
//	event := eventloop.NewCustomEvent("userLogin", map[string]any{
//	    "username": "alice",
//	    "timestamp": time.Now(),
//	})
//	target.DispatchEvent(event.Event())
//
//	// Listener can access the data
//	target.AddEventListener("userLogin", func(e *eventloop.Event) {
//	    if data, ok := e.Detail().(map[string]any); ok {
//	        fmt.Println("User logged in:", data["username"])
//	    }
//	})
type CustomEvent struct {
	// Embedded Event provides all standard event properties and methods.
	Event
}

// NewCustomEvent creates a new CustomEvent with the specified type and detail.
//
// Parameters:
//   - eventType: The type/name of the event
//   - detail: Custom data to associate with the event (accessible via Detail())
//
// Returns:
//   - A new CustomEvent with Bubbles=false and Cancelable=false
func NewCustomEvent(eventType string, detail any) *CustomEvent {
	return &CustomEvent{
		Event: Event{
			Type:   eventType,
			detail: detail,
		},
	}
}

// NewCustomEventWithOptions creates a new CustomEvent with specified options.
//
// Parameters:
//   - eventType: The type/name of the event
//   - detail: Custom data to associate with the event
//   - bubbles: Whether the event bubbles up through the DOM tree
//   - cancelable: Whether the event can be canceled
//
// Returns:
//   - A new CustomEvent configured with the specified options
func NewCustomEventWithOptions(eventType string, detail any, bubbles, cancelable bool) *CustomEvent {
	return &CustomEvent{
		Event: Event{
			Type:       eventType,
			detail:     detail,
			Bubbles:    bubbles,
			Cancelable: cancelable,
		},
	}
}

// EventPtr returns a pointer to the embedded Event for use with DispatchEvent.
//
// This is a convenience method since DispatchEvent expects *Event but CustomEvent
// embeds Event (not *Event).
//
// Usage:
//
//	customEvent := eventloop.NewCustomEvent("myEvent", data)
//	target.DispatchEvent(customEvent.EventPtr())
func (ce *CustomEvent) EventPtr() *Event {
	return &ce.Event
}
