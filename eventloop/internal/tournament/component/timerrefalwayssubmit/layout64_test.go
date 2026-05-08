//go:build amd64 || arm64

package timerrefalwayssubmit

import (
	"testing"
	"unsafe"
)

func TestNativeLayout64(t *testing.T) {
	if got := unsafe.Sizeof(entry{}); got != 4 {
		t.Fatalf("entry size = %d, want 4", got)
	}
	if got := unsafe.Sizeof(Core{}); got != 72 {
		t.Fatalf("core size = %d, want 72", got)
	}
}
