package eventloop

import (
	"sync"
)

// EventListenerFunc is a callback function for [EventTarget.AddEventListener].
// The callback receives the dispatched [Event] and can inspect/modify its state.
type EventListenerFunc func(event *Event)

// ListenerID identifies a currently registered event listener for removal.
// Values are unique among live registrations on one EventTarget. A value is
// invalid after successful removal or once-listener claim. If the allocation
// counter completes a full uint64 cycle, a released value may be reused; a
// stale same-type ID can then exhibit ABA behavior and name a later listener.
// In Go, functions cannot be reliably compared for equality, so registration
// returns an ID instead of accepting a function value for removal.
type ListenerID uint64

// listenerEntry pairs a listener with its unique ID for removal.
type listenerEntry struct { //nolint:govet // betteralign:ignore
	id       ListenerID
	listener EventListenerFunc
	once     bool // if true, remove after first dispatch
	active   bool
}

// EventTarget is a synchronous, Go-native typed event dispatcher.
//
// Listener registration, removal, queries, and dispatch are safe for concurrent
// use. Listeners run synchronously on the goroutine that calls DispatchEvent and
// never under the target lock. Concurrent dispatch may therefore invoke an
// ordinary listener concurrently. The zero value is ready for use. EventTarget
// contains synchronization state and must not be copied. Use a distinct zero
// value or call NewEventTarget when independent listener state is required.
//
// The API borrows familiar event names, but it has no DOM tree, capture phase,
// or bubbling engine. JavaScript EventTarget semantics belong to goja-eventloop.
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
	listeners      map[string][]*listenerEntry // eventType -> listeners
	nextListenerID ListenerID
	mu             sync.RWMutex
}

// The inline capacity is selected from exact production-path fitness and
// full-dispatch benchmarks, balancing ordinary and parallel dispatch cost
// against the registry's fixed size. A map preserves constant-time lookup
// beyond the inline capacity.
const inlineActiveEventDispatchCapacity = 4

// Keep a modest overflow working set to avoid rebuilding its map during common
// parallel bursts. A larger high-water map is released as soon as it is idle.
const retainedOverflowEventDispatchCapacity = inlineActiveEventDispatchCapacity * inlineActiveEventDispatchCapacity

var activeEventDispatches struct {
	sync.Mutex
	inline        [inlineActiveEventDispatchCapacity]eventDispatchState
	inlineCount   int
	overflow      map[*Event]eventDispatchState
	overflowCount int
	overflowLarge bool
}

type eventDispatchState struct {
	event                       *Event
	defaultPrevented            bool
	propagationStopped          bool
	immediatePropagationStopped bool
}

type eventDispatchStateUpdate uint8

const (
	eventDispatchDefaultPrevented eventDispatchStateUpdate = 1 << iota
	eventDispatchPropagationStopped
	eventDispatchImmediatePropagationStopped
)

type eventDispatchStateRef struct {
	event       *Event
	inlineIndex int
}

// Event is the mutable value passed to [EventTarget.DispatchEvent] listeners.
//
// Event is not safe for concurrent use. It may be reused after DispatchEvent
// returns; each dispatch resets Target, DefaultPrevented, and propagation state.
// Dispatching the same Event pointer recursively is a programming error and
// panics. Dispatching a copied Event establishes an independent identity at
// entry, even when the copy was made during another dispatch.
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

	// Bubbles is caller-owned metadata. EventTarget has no parent traversal.
	Bubbles bool

	// Cancelable indicates whether the event can be canceled.
	// Default is false.
	Cancelable bool

	// detail holds custom event data (used by CustomEvent).
	detail any
}

