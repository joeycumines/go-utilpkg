# Subagent Review: Chunk 1 - Critical Fixes

**Review Session ID**: 13-CHUNK1-CRITICAL
**Date**: 2026-01-23
**Status**: **FAILED**
**Reviewer**: Subagent (Antigravity)

## 1. Executive Summary

This review guarantees that the current implementation of `goja-eventloop` and `eventloop` contains **CRITICAL** defects that must be resolved before proceeding. The implementation is unsafe and functionally incoherent with respect to standard Promise semantics.

**Verdict**: The PR cannot be merged. Immediate remediation is required for:
1.  **WaitGroup Panic**: `SetInterval` logic will panic the runtime.
2.  **Broken Promise Chaining**: `ChainedPromise` completely lacks the Promise Resolution Procedure (adopting state of returned promises).
3.  **Type Unsafety**: Goja adapter fails to correctly unwrap promises returned from handlers.

## 2. Detailed Findings

### 2.1. Critical: WaitGroup Negative Counter in `eventloop/js.go`

**Location**: `eventloop/js.go`, Line 314 (`SetInterval`)
**Severity**: **CRITICAL (Panic)**

**Analysis**:
The `SetInterval` function attempts to cleanup on scheduling failure:
```go
loopTimerID, err := js.loop.ScheduleTimer(delay, wrapper)
if err != nil {
    state.wg.Done() // <--- CRITICAL BUG
    return 0, err
}
```
The `state.wg.Add(1)` call resides **inside** the `wrapper` closure (Line 261). At the time of `ScheduleTimer` failure, `wrapper` has never executed, so `Add(1)` has never been called.
Calling `wg.Done()` on a zero counter results in a strictly reproducible panic: `sync: negative WaitGroup counter`.

**Requirement**: Remove the `state.wg.Done()` call. The WaitGroup serves to track *running executions*, not the scheduling attempt itself.

### 2.2. Critical: Missing Promise Resolution Procedure in `eventloop/promise.go`

**Location**: `eventloop/promise.go`, `resolve()` method
**Severity**: **BLOCKER**

**Analysis**:
The `ChainedPromise` implementation treats all values as simple values. It does **not** check if the value is a Promise or Thenable.
```go
func (p *ChainedPromise) resolve(value Result, js *JS) {
    // ...
    p.value = value // Stores promise as value!
    // ...
}
```
If a `Then` handler returns a `*ChainedPromise`, the new promise effectively resolves to *the promise object itself* (nested promise), rather than *waiting* for it. This breaks the fundamental `p.then(() => p2)` chaining behavior required by Promise/A+.

**Requirements**:
1.  Update `resolve` to check if `value` implements the `Promise` interface (or is `*ChainedPromise`).
2.  If it is a promise, `p` must **adopt** its state (wait for it to settle, then resolve/reject `p` with the result).

### 2.3. Critical: Goja Adapter Promise Wrapping in `goja-eventloop/adapter.go`

**Location**: `goja-eventloop/adapter.go`, `gojaFuncToHandler`
**Severity**: **BLOCKER**

**Analysis**:
When a JavaScript handler returns a value, `gojaFuncToHandler` blindly exports it:
```go
return ret.Export()
```
If the user returns a `Promise` (which is a Goja Object wrapper), `Export()` returns a `map[string]interface{}`.
Coupled with Issue 2.2, the next promise resolves with this `map`, completely breaking the chain. The user sees "first then works, second then fails" because the second `then` receives a map instead of the expected value.

**Requirement**:
`gojaFuncToHandler` must check if `ret` is a wrapped Promise object (using `_internalPromise`). If so, it must return the underlying `*ChainedPromise`. This interacts with Fix 2.2 to ensure the chain waits for this promise.

### 2.4. Critical: Panic Risk in `loop.go` (Heap Corruption)

**Location**: `loop.go`, `CancelTimer` (inferred)
**Severity**: **HIGH**

**Analysis**:
The report mentions "index out of range [-1]". This typically occurs when `heap.Remove` is called with index -1. While `loop.go` was not fully audited in this pass, safety checks must be added to ensure `heapIndex` is non-negative before any heap operation, even if logic suggests it "should" be safe.

## 3. Remediation Plan

1.  **Fix `SetInterval`**: Delete incorrect `wg.Done()`.
2.  **Implement Promise Resolution**: Modify `ChainedPromise` to detect and unwrap `Future`/`Promise` values during resolution.
3.  **Fix Adapter Unwrapping**: Update `gojaFuncToHandler` to extract `_internalPromise` from return values.
4.  **Audit/Fix Heap Safety**: Ensure `timer.heapIndex` guards.

**Conclusion**: The codebase is not ready. Proceed to `Address Chunk 1 Issues` immediately.
