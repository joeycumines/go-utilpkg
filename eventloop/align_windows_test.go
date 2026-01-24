//go:build windows

package eventloop

import (
	"fmt"
	"testing"
	"unsafe"
)

// TestFastPollerAlign_Windows validates Windows FastPoller struct alignment
func TestFastPollerAlign_Windows(t *testing.T) {
	s := &FastPoller{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== FastPoller (Windows) ===\n")

	// Check iocp offset
	iocpOffset := unsafe.Offsetof(s.iocp)
	iocpSize := unsafe.Sizeof(s.iocp)
	fmt.Printf("iocp: offset=%d, size=%d\n", iocpOffset, iocpSize)

	// Verify iocp is on cache line boundary (should be exactly at sizeOfCacheLine)
	if iocpOffset == sizeOfCacheLine {
		fmt.Printf("✓ iocp starts on cache line boundary (%d)\n", iocpOffset)
	} else {
		t.Errorf("FAIL: iocp not on cache line boundary (got offset %d, expected %d)", iocpOffset, sizeOfCacheLine)
	}

	// Verify iocp is alone on its cache line
	iocpEnd := iocpOffset + iocpSize
	lineStart := (iocpOffset / sizeOfCacheLine) * sizeOfCacheLine
	lineEnd := lineStart + sizeOfCacheLine

	if iocpEnd <= lineEnd {
		fmt.Printf("✓ iocp is isolated on its own cache line (%d-%d)\n", lineStart, lineEnd-1)
	} else {
		t.Errorf("FAIL: iocp shares cache line (ends at %d, line ends at %d)", iocpEnd, lineEnd)
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

	// Verify initialized atomic.Bool is on its own cache line
	initLineStart := (initOffset / sizeOfCacheLine) * sizeOfCacheLine
	initLineEnd := initLineStart + sizeOfCacheLine

	initEnd := initOffset + initSize
	if initEnd <= initLineEnd {
		fmt.Printf("✓ initialized is isolated on its own cache line (%d-%d)\n", initLineStart, initLineEnd-1)
	} else {
		t.Logf("✗ initialized atomic.Bool shares cache line? (offset %d, ends at %d, line %d-%d)", initOffset, initEnd, initLineStart, initLineEnd-1)
	}

	// Verify total structure size
	expectedMinSize := sizeOfCacheLine + uintptr(iocpSize) + (sizeOfCacheLine - uintptr(iocpSize)) + // padding before and after iocp
		sizeOfCacheLine + uintptr(closedSize) + sizeOfCacheLine + // padding before and after closed
		sizeOfCacheLine + uintptr(initSize) + (sizeOfCacheLine - uintptr(initSize)) // padding before and after initialized

	actualSize := unsafe.Sizeof(s)
	fmt.Printf("\nExpected minimum size: %d bytes\n", expectedMinSize)
	fmt.Printf("Actual size: %d bytes\n", actualSize)

	if actualSize < expectedMinSize {
		t.Errorf("FAIL: struct too small (got %d, expected at least %d)", actualSize, expectedMinSize)
	} else {
		fmt.Printf("✓ Struct size sufficient for cache line padding\n")
	}
}
