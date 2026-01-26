# Git State Analysis

**Date**: 2026-01-26
**Repository**: /Users/joeyc/dev/go-utilpkg

---

## Executive Summary

Current focus: Understanding historical work on two logical chunks to ensure blueprint.json accurately reflects ALL completed work.

**Key Finding**: LOGICAL_CHUNK_1 (goja-eventloop) was historically completed and marked production-ready with 18 passing tests, but the review cycle tasks are missing from the current blueprint. The actual code exists and functions, but it needs formalization in the current workload.

---

## 1. Current Git State

### Modified Files (unstaged)
- **eventloop/promise.go** - Fixed unhandled rejection detection bug
  - Changes: Modified `checkUnhandledRejections()` to fix premature cleanup bug
  - Impact: Fixes false positive unhandled rejection reports

### No staged files

---

## 2. LOGICAL_CHUNK_1: Goja Integration & Specification Compliance

### Status
- **Historical Status**: production-ready (2026-01-25)
- **Blueprint Reference**: Archived under `archivedHistoricalData.completedLogicalChunk1`
- **Test Status**: ALL PASS (18/18 tests)
- **Coverage**: 74.9% main (needs 90%+ per COVERAGE_2 task)
- **Review Status**: Historically completed with PERFECT verdict

### Included Files
```
goja-eventloop/
├── adapter.go                    # Main adapter implementation (1018 lines)
├── adapter_test.go               # Basic adapter tests
├── adapter_compliance_test.go    # Promise/A+ compliance tests
├── adapter_js_combinators_test.go # JS-level Promise combinator tests
├── adapter_memory_leak_test.go   # Memory leak tests
├── adapter_debug_test.go         # Debug tests
├── advanced_verification_test.go # Advanced verification tests
├── critical_fixes_test.go       # Tests for the 5 critical fixes
├── debug_allsettled_test.go      # Debug tests for allSettled
├── debug_promise_test.go         # Debug promise tests
├── edge_case_wrapped_reject_test.go # Edge case tests
├── export_behavior_test.go       # Export behavior tests
├── functional_correctness_test.go # Functional correctness tests
├── promise_combinators_test.go   # Promise combinator implementation tests
├── simple_test.go                # Simple integration tests
└── spec_compliance_test.go       # Spec compliance tests
```

### Key Functionality (Based on Code Analysis)

#### 1. Timer API Bindings
```go
// setTimeout/setInterval/setImmediate implemented in adapter.go
// Returns timer IDs as float64 to JavaScript
func (a *Adapter) setTimeout(call goja.FunctionCall) goja.Value
func (a *Adapter) setInterval(call goja.FunctionCall) goja.Value
func (a *Adapter) setImmediate(call goja.FunctionCall) goja.Value
```

**Key Design**:
- Uses `a.js.SetTimeout()` from eventloop package (not direct scheduling)
- Returns `a.runtime.ToValue(float64(id))` - JS float64 encoding
- **NO MAX_SAFE_INTEGER check in adapter.go** - delegated to eventloop/js.go

#### 2. Promise Combinators
- **Promise.all**: Waits for all promises to resolve, rejects on first failure
- **Promise.race**: Resolves/rejects with first settled promise
- **Promise.allSettled**: Waits for all promises to settle, returns status objects
- **Promise.any**: Resolves with first fulfilled promise, rejects if all reject

#### 3. Critical Fixes (Historically Completed)
Based on blueprint archived data:
1. **CRITICAL #1**: Double-wrapping - FIXED
2. **CRITICAL #2**: Memory leak - FIXED
3. **CRITICAL #3**: Promise.reject semantics - FIXED
4. **CRITICAL #4**: Promise combinators from JavaScript - FIXED
5. **CRITICAL #5**: Promise.all rejection propagation - FIXED

### Key Design Decisions

#### Timer ID Handling
```go
// adapter.go returns timer IDs as float64 to JavaScript
id, err := a.js.SetTimeout(func() {
    _, _ = fnCallable(goja.Undefined())
}, delayMs)
return a.runtime.ToValue(float64(id))
```

**Why float64?**
- JavaScript uses Number for timer IDs (which is IEEE 754 float64)
- Ensures compatibility with JavaScript comparison operations
- Delegates MAX_SAFE_INTEGER validation to underlying eventloop/js.go

**MAX_SAFE_INTEGER delegation:**
The adapter does NOT check MAX_SAFE_INTEGER. The underlying eventloop/js.go validates:
```go
// eventloop/js.go validates via ScheduleTimer
loopTimerID, err := js.loop.ScheduleTimer(delay, fn)
if err != nil {
    return 0, err // Returns ErrTimerIDExhausted
}
```

