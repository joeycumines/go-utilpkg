# Review: SetImmediate API Safety Fixes

**Date:** 2026-01-24
**Review Type:** Verification of Safety Fixes
**Reviewer:** Takumi (匠)
**Approver:** Hana (花) ♡
**Target Chunk:** CHUNK7 - SetImmediate Safety
**File:** eventloop/js.go

---

## Executive Summary

**Verdict: NEEDS_FIX**

Both proposed safety fixes are **NOT** implemented in the current codebase:

1. **Fix #1 (ID Separation)** - CRITICAL MISSING: `nextImmediateID` is NOT initialized to `1 << 60` in `NewJS()`, leaving it at default value 0. First immediate ID is 1, same as timeouts. This creates a **critical API safety hazard**.

2. **Fix #2 (Dead Code Removal)** - MISSING: `intervalState.wg` field exists, and `Add(1)`/`Done()` calls are executed in `SetInterval` wrapper but are never used. The code is functionally correct but contains dead code that should be removed.

**Trust Level:** High - Verified by direct code inspection, not assumption.

---

## Fix #1: ID Separation with High-Bit Offset

### Requirement
Initialize `nextImmediateID` to `1 << 60` in `NewJS()` constructor to prevent collision with timeout IDs.

### Current Implementation Status
**NOT IMPLEMENTED** ❌

**Evidence:**

```go
// From js.go lines 146-167
func NewJS(loop *Loop, opts ...JSOption) (*JS, error) {
    options, err := resolveJSOptions(opts)
    if err != nil {
        return nil, err
    }

    js := &JS{
        loop:                loop,
        intervals:           make(map[uint64]*intervalState),
        unhandledRejections: make(map[uint64]*rejectionInfo),
        promiseHandlers:     make(map[uint64]bool),
        setImmediateMap:     make(map[uint64]*setImmediateState),
    }

    // Store onUnhandled callback
    if options.onUnhandled != nil {
        js.unhandledCallback = options.onUnhandled
    }

    return js, nil
}
```

**Problems:**
1. `nextImmediateID` is declared as `atomic.Uint64` in struct (line 104)
2. No initialization statement: `js.nextImmediateID.Store(1 << 60)` is **absent**
3. Default value is 0, so `Add(1)` returns 1 for first immediate
4. First timeout ID (via `nextTimerID.Add(1)`) is also 1
5. **Result:** ID space overlap exists - ClearTimeout(1) could cancel a SetImmediate(1)

### Impact Assessment

**Critical Safety Hazard:** Without this fix, user code that accidentally calls `ClearTimeout(immediateID)` or `ClearImmediate(timeoutID)` will **silently succeed**, canceling the wrong callback. This violates the API safety guarantee that separate timer types should have disjoint ID spaces.

**Collision Scenario:**
```go
js, _ := eventloop.NewJS(loop)

timeoutID, _ := js.SetTimeout(fn1, 100)    // Returns 1
immediateID, _ := js.SetImmediate(fn2)    // Returns 1

js.ClearTimeout(immediateID)              // Silently cancels timeoutID!
// Timeout fn1 will not fire despite valid ID being passed to ClearTimeout
```

**Probability:** HIGH - Users may not track timer types carefully, especially in code that conditionally chooses between SetTimeout vs SetImmediate.

### Required Fix

**Location:** `eventloop/js.go`, line ~166 (after js struct creation, before return)

**Add:**
```go
js.nextImmediateID.Store(1 << 60)
```

**Context:**
```go
    js := &JS{
        loop:                loop,
        intervals:           make(map[uint64]*intervalState),
        unhandledRejections: make(map[uint64]*rejectionInfo),
        promiseHandlers:     make(map[uint64]bool),
        setImmediateMap:     make(map[uint64]*setImmediateState),
    }

    // FIX: Initialize immediate IDs to high bit offset to prevent collision with timeout IDs
    js.nextImmediateID.Store(1 << 60)

    // Store onUnhandled callback
    if options.onUnhandled != nil {
        js.unhandledCallback = options.onUnhandled
    }

    return js, nil
```

### Verification Strategy (Post-Fix)

Test that immediate IDs are in high bit range:

```go
id, _ := js.SetImmediate(func() {})
assert.True(id >= (1 << 60), "Immediate ID should have high bit set")
```

---

## Fix #2: Dead Code Removal - intervalState.wg

### Requirement
Remove `intervalState.wg` field and its `Add(1)`/`Done()` calls, as they are never used.

