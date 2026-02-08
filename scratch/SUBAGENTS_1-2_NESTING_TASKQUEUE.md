# SUBAGENT 1: TIMER NESTING & CLAMPING

**Investigative Focus**: "timer nesting levels" and "clamping" compliance

**Spec Reference**: https://html.spec.whatwg.org/multipage/timers-and-user-prompts.html

---

## SUCCINCT SUMMARY

The eventloop implementation has a CRITICAL COMPLIANCE GAP in timer nesting depth tracking: the global `timerNestingDepth` is incremented during timer EXECUTION, not during SCHEDULING. This causes sequential `setTimeout(fn, 0)` chains (depth=1→2→3...) to NOT receive 4ms clamping, violating HTML spec Section 8.6 step 5 which requires incrementing nesting level AFTER queueing (step 10) but BEFORE clamping (step 5), meaning sequential timers from the same call stack should increment depth between each scheduling. Additionally, `setInterval` wrapper rescheduling does NOT increment nesting level despite spec step 11 requiring full timer initialization re-execution.

---

## DETAILED ANALYSIS

### 1. SPECIFICATION REQUIREMENTS (Section 8.6)

The HTML spec defines timer initialization steps (paraphrased):
- **Step 3**: Determine nesting level - if current task created by timer algorithm, use that task's timer nesting level; otherwise 0
- **Step 4**: Negative timeout → 0
- **Step 5**: If nesting level > 5 AND timeout < 4ms → timeout = 4ms
- **Step 10**: Increment nesting level by 1
- **Step 11**: Set task's timer nesting level to nesting level
- **Step 12**: Queue task on timer task source
- **For setInterval repeats (step 11)**: Re-run timer initialization steps again

**Critical Temporal Order**: Step 5 (clamping check) MUST use the nesting level BEFORE Step 10 (increment), but the timer must already be queued or the depth must be tracked globally for sequential timers to work.

### 2. IMPLEMENTATION ANALYSIS

**loop.go ScheduleTimer (lines 1650-1670)**:
```go
func (l *Loop) ScheduleTimer(delay time.Duration, fn func()) (TimerID, error) {
    currentDepth := l.timerNestingDepth.Load()
    if currentDepth > 5 {
        minDelay := 4 * time.Millisecond
        if delay >= 0 && delay < minDelay {
            delay = minDelay
        }
    }
    // ... scheduling
    t.nestingLevel = currentDepth  // Store for later execution
    // ...
}
```

**loop.go runTimers (lines 1611-1634)**:
```go
if !t.canceled.Load() {
    oldDepth := l.timerNestingDepth.Load()
    newDepth := t.nestingLevel + 1
    l.timerNestingDepth.Store(newDepth)
    defer l.timerNestingDepth.Store(oldDepth)
    l.safeExecute(t.task)
    // ...
}
```

### 3. IDENTIFIED COMPLIANCE GAPS

#### GAP 1: Sequential Timer Scheduling (CRITICAL)

When timers are scheduled sequentially (not nested):
```javascript
setTimeout(() => {
    setTimeout(() => {  // Depth should be 2
        setTimeout(() => {  // Depth should be 3
            // ...
        }, 0);
    }, 0);
}, 0);
```

**Expected Behavior (per spec)**:
- Timer 1: nesting=0 → clamped? NO (0 ≤ 5)
- Timer 2: nesting=1 → clamped? NO (1 ≤ 5)
- Timer 3: nesting=2 → clamped? NO (2 ≤ 5)
- Timer 4: nesting=3 → clamped? NO (3 ≤ 5)
- Timer 5: nesting=4 → clamped? NO (4 ≤ 5)
- Timer 6: nesting=5 → clamped? NO (spec says "> 5", not ">= 5")
- Timer 7: nesting=6 → clamped? YES (6 > 5, timeout=0 < 4ms → 4ms)
- Timer 8: nesting=7 → clamped? YES
- ...

**Actual Implementation Behavior**:
- Timer 1: currentDepth=0, stores nestingLevel=0, executes, sets depth=1
- Timer 2: currentDepth=1, stores nestingLevel=1, executes, sets depth=2
- Timer 3: currentDepth=2, stores nestingLevel=2, executes, sets depth=3
- Timer 4: currentDepth=3, stores nestingLevel=3, executes, sets depth=4
- Timer 5: currentDepth=4, stores nestingLevel=4, executes, sets depth=5
- Timer 6: currentDepth=5, stores nestingLevel=5, executes, sets depth=6
- Timer 7: currentDepth=6, stores nestingLevel=6, clamped to 4ms
- ...

