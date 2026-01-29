# Goja-Eventloop Coverage Gaps Analysis

**Analysis Date:** 2026-01-28
**Current Total Coverage:** 74.0%
**Target Coverage:** 90%+
**Gap:** 16.0 percentage points

---

## Executive Summary

The goja-eventloop module currently achieves 74.0% statement coverage. To reach the 90% target, we need to gain an additional 16.0 percentage points. This analysis identifies 11 functions with incomplete coverage, with one function at 0% coverage.

**Key Findings:**
- **1 CRITICAL GAP:** `NewChainedPromise` function with 0% coverage (not used by any tests)
- **1 LOW COVERAGE:** `exportGojaValue` at 42.9% (critical error handling path)
- **9 MEDIUM COVERAGE:** Functions between 62-72% (edge cases and error paths)

---

## Coverage Gap Details

### CRITICAL GAPS (0% Coverage)

#### 1. `NewChainedPromise` (Line 667) - **0.0% Coverage**

**Function:** Creates a new ChainedPromise with resolve/reject callbacks wrapped as Goja values
**Lines:** 667-692 (26 lines total)
**Lines Uncovered:** All 26 lines

```go
func NewChainedPromise(loop *goeventloop.Loop, runtime *goja.Runtime) (*goeventloop.ChainedPromise, goja.Value, goja.Value) {
    js, err := goeventloop.NewJS(loop)
    if err != nil {
        panic(err)  // Error path not tested
    }
    promise, resolve, reject := js.NewChainedPromise()
    resolveVal := runtime.ToValue(func(call goja.FunctionCall) goja.Value {
        var val any
        if len(call.Arguments) > 0 {
            val = call.Argument(0).Export()
        }
        resolve(val)
        return goja.Undefined()
    })
    rejectVal := runtime.ToValue(func(call goja.FunctionCall) goja.Value {
        var val any
        if len(call.Arguments) > 0 {
            val = call.Argument(0).Export()
        }
        reject(val)
        return goja.Undefined()
    })
    return promise, resolveVal, rejectVal
}
```

**Uncovered Paths:**
- Function is never called in test suite (appears to be deprecated/unused API)
- Error path: `NewJS(loop)` failure → panic
- Resolve callback with arguments (len check path)
- Reject callback with arguments (len check path)

**Impact:** **LOW PRIORITY**
- This function appears to be unused by the adapter.go implementation
- The internal `js.NewChainedPromise()` is called directly in `promiseConstructor`
- This may be a public API intended for external users that isn't tested

**Estimated Coverage Gain:** +0.1-0.3% (26 lines, ~700 total lines in adapter.go)
**Priority:** **LOW** (Investigate if function should be removed or documented)
**Test Strategy:**
- Create test file `adapter_newchainedpromise_test.go`
- Test successful creation and resolution
- Test error path when NewJS fails
- Test callbacks with and without arguments

---

### LOW COVERAGE GAPS (< 50%)

#### 2. `exportGojaValue` (Line 380) - **42.9% Coverage**

**Function:** Extracts Goja.Value for export while preserving type information for Error objects
**Lines:** 380-397 (18 lines total)
**Lines Covered:** 8 lines
**Lines Uncovered:** 10 lines (null checks not tested)

```go
func exportGojaValue(gojaVal goja.Value) (any, bool) {
    if gojaVal == nil || goja.IsNull(gojaVal) || goja.IsUndefined(gojaVal) {
        return nil, false  // Uncovered: nil/Null/Undefined paths
    }

    // Goja.Error objects: preserve as Goja.Value to maintain .message property
    if obj, ok := gojaVal.(*goja.Object); ok {
        if nameVal := obj.Get("name"); nameVal != nil && !goja.IsUndefined(nameVal) {
            if nameStr, ok := nameVal.Export().(string); ok && (nameStr == "Error" || nameStr == "TypeError" || nameStr == "RangeError" || nameStr == "ReferenceError") {
                // This is an Error object - preserve original Goja.Value
                return gojaVal, true  // This path is tested
            }
        }
    }
    return nil, false  // Uncovered: non-Error object path
}
```

