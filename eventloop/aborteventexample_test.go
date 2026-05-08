package eventloop_test

import (
	"errors"
	"fmt"

	eventloop "github.com/joeycumines/go-eventloop"
)

// Example_abortController demonstrates the abort pattern for cancellation.
//
// AbortController is a Go-native mechanism for canceling asynchronous work.
func Example_abortController() {
	// Create an AbortController
	controller := eventloop.NewAbortController()
	signal := controller.Signal()

	// Register an abort handler
	signal.OnAbort(func(reason any) {
		fmt.Printf("Operation aborted: %v\n", reason)
	})

	// Simulate starting an operation
	fmt.Println("Starting operation...")

	controller.Abort(errors.New("user cancelled"))

	// Check abort status
	if signal.Aborted() {
		fmt.Println("Signal is aborted")
	}

	// Output:
	// Starting operation...
	// Operation aborted: user cancelled
	// Signal is aborted
}

// Example_eventTarget demonstrates synchronous Go-native custom event dispatch.
// EventTarget supports adding listeners, dispatching events, and removing
// listeners by ID.
func Example_eventTarget() {
	et := eventloop.NewEventTarget()

	// Add a listener for "data" events
	id := et.AddEventListener("data", func(event *eventloop.Event) {
		fmt.Printf("Event type: %s\n", event.Type)
	})

	// Dispatch an event — the listener receives it
	et.DispatchEvent(&eventloop.Event{
		Type: "data",
	})

	// Remove the listener
	et.RemoveEventListenerByID("data", id)

	// Dispatch again — no listener receives it (returns true since event
	// was not canceled via PreventDefault).
	dispatched := et.DispatchEvent(&eventloop.Event{
		Type: "data",
	})

	fmt.Printf("Dispatched after removal: %v\n", dispatched)

	// Output:
	// Event type: data
	// Dispatched after removal: true
}
