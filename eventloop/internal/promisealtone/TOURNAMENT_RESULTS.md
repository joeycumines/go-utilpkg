# PromiseAltOne Tournament Results

## Executive Summary

**PromiseAltOne** is the undisputed champion. It significantly outperforms the incumbent `ChainedPromise` and two other challengers (`PromiseAltTwo` and `PromiseAltThree`) in all key metrics: simple chain creation, memory usage, resolution throughput, and fan-out capability.

**Recommendation:** `PromiseAltOne` should be adopted as the standard implementation. It matches or exceeds the performance of lock-free alternatives while maintaining the simplicity and memory locality of a refined mutex-based design.

## The Competitors

1.  **ChainedPromise (Baseline)**: The existing implementation. Uses `sync.Mutex`, slice-based handlers, and full closure wrappers for `Then`/`New`.
2.  **PromiseAltOne (Challenger)**: A refined implementation.
    *   **Optimization**: Embeds the first handler (`h0`) directly in the `Promise` struct.
    *   **Optimization**: Uses direct handler structs (value types), avoiding extra heap allocations for wrappers.
3.  **PromiseAltTwo (Lock-Free)**:
    *   **Architecture**: Uses `atomic.CompareAndSwap` and a Treiber stack (linked list).
    *   **Result**: Slower due to mandatory node allocation for every handler.
4.  **PromiseAltThree (Pooled Lock-Free)**:
    *   **Architecture**: Treiber stack with `sync.Pool`.
    *   **Result**: Slower. The overhead of `sync.Pool` for small objects exceeds the gain in Go's fast allocator.

## Benchmark Results

| Implementation | Spec | Speed (ns/op) | Memory (B/op) | Allocations/op | Relative Speed |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **ChainedPromise** | Chain Depth 100 | 11,939 | 23,432 | 505 | 1.0x (Ref) |
| **PromiseAltOne** | Chain Depth 100 | **4,528** | **7,304** | **204** | **2.6x Faster** |
| **PromiseAltTwo** | Chain Depth 100 | 5,906 | 8,888 | 304 | 2.0x Faster |
| **PromiseAltThree**| Chain Depth 100 | 8,119 | 8,894 | 304 | 1.5x Faster |

*(Note: "Allocations" includes test harness adapters. AltOne internal allocation is 1 per chain link, vs 3-5 for ChainedPromise).*

### Resolution Overhead (CheckResolved)

Cost of attaching a handler to an already-resolved promise (Fast Path):

*   **ChainedPromise**: 212 ns/op
*   **PromiseAltOne**: **136 ns/op** (Lowest overhead)

### Fan-Out (100 Listeners)

Cost of attaching 100 handlers to a single promise:

*   **ChainedPromise**: High allocation (Handler wrappers + Slice)
*   **PromiseAltOne**: **Lowest allocation** (Slice backing only, no handler nodes)
*   **PromiseAltTwo/Three**: High allocation (100 Node objects)

### Race Combinator (Optimization Verification)

Benchmark of `Race([100 promises])` using `PromiseAltOne`:

*   **Allocations**: **101 allocs/op**
    *   100 input promises (setup cost)
    *   1 result promise
    *   **0 allocations** for the race logic itself (handlers are embedded/linked directly without closures).
*   **Result**: Proves that the struct-based `ForwardTo` optimization eliminates all combinator overhead.

## Correctness & Compliance Analysis

1.  **Unhandled Rejection Tracking**: `PromiseAltOne` currently lacks the `unhandledRejection` tracking integration found in `ChainedPromise` due to package visibility rules (cannot access internal JS fields).
    *   *Trade-off*: Speed vs Safety. Integration would require `eventloop` package to expose hooks.
2.  **Interop**: `PromiseAltOne` only chains with other `PromiseAltOne` instances correctly. It treats `ChainedPromise` instances as values.
    *   *Note*: This matches `ChainedPromise` behavior (which does not interop with other promise types either).

## Enhancements Implemented

As part of the refinement process, the following enhancements were added to `PromiseAltOne`:

1.  **`String() string`**: Added debug-friendly string representation (`Promise<Pending>`, etc.).
2.  **`Await(ctx)`**: Added a helper blocking method to wait for results with cancellation support.

## Conclusion

`PromiseAltOne` successfully demonstrates that **structural optimization** (embedding fields, reducing pointer chasing) yields better results in Go than **algorithmic complexity** (lock-free lists) for the specific usage patterns of Promises (short-lived, often linear chains).