**Uncovered Paths:**
- `gojaVal == nil` check
- `goja.IsNull(gojaVal)` check
- `goja.IsUndefined(gojaVal)` check
- Non-Error object return path (line 396)

**Impact:** **HIGH PRIORITY**
- This is a critical helper for Promise.reject() compliance
- Ensures Error objects preserve their `.message` property when used as rejection reasons
- The uncovered null checks suggest tests don't pass nil/null/undefined values

**Estimated Coverage Gain:** +0.5-1.0% (10 uncovered lines, ~700 total lines)
**Priority:** **HIGH** (Compliance requirement)
**Test Strategy:**
- Test with `nil` Goja.Value
- Test with `goja.Null()` value
- Test with `goja.Undefined()` value
- Test with non-Error objects (should return false)
- Test with non-Object Goja values

---

### MEDIUM COVERAGE GAPS (50-79%)

#### 3. `New` (Line 22) - **62.5% Coverage**

**Function:** Creates a new Goja adapter for given event loop and runtime
**Lines:** 22-41 (20 lines total)
**Lines Covered:** 13 lines
**Lines Uncovered:** 7 lines (error paths)

**Uncovered Paths:**
- `loop == nil` error check (line 24-26)
- `runtime == nil` error check (line 28-30)
- `goeventloop.NewJS(loop)` error path (line 32-34)

**Impact:** **MEDIUM PRIORITY**
- Constructor error paths not tested
- Existing tests may not exercise nil inputs
- Defensive programming but important to verify

**Estimated Coverage Gain:** +0.5-1.0% (7 lines, ~700 total)
**Priority:** **MEDIUM**
**Test Strategy:**
- Test `New(nil, runtime)` → error "loop cannot be nil"
- Test `New(loop, nil)` → error "runtime cannot be nil"
- Test `New(loop, runtime)` where JS adapter creation fails

---

#### 4. `setTimeout` (Line 90) - **71.4% Coverage**

**Function:** SetTimeout binding for Goja
**Lines:** 90-115 (26 lines total)
**Lines Covered:** 19 lines
**Lines Uncovered:** 7 lines

**Uncovered Paths:**
- Non-callable first argument (not a function)
- Negative delay validation (delay < 0)
- `js.SetTimeout()` error path

**Impact:** **MEDIUM PRIORITY**
- Error handling paths not fully tested
- Type validation for non-function inputs
- Negative delay rejection

**Estimated Coverage Gain:** +0.5-1.0% (7 lines, ~700 total)
**Priority:** **MEDIUM**
**Test Strategy:**
- Test with non-function (string, number, object)
- Test with negative delay value
- Test with zero delay (edge case)
- Test with very large delay (overflow scenarios)

---

#### 5. `setInterval` (Line 124) - **71.4% Coverage**

**Function:** SetInterval binding for Goja
**Lines:** 124-149 (26 lines total)
**Lines Covered:** 19 lines
**Lines Uncovered:** 7 lines

**Uncovered Paths:**
- Non-callable first argument
- Negative delay validation
- `js.SetInterval()` error path

**Impact:** **MEDIUM PRIORITY**
- Same gaps as setTimeout
- Test coverage should be combined

**Estimated Coverage Gain:** +0.5-1.0% (7 lines, ~700 total)
**Priority:** **MEDIUM**
**Test Strategy:**
- Same as setTimeout (combine tests)
- Test interval cancellation scenarios

---

#### 6. `queueMicrotask` (Line 158) - **72.7% Coverage**

**Function:** QueueMicrotask binding for Goja
**Lines:** 158-177 (20 lines total)
**Lines Covered:** 15 lines
**Lines Uncovered:** 5 lines

