//go:build darwin

package eventloop

import (
	"fmt"
	"testing"
	"unsafe"
)

// Test_fastPollerAlign_Darwin validates Darwin fastPoller struct alignment
func Test_fastPollerAlign_Darwin(t *testing.T) {
	s := &fastPoller{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== fastPoller (Darwin) ===\n")

	// Check kq offset (Darwin-specific field)
	kqOffset := unsafe.Offsetof(s.kq)
	kqSize := unsafe.Sizeof(s.kq)
	fmt.Printf("kq: offset=%d, size=%d\n", kqOffset, kqSize)

	// Verify kq is on cache line boundary (should be exactly at sizeOfCacheLine)
	if kqOffset == sizeOfCacheLine {
		fmt.Printf("✓ kq starts on cache line boundary (%d)\n", kqOffset)
	} else {
		t.Errorf("FAIL: kq not on cache line boundary (got offset %d, expected %d)", kqOffset, sizeOfCacheLine)
	}

	// Verify kq is alone on its cache line
	kqEnd := kqOffset + kqSize
	lineStart := (kqOffset / sizeOfCacheLine) * sizeOfCacheLine
	lineEnd := lineStart + sizeOfCacheLine

	if kqEnd <= lineEnd {
		fmt.Printf("✓ kq is isolated on its own cache line (%d-%d)\n", lineStart, lineEnd-1)
	} else {
		t.Errorf("FAIL: kq shares cache line (ends at %d, line ends at %d)", kqEnd, lineEnd)
	}

	// Check closed offset
	closedOffset := unsafe.Offsetof(s.closed)
	closedSize := unsafe.Sizeof(s.closed)
	fmt.Printf("closed: offset=%d, size=%d\n", closedOffset, closedSize)

	// Verify closed atomic.Bool is on its own cache line
	closedLineStart := (closedOffset / sizeOfCacheLine) * sizeOfCacheLine
	closedLineEnd := closedLineStart + sizeOfCacheLine

	closedEnd := closedOffset + closedSize
	if closedOffset >= closedLineStart && closedEnd <= closedLineEnd {
		fmt.Printf("✓ closed is isolated on its own cache line (%d-%d)\n", closedLineStart, closedLineEnd-1)
	} else {
		t.Errorf("FAIL: closed atomic.Bool shares cache line (offset %d, ends at %d, line %d-%d)", closedOffset, closedEnd, closedLineStart, closedLineEnd-1)
	}

	// Check initialized offset
	initOffset := unsafe.Offsetof(s.initialized)
	initSize := unsafe.Sizeof(s.initialized)
	fmt.Printf("initialized: offset=%d, size=%d\n", initOffset, initSize)

	// Verify initialized atomic.Bool is on its own cache line (separate from closed)
	initLineStart := (initOffset / sizeOfCacheLine) * sizeOfCacheLine
	initLineEnd := initLineStart + sizeOfCacheLine

	initEnd := initOffset + initSize
	if initOffset >= initLineStart && initEnd <= initLineEnd {
		fmt.Printf("✓ initialized is isolated on its own cache line (%d-%d)\n", initLineStart, initLineEnd-1)
	} else {
		t.Errorf("FAIL: initialized shares cache line (offset %d, ends at %d, line %d-%d)", initOffset, initEnd, initLineStart, initLineEnd-1)
	}

	// Verify closed and initialized are on DIFFERENT cache lines
	if closedLineStart != initLineStart {
		fmt.Printf("✓ closed and initialized on different cache lines (%d vs %d)\n", closedLineStart, initLineStart)
	} else {
		t.Errorf("FAIL: closed and initialized share same cache line (%d)", closedLineStart)
	}

	// Verify total structure size
	expectedMinSize := sizeOfCacheLine + kqSize + (sizeOfCacheLine - kqSize) + // padding before and after kq
		sizeOfCacheLine + uintptr(closedSize) + (sizeOfCacheLine - uintptr(closedSize)) + // padding before and after closed
		sizeOfCacheLine + uintptr(initSize) // padding before initialized + initialized

	actualSize := unsafe.Sizeof(*s)
	fmt.Printf("\nExpected minimum size: %d bytes\n", expectedMinSize)
	fmt.Printf("Actual size: %d bytes\n", actualSize)

	if actualSize < expectedMinSize {
		t.Errorf("FAIL: struct too small (got %d, expected at least %d)", actualSize, expectedMinSize)
	} else {
		fmt.Printf("✓ Struct size sufficient for cache line padding\n")
	}
}
