# COVERAGE_1.1: Eventloop Coverage Gap Analysis

**Analysis Date**: 2026-01-26
**Target**: 90%+ coverage per blueprint COVERAGE_1 task
**Current State**: Main package 77.5%, internal packages 57.7%-72.7%

---

## Summary: Overall Coverage Numbers

| Package | Current Coverage | Target Coverage | Gap | Priority |
|---------|------------------|------------------|-----|----------|
| **main** | 77.5% | 90% | -12.5% | HIGH |
| internal/alternatetwo | 72.7% | 90% | -17.3% | HIGH |
| internal/alternateone | 69.3% | 90% | -20.7% | HIGH |
| internal/alternatethree | 57.7% | 85% | -27.3% | CRITICAL |

**Combined Total**: 71.6%

**Test Status**: All 200+ tests PASS ✅

---

## Detailed Findings: Uncovered/Low-Coverage Functions

### Priority Legend
- **CRITICAL**: Core logic affecting correctness (must fix)
- **HIGH**: Important but acceptable gap (should fix)
- **MEDIUM**: Edge cases or platform-specific (optional)

---

### MAIN PACKAGE (77.5% - 0% Functions Listed)

| File:Line | Function | Coverage | Category | Notes |
|-----------|----------|----------|----------|-------|
| loop.go:987 | handlePollError | 0.0% | **CRITICAL** | Error path in poll() - catastrophic error handling |
| promise.go:411 | ThenWithJS | 0.0% | **CRITICAL** | JS promise .then() with JS callback integration |
| promise.go:502 | thenStandalone | 0.0% | **CRITICAL** | Internal helper for .then() chains |
| promise.go:793 | All | 0.0% | **HIGH** | Promise.all combinator |
| promise.go:858 | Race | 0.0% | **HIGH** | Promise.race combinator |
| promise.go:904 | AllSettled | 0.0% | **HIGH** | Promise.allSettled combinator |
| promise.go:966 | Any | 0.0% | **HIGH** | Promise.any combinator |
| promise.go:1054+ | 3 Error methods | 0.0% | MEDIUM | Custom error types - rarely used directly |
| promisify.go:22 | Error | 0.0% | MEDIUM | Custom error - rarely used directly |
| state.go:91 | TransitionAny | 0.0% | **HIGH** | State machine - arbitrary transition |
| state.go:101 | IsTerminal | 0.0% | **HIGH** | State machine - terminal check |
| state.go:112 | CanAcceptWork | 0.0% | **HIGH** | State machine - work acceptance check |
| wakeup_darwin.go:52 | drainWakeUpPipe | 0.0% | MEDIUM | Platform-specific cleanup on error |
| wakeup_darwin.go:58 | isWakeFdSupported | 0.0% | MEDIUM | Platform-specific support check |
| wakeup_darwin.go:68 | submitGenericWakeup | 0.0% | MEDIUM | Platform-specific fallback唤醒 |

**Low Coverage <80% (but >0%):**
| File:Line | Function | Coverage | Category |
|-----------|----------|----------|----------|
| js.go:463 | ClearImmediate | 0.0% | **HIGH** |
| loop.go:1210 | Wake | 50.0% | MEDIUM |
| promise.go:584 | Finally | 55.8% | HIGH |
| state.go:37 | String | 42.9% | MEDIUM |
| loop.go:1581 | safeExecuteFn | 66.7% | MEDIUM |

---

### INTERNAL/ALTERNATEONE (69.3% - EXTENSIVE GAPS)

**0% Coverage Functions:**

| File:Line | Function | Category | Notes |
|-----------|----------|----------|-------|
| errors.go:56, 116 | LoopError.Error | MEDIUM | Error string methods |
| errors.go:64 | LoopError.Unwrap | MEDIUM | Error unwrapping |
| errors.go:69 | NewLoopError | MEDIUM | Error constructor |
| errors.go:121 | WrapError | MEDIUM | Error wrapping |
| errors.go:129 | WrapErrorWithPhase | MEDIUM | Error wrapping with phase |
| errors.go:137 | WrapErrorWithContext | MEDIUM | Error wrapping with context |
| ingress.go:68 | Pop | MEDIUM | Non-pop-specific Pop (never called externally?) |
| ingress.go:143 | LengthLocked | MEDIUM | Internal helpers |
| ingress.go:157 | InternalLength | MEDIUM | Internal helpers |
| ingress.go:164 | MicrotaskLength | MEDIUM | Internal helpers |
| loop.go:88 | Less | MEDIUM | Heap comparison (always default?) |
| loop.go:570 | ScheduleMicrotask | **HIGH** | Direct microtask submission |
| loop.go:595 | RegisterFD | **HIGH** | FD registration (not used in tests?) |
| loop.go:600 | UnregisterFD | **HIGH** | FD unregistration (not used in tests?) |
| poller_darwin.go:156 | UnregisterFD | MEDIUM | Platform-specific FD unregistration |
| poller_darwin.go:180 | ModifyFD | MEDIUM | Platform-specific FD modification |
| poller_darwin.go:302 | IsClosed | MEDIUM | Poller closed check |
| shutdown.go:34 | SetLogger | MEDIUM | Logger setter |
| shutdown.go:127 | rejectPromises | **HIGH** | Reject promises on shutdown |
| state.go:40 | String | MEDIUM | State string representation |
| state.go:144 | IsTerminal | MEDIUM | Terminal state check |
| state.go:149 | IsRunning | MEDIUM | Running state check |
| state.go:156 | CanAcceptWork | MEDIUM | Work acceptance check |

