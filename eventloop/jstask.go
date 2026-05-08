package eventloop

// microtaskFunc is a callback function for [JS.QueueMicrotask]. During normal
// [Loop.Run] execution, the callback is invoked on the logical callback-owner
// goroutine.
type microtaskFunc func()

// QueueMicrotask schedules a microtask to run before any pending timer callbacks.
//
// Microtasks are processed in FIFO order and have higher priority than timers.
// A microtask scheduled from within another microtask will be processed in the
// same tick, before any timer callbacks.
//
// This follows the JavaScript queueMicrotask semantics and is used internally
// by the Promise implementation for then/catch/finally handlers.
// QueueMicrotask panics if fn is nil.
func (js *JS) QueueMicrotask(fn microtaskFunc) error {
	if fn == nil {
		panic("eventloop: nil QueueMicrotask callback")
	}

	return js.loop.ScheduleMicrotask(fn)
}

// NextTick schedules a function in the nextTick queue.
//
// This emulates Node.js process.nextTick() semantics. At a microtask checkpoint,
// all pending NextTick callbacks run before the next Promise / queueMicrotask
// batch. A NextTick admitted during an active Promise batch waits until that
// batch exhausts rather than preempting its remaining handlers.
//
// Unlike setTimeout(fn, 0) which schedules for the next tick, NextTick callbacks
// queued by synchronous code execute at its checkpoint before pending Promise
// handlers.
//
// Returns:
//   - Error if the loop is shut down.
//
// Example:
//
//	js.NextTick(func() {
//	    fmt.Println("This runs before promises")
//	})
//
//	promise.Then(func(r any) any {
//	    fmt.Println("This runs after nextTick")
//	    return nil
//	}, nil)
//
// Thread Safety: Safe to call from any goroutine.
// NextTick panics if fn is nil.
func (js *JS) NextTick(fn func()) error {
	if fn == nil {
		panic("eventloop: nil NextTick callback")
	}
	return js.loop.ScheduleNextTick(fn)
}
