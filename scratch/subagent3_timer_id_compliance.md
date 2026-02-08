# Timer ID Management Compliance Analysis
## WHATWG HTML Spec Section 8.6 Timers

**Date:** 9 February 2026  
**Investigator:** Takumi (匠)  
**Reference:** https://html.spec.whatwg.org/multipage/timers-and-user-prompts.html

---

## 1. SUCCINCT SUMMARY

The eventloop implementation has **CRITICAL COMPLIANCE GAPS** in timer ID management. While it correctly generates unique positive integer timer IDs and implements basic clearTimeout/clearInterval functionality, it **completely lacks** the spec-mandated "map of setTimeout and setInterval IDs" with its associated "unique internal value" (handle) mechanism. This omission means the two-stage verification at timer execution time—designed to prevent race conditions when IDs are reused after clearing—is not implemented, creating potential for incorrect timer execution in concurrent scenarios.

---

## 2. DETAILED ANALYSIS

### 2.1 Timer ID Generation (Section 8.6, Step 2)

**Spec Requirement:**
> "Let id be an implementation-defined integer that is greater than zero and does not already exist in global's map of setTimeout and setInterval IDs."

**Analysis:**
- ✅ Implementation uses `atomic.Uint64` counters (`nextTimerID`) for sequential ID generation
- ✅ IDs start at 1 (verified in `TestHTML5_TimerIDsStartFromOne`)
- ✅ IDs never repeat within their namespace (atomic increment guarantees this)
- ✅ Maximum ID is capped at `MAX_SAFE_INTEGER` (2^53 - 1) to prevent float precision issues

**Implementation Reference:**
```go
// js.go:124
nextTimerID atomic.Uint64

// js.go:336 (SetInterval)
id := js.nextTimerID.Add(1)
if id > maxSafeInteger {
    return 0, ErrIntervalIDExhausted
}

// loop.go:1780 (ScheduleTimer - SetTimeout path)
t.id = TimerID(l.nextTimerID.Add(1))
```

---

### 2.2 Unique Handle Mechanism (Section 8.6, Introductory Paragraph)

**Spec Requirement:**
> "Objects that implement the WindowOrWorkerGlobalScope mixin have a map of setTimeout and setInterval IDs, which is an ordered map, initially empty. Each key in this map is a positive integer, corresponding to the return value of a setTimeout() or setInterval() call. Each value is a unique internal value, corresponding to a key in the object's map of active timers."

**From common-microsyntaxes.html (§2.3.11):**
> "A unique internal value is a value that is serializable, comparable by value, and never exposed to script. To create a new unique internal value, return a unique internal value that has never previously been returned by this algorithm."

**Analysis:**
- ❌ **CRITICAL GAP**: Implementation does NOT have the "map of setTimeout and setInterval IDs"
- ❌ **CRITICAL GAP**: No "unique internal value" (uniqueHandle) is created or stored
- ❌ **CRITICAL GAP**: No separate "map of active timers" keyed by uniqueHandle
- The spec requires TWO maps with a two-level relationship:
  1. `map of setTimeout/setInterval IDs`: ID → uniqueHandle
  2. `map of active timers`: uniqueHandle → expiry time

**Spec-Compliant Structure:**
```
map of setTimeout/setInterval IDs:
  ID=1 → uniqueHandle=0xABC123
  ID=2 → uniqueHandle=0xDEF456

map of active timers:
  uniqueHandle=0xABC123 → expiryTime=1234567890ms
  uniqueHandle=0xDEF456 → expiryTime=1234567900ms
```

**Actual Implementation Structure:**
```
timerMap (loop.go):
  TimerID=1 → *timer{task, when, nestingLevel, ...}
  TimerID=2 → *timer{...}

intervals (js.go):
  uint64=1 → *intervalState{fn, delayMs, ...}
  uint64=2 → *intervalState{...}
```

---

### 2.3 Two-Stage Verification at Timer Execution (Section 8.6, Step 9)

**Spec Requirement:**
The spec defines two verification steps **within the task closure** (step 9):

```
9. Let task be a task that runs the following substeps:
   ...
   2. If id does not exist in global's map of setTimeout and setInterval IDs, then abort these steps.
   3. If global's map of setTimeout and setInterval IDs[id] does not equal uniqueHandle, then abort these steps.
   
   Note: This accommodates for the ID having been cleared by a clearTimeout() or clearInterval() call,
   and being reused by a subsequent setTimeout() or setInterval() call.
```

