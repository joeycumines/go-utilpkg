# Analysis of promisealtone

## 1. Is it even competitive with ChainedPromise?
**Yes.** 
Initial benchmarks indicate `promisealtone` is already superior in both execution time and memory usage compared to the standard `eventloop.ChainedPromise`.

**Benchmark Results:**
- **Chain**: `promisealtone` is ~18% faster (251ns vs 308ns) and reduces allocs/bytes.
- **DeepChain**: `promisealtone` is ~24% faster (75ns vs 100ns).

## 2. Why? Why not?
**Why it is faster:**
1.  **Locking Strategy**: `promisealtone` uses `atomic.Load` to check state without locking in the hot path (`State()`, `Value()`, `Result()`), whereas `ChainedPromise` uses `sync.RWMutex.RLock`. `sync.Mutex` (used for writes) is generally lighter than `sync.RWMutex` when contention is low or successfully avoided via atomic checks.
2.  **Struct Size**: The `Promise` struct is slightly more compact, improving cache locality.
3.  **Reduced Overhead**: Simplified logic for state transitions.

## 3. Is it optimal?
**No.**
While faster, it suffers from significant allocation overhead:
- **8 allocations per Chain op**: This includes the Promise struct, the `resolve/reject` closures (2), the `handlers` slice (1), and the next Promise in the chain (plus its closures/slice).
- **Closure Overhead**: Every `Then` call creates new closures for resolution, even though they just delegate to the method.
- **Slice Overhead**: Every Promise allocates a slice for handlers, even though 99% of Promises have exactly one handler (the next step in the chain).

## 4. Can it be made optimal?
**Yes.**
We can apply the following "Tournament-Style" optimizations:
1.  **Eliminate Closure Allocations**: Refactor the internal `handler` to point directly to the target `*Promise` instead of storing `resolve`/`reject` function closures. This saves 2 allocations per `Then`.
2.  **Inline First Handler**: Embed the first handler directly in the `Promise` struct. This eliminates the slice allocation for the common 1-to-1 chaining case.
3.  **Internal Factory**: Create a `newPromise` internal method that doesn't generate the public `resolve/reject` closures, used exclusively by `Then`.

## 5. Can you produce a tournament-style equivalent for promises?
**Yes.**
The plan is to refactor `promisealtone` in-place to implement the optimizations listed above. This will result in a "Tournament-Ready" implementation that minimizes allocations (targeting 1 alloc per chain step instead of 4-8) and maximizes throughput.

---
**Action Plan:**
1.  Refactor `handler` struct to use `target *Promise`.
2.  Implement `firstHandler` optimization.
3.  Implement `newPromise` helper.
4.  Verify with benchmarks.
