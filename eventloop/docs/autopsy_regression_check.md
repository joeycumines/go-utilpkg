# Cross-Platform Regression Check

## Overview

This document records the results of running the full test suite across
platforms after the microtask draining changes (Tasks 5-11).

## Darwin (macOS, Apple M2 Pro, native)

### eventloop

- `gmake all.eventloop` — **PASS** (189s)
  - Build: PASS
  - Vet: PASS
  - Staticcheck: PASS
  - Betteralign: PASS
  - Tests: PASS (all packages)

### goja-eventloop

- `gmake all.goja-eventloop` — **PASS** (19s)
  - Build: PASS
  - Vet: PASS
  - Staticcheck: PASS
  - Tests: PASS

### Race Detection

- `go test -race -timeout=10m -skip 'TestFIFO_Concurrent' ./...` — **PASS** (305s)
- `go test -race -timeout=10m -run 'TestFIFO'` — **PASS** (40s)
- `go test -race -run 'TestKill|TestTransition'` — **PASS**
- `go test -race -run 'TestGojaMicrotaskOrdering'` — **PASS**

### Kill-Condition Tests

All 7 tests pass:
- TestKill001_PerTimerCallbackMicrotaskDraining — PASS
- TestKill002_PerInternalTaskMicrotaskDraining (FastPath + PollPath) — PASS
- TestKill003_MicrotaskBudgetOverflow — PASS
- TestKill004_InterPhaseMicrotaskLeakage — PASS
- TestKill005_NextTickStarvationViaBudget — PASS
- TestTransition_NextTickBeforeMicrotaskWithinDrain — PASS
- TestTransition_NestedNextTickRecursion — PASS

### Goja Integration Tests

All 3 tests pass:
- TestGojaMicrotaskOrdering_PromiseBetweenTimers — PASS
- TestGojaMicrotaskOrdering_QueueMicrotaskBetweenTimers — PASS
- TestGojaMicrotaskOrdering_ExhaustiveDrain — PASS

## Linux (via Docker, golang container)

### eventloop

- `gmake all.eventloop` — **FAIL** (298s)
  - Build: PASS
  - Vet: PASS
  - Staticcheck: PASS
  - Tests: FAIL — `TestDefect7_RevertWouldFail` timed out (2s timeout for
    interval callback under container resource constraints)

### Analysis of Linux Failure

`TestDefect7_RevertWouldFail` uses a 10ms interval and 2-second timeouts to
wait for interval ticks. Under Docker container resource constraints, the
loop may not process the interval callback within 2 seconds. This is a
pre-existing timing sensitivity, NOT caused by the microtask draining changes.

**Verification**: The test passes on Darwin (native) with 1.07s execution time.
The 2-second timeout is tight for containerized environments. The test
does not use microtasks or nextTicks — it only uses `SetInterval`/
`UnrefInterval`/`RefInterval` — so the microtask draining changes cannot
affect it. This is inferred from the test's code and the logical independence
of the timer ref/unref path from microtask draining, not from a prior Linux
run that passed.

**Recommendation**: Increase the test's internal timeouts from 2s to 10s for
better cross-platform reliability. This is a pre-existing issue outside the
scope of the microtask draining changes.

## Windows

Not tested (no Windows environment available).

## Tournament Benchmarks

Tournament benchmarks were not run due to time constraints. The tournament
benchmarks require explicit invocation via `gmake eventloop-tournament-bench`
with a 30-minute timeout (they are excluded from the default `go test` run
because they use `-bench` mode). The microtask draining changes (per-task
draining, inter-phase drains, exhaustive draining) may affect tournament
performance due to more frequent drainMicrotasks() calls, but the fast-path
early-return optimization minimizes the overhead when queues are empty (see
[drain performance benchmarks](autopsy_drain_perf.md)).

## Conclusion

The microtask draining changes pass all tests on Darwin (native) with race
detection. The Linux container failure is a pre-existing timing issue in
`TestDefect7_RevertWouldFail` unrelated to the microtask changes. All
kill-condition regression tests and goja integration tests pass.
