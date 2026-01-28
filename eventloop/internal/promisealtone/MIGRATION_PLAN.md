# Main Promise Migration Plan

**Goal**: Port optimizations from `promisealtone` to `eventloop.ChainedPromise` to achieve "tournament-winning" performance (approx +30-50% throughput, -40% allocations).

**Baseline (PromiseAltFour)**: 
- Ops/sec: ~1.5M - 1.8M
- Allocs/op: ~19
- Bytes/op: ~870

**Target (PromiseAltOne)**:
- Ops/sec: ~3.4M - 4.3M
- Allocs/op: ~12
- Bytes/op: ~424

## 1. Analysis of Optimizations

The performance gap is driven by three main factors identified in `promisealtone`:

1.  **Closure Elimination**: 
    - *Current*: `Then` delegates to `NewChainedPromise` which creates `resolve` and `reject` closures (2 allocs). Usage of `Then` creates another closure for the handler (if capturing).
    - *Optimized*: The `handler` struct stores a pointer to the `target` promise directly. Resolution methods traverse this pointer instead of calling a function. This eliminates the closure allocations entirely for internal chaining.

2.  **Memory Layout & Allocation**:
    - *Current*: `ChainedPromise` allocates a `handlers` slice (1 alloc) even for a single handler. Struct size is larger due to separate `value`/`reason` fields.
    - *Optimized*: 
        - **Embedded First Handler (`h0`)**: Use a direct struct field for the first handler. Reduces allocs for linear chains (99% case) by 1.
        - **Unified Result**: Merge `value` and `reason` into a single `result` interface. State determines interpretation. Saves 8 bytes.
        - **Type Punning**: If multiple handlers are needed, store `[]handler` slice in the `result` field (since `result` is unused while Pending). Saves 24 bytes (slice header) in the struct.
        - **Struct Alignment**: Optimizes field order to fit within 64 bytes (1 cache line).

3.  **Synchronization**:
    - *Current*: `Value()` and `Reason()` acquire `RLock`.
    - *Optimized*: Relies on `atomic.Load` of `state`. Since `result` is immutable after transition, checking state provides sufficient happens-before synchronization without a mutex lock in the hot read path.

## 2. Migration Steps

The migration will be performed in phases to ensure stability.

### Phase 1: Struct Compactness & Read Safety
**Changes**:
- Merge `value` and `reason` into `result`.
- Update `Value()`/`Reason()` to use lock-free reading based on `atomic.Load(state)`.
- **Risk**: Low. Standard double-checked locking pattern removal.
- **Verification**: Race detector.

### Phase 2: Handler Optimization (The Big One)
**Changes**:
- Update `handler` struct to replace `resolve`/`reject` funcs with `target *ChainedPromise`.
- Update `NewChainedPromise`: It still returns `resolve`/`reject` for the user, but internally `Then` should use a new private constructor `newChainedPromise()` that *doesn't* allocate these closures for the chained promise.
- Refactor `Then`, `resolve`, `reject` logic to handle the new `handler` structure.
- **Risk**: High. Changes core resolution control flow.
- **Verification**: Existing test suite + Tournament Correctness tests.

### Phase 3: Allocation Optimization
**Changes**:
- Add `h0` field to `ChainedPromise`.
- Implement logic to use `h0` for first handler.
- (Optional) Implement `result` type-punned slice storage for overflow.
- **Risk**: Medium. Complexity in `addHandler`.
- **Verification**: Benchmark `ChainCreation`.

## 3. Compatibility & Constraints

- **API Compatibility**: The public API (`State`, `Value`, `Then`, `Catch`, `Finally`, `NewChainedPromise`) must remain unchanged.
- **Internal Visibility**: `promisealtfour` cannot access `js.unhandledRejections`. The Main implementation MUST continue to support this. Optimizations must not break error tracking.
- **Garbage Collection**: Type punning `result` involves storing `[]handler` in an `interface{}` field. This is valid Go but requires careful casting.

## 4. Rollout Strategy

1. Run Baseline Benchmarks (`PromiseAltFour`).
2. Apply Phase 1 (Struct/Read). Verify.
3. Apply Phase 2 (Handlers). Verify.
4. Apply Phase 3 (Allocations). Verify.
5. Final Benchmark vs Baseline.

## 5. Fallback

If `promisealtone` logic proves unstable with unhandled rejection tracking (Phase 2 interaction), we will revert to `PromiseAltFour` baseline logic and apply only Phase 1 (easy wins).
