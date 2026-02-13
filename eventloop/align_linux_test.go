//go:build linux

package eventloop

import (
	"fmt"
	"testing"
	"unsafe"
)

// Test_fastPollerAlign_Linux validates Linux fastPoller struct alignment
func Test_fastPollerAlign_Linux(t *testing.T) {
	s := &fastPoller{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== fastPoller (Linux) ===\n")

	// Check epfd offset (Linux-specific field)
	epfdOffset := unsafe.Offsetof(s.epfd)
	epfdSize := unsafe.Sizeof(s.epfd)
	fmt.Printf("epfd: offset=%d, size=%d\n", epfdOffset, epfdSize)

	// Verify epfd is on cache line boundary (should be exactly at sizeOfCacheLine)
	if epfdOffset == sizeOfCacheLine {
		fmt.Printf("✓ epfd starts on cache line boundary (%d)\n", epfdOffset)
	} else {
		t.Errorf("FAIL: epfd not on cache line boundary (got offset %d, expected %d)", epfdOffset, sizeOfCacheLine)
	}

	// Verify epfd is alone on its cache line
	epfdEnd := epfdOffset + epfdSize
	lineStart := (epfdOffset / sizeOfCacheLine) * sizeOfCacheLine
	lineEnd := lineStart + sizeOfCacheLine

	if epfdEnd <= lineEnd {
		fmt.Printf("✓ epfd is isolated on its own cache line (%d-%d)\n", lineStart, lineEnd-1)
	} else {
		t.Errorf("FAIL: epfd shares cache line (ends at %d, line ends at %d)", epfdEnd, lineEnd)
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
	expectedMinSize := sizeOfCacheLine + epfdSize + (sizeOfCacheLine - epfdSize) + // padding before and after epfd
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