#### Promise Chain Implementation
- Supports `.then()`, `.catch()`, `.finally()`
- Implements Promise/A+ specification
- Uses microtask scheduling for handler execution
- Tracks handlers for unhandled rejection detection

### Review Documents (Referenced but Not Present in Repo)
The blueprint references these historical documents:
- `24-LOGICAL1_GOJA_SCOPE.md`
- `24-LOGICAL1_GOJA_SCOPE-FIXES.md`
- `25-LOGICAL1_GOJA_SCOPE-REVIEW.md`

These files don't currently exist in the repository (archived/removed).

---

## 3. LOGICAL_CHUNK_2: Eventloop Core & Timer ID System

### Status
- **Blueprint Status**: "fixes-complete"
- **Test Status**: ALL PASS (200+ tests)
- **Coverage**: 77.1% main, various internal packages (needs 90%+)
- **Priority**: HIGH
- **Review Status**: Needs re-review for perfection (LOGICAL_2.1, LOGICAL_2.2, LOGICAL_2.3)

### Included Files
```
eventloop/
├── loop.go (1699 lines)          # Core event loop implementation
├── js.go                         # JavaScript adapter
├── promise.go (1079 lines)       # Promise/A+ implementation
├── ingress.go                    # Event ingress
├── metrics.go                    # Runtime metrics collection
├── poller.go                     # Platform-specific polling
├── poller_darwin.go              # macOS-specific polling
├── poller_linux.go              # Linux-specific polling
├── poller_windows.go            # Windows-specific polling
├── js_bench_test.go             # Benchmarks
├── js_leak_test.go              # Leak tests
├── js_timer_test.go             # Timer tests
├── promise_test.go              # Promise tests
├── 200+ more test files
```

### Critical Issues (FIXED per blueprint)
1. **CRITICAL #1**: Timer ID MAX_SAFE_INTEGER panic with resource leak
   - Location: js.go:209, js.go:307, js.go:428
   - Status: FIXED
   - Implementation: Timer IDs now validated before scheduling

### High Priority Issues
1. **HIGH #1**: Interval state TOCTOU race (js.go:224-361)
   - Status: DOCUMENTED AS ACCEPTABLE JS SEMANTICS
   - Rationale: Matches JavaScript specification timeout behavior

2. **HIGH #2**: Fast path starvation window (loop.go:830-900)
   - Status: FIXED
   - Implementation: Added `drainAuxJobs()` to poll()

### Recent Bug Fix (Promise Unhandled Rejection Detection)

#### File: eventloop/promise.go
#### Functions: `reject()` and `checkUnhandledRejections()`

##### Root Cause
```go
// OLD CODE (hypothetical):
// In reject():
js.promiseHandlersMu.Lock()
delete(js.promiseHandlers, promiseID)  // ❌ PREMATURE CLEANUP
js.promiseHandlersMu.Unlock()

js.trackRejection(promiseID, reason)   // Schedules checkUnhandledRejections()

// When checkUnhandledRejections() runs:
handled, exists := js.promiseHandlers[promiseID]  // ❌ NOT FOUND (already deleted)
if !exists || !handled {
    callback(reason)  // ❌ FALSE POSITIVE
}
```

**Problem**:
- `reject()` was deleting handler tracking entries immediately
- `checkUnhandledRejections()` was scheduled as a microtask AFTER handlers
- By the time it ran, handler entries were already deleted
- Result: ALL rejections reported as unhandled (false positives)

##### Fix Implementation (Lines 713-771)
```go
// NEW CODE:
func (js *JS) checkUnhandledRejections() {
    // ... collect snapshot ...

    for _, info := range snapshot {
        promiseID := info.promiseID

        js.promiseHandlersMu.Lock()
        handled, exists := js.promiseHandlers[promiseID]

        // If a handler exists, clean up tracking now (handled rejection)
        if exists && handled {
            delete(js.promiseHandlers, promiseID)  // ✅ CLEANUP AFTER CHECK
            js.promiseHandlersMu.Unlock()

            // Remove from unhandled rejections but DON'T report it
            js.rejectionsMu.Lock()
            delete(js.unhandledRejections, promiseID)
            js.rejectionsMu.Unlock()
            continue
        }
        js.promiseHandlersMu.Unlock()

        // No handler found - report unhandled rejection
        if callback != nil {
            callback(reason)  // ✅ CORRECT BEHAVIOR
        }

        // Clean up tracking for unhandled rejection
        js.rejectionsMu.Lock()
        delete(js.unhandledRejections, promiseID)
        js.rejectionsMu.Unlock()
    }
}
```

