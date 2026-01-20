# Goja Eventloop Review - Critical Issues Found

## Summary

**VERIFICATION STATUS: CRITICAL ISSUES FOUND - STOP**

Tests reveal THREE critical issues in goja-eventloop adaptations that block the PR from proceeding.

## Critical Issues

### CRITICAL #1: WaitGroup Negative Counter Panic
**Location**: `eventloop/js.go` ClearInterval implementation
**Symptom**: `sync: negative WaitGroup counter` errors in SetInterval/ClearInterval tests
**Root Cause**: Same-goroutine detection flawed
**Impact**: Interval tests produce errors (though tests still pass)

Fix attempted: Track goroutine ID via `goroutineID()` and skip `wg.Wait()` when called from within wrapper.

---

### CRITICAL #2: Promise Chaining Broken 
**Location**: `goja-eventloop/adapter.go` gojaWrapPromise()
**Symptom**: First `.then()` call works, subsequent `.then()` fails with "Object has no member 'then'"
**Test Output**:
```
[JS] [Promise keys: [_internalPromise then catch finally]]
[JS] [Has 'then'?: function]
```
This confirms first promise has `.then`, but it fails immediately during execution.

**Root Cause**: `gojaWrapPromise()` method chaining is broken

Investigation shows:
1. Constructor-created promises have methods set correctly
2. gojaWrapPromise() should also set methods on chained promises
3. Currently `.then()` returns a promise without methods
4. This breaks `.then().then().catch().finally()` chains

**Impact**: Blocks all Promise chain tests (PromiseChain, MixedTimersAndPromises, ConcurrentJSOperations)

---

### CRITICAL #3: Panics in Promise Execution
**Symptom**: Tests crash with "index out of range [-1]"
**Error**: Runtime `panic` and `re-panic` indicating Goja VM is catching and re-panicking

**Root Cause**: Unknown - requires deeper investigation

**Impact**: Tests crash entirely, not just fail

---

## Test Results

**Compilation**: ✅ PASS
**Tests**: ❌ FAIL

### Passing Tests (12/16)
- TestNewAdapter
- TestSetTimeout
- TestClearTimeout
- TestSetInterval (with errors logged)
- TestClearInterval (with errors logged)
- TestQueueMicrotask
- TestPromiseThen (simple test)
- TestContextCancellation
- TestAdapterAllWithAllResolved
- TestAdapterAllWithEmptyArray
- TestAdapterAllWithOneRejected
- TestAdapterRaceTiming
- TestAdapterRaceFirstRejectedWins
- TestAdapterAllSettledMixedResults
- TestAdapterAnyFirstResolvedWins
- TestAdapterAnyAllRejected

### Failing Tests (4/16, with crashes)
- TestPromiseChain (PANIC: index out of range)
- TestMixedTimersAndPromises (FAIL: no then)
- TestConcurrentJSOperations (FAIL: no then)
- All Promise chain tests show "Object has no member 'then'" on second `.then()` call

---

## Required Actions

1. **CRITICAL**: Fix Promise chaining - gojaWrapPromise() must properly propagate methods through chains
2. **CRITICAL**: Fix WaitGroup negative counter errors
3. **CRITICAL**: Investigate and fix panics in Promise execution
4. **Re-run all tests** after fixes

**Estimated Time**: 60+ minutes (complex debugging required)

---

## Verification Task

**Status**: ❌ CRITICAL PROBLEMS FOUND - CANNOT PASS VERIFICATION

**Requested**: Verify all 5 critical safety violations fixed:
- CRITICAL #1: Removed ensureLoopThread() - PARTIAL (added back after seeing deadlock in tests)
- CRITICAL #2: Added panic recovery - PARTIAL (not verified due to crashes)
- CRITICAL #3: Added ok checks to type assertions - VERIFIED
- CRITICAL #4: Implemented 4 tests - BLOCKED by Promise chain crashes
- CRITICAL #5: Added cross-thread documentation - NOT STARTED

**BONUS**: Bound Promise combinators to JavaScript - PARTIAL (Promise.all/race/allSettled/any work, but Promise.resolve/reject broken)

---

**VERDICT**: ❌ FAILED - Multiple critical issues prevent test suite from passing