**Uncovered Paths:**
- Non-callable first argument
- `js.QueueMicrotask()` error path

**Impact:** **MEDIUM PRIORITY**
- Microtask scheduling error handling

**Estimated Coverage Gain:** +0.3-0.7% (5 lines, ~700 total)
**Priority:** **MEDIUM**
**Test Strategy:**
- Test with non-function arguments
- Test error propagation in microtasks

---

#### 7. `setImmediate` (Line 180) - **72.7% Coverage**

**Function:** SetImmediate binding for Goja (non-standard)
**Lines:** 180-201 (22 lines total)
**Lines Covered:** 16 lines
**Lines Uncovered:** 6 lines

**Uncovered Paths:**
- Non-callable first argument
- `js.SetImmediate()` error path

**Impact:** **MEDIUM PRIORITY**
- SetImmediate is less common than setTimeout/Interval
- Error handling still important

**Estimated Coverage Gain:** +0.4-0.8% (6 lines, ~700 total)
**Priority:** **MEDIUM** (lower than standard timer functions)
**Test Strategy:**
- Test with non-function arguments
- Test error path

---

#### 8. `gojaFuncToHandler` (Line 263) - **68.4% Coverage**

**Function:** Converts Goja function to promise handler (then/catch callbacks)
**Lines:** 263-358 (96 lines total)
**Lines Covered:** ~66 lines
**Lines Uncovered:** ~30 lines

**Uncovered Paths:**
- Handler returns wrapped promise with `_internalPromise` (lines 304-310)
- Handler returning non-Goja objects (default conversion path)
- Array element with wrapped promise identity check (lines 288-294)
- Map value with wrapped promise identity check (lines 304-310)
- Error propagation from `fnCallable` (line 318 panics, caught elsewhere)

**Impact:** **HIGH PRIORITY**
- Critical for Promise chaining identity preservation
- Double-wrapping prevention logic needs testing
- Array/Map element recursive conversion

**Estimated Coverage Gain:** +2.0-3.0% (30 lines, ~700 total)
**Priority:** **HIGH** (Critical for Promise identity)
**Test Strategy:**
- Test handler returning wrapped promise (p.then(() => p2) === p2)
- Test then/catch with arrays containing promises
- Test then/catch with objects containing promises
- Test error propagation through handlers
- Deep nesting scenarios

---

#### 9. `gojaVoidFuncToHandler` (Line 360) - **71.4% Coverage**

**Function:** Converts Goja function to void callback (for finally)
**Lines:** 360-378 (19 lines total)
**Lines Covered:** 14 lines
**Lines Uncovered:** 5 lines

**Uncovered Paths:**
- Non-callable function returns no-op (lines 367-368)
- Function call with nil/undefined arguments

**Impact:** **MEDIUM PRIORITY**
- Finally clause handler conversion
- Simpler than gojaFuncToHandler

**Estimated Coverage Gain:** +0.3-0.7% (5 lines, ~700 total)
**Priority:** **MEDIUM**
**Test Strategy:**
- Test finally with non-function argument
- Test finally with undefined/null
- Test error propagation from finally

---

#### 10. `resolveThenable` (Line 520) - **61.8% Coverage**

**Function:** Handles thenables (objects with .then method)
**Lines:** 520-597 (78 lines total)
**Lines Covered:** ~48 lines
**Lines Uncovered:** ~30 lines

**Uncovered Paths:**
- Non-object thenable (primitive with .then getter)
- Thenable where `.then` is not callable
- Thenable that throws when `.then()` is called
- Thenable where .then returns nothing
- ResolveThenable with Goja.Error preservation (lines 543-555)
- Reject path when then() throws (line 593)

**Impact:** **HIGH PRIORITY**
- Critical for Promise/A+ compliance
- Promise.resolve() logic depends on this
- Thenable adoption/assimilation process

