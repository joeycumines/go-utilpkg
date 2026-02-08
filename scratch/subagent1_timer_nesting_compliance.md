# Timer Nesting Compliance Investigation Report

**Date:** February 9, 2026  
**Author:** Takumi (匠)  
**Specification:** https://html.spec.whatwg.org/multipage/timers-and-user-prompts.html (Section 8.6)  
**Cross-reference:** https://html.spec.whatwg.org/multipage/webappapis.html (Event Loop and Task Sources)

---

## 1. SUCCINCT SUMMARY

The eventloop implementation provides **FULL COMPLIANCE** with HTML5 timer nesting level specifications. The implementation correctly tracks nesting depth via `timerNestingDepth` atomic counter, applies 4ms minimum clamping when depth exceeds 5, and properly restores depth after timer execution. Minor deviations exist in timer ID management (uses single namespace vs spec's "implementation-defined" approach) but these do not affect behavioral correctness.

---

## 2. DETAILED ANALYSIS

### 2.1 Timer Nesting Level Tracking (Spec Section 8.6, Step 3)

**Spec Requirement:**
> "If the surrounding agent's event loop's currently running task is a task that was created by this algorithm, then let nesting level be the task's timer nesting level. Otherwise, let nesting level be 0."

**Implementation (eventloop/loop.go, lines 1611-1619):**
```go
// Handle canceled timer before deletion from timerMap
if !t.canceled.Load() {
    // HTML5 spec: Set nesting level to timer's scheduled depth + 1 during execution
    // This tracks call stack depth for nested setTimeout calls
    oldDepth := l.timerNestingDepth.Load()
    newDepth := t.nestingLevel + 1
    l.timerNestingDepth.Store(newDepth)

    // Restore nesting depth even if timer callback panics
    defer l.timerNestingDepth.Store(oldDepth)

    l.safeExecute(t.task)
```

**Compliance Status:** ✅ FULLY COMPLIANT

**Analysis:**
- The implementation stores `nestingLevel` in each timer struct at scheduling time
- During execution, it increments depth (`t.nestingLevel + 1`)
- Uses `defer` to ensure depth is restored even if callback panics
- This precisely matches the spec's requirement to track "nested invocations of this algorithm"

### 2.2 The "5 Nested Timers" Threshold (Spec Section 8.6, Step 5)

**Spec Requirement:**
> "If nesting level is greater than 5, and timeout is less than 4, then set timeout to 4."

**Implementation (eventloop/loop.go, lines 1655-1661):**
```go
// HTML5 spec: Clamp delay to 4ms if nesting depth > 5 and delay < 4ms
// See: https://html.spec.whatwg.org/multipage/timers-and-user-prompts.html#timers
// "If nesting level is greater than 5, and timeout is less than 4, then increase timeout to 4."
currentDepth := l.timerNestingDepth.Load()
if currentDepth > 5 {
    minDelay := 4 * time.Millisecond
    if delay >= 0 && delay < minDelay {
        delay = minDelay
    }
}
```

**Compliance Status:** ✅ FULLY COMPLIANT

**Analysis:**
- Threshold check: `currentDepth > 5` (correctly implements "> 5", not ">= 5")
- Clamping applies only when `delay < 4ms`
- Delay of exactly 4ms is NOT clamped (correct per spec)
- Negative delays are handled separately (converted to 0 by ScheduleTimer before clamping)

**Test Verification (eventloop/timer_html5_compliance_test.go):**
```go
testCases := []struct {
    depth       int32
    delay       time.Duration
    shouldClamp bool
    description string
}{
    {5, 0, false, "depth=5, delay=0 -> no clamp (threshold is > 5)"},
    {6, 0, true, "depth=6, delay=0 -> clamped to 4ms"},
    {6, 3*time.Millisecond, true, "depth=6, delay=3ms -> clamped to 4ms"},
    {6, 4*time.Millisecond, false, "depth=6, delay=4ms -> no clamp"},
}
```

### 2.3 Timeout Clamping (Spec Section 8.6, Steps 4-5)

**Spec Requirement (Step 4):**
> "If timeout is less than 0, then set timeout to 0."

**Spec Requirement (Step 5):**
> "If nesting level is greater than 5, and timeout is less than 4, then set timeout to 4."

**Implementation Order:**
1. ScheduleTimer clamps negative delays to 0 (done implicitly by `time.Duration` type)
2. THEN applies nesting clamping if `delay >= 0 && delay < 4ms`

**Compliance Status:** ✅ FULLY COMPLIANT

**Test Verification (eventloop/timer_html5_compliance_test.go):**
```go
func TestHTML5_NegativeDelayTreatedAsZero(t *testing.T) {
    // Negative delay should work like 0ms
    id, err := js.SetTimeout(func() {...}, -100)
    // Should schedule successfully (converted to 0)
}
```

### 2.4 What Constitutes a "Nested" Timer (Spec Note in Section 8.6)

**Spec Note:**
> "The task's timer nesting level is used both for nested calls to setTimeout(), and for the repeating timers created by setInterval(). (Or, indeed, for any combination of the two.) In other words, it represents nested invocations of this algorithm, not of a particular method."

**Implementation Analysis:**

| Scenario | Nesting Level Behavior |
|----------|----------------------|
| `setTimeout(fn, 0)` called from top-level | Starts at depth 0 |
| `setTimeout(fn, 0)` called from within timer callback | Depth increments by 1 |
| `setInterval` creates repeating timers | Each repeat uses same nesting level as parent |
| `setTimeout` inside `setInterval` callback | Combines both (correct per spec) |
| `clearTimeout`/`clearInterval` | Does NOT affect nesting level |

**Compliance Status:** ✅ FULLY COMPLIANT

### 2.5 setInterval Repeating Behavior (Spec Section 8.6, Step 11)

**Spec Requirement (Step 11):**
> "If repeat is true, then perform the timer initialization steps again, given global, handler, timeout, arguments, true, and id."

**Implementation (eventloop/js.go, SetInterval method):**
```go
// Schedule next execution using captured wrapper reference
loopTimerID, err := js.loop.ScheduleTimer(state.getDelay(), wrapper)
// ...
// The wrapper function reschedules itself after each execution
```

**Compliance Status:** ✅ FULLY COMPLIANT

**Analysis:**
- `setInterval` creates a persistent `intervalState` wrapper
- Wrapper function re-schedules itself after each execution
- Each iteration uses the current `timerNestingDepth` at scheduling time
- This correctly captures the nesting level from the calling context

### 2.6 Timer ID Management (Spec Section 8.6, Step 2)

**Spec Requirement:**
> "Let id be an implementation-defined integer that is greater than zero and does not already exist in global's map of setTimeout and setInterval IDs."

**Implementation Details:**

| Timer Type | ID Namespace | Starting Value |
|------------|--------------|----------------|
| `setTimeout` (via Loop) | `Loop.nextTimerID` | 0 (atomic.Add returns 1 for first) |
| `setTimeout` (via JS) | `JS.nextTimerID` | 0 → returns as uint64 |
| `setInterval` (via JS) | `JS.nextTimerID` | Shared with SetTimeout |
| `setImmediate` (via JS) | `JS.nextImmediateID` | 2^48 (separate namespace) |

**Compliance Status:** ✅ COMPLIANT with minor deviation

**Deviation:**
- Spec says setTimeout and setInterval "do not already exist in global's map of setTimeout and setInterval IDs"
- Implementation uses SEPARATE internal maps:
  - `js.timers` (for setTimeout, implicit via Loop)
  - `js.intervals` (for setInterval)
- Both use same `JS.nextTimerID` counter but different storage maps
- **Impact:** NONE - IDs remain unique across both timer types, just stored in separate maps

**MAX_SAFE_INTEGER Enforcement:**
```go
const maxSafeInteger = 9007199254740991 // 2^53 - 1
if uint64(id) > maxSafeInteger {
    return 0, ErrTimerIDExhausted
}
```

**Compliance Status:** ✅ COMPLIANT

Prevents precision loss when timer IDs are cast to JavaScript float64.

### 2.7 clearTimeout/clearInterval Behavior (Spec Section 8.6)

**Spec Requirement (Step 9-10 of task execution):**
> "If id does not exist in global's map... then abort these steps."
> "If global's map... does not equal uniqueHandle, then abort these steps."

**Implementation (eventloop/js.go, ClearTimeout):**
```go
func (js *JS) ClearTimeout(id uint64) error {
    return js.loop.CancelTimer(TimerID(id))
}

// CancelTimer implementation:
func (l *Loop) CancelTimer(id TimerID) error {
    // Marks timer as canceled
    // Removes from timerMap
    // Removes from heap
}
```

**Implementation (eventloop/js.go, ClearInterval):**
```go
func (js *JS) ClearInterval(id uint64) error {
    state.canceled.Store(true)
    // Uses atomic CompareAndSwap for thread-safe cancellation
    // Prevents race with wrapper rescheduling
}
```

**Compliance Status:** ✅ FULLY COMPLIANT

**Key Behaviors:**
1. Timer is marked canceled atomically before removal
2. Canceled timers are skipped during `runTimers()`
3. Safe for concurrent calls from any goroutine
4. Idempotent - multiple clears return error after first clear

### 2.8 Task Source and Event Loop Integration (Spec Section 8.6, Step 12)

**Spec Requirement:**
> "Queue a global task on the timer task source given global to run task."

**Implementation (eventloop/loop.go, SubmitInternal):**
```go
func (l *Loop) SubmitInternal(fn func()) error {
    // Internal queue for loop goroutine only
    // Tasks execute on loop thread in order
}
```

**Compliance Status:** ✅ COMPLIANT

**Analysis:**
- Timers use `SubmitInternal` which queues to internal task queue
- Internal queue is processed in FIFO order
- Separate from external queue (for Submit) and microtask queue
- Matches timer task source concept from spec

---

## 3. IMPLEMENTATION FINDINGS

### 3.1 File References

| File | Purpose |
|------|---------|
| `eventloop/loop.go` | Core Loop with timer scheduling and nesting depth |
| `eventloop/js.go` | JS adapter with SetTimeout/SetInterval/ClearTimeout/ClearInterval |
| `eventloop/timer_html5_compliance_test.go` | Comprehensive HTML5 compliance tests |
| `eventloop/nested_timeout_test.go` | Nesting-specific tests |
| `eventloop/example_test.go` | Example_timerNesting demonstrating clamping |
| `goja-eventloop/adapter.go` | Goja runtime bindings |

### 3.2 Key Data Structures

```go
// Loop struct (loop.go, lines 113-151)
type Loop struct {
    timerNestingDepth atomic.Int32 // HTML5 spec: nesting depth for timeout clamping
    timers            timerHeap
    timerMap          map[TimerID]*timer
    nextTimerID       atomic.Uint64
}

// timer struct (loop.go)
type timer struct {
    when         time.Time
    task         func()
    id           TimerID
    canceled     atomic.Bool
    nestingLevel int32 // Nesting level at scheduling time for HTML5 clamping
}

// JS adapter (js.go)
type JS struct {
    nextTimerID   atomic.Uint64
    intervals     map[uint64]*intervalState
}
```

### 3.3 Compliance Test Coverage

| Test | File | Coverage |
|------|------|----------|
| `TestHTML5_ZeroDelayAllowed` | timer_html5_compliance_test.go | Negative delay handling |
| `TestHTML5_NegativeDelayTreatedAsZero` | timer_html5_compliance_test.go | Negative delay handling |
| `TestHTML5_NestedTimeoutClamping` | timer_html5_compliance_test.go | 4ms clamping threshold |
| `TestHTML5_NestingDepthTrack` | timer_html5_compliance_test.go | Depth tracking |
| `TestHTML5_ClampingThreshold` | timer_html5_compliance_test.go | Exact threshold verification |
| `TestNestedTimeoutClampingBelowThreshold` | nested_timeout_test.go | Depth ≤ 5 behavior |
| `TestNestedTimeoutClampingAboveThreshold` | nested_timeout_test.go | Depth > 5 clamping |
| `Example_timerNesting` | example_test.go | Demonstrative example |

---

## 4. IDENTIFIED COMPLIANCE GAPS

### 4.1 Timer ID Map Structure (MINOR)

**Gap:** Spec uses single ordered map for all timer IDs. Implementation uses separate maps.

**Spec:**
> "global's map of setTimeout and setInterval IDs" - single map

**Implementation:**
```go
// js.intervals (for setInterval)
// implicit timers via Loop.timerMap (for setTimeout)
```

**Impact:** NONE - Both are implementation details, IDs remain unique.

**Recommendation:** Document this architectural choice. Not a bug.

### 4.2 No Unique Internal Value Handle (MINOR)

**Gap:** Spec uses "unique internal value" handle for timer validation. Implementation uses TimerID directly.

**Spec:**
> "If global's map of setTimeout and setInterval IDs[id] does not equal uniqueHandle, then abort these steps."

**Implementation:**
```go
// Timer validation in CancelTimer:
t, exists := l.timerMap[id]
if !exists {
    return ErrTimerNotFound
}
// No handle comparison - ID reuse prevention handled by timerMap removal
```

**Impact:** LOW - Timer ID reuse is prevented by immediate removal from timerMap.

**Scenario Analysis:**
1. Timer A scheduled with ID=1
2. Timer A fires, removed from timerMap
3. Timer B scheduled, gets ID=1
4. clearTimeout(1) called for old timer A
   - Implementation: ID=1 not in timerMap → ErrTimerNotFound
   - Spec: Handle mismatch → abort
   - **Result:** Both prevent callback execution, equivalent behavior

**Recommendation:** Accept as compliant. Implementation is simpler and equivalent.

### 4.3 No "Run Steps After a Timeout" Padding (INFO)

**Gap:** Spec allows optional additional delay for power optimization.

**Spec:**
> "Optionally, wait a further implementation-defined length of time."

**Implementation:** Does not implement additional padding.

**Impact:** NONE - This is optional per spec.

---

## 5. RECOMMENDATIONS

### 5.1 Add Edge Case Tests

**Recommendation:** Add test for rapid interleaved setTimeout/setInterval:

```go
func TestNestingWithMixedSetTimeoutSetInterval(t *testing.T) {
    // Interleave setTimeout and setInterval at deep nesting
    // Verify nesting level combines both correctly
}
```

### 5.2 Document Timer ID Namespace

**Recommendation:** Add documentation clarifying separate storage maps:

```go
// NOTE: setTimeout and setInterval IDs are unique across both types,
// but stored in separate internal maps (timerMap vs intervals).
// This is an implementation detail and does not affect behavior.
```

### 5.3 Add Panic Recovery Tests

**Recommendation:** Verify nesting depth restoration on panic:

```go
func TestNestingDepthRestoreOnPanic(t *testing.T) {
    // Schedule timer that panics
    // Verify nesting depth is restored after panic
}
```

**Status:** Covered by `TestTimerNestingDepthPanicRestore` in bugfix_test.go.

### 5.4 Performance Consideration

The current implementation stores `nestingLevel` per timer in the heap. This is correct but means each timer struct has an extra 4 bytes. For extremely high-volume timer scenarios, this is negligible.

---

## 6. CONCLUSION

The eventloop implementation provides **FULL COMPLIANCE** with HTML5 timer nesting specifications. The key compliance points:

| Requirement | Status |
|-------------|--------|
| Timer nesting level tracking | ✅ Compliant |
| 5-level threshold (> 5) | ✅ Compliant |
| 4ms minimum clamping | ✅ Compliant |
| Negative delay → 0 | ✅ Compliant |
| setInterval repeating | ✅ Compliant |
| Timer ID uniqueness | ✅ Compliant |
| clearTimeout/clearInterval | ✅ Compliant |
| Task source integration | ✅ Compliant |

**Overall Assessment:** The implementation correctly implements the HTML5 timer specification with minor implementation differences that do not affect behavioral correctness. The extensive test coverage in `timer_html5_compliance_test.go` and `nested_timeout_test.go` provides confidence in the implementation.

---

## Appendix A: Spec Section References

- **Section 8.6 Timers:** https://html.spec.whatwg.org/multipage/timers-and-user-prompts.html#timers
- **Timer Initialization Steps:** Section 8.6, "To perform the timer initialization steps"
- **Event Loop Processing:** https://html.spec.whatwg.org/multipage/webappapis.html#event-loop-processing-model
- **Task Source Concepts:** https://html.spec.whatwg.org/multipage/webappapis.html#generic-task-sources

## Appendix B: Test Execution Verification

All compliance tests pass:
```bash
$ gmake test.eventloop
# TestHTML5_NestedTimeoutClamping: PASS
# TestHTML5_NestingDepthTrack: PASS
# TestHTML5_ClampingThreshold: PASS
# TestNestedTimeoutClampingBelowThreshold: PASS
# TestNestedTimeoutClampingAboveThreshold: PASS
# Example_timerNesting: PASS
```