---

### INTERNAL/ALTERNATETHREE (57.7% - MASSIVE GAPS)

**0% Coverage Functions (NEARLY ALL PROMISE IMPLEMENTATION):**

| File:Line | Function | Category | Notes |
|-----------|----------|----------|-------|
| promise.go:50 | State | MEDIUM | Promise state getter |
| promise.go:56 | Result | MEDIUM | Promise result getter |
| promise.go:63 | ToChannel | MEDIUM | Promise to channel conversion |
| promise.go:84 | Resolve | **CRITICAL** | Resolve promise (external API) |
| promise.go:98 | Reject | **CRITICAL** | Reject promise (external API) |
| promise.go:114 | fanOut | **CRITICAL** | Fanout to handlers (CORE LOGIC) |
| promisify.go:22 | Error | MEDIUM | Custom error |
| promisify.go:42 | Promisify | **HIGH** | Promisify function |
| registry.go:43 | NewPromise | **CRITICAL** | Create new promise |
| registry.go:237 | compactAndRenew | **HIGH** | Registry compaction on pressure |
| loop.go:943 | Wake | MEDIUM | Wake loop goroutine |
| loop.go:1099 | scheduleMicrotask | MEDIUM | Internal microtask scheduling |

**Low Coverage <50%:**
| File:Line | Function | Coverage | Category |
|-----------|----------|----------|----------|
| registry.go:61 | Scavenge | 13.8% | **HIGH** | Weak pointer GC - CRITICAL for memory leak prevention |
| loop.go:582 | drainMicrotasks | 29.4% | MEDIUM | Microtask drain on shutdown |

---

### INTERNAL/ALTERNATETWO (72.7% - MODERATE GAPS)

**0% Coverage Functions:**

| File:Line | Function | Category | Notes |
|-----------|----------|----------|-------|
| arena.go:25 | Alloc | MEDIUM | Arena allocation |
| arena.go:32 | Reset | MEDIUM | Arena reset |
| arena.go:72 | GetResult | MEDIUM | Arena result getter |
| arena.go:77 | PutResult | MEDIUM | Arena result putter |
| loop.go:433 | RegisterFD | MEDIUM | FD registration |
| loop.go:438 | UnregisterFD | MEDIUM | FD unregistration |
| loop.go:443 | CurrentTickTime | MEDIUM | Tick time getter |
| poller_darwin.go:112 | UnregisterFD | MEDIUM | Darwin FD unregistration |
| poller_darwin.go:132 | ModifyFD | MEDIUM | Darwin FD modification |
| state.go:24 | String | MEDIUM | State string |
| state.go:80 | TransitionAny | MEDIUM | Any transition |
| state.go:90 | IsTerminal | MEDIUM | Terminal check |
| state.go:95 | IsRunning | MEDIUM | Running check |
| state.go:101 | CanAcceptWork | MEDIUM | Work acceptance check |

---

## Priority List: Top 7 Areas Needing Tests

### 1. **Promise Combinators (CRITICAL)** - Gain ~5-8% coverage
**Location**: `promise.go:793-1076`
**Missing**: `All`, `Race`, `AllSettled`, `Any`
**Impact**: Core Promise/A+ functionality - JavaScript standard compliance

**Test Plan**:
```go
// Test cases needed:
- Promise.all with mixed resolution/rejection
- Promise.all with empty array
- Promise.all with non-array input
- Promise.race with immediate resolution
- Promise.race with delayed operations
- Promise.race with all rejected
- Promise.allSettled with mixed outcomes
- Promise.any with one success
- Promise.any with all rejected (AggregateError)
- Deep nesting of combinators
```

---

### 2. **JS Promise Integration (CRITICAL)** - Gain ~3-5% coverage
**Location**: `promise.go:411, 502`
**Missing**: `ThenWithJS`, `thenStandalone`
**Impact**: JavaScript callback integration in goja-eventloop

**Test Plan**:
```go
// Test cases needed:
- .then() with JavaScript function callbacks
- .then() with mixed Go/JS callbacks
- .then() chains with JS functions
- Error propagation through JS callbacks
- thenStandalone edge cases
```

---

### 3. **Error Path in Poll Operation (CRITICAL)** - Gain ~2% coverage
**Location**: `loop.go:987`
**Missing**: `handlePollError`
**Impact**: Catastrophic error handling - system stability

**Test Plan**:
```go
// Test cases needed:
- Poll returns error (OS-level failure)
- Poll closes unexpectedly
- Repeated poll failures
- Error recovery after poll failure
- Metrics capture poll errors
```

---

