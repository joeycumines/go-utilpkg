# Group 1: Goja Integration Exhaustive Review

## Succinct Summary
The `goja-eventloop` integration is functionally complete and robustly implements the Promise/A+ specification and timer logic, but contains a **critical policy violation** regarding "flakiness": the `ClearInterval` deadlock detection relies on a non-deterministic 1ms timeout. Additionally, `TestClearTimeout` introduces a data race by executing `RunString` (which accesses the non-thread-safe Runtime) concurrently with `loop.Run`. These two issues must be resolved to meet the "Zero Tolerance" standard.

## Detailed Analysis

### 1. Thread Safety and Deadlock Avoidance
**Critical Finding**: In `eventloop/js.go`, `ClearInterval` uses a heuristic to avoid deadlocks when called from within the interval callback:

```go
select {
case <-doneCh:
    // Wrapper finished
case <-time.After(1 * time.Millisecond):
    // Timeout means deadlock ... skipping wait
}
```

This violates the user's "Zero Tolerance: Timing-dependent failures" rule. While it works in practice, a loaded system might take >1ms to signal `doneCh`, causing the code to assume a deadlock and skip the wait incorrectly (though harmlessly due to the `canceled` flag, it is semantically "loose").

**Recommendation**: Implement a deterministic check. The `Loop` should expose a way to check if the current goroutine is the loop thread (e.g., using `gls` or passing context). But a simpler, robust solution exists for the specific case of `ClearInterval` called from the callback.

**Proposed Solution**: Add a `currentIntervalID atomic.Uint64` (or just `uint64` guarded by the Loop's context/single-thread guarantees) to the `JS` struct.
When the `SetInterval` wrapper executes:
1. Set `js.currentIntervalID = id`.
2. Run user function.
3. Defer set `js.currentIntervalID = 0`.
Note: Since `JS` struct is shared and callbacks run on the Loop, and Loop is single-threaded, we can just use a `atomic.Uint64` (or even a plain field if we are sure `ClearInterval` checks it safely). `ClearInterval` can be called from ANY thread. If called from the Loop thread (inside callback), `currentIntervalID` will match. If called from another thread, it won't.
So `ClearInterval` logic:
```go
if js.currentIntervalID.Load() == id {
    // We are cancelling OURSELVES from within the callback.
    // Do not wait for wg, or we deadlock.
    log.Println("Self-cancellation detected, skipping wait")
    return nil 
}
// Otherwise, wait for wg.
state.wg.Wait()
```
This is **deterministic** and requires no timeouts.

### 2. Test Correctness (Race Conditions)
**Critical Finding**: `goja-eventloop/adapter_test.go`: `TestClearTimeout` allows a race condition.
```go
// Run loop in background BEFORE RunString
go func() { done <- loop.Run(ctx) }()
_, err = runtime.RunString(...)
```
`Runtime` is NOT thread-safe. `RunString` executes on the testing goroutine. `loop.Run` executes on a background goroutine. If the loop processes any event (like a timer expiring) that touches the Runtime while `RunString` is active, the runtime state is corrupted.
While `TestClearTimeout` likely "works" because the timer is 100ms and `RunString` finishes instantly, this is statistically unsafe and violates the "EXHAUSTIVE correctness" guarantee.

**Fix**: `RunString` should be executed BEFORE the loop starts (since the timers are scheduled but don't fire), OR the test should ensure `RunString` is atomic with respect to Loop execution. Given `RunString` schedules and clears synchronously, the Loop isn't needed *during* `RunString`. Move `go loop.Run` after `RunString`.

### 3. Missing Features
- **setImmediate**: Implemented as `setTimeout(fn, 0)`. Valid and Verified.
- **Promise Combinators**: All implemented and verified.

### 4. Code Quality & Modularity
- **Dependency Rule**: `eventloop/go.mod` does NOT import `goja`. **PASS**.
- **Typos**: `eventloop/js.go:305`: "wrappers function" -> "wrapper function".

### 5. API Completeness
- `ClearInterval` uses `sync.WaitGroup` correctly to ensure clean teardown, except for the timeout issue.
- `Promise` integration correctly unwraps `_internalPromise` to support native chaining logic, avoiding double-wrapping.

## Action Plan
1.  **Refactor `ClearInterval`**: Implement the deterministic `currentIntervalID` check in `eventloop/js.go` to replace the 1ms timeout.
2.  **Fix Test Race**: Move `go loop.Run` after `RunString` in `TestClearTimeout` in `goja-eventloop/adapter_test.go`.
3.  **Run Tests**: Verify all tests pass with `-race`.