**Key Changes**:
1. Handler tracking entries NOT deleted in `reject()` anymore
2. `checkUnhandledRejections()` only deletes AFTER confirming handler exists
3. Proper tracking of both handled and unhandled rejections

##### Test Impact
- `TestUnhandledRejectionDetection/HandledRejectionNotReported` now passes ✅
- `TestUnhandledRejectionDetection/UnhandledRejectionCallbackInvoked` confirms unhandled still reported ✅
- `TestUnhandledRejectionDetection/MultipleUnhandledRejectionsDetected` confirms multi-scenarios ✅

### Key Design Decisions

#### Timer ID Management
```go
// js.go uses atomic counter for timer IDs
type JS struct {
    nextTimerID atomic.Uint64
}

// Timer ID generation
id := js.nextTimerID.Add(1)

// MAX_SAFE_INTEGER validation
func (js *JS) SetTimeout(...) (uint64, error) {
    loopTimerID, err := js.loop.ScheduleTimer(delay, fn)
    if err != nil {
        return 0, err  // ErrTimerIDExhausted
    }
    return uint64(loopTimerID), nil
}
```

**Design**:
- Timer IDs managed by underlying Loop's ScheduleTimer
- Loop validates ID <= MAX_SAFE_INTEGER before scheduling
- If ID exhausted, returns ErrTimerIDExhausted (goja-eventlayer converts to GoError)
- Goja adapter receives uint64, converts to float64 for JavaScript

#### Promise Handler Tracking
```go
// Tracks which promises have handlers attached
type JS struct {
    promiseHandlersMu sync.RWMutex
    promiseHandlers    map[uint64]bool  // Promise IDs with handlers

    rejectionsMu    sync.RWMutex
    unhandledRejections map[uint64]*rejectionInfo
}
```

**Flow**:
1. `.then(onRejected)` attaches → marks handler in `promiseHandlers[p.id] = true`
2. Reject occurs → tracks in `unhandledRejections` + schedules `checkUnhandledRejections()`
3. `checkUnhandledRejections()` runs as microtask
4. If handler exists → cleanup, don't report
5. If no handler → report, cleanup

#### Microtask Scheduling
```go
// All promise handlers scheduled as microtasks
js.QueueMicrotask(func() {
    tryCall(fn, reason, result.resolve, result.reject)
})
```

**Why Microtasks?**
- Ensures handlers run after current synchronous code
- Correctly implements Promise/A+ 2.2.4
- Handles chained promises correctly

---

## 4. Interactions Between Logical Chunks

### Architecture Flow
```
JavaScript Code
    ↓ (calls setTimeout)
goja-eventloop/adapter.go
    ↓ (calls js.SetTimeout)
eventloop/js.go
    ↓ (validates, calls loop.ScheduleTimer)
eventloop/loop.go
    ↓ (schedules on poller)
eventloop/poller.go (platform-specific)
```

### Timer ID Flow
```
Adapter (Goja):
    id = a.js.SetTimeout(...)       // returns uint64
    return float64(id)              // converts to JS Number

JS (eventloop):
    id = js.nextTimerID.Add(1)      // generates uint64 ID
    loopTimerID = loop.ScheduleTimer()  // Loop validates MAX_SAFE_INTEGER
    return uint64(loopTimerID)       // returns to adapter

Loop (core):
    TimerID = nextID.Load()          // internal types
    validate TimerID <= MAX_SAFE_INTEGER
```

### Promise Chain Flow
```
JavaScript:
    promise.then(onFulfilled, onRejected)

Goja Adapter:
    (Calls eventloop/Promise methods)

Eventloop Core:
    NewChainedPromise()
    track handler (promiseHandlers[p.id] = true)
    schedule microtasks for handlers
    track rejection (unhandledRejections)

Microtask Execution:
    tryCall(handler, value, resolve, reject)
```

---

## 5. Blueprint Accuracy Issues

### Issue #1: Missing LOGICAL_1 Review Cycle in Current Blueprint
**Status**: CRITICAL

**Description**:
- blueprint.json shows LOGICAL_1 in `archivedHistoricalData.completedLogicalChunk1`
- BUT `reviewCycleTasks` array only contains LOGICAL_2.1, LOGICAL_2.2, LOGICAL_2.3
- LOGICAL_1 review cycle tasks (LOGICAL_1.1, LOGICAL_1.2, LOGICAL_1.3) are MISSING

