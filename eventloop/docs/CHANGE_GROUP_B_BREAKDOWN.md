# CHANGE_GROUP_B Review Breakdown

**Total Files**: 245 files changed
**Total Lines**: 65,455 additions, 173 deletions
**Branch**: eventloop (vs main)

## Logical Sub-Groups (Ordered by Risk/Complexity)

### SUBGROUP_B1: Promise/A+ Implementation (HIGHEST RISK)
**Files**:
- eventloop/promise.go (1079 lines) - Promise/A+ core
- eventloop/js.go - Promise handler tracking, unhandled rejection detection
- eventloop/promise_test.go (if modified)

**Related Files**:
- eventloop/internal/alternateone/* (promise implementations)
- eventloop/internal/alternatetwo/* (promise implementations)
- eventloop/internal/alternatethree/* (promise implementations)

**Scope**:
- Promise state machine (pending, fulfilled, rejected, settled)
- Promise chaining (.then(), .catch(), .finally())
- Handler scheduling and microtask queue
- Unhandled rejection detection (FIXED in CHANGE_GROUP_A)
- Memory leak prevention (registry scavenging)
- 3 alternate implementations for comparison

**Risk**: CRITICAL - Promise/A+ compliance is essential, memory safety critical
**Estimated Time**: 2-3 hours
**Review Sequence**: 36

---

### SUBGROUP_B2: Eventloop Core System (HIGH RISK)
**Files**:
- eventloop/loop.go (1699 lines) - Core event loop
- eventloop/ingress.go - Event ingress handling
- eventloop/metrics.go - Latency, TPS, queue depth metrics
- eventloop/state.go - Loop state management
- eventloop/registry.go - Timer/FD registry

**Related Files**:
- eventloop/poller.go - Poller interface
- eventloop/poller_darwin.go - macOS kqueue implementation
- eventloop/poller_linux.go - Linux epoll implementation
- eventloop/poller_windows.go - Windows IOCP implementation

**Scope**:
- Timer pool and scheduling
- Fast path mode and microtask budget
- Poller abstraction for multi-platform support
- Metrics collection with thread-safety
- State transitions and lifecycle
- Timer ID management (MAX_SAFE_INTEGER validation)
- Auxiliary job draining for starvation prevention

**Risk**: HIGH - Core event loop correctness, thread-safety, performance-critical
**Estimated Time**: 2-3 hours
**Review Sequence**: 37

---

### SUBGROUP_B3: Goja Integration (MEDIUM RISK)
**Files**:
- goja-eventloop/adapter.go (1018 lines) - Main adapter
- goja-eventloop/*_test.go (18 test files)

**Scope**:
- JavaScript adapter bridging Goja VM and eventloop
- Promise combinators (all, race, allSettled, any)
- Timer API bindings (setTimeout, setInterval, setImmediate)
- JS float64 encoding for timer IDs
- Promise.then() with JavaScript callbacks
- Double-wrapping prevention (CRITICAL FIX - historical)
- Memory leak prevention (CRITICAL FIX - historical)
- Promise.reject semantics (CRITICAL FIX - historical)

**Risk**: MEDIUM - API-facing layer, but historically reviewed and fixed
**Estimated Time**: 1-2 hours
**Review Sequence**: 38

---

### SUBGROUP_B4: Alternate Implementations & Tournament (LOW RISK)
**Files**:
- eventloop/internal/alternateone/* (715 lines in loop.go)
- eventloop/internal/alternatetwo/* (loop.go + other files)
- eventloop/internal/alternatethree/* (1104 lines in loop.go)
- eventloop/internal/tournament/* - Benchmark results
- eventloop/docs/tournament/* - Tournament documentation

**Scope**:
- Three alternate architectural approaches
- Performance comparison results
- Benchmark suite and results

**Risk**: LOW - Research/implementations, not production code
**Estimated Time**: 1 hour
**Review Sequence**: 39

---

## Review Pattern for Each Subgroup

For each subgroup (B1-B4), execute:

1. **REVIEW (TASK_X_1)**: Run #runSubagent with comprehensive prompt
   - Focus: Correctness, thread-safety, memory safety, error handling
   - Output: Review document `./eventloop/docs/reviews/<SEQUENCE>-<SUBGROUP>.md`
   - Verdict: CORRECT / NEEDS FIXES / BLOCKING

2. **FIX (TASK_X_2)**: Address all found issues
   - Create fixes document `<SEQUENCE>-<SUBGROUP>-FIXES.md`
   - Run full test suite with `-race` flag
   - Verify all fixes work

3. **RE-REVIEW (TASK_X_3)**: Run #runSubagent again with SAME prompt
   - Focus: Perfection, edge cases, completeness
   - Output: Re-review document `<SEQUENCE>-<SUBGROUP>-REVIEW.md`
   - If ANY issues found: Restart ALL THREE tasks for this subgroup

## Order of Execution

**ORDER: B1 → B2 → B3 → B4**

Rationale:
- B1 (Promise) is most critical - Promise/A+ compliance affects everything
- B2 (Core) is second - Core event loop correctness affects all components
- B3 (Goja) is third - API-facing but historically reviewed
- B4 (Alternates) is last - Low risk, research only

## Dependencies

- B2 (Core) depends on B1 (Promise) - core system uses promises
- B3 (Goja) depends on both B1 and B2 - adapts both systems
- B4 (Alternates) is independent - parallelizable

## Success Criteria

ALL subgroups must achieve:
- ✅ Verdict: CORRECT (no blocking issues)
- ✅ All tests pass
- ✅ Race detector clean
- ✅ Documentation complete

Once ALL subgroups complete → CHANGE_GROUP_B complete → Proceed to coverage tasks