// NewEventTarget creates a new EventTarget.
func NewEventTarget() *EventTarget {
	return &EventTarget{
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
//   - ListenerID: An identifier unique among this target's live registrations
//
// Thread Safety: Safe to call concurrently.
//
// AddEventListener panics only if every non-zero ListenerID is simultaneously
// in use and no unique identifier can be allocated.
//
// The shape is inspired by addEventListener, with Go-specific behavior:
//   - No options object (capture, once, passive, signal)
//   - Returns an ID for reliable removal (Go functions can't be compared)
func (et *EventTarget) AddEventListener(eventType string, listener EventListenerFunc) ListenerID {
	return et.addListenerInternal(eventType, listener, false)
}

// AddEventListenerOnce registers a listener claimed by at most one dispatch.
//
// The listener is removed before its callback starts, including across concurrent
// and recursive dispatches. A callback panic does not restore it.
//
// Thread Safety: Safe to call concurrently.
//
// AddEventListenerOnce panics only if every non-zero ListenerID is simultaneously
// in use and no unique identifier can be allocated.
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
	if et.listeners == nil {
		et.listeners = make(map[string][]*listenerEntry)
		if et.nextListenerID == 0 {
			et.nextListenerID = 1
		}
	}
	id := et.allocateListenerIDLocked()

	entry := &listenerEntry{
		id:       id,
		listener: listener,
		once:     once,
		active:   true,
	}

	et.listeners[eventType] = append(et.listeners[eventType], entry)
	return id
}

func (et *EventTarget) allocateListenerIDLocked() ListenerID {
	// Before the first actual uint64 wrap, nextListenerID has never been used,
	// so allocation is O(1) and requires no scan under the target lock.
	if id := et.nextListenerID; id != 0 {
		et.nextListenerID++
		return id
	}

	// Zero is the permanent wrapped-mode sentinel. Search released IDs globally
	// and leave the sentinel intact so a reused value can never re-enter the
	// monotonic path and collide with another live registration.
	return et.allocateWrappedListenerIDLocked(1)
}

func (et *EventTarget) allocateWrappedListenerIDLocked(first ListenerID) ListenerID {
	for id := first; id != 0; id++ {
		if !et.listenerIDUsedLocked(id) {
			return id
		}
	}
	panic("eventloop: EventTarget listener IDs exhausted")
}

func (et *EventTarget) listenerIDUsedLocked(id ListenerID) bool {
	for _, entries := range et.listeners {
		for _, entry := range entries {
			if entry != nil && entry.id == id {
				return true
			}
		}
	}
	return false
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
// A true result prevents future claims. A callback whose claim already won may
// still run or be running when removal returns.
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
			et.removeListenerLocked(eventType, entries, i, entry)
			return true
		}
	}

	return false
}

// DispatchEvent dispatches an event to all registered listeners.
//
// The event's Target is set to this EventTarget. Cancellation and propagation
// state are reset at entry, allowing reuse after a completed dispatch. The Type
// value at entry selects the listener snapshot and is restored before each later
// listener and when dispatch returns. Target and the accumulated cancellation
// and propagation outcome are likewise restored, so replacing the whole Event
// value inside a listener cannot erase dispatch control already requested by
// PreventDefault or StopImmediatePropagation.
//
// Snapshot members are considered in registration order. Removal that wins
// before a member's callback-start claim suppresses it. Once listeners are
// atomically removed at that claim. Listeners added after the snapshot wait for
// a later dispatch. A listener panic propagates, stops later callbacks, and does
// not strand once or Event dispatch state.
//
// DispatchEvent returns false once a listener calls PreventDefault while the
// event is cancelable, or leaves both Cancelable and the exported
// DefaultPrevented field true at callback return. That cancellation outcome is
// sticky for the dispatch: later field mutation or whole-value replacement
// cannot clear it. A nil event is a no-op returning true. DispatchEvent panics
// if event is already being dispatched.
func (et *EventTarget) DispatchEvent(event *Event) bool {
	if event == nil {
		return true
	}

	dispatchState := beginEventDispatch(event)
	defer endEventDispatch(event)
	eventType := event.Type
	completed := false
	event.Target = et
	event.DefaultPrevented = false
	event.propagationStopped = false
	event.immediatePropagationStopped = false
	defer func() {
		if !completed {
			dispatchState.restore(event, eventType, et)
		}
	}()

	// Snapshot membership, but retain shared entries so removal remains visible
	// until each listener crosses its callback-start claim.
	et.mu.RLock()
	entries := make([]*listenerEntry, len(et.listeners[eventType]))
	copy(entries, et.listeners[eventType])
	et.mu.RUnlock()
	if len(entries) == 0 {
		completed = true
		return true
	}

	for _, entry := range entries {
		state := dispatchState.snapshot()
		if state.immediatePropagationStopped {
			break
		}

		listener := et.claimListener(eventType, entry)
		if listener == nil {
			continue
		}

		applyEventDispatchState(event, eventType, et, state)
		listener(event)
		dispatchState.merge(event)
	}

	state := dispatchState.snapshot()
	applyEventDispatchState(event, eventType, et, state)
	completed = true
	return !state.defaultPrevented
}

