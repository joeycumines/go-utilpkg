//go:build amd64 || arm64

package timerrefrwmutex

import (
	"testing"
	"unsafe"
)

func TestNativeLayout64(t *testing.T) {
	if got := unsafe.Sizeof(entry{}); got != 4 {
		t.Fatalf("entry size = %d, want 4", got)
	}
	if got := unsafe.Sizeof(Core{}); got != 40 {
		t.Fatalf("core size = %d, want 40", got)
	}
}
