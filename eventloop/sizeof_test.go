package eventloop

import (
	"sync/atomic"
	"testing"
	"unsafe"

	"golang.org/x/sys/cpu"
)

// Special case - we use 128 bytes for cache line size on all platforms.
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