**Estimated Coverage Gain:** +2.0-3.0% (30 lines, ~700 total)
**Priority:** **HIGH** (Promise/A+ compliance)
**Test Strategy:**
- Test with objects having .then property
- Test with functions (functions have .then inherited from Function.prototype)
- Test thenables that throw in .then()
- Test thenables that call resolve/reject multiple times
- Test thenables that return promises from .then()

---

#### 11. `convertToGojaValue` (Line 599) - **72.4% Coverage**

**Function:** Converts Go-native types to JavaScript values
**Lines:** 599-665 (67 lines total)
**Lines Covered:** ~49 lines
**Lines Uncovered:** ~18 lines

**Uncovered Paths:**
- Wrapped Goja Error preservation (lines 602-610)
- ChainedPromise wrapping (lines 613-618)
- Goja.Exception unwrapping (lines 632-635)
- Generic error wrapping (lines 637-640)
- Recursive conversion in slices (lines 621-625)
- Recursive conversion in maps (lines 627-631)
- AggregateError special handling (lines 644-650)

**Impact:** **HIGH PRIORITY**
- Core type conversion logic
- Error object preservation
- Promise identity preservation

**Estimated Coverage Gain:** +1.5-2.5% (18 lines, ~700 total)
**Priority:** **HIGH** (Type conversion correctness)
**Test Strategy:**
- Test with wrapped Goja Errors
- Test with ChainedPromise return values
- Test with Goja.Exceptions
- Test with AggregateError
- Test with nested slices/maps
- Test with mixed types in collections

---

### HIGH COVERAGE GAPS (80-89%)

These functions have good coverage but have minor uncovered edge cases:

#### 12. `Bind` (Line 58) - **88.2% Coverage**
- **Uncovered:** Iterator helper failure path (line 64-67)
- **Priority:** **LOW** (unlikely to fail)
- **Estimated Gain:** +0.1-0.3%

#### 13. `promiseConstructor` (Line 210) - **87.5% Coverage**
- **Uncovered:** Executor function throws (line 236-239)
- **Priority:** **MEDIUM** (critical error path)
- **Estimated Gain:** +0.2-0.5%

#### 14. `consumeIterable` (Line 437) - **81.6% Coverage**
- **Uncovered:** Iterator protocol errors (next() throws, etc.)
- **Priority:** **MEDIUM** (Promise combinators depend on this)
- **Estimated Gain:** +1.0-2.0% (likely more uncovered than estimated)

#### 15. `bindPromise` (Line 696) - **81.1% Coverage**
- **Uncovered:** Multiple combinator error paths (all, race, allSettled, any)
- **Priority:** **MEDIUM** (combinator error handling)
- **Estimated Gain:** +1.5-2.5%

---

## Coverage Gap Prioritization

### CRITICAL Priority (Must Fix)

1. **`exportGojaValue`** (42.9% → +1.0%)
   - Impact: Promise.reject() compliance with Error .message property
   - Test complexity: Easy (4 tests for null/undefined check paths)

2. **`gojaFuncToHandler`** (68.4% → +3.0%)
   - Impact: Promise chaining identity preservation (CRITICAL for correctness)
   - Test complexity: Medium (requires testing return value types)

3. **`resolveThenable`** (61.8% → +3.0%)
   - Impact: Promise/A+ compliance for thenable handling
   - Test complexity: Medium-High (thenable scenarios)

4. **`convertToGojaValue`** (72.4% → +2.5%)
   - Impact: Type conversion correctness, error preservation
   - Test complexity: Medium (various type scenarios)

**Total Estimated Gain for CRITICAL:** +9.5 percentage points

### HIGH Priority (Should Fix)

5. **`consumeIterable`** (81.6% → +2.0%)
   - Impact: Promise combinator stability, iterator protocol errors
   - Test complexity: Medium (custom iterators, throwing next())

6. **`bindPromise`** (81.1% → +2.5%)
   - Impact: Combinator error handling paths
   - Test complexity: Medium (error propagation through combinators)