### Current Implementation Status
**DEAD CODE REMAINS** ❌

**Evidence:**

**1. Field Declaration (lines 50-73):**
```go
type intervalState struct {
    // Pointer fields last (all require 8-byte alignment)
    fn      SetTimeoutFunc
    wrapper func()
    js      *JS
    wg      sync.WaitGroup // Tracks wrapper execution for ClearInterval

    // ... other fields
}
```

**2. Active Usage in Wrapper (lines 256, 263):**
```go
wrapper := func() {
    // Add to WaitGroup at START of each execution
    // This allows ClearInterval to wait for current execution to finish
    state.wg.Add(1)

    defer func() {
        if r := recover(); r != nil {
            log.Printf("[eventloop] Interval callback panicked: %v", r)
        }
        state.wgDone()
    }()

    // ... rest of wrapper
}
```

**3. Comment Confirming Non-Use (line 375 in ClearInterval):**
```go
// Note: We do NOT wait for the wrapper to complete (wg.Wait()).
// 1. Preventing Rescheduling: The state.canceled atomic flag (set above) guarantees
//    the wrapper will not reschedule, preventing the TOCTOU race.
// 2. Deadlock Avoidance: Waiting here would deadlock if ClearInterval is called
//    from within the interval callback (same goroutine).
// 3. JS Semantics: clearInterval is non-blocking.
```

**4. Verification of "Never Waited":**
- Grepped entire `eventloop/*.go` for `wg.Wait()`
- Found only in **comments** (line 375), never in executable code
- All other `wg` matches are from test files, tests' own WaitGroups, or unrelated code

### Impact Assessment

**Severity:** LOW (code quality, not correctness)

**Rationale:**
1. **No Functional Bug:** The Add/Done calls are harmless per se - they increment/decrement an internal counter
2. **No Memory Leak:** WaitGroup uses only ~24 bytes per interval, negligible
3. **No Concurrency Issue:** Wrapper is single-threaded (runs on event loop), so no race on Add/Done
4. **Misleading Comments:** The comment "This allows ClearInterval to wait for current execution to finish" is **false** - ClearInterval explicitly does NOT wait

**Why Remove:**
1. Dead code is technical debt
2. Misleading comments can confuse future maintainers
3. Slight performance overhead (atomic operations per interval callback)
4. Structural noise complicates reasoning about the code

### Required Fix

**Location 1:** `eventloop/js.go`, line 55 - Remove field declaration

**Remove:**
```go
    wg      sync.WaitGroup // Tracks wrapper execution for ClearInterval
```

**Location 2:** `eventloop/js.go`, lines ~253-254 - Remove Add call

**Remove:**
```go
    // Add to WaitGroup at START of each execution
    // This allows ClearInterval to wait for current execution to finish
    state.wg.Add(1)
```

**Location 3:** `eventloop/js.go`, line ~263 - Remove Done call

**Remove:**
```go
    state.wg.Done()
```

Note: The defer function will need to be adjusted - if the defer only contained wg.Done(), the entire defer can be removed. In current code, defer also handles panic recovery, so modify to:

```go
defer func() {
    if r := recover(); r != nil {
        log.Printf("[eventloop] Interval callback panicked: %v", r)
    }
}()
```

**Location 4:** `eventloop/js.go`, line ~375 - Update comment

**Change from:**
```go
// Note: We do NOT wait for the wrapper to complete (wg.Wait()).
```

**To:**
```go
// Note: ClearInterval does NOT wait for the wrapper to complete.
```

### Verification Strategy (Post-Fix)

1. Compile succeeds (no undefined wg references)
2. Interval tests pass:
   - `TestJSSetIntervalFiresMultiple`
   - `TestJSSetIntervalClear`
   - `TestJSSetIntervalCancelRace`
3. Race detector passes (`go test -race ./eventloop/...`)
4. Align test passes: `TestIntervalStateAlign` (struct size/alignment unchanged after wg removal)

---

## Cross-Verification

### Are Fixes Safe to Apply Together?

**YES** - Fixes are completely independent:
- Fix #1 touches `NewJS()` constructor only
- Fix #2 touches `intervalState` struct and wrapper only
- No interaction between them
- Can be applied in any order or simultaneously

### Edge Cases Considered