func beginEventDispatch(event *Event) eventDispatchStateRef {
	activeEventDispatches.Lock()
	if activeEventDispatches.inlineCount != 0 {
		remaining := activeEventDispatches.inlineCount
		for i := range activeEventDispatches.inline {
			if activeEventDispatches.inline[i].event == nil {
				continue
			}
			if activeEventDispatches.inline[i].event == event {
				activeEventDispatches.Unlock()
				panic("eventloop: Event is already being dispatched")
			}
			remaining--
			if remaining == 0 {
				break
			}
		}
	}
	if activeEventDispatches.overflowCount != 0 {
		if _, exists := activeEventDispatches.overflow[event]; exists {
			activeEventDispatches.Unlock()
			panic("eventloop: Event is already being dispatched")
		}
	}
	if activeEventDispatches.inlineCount < len(activeEventDispatches.inline) {
		for i := range activeEventDispatches.inline {
			if activeEventDispatches.inline[i].event != nil {
				continue
			}
			activeEventDispatches.inline[i] = eventDispatchState{event: event}
			activeEventDispatches.inlineCount++
			activeEventDispatches.Unlock()
			return eventDispatchStateRef{event: event, inlineIndex: i}
		}
		activeEventDispatches.Unlock()
		panic("eventloop: active Event dispatch registry is inconsistent")
	}
	if activeEventDispatches.overflow == nil {
		activeEventDispatches.overflow = make(map[*Event]eventDispatchState)
	}
	activeEventDispatches.overflow[event] = eventDispatchState{event: event}
	activeEventDispatches.overflowCount++
	if activeEventDispatches.overflowCount > retainedOverflowEventDispatchCapacity {
		activeEventDispatches.overflowLarge = true
	}
	activeEventDispatches.Unlock()
	return eventDispatchStateRef{event: event, inlineIndex: -1}
}

func endEventDispatch(event *Event) {
	activeEventDispatches.Lock()
	if activeEventDispatches.inlineCount != 0 {
		remaining := activeEventDispatches.inlineCount
		for i := range activeEventDispatches.inline {
			if activeEventDispatches.inline[i].event == nil {
				continue
			}
			if activeEventDispatches.inline[i].event == event {
				activeEventDispatches.inline[i] = eventDispatchState{}
				activeEventDispatches.inlineCount--
				activeEventDispatches.Unlock()
				return
			}
			remaining--
			if remaining == 0 {
				break
			}
		}
	}
	if _, exists := activeEventDispatches.overflow[event]; !exists {
		activeEventDispatches.Unlock()
		panic("eventloop: inactive Event dispatch")
	}
	delete(activeEventDispatches.overflow, event)
	activeEventDispatches.overflowCount--
	if activeEventDispatches.overflowCount == 0 {
		if activeEventDispatches.overflowLarge {
			// Event keys are already deleted, but an empty map otherwise retains
			// its high-water bucket capacity for the process lifetime.
			activeEventDispatches.overflow = nil
		}
		activeEventDispatches.overflowLarge = false
	}
	activeEventDispatches.Unlock()
}