**And again at step 19:**
```
19. If id does not exist in global's map of setTimeout and setInterval IDs, then abort these steps.
20. If global's map of setTimeout and setInterval IDs[id] does not equal uniqueHandle, then abort these steps.

Note: The ID might have been removed via the author code in handler calling clearTimeout() or clearInterval().
Checking that uniqueHandle isn't different accounts for the possibility of the ID, after having been cleared,
being reused by a subsequent setTimeout() or setInterval() call.
```

**Analysis:**
- ❌ **CRITICAL GAP**: The two-stage verification is NOT implemented
- The spec requires:
  1. Check if ID still exists in the map
  2. Check if the stored uniqueHandle matches our captured uniqueHandle
- Only if BOTH checks pass should the timer execute
- This prevents the "clear → reuse → old callback fires" race condition

**Implementation Reality:**
```go
// loop.go:runTimers() - No two-stage verification!
func (l *Loop) runTimers() {
    now := l.CurrentTickTime()
    for len(l.timers) > 0 {
        if l.timers[0].when.After(now) {
            break
        }
        t := heap.Pop(&l.timers).(*timer)

        // Only checks if timer was canceled - NOT spec-compliant verification!
        if !t.canceled.Load() {
            l.safeExecute(t.task)
        }
        // ...
    }
}
```

---

### 2.4 The clearTimeout → setTimeout Reuse Race

**Race Condition Scenario (per spec design):**

```
Time    Thread A                          Thread B
----    --------                          --------
t0      Timer #5 fires (saved uniqueHandle=0xABC)
t1                                        clearTimeout(5)
t2                                        - removes ID=5 from map
t3                                        setTimeout(fn, 100)
t4                                        - ID=5 reused (now maps to 0xDEF)
t5      Timer task continues execution
t6      Check 1: "ID=5 exists?" → YES
t7      Check 2: "map[5] == 0xABC?" → NO! (it's 0xDEF now)
t8      → ABORT (correctly skip old callback!)
```

**Without uniqueHandle verification:**
```
Time    Thread A                          Thread B
----    --------                          --------
t0      Timer #5 fires (closure has id=5)
t1                                        clearTimeout(5)
t2                                        - removes from timerMap
t3                                        setTimeout(fn, 100)
t4                                        - ID=5 reused in timerMap
t5      Timer task continues
t6      Check: "timer exists?" → YES (new timer with id=5)
t7      → EXECUTES OLD CALLBACK (WRONG!)
```

**Implementation Status:**
- ❌ This race condition is NOT properly protected against
- The current implementation relies on the timer being removed from `timerMap` before a new timer could fire, but this is NOT guaranteed

---

### 2.5 Difference Between "map of setTimeout and setInterval IDs" and "map of active timers"

**Map of setTimeout and setInterval IDs:**
- Purpose: Track all timers created by user code that haven't been cleared
- Keys: Timer IDs (positive integers, returned to user)
- Values: Unique internal values (handles for timer instances)
- Behavior: User-facing; cleared when `clearTimeout/clearInterval` called

**Map of Active Timers:**
- Purpose: Track timers that are currently scheduled to fire
- Keys: Unique internal values (handles)
- Values: Expiry times (DOMHighResTimeStamp)
- Behavior: Internal to the timer scheduling system

**Critical Difference:**
The separation ensures that:
1. `clearTimeout(id)` removes the mapping in the **first map** only
2. The **second map** tracks when timers should actually fire
3. The two-stage verification uses **both maps** to ensure correctness

**Implementation Confusion:**
The current implementation conflates these into `timerMap[TimerID]*timer`, which:
- ✅ Stores timer state for cancellation
- ✅ Tracks expiry times via `timer.when`
- ❌ Does NOT provide the uniqueHandle abstraction needed for two-stage verification

---

## 3. IMPLEMENTATION FINDINGS

### 3.1 File References