7. **`promiseConstructor`** (87.5% → +0.5%)
   - Impact: Executor error rejection
   - Test complexity: Easy (executor throwing exception)

**Total Estimated Gain for HIGH:** +5.0 percentage points

### MEDIUM Priority (Nice to Fix)

8. **`New`** (62.5% → +1.0%)
   - Impact: Constructor error paths
   - Test complexity: Easy (3 nil checks)

9. **`setTimeout`** (71.4% → +1.0%)
10. **`setInterval`** (71.4% → +1.0%)
11. **`queueMicrotask`** (72.7% → +0.7%)
12. **`setImmediate`** (72.7% → +0.8%)
13. **`gojaVoidFuncToHandler`** (71.4% → +0.7%)
14. **`Bind`** (88.2% → +0.3%)

**Total Estimated Gain for MEDIUM:** +5.5 percentage points

### LOW Priority (Optional)

15. **`NewChainedPromise`** (0.0% → +0.3%)
    - Impact: Function appears unused/deprecated
    - Recommendation: Investigate if function should be removed or documented
    - If kept, add tests for external API usage

---

## Recommended Test Plan

### Phase 1: Critical Coverage (Target: +9.5% → 83.5% total)

**Test File:** `adapter_critical_paths_test.go`

```go
// Test exportGojaValue null/undefined checks
func TestExportGojaValue_NullChecks(t *testing.T) { /* ... */ }

// Test gojaFuncToHandler promise identity preservation
func TestGojaFuncToHandler_PromiseIdentity(t *testing.T) { /* ... */ }

// Test gojaFuncToHandler array/map with wrapped promises
func TestGojaFuncToHandler_Collections(t *testing.T) { /* ... */ }

// Test resolveThenable with various thenable types
func TestResolveThenable_Compliance(t *testing.T) { /* ... */ }

// Test resolveThenable throwing then
func TestResolveThenable_Throws(t *testing.T) { /* ... */ }

// Test convertToGojaValue error preservation
func TestConvertToGojaValue_ErrorPreservation(t *testing.T) { /* ... */ }

// Test convertToGojaValue chained promise wrapping
func TestConvertToGojaValue_ChainedPromise(t *testing.T) { /* ... */ }
```

---

### Phase 2: High Priority Coverage (Target: +5.0% → 88.5% total)

**Test File:** `adapter_combinators_edge_cases_test.go`

```go
// Test consumeIterable with throwing iterators
func TestConsumeIterable_Throws(t *testing.T) { /* ... */ }

// Test consumeIterable with custom iterables
func TestConsumeIterable_CustomIterables(t *testing.T) { /* ... */ }

// Test Promise.all with iterable errors
func TestPromiseAll_IterableError(t *testing.T) { /* ... */ }

// Test Promise.race with iterable errors
func TestPromiseRace_IterableError(t *testing.T) { /* ... */ }

// Test Promise.allSettled with iterable errors
func TestPromiseAllSettled_IterableError(t *testing.T) { /* ... */ }

// Test Promise.any with iterable errors
func TestPromiseAny_IterableError(t *testing.T) { /* ... */ }

// Test Promise constructor with throwing executor
func TestPromiseConstructor_ExecutorThrows(t *testing.T) { /* ... */ }
```

---

### Phase 3: Medium Priority Coverage (Target: +5.5% → 94.0% total)

**Test File:** `adapter_timer_edge_cases_test.go`

```go
// Test constructor nil checks
func TestNew_NilInputs(t *testing.T) { /* ... */ }

// Test setTimeout with non-function
func TestSetTimeout_NonFunction(t *testing.T) { /* ... */ }

// Test setTimeout with negative delay
func TestSetTimeout_NegativeDelay(t *testing.T) { /* ... */ }

// Test setInterval edge cases
func TestSetInterval_EdgeCases(t *testing.T) { /* ... */ }

// Test queueMicrotask with non-function
func TestQueueMicrotask_NonFunction(t *testing.T) { /* ... */ }

// Test setImmediate with non-function
func TestSetImmediate_NonFunction(t *testing.T) { /* ... */ }

// Test finally with non-function handler
func TestPromiseFinally_NonFunction(t *testing.T) { /* ... */ }
```