func (r eventDispatchStateRef) snapshot() eventDispatchState {
	if r.inlineIndex >= 0 {
		state := activeEventDispatches.inline[r.inlineIndex]
		if state.event != r.event {
			panic("eventloop: inactive Event dispatch")
		}
		return state
	}
	activeEventDispatches.Lock()
	state, ok := activeEventDispatches.overflow[r.event]
	activeEventDispatches.Unlock()
	if !ok {
		panic("eventloop: inactive Event dispatch")
	}
	return state
}

func (r eventDispatchStateRef) merge(event *Event) {
	if r.inlineIndex >= 0 {
		state := &activeEventDispatches.inline[r.inlineIndex]
		if state.event != r.event {
			panic("eventloop: inactive Event dispatch")
		}
		state.defaultPrevented = state.defaultPrevented || event.Cancelable && event.DefaultPrevented
		state.propagationStopped = state.propagationStopped || event.propagationStopped
		state.immediatePropagationStopped = state.immediatePropagationStopped || event.immediatePropagationStopped
		return
	}
	activeEventDispatches.Lock()
	state, ok := activeEventDispatches.overflow[r.event]
	if !ok {
		activeEventDispatches.Unlock()
		panic("eventloop: inactive Event dispatch")
	}
	activeEventDispatches.overflow[r.event] = mergeEventDispatchState(state, event)
	activeEventDispatches.Unlock()
}

func mergeEventDispatchState(state eventDispatchState, event *Event) eventDispatchState {
	state.defaultPrevented = state.defaultPrevented || event.Cancelable && event.DefaultPrevented
	state.propagationStopped = state.propagationStopped || event.propagationStopped
	state.immediatePropagationStopped = state.immediatePropagationStopped || event.immediatePropagationStopped
	return state
}

func (r eventDispatchStateRef) restore(event *Event, eventType string, target *EventTarget) {
	applyEventDispatchState(event, eventType, target, r.snapshot())
}

func applyEventDispatchState(event *Event, eventType string, target *EventTarget, state eventDispatchState) {
	event.Type = eventType
	event.Target = target
	event.DefaultPrevented = state.defaultPrevented
	event.propagationStopped = state.propagationStopped
	event.immediatePropagationStopped = state.immediatePropagationStopped
}

func markEventDispatchState(event *Event, update eventDispatchStateUpdate) {
	activeEventDispatches.Lock()
	defer activeEventDispatches.Unlock()
	for i := range activeEventDispatches.inline {
		if activeEventDispatches.inline[i].event == event {
			activeEventDispatches.inline[i] = applyEventDispatchStateUpdate(activeEventDispatches.inline[i], update)
			return
		}
	}
	if state, ok := activeEventDispatches.overflow[event]; ok {
		activeEventDispatches.overflow[event] = applyEventDispatchStateUpdate(state, update)
	}
}

func applyEventDispatchStateUpdate(state eventDispatchState, update eventDispatchStateUpdate) eventDispatchState {
	if update&eventDispatchDefaultPrevented != 0 {
		state.defaultPrevented = true
	}
	if update&eventDispatchPropagationStopped != 0 {
		state.propagationStopped = true
	}
	if update&eventDispatchImmediatePropagationStopped != 0 {
		state.immediatePropagationStopped = true
	}
	return state
}

func lookupEventDispatchState(event *Event) (eventDispatchState, bool) {
	activeEventDispatches.Lock()
	defer activeEventDispatches.Unlock()
	for i := range activeEventDispatches.inline {
		if activeEventDispatches.inline[i].event == event {
			return activeEventDispatches.inline[i], true
		}
	}
	state, ok := activeEventDispatches.overflow[event]
	return state, ok
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
		if et.listeners == nil {
			return
		}
		for _, entries := range et.listeners {
			deactivateListeners(entries)
		}
		et.listeners = make(map[string][]*listenerEntry)
	} else {
		deactivateListeners(et.listeners[eventType])
		delete(et.listeners, eventType)
	}
}