**PROBLEM**: Timer 6 should NOT be clamped (nesting=5, not >5), but implementation clamps it because it increments AFTER execution, not during scheduling. When Timer 7 is scheduled, depth is already 6 from Timer 6's execution.

**Spec Analysis**: The spec's step 3 says "nesting level be the task's timer nesting level" - meaning the level at which the timer was SCHEDULED, not the depth during execution. Sequential timers from the same call stack SHOULD increment the global depth between each scheduling to properly track "nested invocations."

**Correct Fix**: Increment `timerNestingDepth` atomically during ScheduleTimer BEFORE clamping.

#### GAP 2: setInterval Repeating Behavior (CRITICAL)

**Spec Step 11**: "If repeat is true, then perform the timer initialization steps again, given global, handler, timeout, arguments, true, and id."

**Implementation** (js.go SetInterval wrapper):
```go
wrapper = func() {
    // ... execute callback ...
    loopTimerID, err := js.loop.ScheduleTimer(state.getDelay(), wrapper)
    // ...
}
```

**Problem**: The wrapper just reschedules itself via ScheduleTimer. When it fires again, it uses the CURRENT `timerNestingDepth` at that moment. But per spec, the repeat should go through "timer initialization steps" again, which INCLUDES incrementing the nesting level (step 10).

**Expected per spec**: Each setInterval iteration should increment the nesting level by 1, because it's a "nested invocation of this algorithm" (per spec note).

**Actual behavior**: setInterval maintains constant nesting level across iterations.

#### GAP 3: Clamping Threshold Logic

Current implementation:
```go
if currentDepth > 5 {
    minDelay := 4 * time.Millisecond
    if delay >= 0 && delay < minDelay {
        delay = minDelay
    }
}
```

**Question**: Is the condition `delay >= 0 && delay < minDelay` correct?

Spec step 4: "If timeout is less than 0, then set timeout to 0."
Spec step 5: "If nesting level is greater than 5, and timeout is less than 4, then set timeout to 4."

**Correct order**:
1. If timeout < 0 → timeout = 0
2. If nesting > 5 AND timeout < 4 → timeout = 4

This means after step 4, timeout is always ≥ 0. So step 5's condition is effectively "timeout < 4".

**Current implementation checks `delay >= 0 && delay < minDelay`**:
- If delay is negative, it's already 0 (Go time.Duration)
- If delay is 0, condition passes → clamp to 4ms
- If delay is 1-3ms, condition passes → clamp to 4ms
- If delay is 4ms, condition fails → no clamp

**This is CORRECT** given Go's time.Duration type cannot be negative.

#### GAP 4: Timer ID Namespace

**Spec**: "Let id be an implementation-defined integer that is greater than zero and does not already exist in global's map of setTimeout and setInterval IDs."

**Implementation**: Uses separate internal maps (`js.intervals` vs `js.timers` via Loop.timerMap) with shared `JS.nextTimerID` counter.

**Impact**: NONE - IDs remain unique, just stored separately. Minor implementation detail.

#### GAP 5: Unique Handle Validation

**Spec**: Uses "unique internal value" handle for validation on execution.

**Implementation**: Uses TimerID directly, removes from timerMap on execution.

**Impact**: LOW - Both prevent execution of cleared timers. Timer ID reuse handled by immediate removal.

### 4. goja-eventloop ADAPTER

The goja-eventloop adapter wraps the core eventloop implementation and inherits all its behavior.

**Compliance inherited**: ✅ All compliance gaps in core eventloop apply to goja-eventloop.

### 5. TEST COVERAGE GAPS

**Missing Test**: Sequential setTimeout chain with deep nesting (>7)
```go
func TestSequentialDeepNestingClamping(t *testing.T) {
    // Schedule 10 sequential setTimeout(fn, 0)
    // Verify timers 1-6 execute without 4ms clamping
    // Verify timers 7-10 execute WITH 4ms clamping
}
```

**Missing Test**: setInterval nesting accumulation
```go
func TestSetIntervalNestingAccumulation(t *testing.T) {
    // Start setInterval at depth 5
    // Verify 5th iteration causes clamping (nesting=10)
}
```

**Existing test `TestHTML5_ClampingThreshold`** sets `timerNestingDepth.Store(tc.depth)` directly before scheduling. This bypasses the increment mechanism entirely, testing only the clamping logic in isolation.

### 6. SUMMARY TABLE

