# goroutineid

[![Go Reference](https://pkg.go.dev/badge/github.com/joeycumines/goroutineid.svg)](https://pkg.go.dev/github.com/joeycumines/goroutineid)

Retrieves the current goroutine's numeric ID with maximal performance.

## Overview

This package provides three complementary mechanisms for getting goroutine IDs:

- **`Get() int64`** — Convenience API with automatic Fast/Slow fallback (~2-5ns when supported). Uses a [sync.Pool] to reduce allocations. **Recommended for most use cases.**
- **`Fast() int64`** — Assembly-based retrieval where supported (~2-5ns). Returns `-1` on unsupported platforms.
- **`Slow(buf []byte) int64`** — Pure-Go fallback using `runtime.Stack` parsing (~1000-2000ns). Works everywhere including WASM.

## Performance

| Method | Platforms                             | Latency |
|--------|---------------------------------------|---------|
| Get    | All (uses Fast when available)        | ~2-5ns  |
| Fast   | amd64, arm64 (Linux, Darwin, Windows) | ~2-5ns  |
| Slow   | All platforms including WASM          | ~2000ns |

## Intended Use

This package is designed for low-level use cases such as re-entrancy detection. It is **not** intended for general application logic or goroutine-local storage—using goroutine IDs for state management is an anti-pattern in Go.

## Quick Start

```go
package example

import "github.com/joeycumines/goroutineid"

func recommended() {
	// Get() uses Fast when available, falls back to Slow automatically
	ID := goroutineid.Get()

	// Use ID...
}

// Fast path (returns -1 if unsupported)
func alternative() {
	ID := goroutineid.Fast()
	if ID == -1 {
		// Fallback path (buffer must be >= 64 bytes)
		ID = goroutineid.Slow(make([]byte, 64))
	}

	// Use ID...
}
```

## Documentation

[https://pkg.go.dev/github.com/joeycumines/goroutineid](https://pkg.go.dev/github.com/joeycumines/goroutineid)