func (et *EventTarget) claimListener(eventType string, target *listenerEntry) EventListenerFunc {
	if target == nil {
		return nil
	}

	et.mu.Lock()
	defer et.mu.Unlock()
	if !target.active {
		return nil
	}
	listener := target.listener
	if !target.once {
		return listener
	}

	entries := et.listeners[eventType]
	for i, entry := range entries {
		if entry == target {
			et.removeListenerLocked(eventType, entries, i, target)
			return listener
		}
	}
	target.active = false
	target.listener = nil
	return nil
}

func (et *EventTarget) removeListenerLocked(eventType string, entries []*listenerEntry, index int, entry *listenerEntry) {
	entry.active = false
	entry.listener = nil
	copy(entries[index:], entries[index+1:])
	last := len(entries) - 1
	entries[last] = nil
	entries = entries[:last]
	if len(entries) == 0 {
		delete(et.listeners, eventType)
		return
	}
	et.listeners[eventType] = entries
}

func deactivateListeners(entries []*listenerEntry) {
	for i, entry := range entries {
		if entry != nil {
			entry.active = false
			entry.listener = nil
		}
		entries[i] = nil
	}
}

// PreventDefault marks the event as having its default action canceled.
//
// This only has effect if the event's Cancelable property is true.
// After calling PreventDefault, the DefaultPrevented property returns true.
//
// EventTarget uses this flag only for the DispatchEvent return value.
func (e *Event) PreventDefault() {
	if e.Cancelable {
		e.DefaultPrevented = true
		markEventDispatchState(e, eventDispatchDefaultPrevented)
	}
}

// StopPropagation records caller-visible propagation metadata.
//
// It does not suppress remaining listeners. EventTarget has no parent traversal.
func (e *Event) StopPropagation() {
	e.propagationStopped = true
	markEventDispatchState(e, eventDispatchPropagationStopped)
}

// StopImmediatePropagation prevents later listeners in this dispatch from being
// claimed and also records propagation as stopped.
func (e *Event) StopImmediatePropagation() {
	e.propagationStopped = true
	e.immediatePropagationStopped = true
	markEventDispatchState(e, eventDispatchPropagationStopped|eventDispatchImmediatePropagationStopped)
}

// PropagationStopped returns true if StopPropagation or StopImmediatePropagation was called.
func (e *Event) PropagationStopped() bool {
	if state, ok := lookupEventDispatchState(e); ok {
		return state.propagationStopped
	}
	return e.propagationStopped
}

// ImmediatePropagationStopped returns true if StopImmediatePropagation was called.
func (e *Event) ImmediatePropagationStopped() bool {
	if state, ok := lookupEventDispatchState(e); ok {
		return state.immediatePropagationStopped
	}
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
//   - bubbles: Value for caller-owned Bubbles metadata; no traversal occurs
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

// CustomEvent is an Event that carries application data through [Event.Detail].
//
// CustomEvent is typically used for application-defined events that need
// to pass data to their listeners.
//
// Usage:
//
//	target := eventloop.NewEventTarget()
//
//	// Register before dispatch.
//	target.AddEventListener("userLogin", func(e *eventloop.Event) {
//	    if data, ok := e.Detail().(map[string]any); ok {
//	        fmt.Println("User logged in:", data["username"])
//	    }
//	})
//
//	// Create and dispatch a custom event with data.
//	event := eventloop.NewCustomEvent("userLogin", map[string]any{
//	    "username": "alice",
//	    "timestamp": time.Now(),
//	})
//	target.DispatchEvent(event.EventPtr())
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
		Type:   eventType,
		detail: detail,
	}
}

// NewCustomEventWithOptions creates a new CustomEvent with specified options.
//
// Parameters:
//   - eventType: The type/name of the event
//   - detail: Custom data to associate with the event
//   - bubbles: Value for caller-owned Bubbles metadata; no traversal occurs
//   - cancelable: Whether the event can be canceled
//
// Returns:
//   - A new CustomEvent configured with the specified options
func NewCustomEventWithOptions(eventType string, detail any, bubbles, cancelable bool) *CustomEvent {
	return &CustomEvent{
		Type:       eventType,
		detail:     detail,
		Bubbles:    bubbles,
		Cancelable: cancelable,
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