| Aspect | Spec Requirement | Implementation | Gap Level |
|--------|-----------------|----------------|----------|
| Nesting depth for sequential setTimeout | Increment during scheduling | Increment during execution | CRITICAL |
| Nesting depth for setInterval repeats | Increment on each repeat | Constant depth | CRITICAL |
| Clamping threshold ("> 5") | Correctly implemented | Correct | NONE |
| Negative timeout → 0 | Correctly implemented | Correct (Go type) | NONE |
| Timer ID uniqueness | Correctly implemented | Correct | NONE |
| Unique handle validation | Uses internal handle | Uses TimerID | LOW (equivalent) |
| setInterval re-init steps | Full re-initialization | Wrapper reschedule | MINOR (affects nesting) |

---

## SUBAGENT 2: TASK QUEUE & ORDERING

**Investigative Focus**: "task queue ordering" and "task sourcing" compliance

---

## SUCCINCT SUMMARY

The eventloop implementations have a SIGNIFICANT DEVIATION from the HTML spec: timers use an internal timer heap instead of a proper task queue with task source separation. The spec mandates FIFO ordering within each task source and explicit "currently running task" tracking for nesting detection, but the Go implementation uses a timer heap ordered by fire time only, which can violate ordering when timers fire simultaneously or when microtask ordering flags trigger intermediate microtask drains. Additionally, the setTimeout callback mechanism lacks the spec's uniqueHandle/ID validation steps.

---

## DETAILED ANALYSIS

### 1. TASK SOURCE COMPLIANCE (Major Non-Compliance)

The WHATWG spec Section 8.1.7.1 defines:
- "Each task is defined as coming from a specific task source"
- "For each event loop, every task source must be associated with a specific task queue"
- Timer tasks come from the "timer task source"

**Current Implementation Issues**:
- `eventloop/loop.go` uses a `timerHeap` (min-heap ordered by `when` time) - NOT a task queue
- No task source tracking or separation exists
- All timers mix into a single heap without FIFO guarantees for same-time timers
- `ScheduleTimer()` at line 1689 pushes to heap, `runTimers()` at line 1594 pops in fire-time order only

**Specification Gap**: The spec requires timers to be queued via "queue a global task on the timer task source" (Section 8.6, step 12), ensuring task source provenance and enabling proper task queue selection.

### 2. TASK QUEUE ORDERING REQUIREMENTS (Non-Compliance)

Spec Section 8.1.7.3 mandates:
- Tasks are dequeued FIFO from the selected queue
- "The microtask queue is not a task queue"
- Multiple task sources can be coalesced into task queues, but ordering within each source must be preserved

**Implementation Behavior**:
- Timers fire strictly by `when` time in `runTimers()` 
- `StrictMicrotaskOrdering` flag causes intermediate `drainMicrotasks()` calls between timers (lines 1599, 1032-1034)
- This creates observable ordering differences from spec when multiple timers expire simultaneously
- No mechanism to preserve creation-order for same-expiration-time timers

### 3. TIMER TASK CREATION ALGORITHM (Steps 6-11 Non-Compliance)

Section 8.6 "set a timeout" steps 6-11 specify critical validation:
- Step 6-8: Capture realm, initiating script, generate uniqueHandle
- **Step 9-11**: Task creation with uniqueHandle for ID validation
- Step 10: "If id does not exist... abort these steps"
- Step 11: Validate uniqueHandle matches before execution

**Current Implementation**:
```go
// loop.go ScheduleTimer (line 1652)
func (l *Loop) ScheduleTimer(delay time.Duration, fn func()) (TimerID, error) {
    // No uniqueHandle capture
    // No initiating script tracking
    // No realm capture
    // Simply schedules function - no ID validation at execution time
}
```

**Missing Critical Validation**:
- No uniqueHandle/internal value for ID validation
- Timer can execute even if cleared during execution window
- No equivalent of "if global's map of setTimeout...IDs[id] != uniqueHandle, abort"

### 4. CURRENTLY RUNNING TASK TRACKING (Partial Implementation)

Spec Section 8.1.7.1: "Each event loop has a currently running task... initially null"

**Implementation**:
```go
// loop.go - no explicit currentlyRunningTask tracking
// However, timerNestingDepth is tracked:
timerNestingDepth atomic.Int32 // HTML5 spec: nesting depth for timeout clamping
```

**Issues**:
- `timerNestingDepth` in `ScheduleTimer()` approximates nesting (lines 1656-1658)
- But actual "currently running task" tracking per spec doesn't exist
- The spec uses this to detect nested timer calls (step 3 of timer algorithm)
- Implementation relies on depth counter, not task identity

### 5. ORDERING GUARANTEES BETWEEN MULTIPLE TIMERS (Non-Compliant)

