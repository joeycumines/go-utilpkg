// Package alternatetwo implements a "Maximum Performance" event loop variant.
//
// This implementation prioritizes throughput, zero allocations, and minimal
// latency over defensive safety measures. Every design decision favors speed
// over safety margins.
//
// # Philosophy: Performance-First Design
//
//  1. Zero Allocations on Hot Paths: No make(), no interface boxing, no closures on critical paths
//  2. Lock-Free Where Possible: Use atomics and CAS loops instead of mutexes
//  3. Cache-Line Awareness: Align data structures to avoid false sharing
//  4. Batch Operations: Amortize overhead across multiple operations
//  5. Assume Correct Usage: Skip validation that slows down correct code
//
// # Key Differences from Main Implementation
//
//   - Lock-Free Queue: MPSC queue with atomic CAS, no mutexes on hot paths
//   - Direct FD Indexing: Array[65536] instead of map for O(1) FD lookup
//   - Minimal Clearing: Only clear used chunk slots, not all 128
//   - Inline Callbacks: Execute I/O callbacks inline without collection
//   - Arena Allocation: Pre-allocated task arenas to reduce GC pressure
//   - Cache-Line Padding: All hot fields separated by 64-byte boundaries
//
// # Performance Expectations
//
//   - Task latency: <10µs (P99)
//   - Lock contention: Near-zero (lock-free critical paths)
//   - Allocations: 0 on hot paths (arena + pools)
//   - Max throughput: 1M+ tasks/sec under ideal conditions
//   - Memory overhead: Higher (pre-allocation trades memory for speed)
//
// # Safety Trade-offs (Acknowledged)
//
// This implementation accepts these risks for performance:
//  1. No invariant validation: Bugs manifest as corruption, not panics
//  2. Optimistic locking: Race conditions possible under extreme load
//  3. Minimal error handling: Some errors silently ignored
//  4. Direct array indexing: FDs > 65535 cause undefined behavior
//  5. Version-based consistency: Stale data possible during modifications
//
// Use only when: Performance is critical AND usage patterns are well-understood
// AND extensive testing validates correctness.
package alternatetwo
