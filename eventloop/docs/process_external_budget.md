# processExternal() Budget Evaluation

## Current Behavior

`processExternal()` (loop.go:1055) has a budget of 1024 tasks per tick.
If more than 1024 tasks are queued, the remaining tasks are deferred to the
next tick. An `OnOverload` callback is signaled if more tasks remain after
the budget is exhausted.

## Node.js Semantics

Node.js processes all available I/O callbacks in the poll phase. There is no
fixed numeric limit like 1024. The poll phase processes all events returned
by epoll/kevent in a single batch.

## Analysis

The 1024 budget serves as **overload protection**:

1. **Prevents starvation**: Without a budget, a flood of external tasks could
   starve timers, internal tasks, and I/O polling.

2. **Batched execution**: Tasks are popped in a batch while holding the
   external mutex (minimizing lock contention), then executed without the
   lock. This is an intentional performance optimization.

3. **OnOverload signal**: When the budget is exceeded, `OnOverload` is
   called, allowing the application to implement backpressure. This has no
   Node.js equivalent but is a useful extension for Go's concurrent
   programming model.

4. **Deferred, not dropped**: Tasks exceeding the budget are not lost — they
   remain in the queue and are processed in the next tick. The only impact
   is increased latency.

## Comparison with processInternalQueue Budget

The internal queue budget is 4096 — 4x larger than the external
budget. This is intentional: internal tasks include promise reactions and
other high-priority work that should be processed more aggressively. External
tasks are general-purpose and can tolerate more latency.

## Recommendation

The 1024 budget should be **documented as a deliberate design deviation**
from Node.js semantics. It serves as overload protection and is not a
correctness issue — tasks are deferred, not dropped.

Making it configurable with a higher default (e.g., 4096 to match the
internal queue) could be considered for future work, but the current value
is reasonable for most workloads.

## Conclusion

The 1024 external task budget is deliberate overload protection, not a bug.
No code changes needed. Documented as a known deviation from Node.js.