### 4. **State Machine Queries (HIGH)** - Gain ~3-4% coverage
**Location**: `state.go:91, 101, 112` + internal packages
**Missing**: `TransitionAny`, `IsTerminal`, `CanAcceptWork`
**Impact**: State machine validation - correctness of state transitions

**Test Plan**:
```go
// Test cases needed:
- IsTerminal returns true for StateTerminated, StateKilled
- CanAcceptWork works across all states
- TransitionAny with valid and invalid transitions
- State machine boundary conditions
- Concurrent state queries
```

---

### 5. **FD Registration/Deregistration (HIGH)** - Gain ~3-5% coverage
**Location**: `loop.go:1290-1345`, internal packages
**Missing**: `RegisterFD`, `UnregisterFD`, `ModifyFD`
**Impact**: I/O multiplexing - network/async I/O testing

**Test Plan**:
```go
// Test cases needed:
- Register and immediately unregister FD
- Register multiple FDs simultaneously
- Modify FD interest (read/write flags)
- Unregister during active poll
- Concurrent FD operations
- Platform-specific Darwin/Linux/Windows behavior
```

---

### 6. **alternatethree Promise Core (CRITICAL)** - Gain ~15-20% coverage
**Location**: `internal/alternatethree/promise.go`
**Missing**: `Resolve`, `Reject`, `fanOut`, `NewPromise`
**Impact**: Alternative promise implementation - core logic untested

**Test Plan**:
```go
// Test cases needed:
- Direct Resolve/Reject calls
- fanOut with multiple handlers
- fanOut with chained promises
- State transitions (pending -> fulfilled/rejected)
- Result() and ToChannel()
- Edge cases: double settlement, settlement after handler attachment
```

---

### 7. **Registry Scavenge & Compaction (HIGH)** - Gain ~5% coverage
**Location**: `internal/alternatethree/registry.go:61-237`
**Low Coverage**: `Scavenge` (13.8%), `compactAndRenew` (0%)
**Impact**: Memory leak prevention - weak pointer GC

**Test Plan**:
```go
// Test cases needed:
- Scavenge collects weak pointers
- Scavenge doesn't collect strong references
- CompactAndRenew evacuates active promises
- Memory pressure triggers compaction
- Long-running registry doesn't leak
- GC interaction with weak pointers
```

---

## Additional Medium Priority Areas

### Platform-Specific Code (Darwin/Linux/Windows)
- `wakeup_darwin.go:52,58,68` - Wake FD fallback paths
- `poller_darwin.go:156,180` - FD modify/unregister

### Shutdown Promise Rejection
- `internal/alternateone/shutdown.go:127` - Reject promises on shutdown

### Microtask Scheduling
- `internal/alternateone/loop.go:570` - Direct microtask submission
- `internal/alternatethree/loop.go:1099` - Internal microtask scheduling

### Metrics Update Paths
- `metrics.go:160,179,197` - UpdateIngress, UpdateInternal, UpdateMicrotask (all 0%)

### ClearImmediate Function (goja-eventloop integration)
- `js.go:463` - ClearImmediate (0% coverage)

---

## Estimation: Coverage Gains From Tests

| Priority Area | Estimated Coverage Gain | Effort |
|---------------|------------------------|--------|
| Promise Combinators | +5-8% | 2-3 hours |
| JS Promise Integration | +3-5% | 1-2 hours |
| Error Path (handlePollError) | +2% | 1 hour |
| State Machine Queries | +3-4% | 1-2 hours |
| FD Registration | +3-5% | 2-3 hours |
| alternatethree Promise Core | +15-20% | 4-6 hours |
| Registry Scavenge | +5% | 2-3 hours |
| **Total Estimated Gain** | **+36-42%** | **13-20 hours** |

**Target Achievement**: Current 77.5% + 36% = **113.5%** ✅
**Realistic Achievement**: Main package can reach **90%+**, internal packages need focused effort

---

## Recommendations

### Immediate Actions (CRITICAL)
1. ✅ **Promise Combinators Tests** - Create `promise_combinators_test.go`
2. ✅ **JS Promise Integration Tests** - Create `promise_js_integration_test.go`
3. ✅ **alternatethree Promise Core Tests** - Create `promise_alternatethree_test.go`

### High Priority (SHOULD)
4. ✅ **Error Path Tests** - Create `poll_error_test.go` for handlePollError
5. ✅ **State Machine Tests** - Create `state_machine_test.go` for state queries
6. ✅ **FD Registration Tests** - Create `fd_registration_test.go`
7. ✅ **Registry Scavenge Tests** - Create `registry_scavenge_test.go`

### Medium Priority (OPTIONAL)
8. Platform-specific fallback paths
9. Shutdown promise rejection
10. Metrics update paths
11. Microtask direct submission

---

## Next Steps

1. **Execute COVERAGE_1.2**: Add tests for Promise combinators (target +5-8%)
2. **Execute COVERAGE_1.3**: Add tests for alternatethree promise core (target +15-20%)
3. **Execute COVERAGE_1.4**: Verify 90%+ coverage achieved across all packages

**COVERAGE_1.1 COMPLETE** ✅
