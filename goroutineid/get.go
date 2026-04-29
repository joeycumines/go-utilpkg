package goroutineid

import (
	"sync"
)

// syncPool is a shared pool of pre-allocated buffers for use by Slow.
// Using sync.Pool reduces GC pressure when Get() is called frequently
// on platforms where Fast() is not supported.
var syncPool = sync.Pool{New: func() any {
	return new([64]byte)
}}

// Get returns the current goroutine's numeric ID. This is the preferred
// API for retrieving goroutine IDs in most use cases.
//
// It first attempts to use [Fast] for optimal performance. If [Fast] returns
// -1 (indicating assembly is not supported on this platform), it falls back
// to [Slow] with a buffer from a [sync.Pool] to reduce allocations in hot
// paths.
//
// Returns a positive integer representing the goroutine's unique identifier.
// Goroutine IDs start at 1 and increment sequentially. The main goroutine
// typically has ID 1.
//
// Note: This function is designed for low-level use cases (e.g., event loop
// re-entrancy detection). For general application logic, consider whether
// you truly need goroutine IDs. You _should not_ use Goroutine IDs to
// implement goroutine-local storage or similar Go-hostile patterns.
func Get() int64 {
	if ID := Fast(); ID != -1 {
		return ID
	}
	// N.B. Split out to improve the likelihood of inlining Get.
	return getSlow()
}

// getSlow handles the allocation fallback.
// Split out for inlining heuristics related reasons.
//
//go:noinline
func getSlow() int64 {
	buf := syncPool.Get().(*[64]byte)
	ID := Slow(buf[:])
	syncPool.Put(buf)
	return ID
}