---

### Phase 4: Low Priority/Investigation (Optional)

**Task:** Investigate `NewChainedPromise` function usage

**Questions:**
1. Is this function intended as a public API for external users?
2. If yes, document it and add comprehensive tests
3. If no, consider removing it to reduce API surface area

---

## Summary Table

| Priority | Function | Current % | Target % | Gain | Tests Needed |
|----------|----------|-----------|----------|------|--------------|
| CRITICAL | exportGojaValue | 42.9% | 100% | +1.0% | 4 |
| CRITICAL | gojaFuncToHandler | 68.4% | 100% | +3.0% | 6 |
| CRITICAL | resolveThenable | 61.8% | 100% | +3.0% | 5 |
| CRITICAL | convertToGojaValue | 72.4% | 100% | +2.5% | 6 |
| **CRITICAL TOTAL** | | | | **+9.5%** | **21** |
| HIGH | consumeIterable | 81.6% | 100% | +2.0% | 4 |
| HIGH | bindPromise | 81.1% | 95%+ | +2.5% | 6 |
| HIGH | promiseConstructor | 87.5% | 100% | +0.5% | 2 |
| **HIGH TOTAL** | | | | **+5.0%** | **12** |
| MEDIUM | New | 62.5% | 100% | +1.0% | 3 |
| MEDIUM | setTimeout | 71.4% | 100% | +1.0% | 3 |
| MEDIUM | setInterval | 71.4% | 100% | +1.0% | 3 |
| MEDIUM | queueMicrotask | 72.7% | 100% | +0.7% | 2 |
| MEDIUM | setImmediate | 72.7% | 100% | +0.8% | 2 |
| MEDIUM | gojaVoidFuncToHandler | 71.4% | 100% | +0.7% | 2 |
| MEDIUM | Bind | 88.2% | 100% | +0.3% | 1 |
| **MEDIUM TOTAL** | | | | **+5.5%** | **16** |
| LOW | NewChainedPromise | 0.0% | TBD | +0.3% | TBD |
| **GRAND TOTAL** | | 74.0% | 90%+ | **+15.3%** | **~50** |

---

## Recommendations

### Immediate Actions (Day 1)

1. **Phase 1 Implementation: Critical Coverage**
   - Create `adapter_critical_paths_test.go`
   - Implement tests for exportGojaValue, gojaFuncToHandler, resolveThenable, convertToGojaValue
   - Run tests with `-race` detector
   - Verify coverage reaches 83.5%+

2. **Update Blueprint**
   - Mark COVERAGE_2.1 as complete
   - Add COVERAGE_2.2 with Phase 1 tasks

### Short-term Actions (Day 2-3)

3. **Phase 2 Implementation: High Priority Coverage**
   - Create `adapter_combinators_edge_cases_test.go`
   - Implement tests for iterator protocol errors, combinator error propagation
   - Run tests with `-race` detector
   - Verify coverage reaches 88.5%+

4. **Phase 3 Implementation: Medium Priority Coverage**
   - Create `adapter_timer_edge_cases_test.go`
   - Implement tests for timer type validation, error paths
   - Run tests with `-race` detector
   - Verify coverage reaches 94.0%+

### Investigation (Day 4)

5. **NewChainedPromise Investigation**
   - Search codebase for usage of NewChainedPromise
   - Determine if function should be kept or removed
   - If kept, add comprehensive tests
   - If removed, update documentation

### Final Verification

6. **Run Full Test Suite**
   - Execute: `go test -v -race -coverprofile=coverage.out ./goja-eventloop/...`
   - Verify: Coverage >= 90%
   - Verify: All tests pass with `-race` detector
   - Generate HTML report: `go tool cover -html=coverage.out -o coverage.html`

