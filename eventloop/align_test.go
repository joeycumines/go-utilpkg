package eventloop

import (
	"fmt"
	"sync/atomic"
	"testing"
	"unsafe"

	"golang.org/x/sys/cpu"
)

// Test_sizeOfCacheLine verifies the sizeOfCacheLine constant is correct
func Test_sizeOfCacheLine(t *testing.T) {
	actual := unsafe.Sizeof(cpu.CacheLinePad{})
	if sizeOfCacheLine < actual {
		t.Errorf("sizeOfCacheLine (%d) is less than actual cache line size (%d)", sizeOfCacheLine, actual)
	}
	// must be neatly divisible
	if sizeOfCacheLine%actual != 0 {
		t.Errorf("sizeOfCacheLine (%d) is not a multiple of actual cache line size (%d)", sizeOfCacheLine, actual)
	}
}

// TestSizeOf verifies sizeof constants
func TestSizeOf(t *testing.T) {
	for _, tc := range [...]struct {
		name     string
		expected uintptr
		actual   uintptr
	}{
		{"sizeOfAtomicUint64", sizeOfAtomicUint64, unsafe.Sizeof(atomic.Uint64{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.actual != tc.expected {
				t.Errorf("expected %d got %d", tc.expected, tc.actual)
			}
		})
	}
}

// Analyze intervalState alignment
func TestIntervalStateAlign(t *testing.T) {
	s := &intervalState{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== intervalState ===\n")
	fmt.Printf("canceled: offset=%d, size=%d\n", unsafe.Offsetof(s.canceled), unsafe.Sizeof(s.canceled))
	fmt.Printf("delayMs: offset=%d, size=%d\n", unsafe.Offsetof(s.delayMs), unsafe.Sizeof(s.delayMs))
	fmt.Printf("currentLoopTimerID: offset=%d, size=%d\n", unsafe.Offsetof(s.currentLoopTimerID), unsafe.Sizeof(s.currentLoopTimerID))
	fmt.Printf("fn: offset=%d, size=%d\n", unsafe.Offsetof(s.fn), unsafe.Sizeof(s.fn))
	fmt.Printf("wrapper: offset=%d, size=%d\n", unsafe.Offsetof(s.wrapper), unsafe.Sizeof(s.wrapper))
	fmt.Printf("js: offset=%d, size=%d\n", unsafe.Offsetof(s.js), unsafe.Sizeof(s.js))
	fmt.Printf("m: offset=%d, size=%d\n", unsafe.Offsetof(s.m), unsafe.Sizeof(s.m))
	fmt.Printf("Total: %d bytes\n", unsafe.Sizeof(*s))
	fmt.Printf("\n")
}

// Analyze JS struct alignment
func TestJSAlign(t *testing.T) {
	s := &JS{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== JS ===\n")
	fmt.Printf("nextTimerID: offset=%d, size=%d\n", unsafe.Offsetof(s.nextTimerID), unsafe.Sizeof(s.nextTimerID))
	fmt.Printf("loop: offset=%d, size=%d\n", unsafe.Offsetof(s.loop), unsafe.Sizeof(s.loop))
	fmt.Printf("unhandledCallback: offset=%d, size=%d\n", unsafe.Offsetof(s.unhandledCallback), unsafe.Sizeof(s.unhandledCallback))
	fmt.Printf("intervals: offset=%d, size=%d\n", unsafe.Offsetof(s.intervals), unsafe.Sizeof(s.intervals))
	fmt.Printf("unhandledRejections: offset=%d, size=%d\n", unsafe.Offsetof(s.unhandledRejections), unsafe.Sizeof(s.unhandledRejections))
	fmt.Printf("promiseHandlers: offset=%d, size=%d\n", unsafe.Offsetof(s.promiseHandlers), unsafe.Sizeof(s.promiseHandlers))
	fmt.Printf("mu: offset=%d, size=%d\n", unsafe.Offsetof(s.mu), unsafe.Sizeof(s.mu))
	fmt.Printf("Total: %d bytes\n", unsafe.Sizeof(*s))
	fmt.Printf("\n")
}

// Analyze Loop struct alignment (partial - key fields)
func TestLoopAlign(t *testing.T) {
	s := &Loop{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== Loop (atomic fields) ===\n")

	// Key atomic fields
	fields := []struct {
		name     string
		offset   uintptr
		size     uintptr
		isAtomic bool
	}{
		{"nextTimerID", unsafe.Offsetof(s.nextTimerID), unsafe.Sizeof(s.nextTimerID), true},
		{"tickElapsedTime", unsafe.Offsetof(s.tickElapsedTime), unsafe.Sizeof(s.tickElapsedTime), true},
		{"loopGoroutineID", unsafe.Offsetof(s.loopGoroutineID), unsafe.Sizeof(s.loopGoroutineID), true},
		{"fastPathEntries", unsafe.Offsetof(s.fastPathEntries), unsafe.Sizeof(s.fastPathEntries), true},
		{"fastPathSubmits", unsafe.Offsetof(s.fastPathSubmits), unsafe.Sizeof(s.fastPathSubmits), true},
		{"timerNestingDepth", unsafe.Offsetof(s.timerNestingDepth), unsafe.Sizeof(s.timerNestingDepth), true},
		{"userIOFDCount", unsafe.Offsetof(s.userIOFDCount), unsafe.Sizeof(s.userIOFDCount), true},
		{"wakeUpSignalPending", unsafe.Offsetof(s.wakeUpSignalPending), unsafe.Sizeof(s.wakeUpSignalPending), true},
		{"fastPathMode", unsafe.Offsetof(s.fastPathMode), unsafe.Sizeof(s.fastPathMode), true},
	}

	// Check cache line isolation for each atomic field
	for i, field := range fields {
		fmt.Printf("%s: offset=%d, size=%d", field.name, field.offset, field.size)

		if field.isAtomic {
			lineStart := (field.offset / sizeOfCacheLine) * sizeOfCacheLine
			lineEnd := lineStart + sizeOfCacheLine
			fieldEnd := field.offset + field.size

			if field.offset >= lineStart && fieldEnd <= lineEnd {
				fmt.Printf(" -> cache line %d-%d (ISOLATED)\n", lineStart, lineEnd-1)
			} else {
				// Check if it shares with previous field
				if i > 0 {
					prevField := fields[i-1]
					prevLineStart := (prevField.offset / sizeOfCacheLine) * sizeOfCacheLine
					prevLineEnd := prevLineStart + sizeOfCacheLine

					if field.offset >= prevLineStart && field.offset < prevLineEnd {
						t.Logf("✗ SHARES cache line with %s (both in line %d-%d)", prevField.name, prevLineStart, prevLineEnd-1)
					}
				}
			}
		} else {
			fmt.Printf("\n")
		}
	}

	fmt.Printf("\n")
	fmt.Printf("Loop total size: %d bytes\n", unsafe.Sizeof(*s))
	fmt.Printf("\n")
}

// Analyze TPSCounter alignment
func TestTPSCounterAlign(t *testing.T) {
	s := &TPSCounter{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== TPSCounter ===\n")

	// Check lastRotation (atomic.Value)
	lastRotationOffset := unsafe.Offsetof(s.lastRotation)
	lastRotationSize := unsafe.Sizeof(s.lastRotation)
	lastRotationEnd := lastRotationOffset + lastRotationSize
	fmt.Printf("lastRotation: offset=%d, size=%d, ends at %d\n", lastRotationOffset, lastRotationSize, lastRotationEnd)

	lineStart := (lastRotationOffset / sizeOfCacheLine) * sizeOfCacheLine
	lineEnd := lineStart + sizeOfCacheLine

	if lastRotationEnd <= lineEnd {
		fmt.Printf("✓ lastRotation is isolated on cache line %d-%d\n", lineStart, lineEnd-1)
	} else {
		t.Logf("✗ lastRotation shares cache line (offset %d, ends at %d, line ends at %d)", lastRotationOffset, lastRotationEnd, lineEnd)
	}

	// Check totalCount (atomic.Int64)
	totalCountOffset := unsafe.Offsetof(s.totalCount)
	totalCountSize := unsafe.Sizeof(s.totalCount)
	fmt.Printf("totalCount: offset=%d, size=%d\n", totalCountOffset, totalCountSize)

	totalCountLineStart := (totalCountOffset / sizeOfCacheLine) * sizeOfCacheLine
	totalCountLineEnd := totalCountLineStart + sizeOfCacheLine

	if totalCountOffset < totalCountLineEnd && (totalCountOffset+totalCountSize) <= totalCountLineEnd {
		fmt.Printf("✓ totalCount is on cache line %d-%d\n", totalCountLineStart, totalCountLineEnd-1)
	} else {
		t.Logf("✗ totalCount shares cache line (offset %d, line %d-%d)", totalCountOffset, totalCountLineStart, totalCountLineEnd-1)
	}

	fmt.Printf("TPSCounter total size: %d bytes\n", unsafe.Sizeof(*s))
	fmt.Printf("\n")
}

// Analyze ChainedPromise alignment
func TestChainedPromiseAlign(t *testing.T) {
	s := &ChainedPromise{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== ChainedPromise ===\n")
	fmt.Printf("id: offset=%d, size=%d\n", unsafe.Offsetof(s.id), unsafe.Sizeof(s.id))
	fmt.Printf("js: offset=%d, size=%d\n", unsafe.Offsetof(s.js), unsafe.Sizeof(s.js))
	fmt.Printf("state: offset=%d, size=%d\n", unsafe.Offsetof(s.state), unsafe.Sizeof(s.state))
	fmt.Printf("mu: offset=%d, size=%d\n", unsafe.Offsetof(s.mu), unsafe.Sizeof(s.mu))
	fmt.Printf("value: offset=%d, size=%d\n", unsafe.Offsetof(s.value), unsafe.Sizeof(s.value))
	fmt.Printf("reason: offset=%d, size=%d\n", unsafe.Offsetof(s.reason), unsafe.Sizeof(s.reason))
	fmt.Printf("handlers: offset=%d, size=%d\n", unsafe.Offsetof(s.handlers), unsafe.Sizeof(s.handlers))
	fmt.Printf("Total: %d bytes\n", unsafe.Sizeof(*s))
	fmt.Printf("\n")
}

// Analyze FastState alignment
func TestFastStateAlign(t *testing.T) {
	s := &FastState{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== FastState ===\n")
	fmt.Printf("_ (pad before): offset=%d, size=%d\n", unsafe.Offsetof(s.v), sizeOfCacheLine)
	fmt.Printf("v: offset=%d, size=%d\n", unsafe.Offsetof(s.v), unsafe.Sizeof(s.v))
	vOffset := unsafe.Offsetof(s.v)
	vSize := unsafe.Sizeof(s.v)
	vEnd := vOffset + vSize
	fmt.Printf("v ends at offset: %d\n", vEnd)
	fmt.Printf("Next cache line boundary: %d\n", ((vEnd/sizeOfCacheLine)+1)*sizeOfCacheLine)

	// Verify v is on its own cache line
	cacheLineStart := vOffset / sizeOfCacheLine * sizeOfCacheLine
	cacheLineEnd := cacheLineStart + sizeOfCacheLine
	if vEnd <= cacheLineEnd {
		fmt.Printf("✓ v is isolated on its own cache line (%d-%d)\n", cacheLineStart, cacheLineEnd-1)
	} else {
		t.Errorf("FAIL: v shares cache line! (ends at %d, cache line ends at %d)", vEnd, cacheLineEnd)
	}

	// Verify post-padding
	postPadSize := sizeOfCacheLine - vSize
	expectedSize := sizeOfCacheLine + uintptr(unsafe.Sizeof(atomic.Uint64{})) + postPadSize
	actualSize := unsafe.Sizeof(*s) // Use *s to get struct size, not pointer size
	if actualSize != expectedSize {
		t.Errorf("Expected size %d, got %d", expectedSize, actualSize)
	} else {
		fmt.Printf("✓ Total size correct: %d bytes (2 cache lines)\n", actualSize)
	}
	fmt.Printf("\n")
}

// Analyze MicrotaskRing alignment
func TestMicrotaskRingAlign(t *testing.T) {
	s := &MicrotaskRing{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== MicrotaskRing ===\n")

	// Check buffer offset
	bufferOffset := unsafe.Offsetof(s.buffer)
	fmt.Printf("buffer offset: %d\n", bufferOffset)
	if bufferOffset == sizeOfCacheLine {
		fmt.Printf("✓ buffer starts on cache line boundary\n")
	} else {
		t.Errorf("FAIL: buffer does NOT start on cache line boundary (offset %d)", bufferOffset)
	}

	// Check head offset
	headOffset := unsafe.Offsetof(s.head)
	headSize := uintptr(unsafe.Sizeof(s.head))
	headEnd := headOffset + headSize
	fmt.Printf("head offset: %d, size: %d, ends at: %d\n", headOffset, headSize, headEnd)
	lineStart := headOffset / sizeOfCacheLine * sizeOfCacheLine
	lineEnd := lineStart + sizeOfCacheLine
	if headEnd <= lineEnd {
		fmt.Printf("✓ head is isolated on its own cache line (%d-%d)\n", lineStart, lineEnd-1)
	} else {
		t.Errorf("FAIL: head shares cache line! (ends at %d, cache line ends at %d)", headEnd, lineEnd)
	}

	// Check tail offset
	tailOffset := unsafe.Offsetof(s.tail)
	tailSize := uintptr(unsafe.Sizeof(s.tail))
	tailEnd := tailOffset + tailSize
	fmt.Printf("tail offset: %d, size: %d, ends at: %d\n", tailOffset, tailSize, tailEnd)
	tailLineStart := tailOffset / sizeOfCacheLine * sizeOfCacheLine
	tailLineEnd := tailLineStart + sizeOfCacheLine
	if tailEnd > tailLineEnd {
		t.Logf("✗ tail and tailSeq share cache line (offset %d, ends at %d, line ends at %d)\n", tailOffset, tailEnd, tailLineEnd)
	} else {
		fmt.Printf("✓ tail on cache line %d-%d\n", tailLineStart, tailLineEnd-1)
	}

	fmt.Printf("Total: %d bytes\n", unsafe.Sizeof(*s))
	fmt.Printf("\n")
}
