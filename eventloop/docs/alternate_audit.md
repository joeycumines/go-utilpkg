# Alternate Implementation Audit

## Overview

This document audits the alternate event loop implementations in
`eventloop/internal/` for the same microtask draining defects (GAP-001 through
GAP-008) identified in the main eventloop.

## alternateone

**Status**: Experimental/reference implementation.

**Architecture**: Uses a `ShutdownManager` pattern with named drain phases.
Microtask draining is handled via `sm.drainMicrotasks()` (shutdown.go:118).

**Findings**: alternateone has a fundamentally different architecture from
the main eventloop. It does not have the same tick()/processInternalQueue()/
processExternal() phase structure. The drainMicrotasks is a simple loop that
drains a microtask queue. No strictMicrotaskOrdering equivalent exists.

**Conclusion**: No GAP-001 through GAP-008 defects apply — different architecture.
Documented as experimental/reference.

## alternatetwo

**Status**: Experimental/reference implementation.

**Architecture**: Has a `tick()` method (loop.go:250) and a simpler loop
structure. No strictMicrotaskOrdering field.

**Findings**: alternatetwo's tick() calls processInternalQueue and
drainMicrotasks but does not have per-callback draining or inter-phase drains.
However, its architecture is simpler and may not need them (depends on usage).

**Conclusion**: Missing per-callback draining (GAP-002 equivalent) but
architecture is too different for direct comparison. Documented as
experimental/reference.

## alternatethree

**Status**: Experimental/reference implementation — closest to main eventloop.

**Architecture**: Has its own `strictMicrotaskOrdering` field (loop.go:127)
and conditional draining at two sites (loop.go:640, 957). Has
`processInternalQueue()` (loop.go:579) which calls `drainMicrotasks()` after
the queue is drained (line 599), but NOT per-task — same GAP-002 defect as
main eventloop. Has `drainMicrotasks()` (loop.go:537) with a budget limit.

**Findings**:
- GAP-001 equivalent: strictMicrotaskOrdering defaults to false (same as main)
- GAP-002 equivalent: processInternalQueue drains all tasks then drains once
- GAP-004 equivalent: drainMicrotasks has a budget limit (same as main's old 1024)
- GAP-005 equivalent: No per-task drain in the internal queue loop

**Conclusion**: alternatethree has the same defects as the main eventloop
had before the fixes. Since it's an experimental implementation in `internal/`,
the fixes should NOT be applied — it serves as a reference for the pre-fix
behavior. If alternatethree is ever promoted to production, it would need the
same fixes.

## Summary

| Implementation | Status | GAP-001 | GAP-002 | GAP-004 | Action |
|---|---|---|---|---|---|
| alternateone | Experimental | N/A (different arch) | N/A | N/A | None |
| alternatetwo | Experimental | N/A | Equivalent | N/A | None |
| alternatethree | Experimental | Same defect | Same defect | Same defect | None (reference) |

All alternate implementations are in `eventloop/internal/` and are not
imported by any production code. They serve as experimental/reference
implementations. No fixes are needed — they document the pre-fix behavior
and alternative architectural approaches.
