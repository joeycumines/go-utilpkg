package goroutineid

import "runtime"

// Fast returns the current goroutine's numeric ID using the fastest
// available mechanism (platform-specific assembly when available).
//
// Returns -1 if assembly is not supported on this platform.
func Fast() int64 {
	return getGoIDImpl()
}

// Slow returns the current goroutine's numeric ID using runtime.Stack
// parsing. This is the reliable fallback that works on ALL platforms
// including WASM.
//
// The buffer must be at least 64 bytes.
func Slow(buf []byte) (ID int64) {
	const (
		prefixLen = 10 // len("goroutine ")
		base      = 10 // for parsing
	)

	if len(buf) < 64 {
		panic("goroutineid: buffer too small")
	}

	if n := runtime.Stack(buf, false); n > prefixLen {
		for _, b := range buf[prefixLen:n] {
			v := b - '0'
			// guards both <'0' and >'9' (due to underflow)
			if v > 9 {
				break
			}
			ID = ID*base + int64(v)
		}
	}

	return ID
}