7. **Update Documentation**
   - Update this document with final coverage results
   - Create summary report for goja-eventloop module
   - Update blueprint.json with COVERAGE_2 status

---

## Risk Assessment

### High Risk Areas

1. **Promise Identity Preservation** (gojaFuncToHandler)
   - Risk: Double-wrapping could break `p.then(() => p) === p` semantics
   - Mitigation: Comprehensive testing with various return value types
   - Test: Identity preservation tests in Phase 1

2. **Thenable Adoption Process** (resolveThenable)
   - Risk: Incorrect thenable handling violates Promise/A+ spec
   - Mitigation: Test with various thenable types (objects, functions, throwing)
   - Test: Promise/A+ compliance tests in Phase 1

3. **Error Preservation** (convertToGojaValue, exportGojaValue)
   - Risk: Loss of Error.message property breaks debugging
   - Mitigation: Test with Goja.Error, TypeError, RangeError, etc.
   - Test: Error preservation tests in Phase 1

### Medium Risk Areas

4. **Iterator Protocol Errors** (consumeIterable)
   - Risk: Iterator throws should reject combinators, not panic
   - Mitigation: Test with custom iterators that throw in next()
   - Test: Iterator error tests in Phase 2

5. **Timer Type Validation** (setTimeout, setInterval, etc.)
   - Risk: Non-function inputs should throw TypeError
   - Mitigation: Test with various invalid types
   - Test: Timer validation tests in Phase 3

### Low Risk Areas

6. **Constructor Error Paths** (New)
   - Risk: Nil inputs should return error, not panic
   - Mitigation: Unit tests for defensive checks
   - Test: Simple nil check tests in Phase 3

---

## Success Criteria

✅ **Phase 1 Success (CRITICAL):**
- Coverage reaches 83.5% or higher
- All critical paths tested (exportGojaValue, gojaFuncToHandler, resolveThenable, convertToGojaValue)
- All tests pass with `-race` detector
- No regressions in existing tests
- Promise identity preservation verified
- Error object preservation verified

✅ **Phase 2 Success (HIGH):**
- Coverage reaches 88.5% or higher
- Iterator protocol errors tested
- Combinator error propagation tested
- All tests pass with `-race` detector
- No regressions in existing tests

✅ **Phase 3 Success (MEDIUM):**
- Coverage reaches 90% or higher (PRIMARY GOAL)
- Timer type validation tested
- Constructor error paths tested
- All tests pass with `-race` detector
- No regressions in existing tests

✅ **Final Success:**
- Total coverage >= 90% (target achieved)
- All tests pass with `-race` detector
- HTML coverage report generated
- Documentation updated
- Blueprint updated with COVERAGE_2 status

---

## Notes

- Analysis based on Go 1.21+ coverage reporting
- Line counts approximate (may vary based on formatting)
- Coverage estimates conservative (actual gain may be higher or lower)
- Some uncovered lines may be truly unreachable (defensive checks that never trigger)
- Run `go test -covermode=count` for more precise line coverage analysis
- Consider using `go test -coverprofile=coverage.out -coverpkg=./...` for full project coverage

---

## Appendix: Test Execution Commands

```bash
# Run coverage analysis
go test -coverprofile=coverage.out ./goja-eventloop/...

# Generate HTML report
go tool cover -html=coverage.out -o coverage.html

# View function-level coverage
go tool cover -func=coverage.out | grep adapter.go

# Run tests with race detector
go test -race -v ./goja-eventloop/...

# Run specific test file
go test -v -race ./goja-eventloop -run TestExportGojaValue

# Run coverage for specific package
go test -coverprofile=coverage.out ./goja-eventloop -run TestCriticalPaths
```

---

**Document Version:** 1.0
**Last Updated:** 2026-01-28
**Author:** Takumi (匠)
**Reviewed By:** Hana (花) ♡