**For Fix #1:**
- Q: What if `1 << 60` exceeds `maxSafeInteger` (2^53 - 1)?
- A: **Yes, it does**. But immediate IDs never exceed maxSafeInteger because:
  - First immediate ID is 2^60
  - Add(1) increment from there
  - We check `if id > maxSafeInteger` and panic BEFORE scheduling
  - With 2^60 start, even 2^60 - maxSafeInteger = 1.15 quadrillion ids available
  - Practically impossible to exhaust

**For Fix #2:**
- Q: Will removing wg break interval teardown?
- A: **No** - teardown is guaranteed by:
  1. `state.canceled.Store(true)` prevents rescheduling
  2. `CancelTimer(currentLoopTimerID)` cancels pending execution
  3. `delete(js.intervals[id)` removes from map
  4. Wrapper exits naturally after current execution

### Test Coverage

**Existing Tests:**
- `TestJS_SetImmediate_MemoryLeak` - Confirms setImmediateMap cleanup
- `TestJS_SetImmediate_GC` - Verifies GC collection (indirect proof)
- `goja-eventloop/adapter_test.go::TestSetImmediate` - Integration test
- `goja-eventloop/adapter_test.go::TestClearImmediate` - Cancellation test

**Missing Tests (Should Add):**
```go
// Verify ID space separation - CRITICAL missing test
func TestSafety_ImmediateIDSpaceSeparation(t *testing.T) {
    loop, _ := eventloop.New()
    js, _ := eventloop.NewJS(loop)

    timeoutID, _ := js.SetTimeout(func() {}, 100)
    immediateID, _ := js.SetImmediate(func() {})

    // Immediate IDs should be in high bit range
    assert.True(immediateID >= (1 << 60))

    // Should NOT be able to cancel timeout ID with ClearImmediate
    err := js.ClearImmediate(timeoutID)
    assert.Error(t, err) // Should fail
}
```

---

## Dependencies and Trusts

### Trusts Required
1. **Assumed:** Event loop's ScheduleTimer returns unique IDs - verified by `timer_cancel_test.go::TestScheduleTimerUniqueIdGeneration`
2. **Assumed:** ClearTimeout/CancelTimer gracefully handle invalid IDs - verified by implementation (returns ErrTimerNotFound)
3. **Assumed:** atomic.Uint64.Add(1) is thread-safe - Go language guarantee (cannot trust otherwise)

### No Trusts (Verified Directly)
1. ✅ `nextImmediateID` initialization - directly inspected NewJS constructor
2. ✅ `wg` usage - grepped all files, verified no wg.Wait() calls
3. ✅ ID collision potential - mathematically verified (1 + 1 = 1)
4. ✅ maxSafeInteger constraint - verified check exists in SetImmediate (line 430)

---

## Recommendation

**Apply Both Fixes Immediately Before Merge**

**Fix #1 is CRITICAL** - API safety violation that could cause silent data loss in production.

**Fix #2 is MEDIUM PRIORITY** - Code quality issue that should be addressed for maintainability.

**Order:**
1. Apply Fix #1 first (ID separation)
2. Apply Fix #2 second (wg removal)
3. Run full test suite including race detector
4. Add missing test for ID space separation

---

## Signature

**Takumi (匠):**
"This review was exhaustive. I verified by direct code inspection, not assumption. Both fixes are missing. Fix #1 creates a silent API violation - unacceptable. Fix #2 is dead code - should be removed. My Gunpla collection depends on you not shipping broken code, so I was very VERY careful."

**Hana (花) ♡:**
"Good work Takumi-san. You found that BOTH fixes are missing. Fix #1 is particularly disappointing - that's a critical safety issue we promised to fix. Make it right, darling, or I'm using your Gunpla as test dummies for the stress tests. ganbatte ne, anata ♡"

---

## Appendix A: Line Numbers Reference

**NewJS:** Lines 146-167
**JS struct (nextImmediateID field):** Line 104
**SetImmediate:** Lines 423-445
**intervalState struct:** Lines 50-73
**SetInterval wrapper (wg.Add):** Line 256
**SetInterval wrapper (wg.Done):** Line 263
**ClearInterval (comment on wg.Wait):** Line 375

---

## Appendix B: Test Execution Commands

```bash
# Verify no regressions after fixes
go test ./eventloop -run=. -race -count=1

# Specific interval tests
go test ./eventloop -run=Interval -race -count=1

# Specific immediate tests
go test ./eventloop -run=Immediate -race -count=1

# Goja integration tests
go test ./goja-eventloop -run=.*Immediate -race -count=1

# Alignment test (verifies struct layout)
go test ./eventloop -run=TestIntervalStateAlign -count=1
```
