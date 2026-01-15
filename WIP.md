# Work In Progress

## Current Goal
Create regression test files for eventloop defects identified in scratch.md

**STATUS: COMPLETE** ✓

## Created Test Files

### A) `eventloop/ingress_torture_test.go`
- **TestMicrotaskRing_WriteAfterFree_Race** - Torture test proving Pop ordering bug (FAILS as expected)
- **TestMicrotaskRing_FIFO_Violation** - Deterministic test proving overflow priority inversion (FAILS as expected)
- **TestMicrotaskRing_NilInput_Liveness** - Proves nil input infinite loop (FAILS as expected)

### B) `eventloop/poller_race_test.go`
- **TestPoller_Init_Race** - Proves initPoller CAS race (requires -race flag)
- **TestIOPollerClosedDataRace** - Proves data race on closed field (requires -race flag)

### C) `eventloop/correctness_test.go`
- **TestCloseFDsInvokedOnce** - Proves double-close issue (verifies idempotency)
- **TestInitPollerClosedReturnsConsistentError** - Cross-platform error consistency
- **TestLockFreeIngressPopWaitsForProducer** - Verifying Pop spin logic works

### D) `eventloop/popbatch_test.go`
- **TestPopBatchInconsistencyWithPop** - Proves PopBatch doesn't spin like Pop
- **TestPopBatchEmptyQueueBehavior** - Baseline test for empty queue behavior

## Verification Results
- All 4 files compile without errors
- The MicrotaskRing tests (3 tests) FAIL as expected, proving the bugs exist
- The poller race tests run (require -race flag for detection)
- The correctness tests verify behavior

---

## Previous Goal (COMPLETE)
~~Consolidate defect findings from `scratch.md` into `eventloop/docs/requirements.md`~~

## Previous Summary of Changes

Successfully updated [eventloop/docs/requirements.md](eventloop/docs/requirements.md) with the following new sections:

### New Sections Added (XVI-XXI):

1. **XVI. Defect Tracking & Required Fixes**
   - Severity classifications (CRITICAL/HIGH/MODERATE/LOW)
   - 10 defects documented with full details
   - Required fixes with code examples
   - Verification requirements per defect

2. **XVII. Verification Test Requirements**
   - Required test files and test names
   - CI integration YAML configuration
   - Pre-merge checklist

3. **XVIII. Verified Correct Components**
   - Components confirmed working correctly
   - Preservation notes for refactoring

4. **XIX. Platform Consistency Requirements**
   - Error constants standardization
   - Method signature requirements
   - Struct layout requirements

5. **XX. Latent Issues & Documented Constraints**
   - Single-consumer constraint documentation
   - getGoroutineID() fragility notes
   - Memory ordering assumptions (Go 1.19+)

6. **XXI. Defect Fix Summary Matrix**
   - Quick reference table
   - Recommended fix order

## Document Stats
- Original: ~850 lines (Sections I-XV)
- Added: ~700 lines (Sections XVI-XXI)
- Final: 1567 lines total

## Verification
- [x] All existing content preserved (Sections I-XV intact)
- [x] All 10 defects from scratch.md documented
- [x] All verified correct components listed
- [x] Platform consistency requirements added
- [x] Latent issues documented
- [x] Fix summary matrix with recommended order

See blueprint.json for task completion status.