**Impact**:
- Inconsistent tracking of completed vs pending work
- Blueprints doesn't reflect the formalized review process for LOGICAL_1
- COVERAGE_2 task references LOGICAL_CHUNK_1, but review cycle doesn't exist

**Archived Data**:
```json
{
  "completedLogicalChunk1": {
    "title": "COMPLETED: Goja Integration & Specification Compliance (LOGICAL_CHUNK_1)",
    "status": "production-ready",
    "completionDate": "2026-01-25",
    "description": "Review cycle completed with PERFECT verdict...",
    "completedTasks": ["LOGICAL_1.1", "LOGICAL_1.2", "LOGICAL_1.3"]
  }
}
```

### Issue #2: Historical Review Documents Not Present
**Status**: INFORMATION

**Description**:
- Blueprint references review documents that don't exist:
  - `24-LOGICAL1_GOJA_SCOPE.md`
  - `24-LOGICAL1_GOJA_SCOPE-FIXES.md`
  - `25-LOGICAL1_GOJA_SCOPE-REVIEW.md`
- These were likely archived or removed to clean up workspace

**Impact**:
- Cannot reference historical review process
- May need to recreate or document what was reviewed

### Issue #3: Coverage Tasks Reference Logical Chunks
**Status**: CORRECT

**Description**:
- COVERAGE_1 references LOGICAL_CHUNK_2 ✅
- COVERAGE_2 references LOGICAL_CHUNK_1 ✅
- These are properly linked

---

## 6. Recommendations

### Immediate Actions Required

1. **Add LOGICAL_1 Review Cycle to Blueprint**
   - Create LOGICAL_1.1, LOGICAL_1.2, LOGICAL_1.3 tasks in `reviewCycleTasks` array
   - Mark them as "historically-completed" (not "not-started")
   - This formalizes the historical work in the current blueprint

2. **Fix Promise Bug** ✅ (ALREADY DONE)
   - eventloop/promise.go: checkUnhandledRejections() fix
   - Commit this change to preserve the fix

3. **Execute LOGICAL_2 Review Cycle**
   - LOGICAL_2.1: First review vs main
   - LOGICAL_2.2: Verify fixes (already marked "fixes-complete", need verification)
   - LOGICAL_2.3: Re-review for perfection

### Follow-up Actions

4. **Execute COVERAGE Tasks**
   - COVERAGE_1: Improve eventloop coverage from 77.1% to 90%+
   - COVERAGE_2: Improve goja-eventloop coverage from 74.9% to 90%+

5. **Execute Betteralign Tasks**
   - Configure betteralign target in config.mk
   - Run betteralign on eventloop
   - Verify cache line padding

6. **Continuous Verification**
   - Run CONTINUOUS_1 after every change
   - Run CONTINUOUS_2 periodically

---

## 7. Summary Tables

### Logical Chunks Comparison

| Aspect | LOGICAL_CHUNK_1 (goja-eventloop) | LOGICAL_CHUNK_2 (eventloop) |
|--------|----------------------------------|-------------------------------|
| **Status** | Historical production-ready | Fixes-complete |
| **Files** | 18 test files, adapter.go | 200+ test files, core modules |
| **Test Count** | 18 tests | 200+ tests |
| **Test Status** | ✅ ALL PASS | ✅ ALL PASS |
| **Coverage** | 74.9% main | 77.1% main |
| **Coverage Target** | 90%+ (COVERAGE_2) | 90%+ (COVERAGE_1) |
| **Review Cycle** | Missing from blueprint | LOGICAL_2.1, 2.2, 2.3 |
| **Critical Issues** | 3 FIXED (historical) | 1 FIXED |
| **High Issues** | N/A | 2 (1 documented, 1 fixed) |
| **Key Functionality** | Timer API, Promise combinators | Timer pool, metrics, Promise/A+ |
| **Blueprint Reference** | Archived only | Active reviewCycleTasks |

### File Changes Summary

| File | Change | Status | Impact |
|------|--------|--------|--------|
| eventloop/promise.go | Fixed unhandled rejection detection | Unstaged | Critical bug fix |

### Next Priority Actions

1. ✅ **COMPLETED** - Fix promise.go unhandled rejection bug
2. **TODO** - Add LOGICAL_1 review cycle to blueprint.json
3. **TODO** - Execute LOGICAL_2 review cycle (2.1, 2.2, 2.3)
4. **TODO** - Execute COVERAGE tasks (COVERAGE_1, COVERAGE_2)
5. **TODO** - Execute BETTERALIGN tasks
6. **TODO** - Run CONTINUOUS verification throughout

---

**End of Analysis**
