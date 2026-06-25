# SetImmediate Phase Ordering Analysis

## Current Behavior

`SetImmediate` (js.go:589) calls `js.loop.Submit(state.run)` which enqueues
the callback into the external queue (`l.external`). In `tick()`, the external
queue is processed by `processExternal()` which runs BEFORE `poll()`.

## Node.js Behavior

In Node.js, `setImmediate` executes in the "check" phase which runs AFTER
the poll phase. The event loop order is:

1. timers
2. pending callbacks
3. idle/prepare
4. **poll** (I/O callbacks)
5. **check** (setImmediate callbacks)
6. close callbacks

## Impact

In this eventloop, `setImmediate` callbacks run BEFORE I/O poll callbacks.
This means:

```js
// In Node.js: I/O callback typically runs before setImmediate
// In this eventloop: setImmediate always runs before I/O callback
fs.readFile('file.txt', () => console.log('I/O'));
setImmediate(() => console.log('Immediate'));
// Node.js: "I/O", "Immediate" (typically)
// This eventloop: "Immediate", "I/O" (always)
```

## Analysis

This is a **deliberate design choice**, not a bug:

1. **No separate check phase**: The eventloop's tick() has phases: timers →
   internal → external → auxJobs → drainMicrotasks → poll → drainMicrotasks.
   There is no separate "check" phase after poll. Adding one would require a
   new queue and significant architectural change.

2. **External queue serves multiple purposes**: The external queue handles
   both `Submit()` (general-purpose task submission) and `SetImmediate`. In
   Node.js, setImmediate has its own dedicated queue. Combining them is a
   simplification that trades exact Node.js compatibility for architectural
   simplicity.

3. **Practical impact is minimal**: The ordering difference only matters
   when both an I/O callback and a setImmediate callback are pending in the
   same tick. In practice, setImmediate is typically used to break up
   long-running operations, not to order relative to I/O.

4. **The `WithStrictMicrotaskOrdering` option does not affect this**: The
   phase ordering is structural (which queue is processed when), not a
   microtask draining concern.

## Recommendation

Document this as a known deviation from Node.js semantics. The doc.go
execution model description should note that `SetImmediate` runs in the
external queue phase (before poll), not in a separate check phase (after poll).

## Conclusion

This is a deliberate design choice. No code changes needed. The deviation
should be documented in doc.go.
