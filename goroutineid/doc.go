// Package goroutineid provides mechanisms for retrieving goroutine
// identifiers with maximal performance.
//
// # ⚠️ DANGER: INTENDED USE CASE
//
// This package is explicitly designed for very specific, quite low-level use
// cases. It was implemented to address detecting re-entrancy in a Go-native
// libuv-like event loop, [github.com/joeycumines/go-eventloop].
//
// DO NOT use this package for general-purpose application logic.
//
// DO NOT use this package to implement Goroutine-Local Storage. Relying on
// goroutine IDs for state management is a fundamental anti-pattern in Go that
// leads to memory leaks, hidden concurrency bugs, and unidiomatic code.
//
// This package offers two complementary approaches:
//
//  1. [Fast] - Assembly-based retrieval when available: Platform-specific
//     assembly code that reads the G struct's goid field directly, achieving
//     ~2-5ns operation. Returns -1 when assembly is not supported
//     on the current platform.
//
//  2. [Slow] - Pure-Go fallback: Uses [runtime.Stack] to parse goroutine ID
//     from the stack trace. Slower (~1000-2000ns) but works on all platforms
//     including WASM.
//
// # API Design
//
// The API is intentionally simple:
//
//   - `Fast() int64` - returns goroutine ID fast, or -1 if unsupported
//   - `Slow(buf []byte) int64` - returns goroutine ID reliably, buffer must be >= 64 bytes
//
// The sentinel value -1 for "not supported" is intentional - goroutine IDs start at 1.
//
// # Performance Characteristics
//
// [Fast] (when supported):
//
//	amd64: ~2-5ns, direct memory read via R14 register
//	arm64: ~5-10ns, TLS register access
//
// [Slow]:
//
//	All platforms: ~1000-2000ns, stack capture + parsing
//
// # Fallback and Allocation Strategies
//
// When Fast() returns -1, you must use Slow(buf). Because Slow() relies on
// [runtime.Stack], the buffer will escape to the heap. To mitigate GC
// pressure in hot paths, choose your allocation strategy carefully:
//
// Pattern 1: One-off / Infrequent (Heap Allocation)
//
// Use only when calls are rare and allocation overhead is acceptable.
//
//	ID := goroutineid.Fast()
//	if ID == -1 {
//	    ID = goroutineid.Slow(make([]byte, 64))
//	}
//
// Pattern 2: High-throughput Concurrent (sync.Pool)
//
// Use when multiple goroutines frequently hit the fallback path.
//
//	var bufPool = sync.Pool{New: func() any { return make([]byte, 64) }}
//	// ...
//	ID := goroutineid.Fast()
//	if ID == -1 {
//	    buf := bufPool.Get().([]byte)
//	    ID = goroutineid.Slow(buf)
//	    bufPool.Put(buf)
//	}
//
// Pattern 3: Single-Threaded Context (Pre-allocated)
//
// Use inside a strictly single-threaded event loop struct to guarantee zero
// allocations and zero sync overhead. DANGER: Fails if accessed concurrently.
//
//	// loop.buf is initialized once as make([]byte, 64)
//	ID := goroutineid.Fast()
//	if ID == -1 {
//	    ID = goroutineid.Slow(loop.buf)
//	}
//
// # Platform Support
//
//   - [Fast] is supported on: amd64, arm64 (all on Linux, Darwin, Windows)
//   - [Fast] returns -1 on: WASM, 386, riscv64, and other unsupported platforms
//   - [Slow] works on: ALL platforms including WASM
package goroutineid