Spec requires: Timers scheduled in same execution context maintain FIFO ordering

**Implementation Reality**:
- `timerHeap.Less()` at line 154 compares only by `when` time
- Two `setTimeout(fn, 100)` calls created 1ms apart will fire in heap order, not creation order
- `ChunkedIngress` external queue uses FIFO but timers bypass it entirely
- setImmediate at line 176 (js.go) bypasses timer heap but creates its own ordering issues

### 6. RELATIONSHIP WITH OTHER TASK SOURCES (Design Flaw)

Spec recognizes multiple task sources:
- DOM manipulation task source
- User interaction task source  
- Networking task source
- Timer task source
- JavaScript engine task source

**Current Implementation**:
```go
// All merged into single processing order:
func (l *Loop) tick() {
    l.runTimers()      // Timer tasks
    l.processInternalQueue()  // Internal tasks
    l.processExternal()  // External tasks
    l.drainMicrotasks()  // Microtasks
    l.poll()             // I/O
}
```

**Critical Missing Distinction**:
- Timer tasks should be queued via "queue a global task on timer task source"
- Currently timers run BEFORE other task source tasks (if both pending)
- This violates spec's task queue selection mechanism (implementation-defined queue choice)
- The spec allows choosing queue order, but not within a task source

### 7. CUSTOM SCHEDULING vs TASK QUEUES (Architecture Violation)

The implementation uses CUSTOM scheduling architecture:
- `timerHeap` - min-heap by fire time (not FIFO queue)
- `MicrotaskRing` - separate microtask queue (correct)
- `ChunkedIngress` external/internal - FIFO queues (correct)
- `FastPathMode` - bypasses normal tick for performance

**Problems**:
- Timer task source NOT implemented as proper task queue
- Missing `task.source` field tracking per spec task struct
- Missing `task.document` and `task.scriptEvaluationEnvironmentSettingsObjectSet` fields
- These fields affect task runnability and agent isolation per spec

### 8. GOJA-EVENTLOOP ADAPTER LAYER ISSUES

`goja-eventloop/adapter.go` at line 198:
```go
func (a *Adapter) setTimeout(call goja.FunctionCall) goja.Value {
    id, err := a.js.SetTimeout(fn, delayMs)
    // Returns float64 directly - loses timer ID type semantics
}
```

**Issues**:
- No validation of timer ID before clearing
- setInterval creates wrapper that captures closure, but timer firing check at line 224-229 lacks spec's uniqueHandle validation
- No race condition handling equivalent to spec's uniqueHandle check

### 9. NESTING DEPTH CLAMPING (Partial Implementation)

Spec step 5: "If nesting level is > 5 and timeout < 4, set timeout to 4"

**Implementation**:
```go
// loop.go ScheduleTimer line 1657
if currentDepth > 5 {
    minDelay := 4 * time.Millisecond
    if delay >= 0 && delay < minDelay {
        delay = minDelay
    }
}
```

Correctly implements nesting depth clamping - GOOD

### 10. MICROTASK TIMING (Correct)

Spec Section 8.7: "queueMicrotask()... runs once JavaScript execution context stack is next empty"

**Implementation**:
- `ScheduleMicrotask()` correctly routes to `MicrotaskRing`
- Microtasks processed at specific points in tick (lines 1599, 1034, 1146)
- `StrictMicrotaskOrdering` flag enables microtask drain after each timer - extends spec behavior

### SUMMARY OF NON-COMPLIANCE

**CRITICAL GAPS**:
1. ❌ Timer task source NOT implemented as proper task queue (uses timerHeap)
2. ❌ No FIFO guarantee for timers with same fire time
3. ❌ Missing uniqueHandle/ID validation mechanism from spec step 9-11
4. ❌ No "currently running task" tracking for timer nesting detection
5. ❌ No task.source, task.document, task.scriptEvaluationEnvironmentSettingsObjectSet tracking
6. ❌ Custom scheduling bypasses spec's "queue a global task on timer task source" algorithm

**PARTIAL COMPLIANCE**:
- ✓ Nesting depth clamping (>5 → 4ms) implemented
- ✓ Microtask queue processing correct
- ✓ Basic timer scheduling with delays

**ARCHITECTURAL MISMATCH**:
The implementation prioritizes PERFORMANCE (min-heap for O(log n) scheduling) over SPEC COMPLIANCE. While functional for most use cases, it will exhibit observable differences from browser behavior in edge cases involving:
- Rapid timer creation with same delays
- Timer clearing during execution window
- Task source interleaving expectations
- Cross-agent timer isolation