| File | Finding |
|------|---------|
| `js.go:124` | `nextTimerID atomic.Uint64` - no uniqueHandle mechanism |
| `js.go:280-330` | `SetTimeout` - no map of setTimeout/setInterval IDs |
| `js.go:333-450` | `SetInterval` - no uniqueHandle mechanism |
| `js.go:452-520` | `ClearTimeout/ClearInterval` - only removes from timerMap/intervals |
| `loop.go:126` | `nextTimerID atomic.Uint64` - no uniqueHandle mechanism |
| `loop.go:1773-1815` | `ScheduleTimer` - creates timer directly, no uniqueHandle |
| `loop.go:1645-1700` | `runTimers` - only checks `canceled.Load()`, not uniqueHandle |
| `loop.go:1820-1860` | `CancelTimer` - removes from timerMap, no handle verification |

### 3.2 Test Coverage Analysis

**Existing Tests:**
- ✅ `TestHTML5_TimerIDUniqueness` - verifies IDs don't repeat
- ✅ `TestHTML5_ClearTimeoutWorks` - basic clear functionality
- ✅ `TestHTML5_ClearTimeoutIdempotent` - multiple clears
- ✅ `TestHTML5_ConcurrentTimerOperations` - thread safety
- ❌ **NO TEST** for clearTimeout → setTimeout ID reuse race
- ❌ **NO TEST** for two-stage verification
- ❌ **NO TEST** for uniqueHandle mechanism

### 3.3 Correctly Implemented Features

1. **Positive Integer IDs**: IDs are always > 0
2. **ID Uniqueness**: Atomic counters ensure no collisions
3. **MAX_SAFE_INTEGER Cap**: Prevents float64 precision loss
4. **Basic clearTimeout**: Works for most common cases
5. **Nesting Depth Tracking**: `timerNestingDepth` implemented
6. **Delay Clamping**: 4ms minimum for nested timers > 5 levels

---

## 4. IDENTIFIED COMPLIANCE GAPS

### GAP 1: Missing "map of setTimeout and setInterval IDs"

**Severity:** CRITICAL  
**Impact:** Two-stage verification cannot be implemented

**Description:**
The spec requires an ordered map where:
- Keys are timer IDs (positive integers)
- Values are unique internal values

**Current State:**
No such map exists. Timer IDs are tracked only in:
- `timerMap[TimerID]*timer` (for one-shot timers)
- `intervals[uint64]*intervalState` (for intervals)

**Required Change:**
Add `setTimeoutSetIntervalMap map[TimerID]uniqueHandle` to the Loop struct

---

### GAP 2: Missing "unique internal value" (uniqueHandle)

**Severity:** CRITICAL  
**Impact:** Cannot prevent clearTimeout → setTimeout race

**Description:**
The spec requires generating a "unique internal value" that:
- Is serializable
- Is comparable by value
- Is never exposed to script
- Has never been returned before

**Current State:**
No uniqueHandle is generated. The spec's step 13:
```html
13. Set uniqueHandle to the result of running steps after a timeout given global, "setTimeout/setInterval", timeout, and completionStep.
```

Is not implemented. Instead, the code directly creates timers.

**Required Change:**
Implement uniqueHandle generation that returns never-before-seen values

---

### GAP 3: Missing Two-Stage Verification at Timer Execution

**Severity:** CRITICAL  
**Impact:** Race condition allows cleared timers to fire

**Description:**
At timer execution time, the spec requires:
```html
2. If id does not exist in global's map of setTimeout and setInterval IDs, then abort these steps.
3. If global's map of setTimeout and setInterval IDs[id] does not equal uniqueHandle, then abort these steps.
```

**Current State:**
The `runTimers()` function only checks:
```go
if !t.canceled.Load() {
    l.safeExecute(t.task)
}
```

This is insufficient because:
- `canceled` flag might not be set before new timer reuses ID
- Even if set, a race window exists between clear and execution

**Required Change:**
Add uniqueHandle verification in `runTimers()` before executing timer callbacks

---

### GAP 4: Missing "map of active timers"

**Severity:** MEDIUM  
**Impact:** Incomplete spec compliance, though less critical

**Description:**
The spec requires a second map keyed by uniqueHandle tracking expiry times.

**Current State:**
Expiry times are stored in `timer.when` field, not in a separate map.

**Impact:**
- Low impact on correctness (data is stored, just differently)
- High impact on spec compliance verification
- May affect debuggability and spec-conformance testing

---

## 5. RECOMMENDATIONS

### 5.1 High Priority: Implement Two-Stage Verification

Add the uniqueHandle mechanism to prevent race conditions:

