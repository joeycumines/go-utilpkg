//go:build linux || darwin

package eventloop

import (
	"fmt"
	"testing"
	"unsafe"
)

// TestFastPollerAlign_Unix validates Unix FastPoller struct alignment (Linux and Darwin)
func TestFastPollerAlign_Unix(t *testing.T) {
	s := &FastPoller{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== FastPoller (Unix: Linux/Darwin) ===\n")

	// Check FD offset (epfd on Linux, kq on Darwin)
	var fdOffset uintptr
	var fdName string
	var fdSize uintptr

	// This test builds for both Linux and Darwin, check which field exists
	epfdOffset := unsafe.Offsetof(s.epfd) // Linux
	kqOffset := unsafe.Offsetof(s.kq)     // Darwin

	if epfdOffset > 0 || kqOffset == 0 {
		fdOffset = epfdOffset
		fdName = "epfd"
		fdSize = unsafe.Sizeof(s.epfd)
	} else {
		fdOffset = kqOffset
		fdName = "kq"
		fdSize = unsafe.Sizeof(s.kq)
	}

	fmt.Printf("%s: offset=%d, size=%d\n", fdName, fdOffset, fdSize)

	// Verify FD field is on cache line boundary (should be exactly at sizeOfCacheLine)
	if fdOffset == sizeOfCacheLine {
		fmt.Printf("✓ %s starts on cache line boundary (%d)\n", fdName, fdOffset)
	} else {
		t.Errorf("FAIL: %s not on cache line boundary (got offset %d, expected %d)", fdName, fdOffset, sizeOfCacheLine)
	}

	// Verify FD field is alone on its cache line
	fdEnd := fdOffset + fdSize
	lineStart := (fdOffset / sizeOfCacheLine) * sizeOfCacheLine
	lineEnd := lineStart + sizeOfCacheLine

	if fdEnd <= lineEnd {
		fmt.Printf("✓ %s is isolated on its own cache line (%d-%d)\n", fdName, lineStart, lineEnd-1)
	} else {
		t.Errorf("FAIL: %s shares cache line (ends at %d, line ends at %d)", fdName, fdEnd, lineEnd)
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

	// Verify total structure size
	expectedMinSize := sizeOfCacheLine + fdSize + (sizeOfCacheLine - fdSize) + // padding before and after FD
		sizeOfCacheLine + uintptr(closedSize) + (sizeOfCacheLine - uintptr(closedSize)) // padding before and after closed

	actualSize := unsafe.Sizeof(s)
	fmt.Printf("\nExpected minimum size: %d bytes\n", expectedMinSize)
	fmt.Printf("Actual size: %d bytes\n", actualSize)

	if actualSize < expectedMinSize {
		t.Errorf("FAIL: struct too small (got %d, expected at least %d)", actualSize, expectedMinSize)
	} else {
		fmt.Printf("✓ Struct size sufficient for cache line padding\n")
	}
}