```go
// Add to Loop struct
uniqueHandleCounter atomic.Uint64
timerHandles map[TimerID]uint64  // map of setTimeout/setInterval IDs

// Add to timer struct
handle uint64  // uniqueHandle for this timer instance

func (l *Loop) ScheduleTimer(...) (TimerID, error) {
    // Generate unique handle
    handle := l.uniqueHandleCounter.Add(1)
    
    // Create timer with handle
    t := &timer{
        id: TimerID(l.nextTimerID.Add(1)),
        handle: handle,
        when: ...,
        task: ...,
    }
    
    // Store handle mapping
    l.timerHandles[t.id] = t.handle
    
    // ... rest of scheduling
}

func (l *Loop) CancelTimer(id TimerID) error {
    // Remove handle mapping FIRST
    handle, exists := l.timerHandles[id]
    if !exists {
        return ErrTimerNotFound
    }
    delete(l.timerHandles, id)
    
    // Then mark timer as canceled
    // ...
}

func (l *Loop) runTimers() {
    // TWO-STAGE VERIFICATION before execution
    t := heap.Pop(&l.timers).(*timer)
    
    // Stage 1: Check if ID still exists
    storedHandle, exists := l.timerHandles[t.id]
    if !exists {
        return  // Timer was cleared, abort
    }
    
    // Stage 2: Check if handle matches
    if storedHandle != t.handle {
        return  // ID was reused, abort
    }
    
    // Now safe to execute
    l.safeExecute(t.task)
}
```

---

### 5.2 Medium Priority: Add Compliance Tests

Create tests for the race condition scenarios:

```go
func TestClearTimeout_IDReuseRace(t *testing.T) {
    // Test that cleared timer doesn't fire after ID is reused
    loop := ...
    js := ...
    
    var executionCount atomic.Int32
    
    // Create first timer
    id1, _ := js.SetTimeout(func() {
        executionCount.Add(1)
    }, 50)
    
    // Clear it
    js.ClearTimeout(id1)
    
    // Immediately create new timer with potentially reused ID
    id2, _ := js.SetTimeout(func() {
        executionCount.Add(1)
    }, 50)
    
    // Wait for timers
    time.Sleep(200 * time.Millisecond)
    
    // Should NOT execute callback from original timer
    if executionCount.Load() > 1 {
        t.Errorf("Cleared timer executed after ID reuse: count=%d", executionCount.Load())
    }
}
```

---

### 5.3 Low Priority: Document Compliance Status

Add documentation noting current compliance status:

```go
// Timer Compliance Notes:
// 
// This implementation aims for HTML5 spec compliance per:
// https://html.spec.whatwg.org/multipage/timers-and-user-prompts.html
//
// KNOWN DEVIATIONS:
// 1. No "unique internal value" mechanism - uses timer pointer directly
// 2. Two-stage verification uses timer.canceled flag instead of handle matching
// 3. Map of active timers is merged with timerMap structure
```

---

### 5.4 Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Cleared timer fires | Medium (under concurrent load) | High (data corruption, logic errors) | Implement uniqueHandle mechanism |
| ID collision | Low (atomic counters) | High | Already mitigated with atomic counters |
| Memory leak | Low | Medium | Current implementation cleans up properly |
| Timing issues | Low | Low | 4ms clamping and nesting depth implemented |

---

## 6. CONCLUSION

The eventloop implementation has **critical compliance gaps** regarding the unique handle mechanism and two-stage verification required by the WHATWG HTML spec. While basic timer functionality works, the absence of the "map of setTimeout and setInterval IDs" with its associated uniqueHandle tracking means the implementation is **not spec-compliant** and has potential race conditions under concurrent use.

**Immediate action recommended**: Implement the uniqueHandle mechanism and two-stage verification to prevent the clearTimeout → setTimeout ID reuse race condition.

---

## 7. REFERENCES

1. WHATWG HTML Living Standard - Section 8.6 Timers  
   URL: https://html.spec.whatwg.org/multipage/timers-and-user-prompts.html

2. Infra Standard - Ordered Maps  
   URL: https://infra.spec.whatwg.org/#ordered-map

3. HTML Standard - Unique Internal Values  
   URL: https://html.spec.whatwg.org/multipage/common-microsyntaxes.html#unique-internal-value

4. ECMAScript - MAX_SAFE_INTEGER  
   URL: https://tc39.es/ecma262/#sec-number.max_safe_integer
