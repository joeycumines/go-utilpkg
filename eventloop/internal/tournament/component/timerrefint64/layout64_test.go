//go:build amd64 || arm64

package timerrefint64

import (
	"testing"
	"unsafe"
)

var (
	_ [4 - unsafe.Sizeof(entry{})]byte
	_ [unsafe.Sizeof(entry{}) - 4]byte
	_ [16 - unsafe.Sizeof(Core{})]byte
	_ [unsafe.Sizeof(Core{}) - 16]byte
	_ [8 - unsafe.Offsetof(Core{}.refed)]byte
	_ [unsafe.Offsetof(Core{}.refed) - 8]byte
)

func TestNativeLayout64(t *testing.T) {
	if got := unsafe.Sizeof(entry{}); got != 4 {
		t.Fatalf("entry size = %d, want 4", got)
	}
	if got := unsafe.Sizeof(Core{}); got != 16 {
		t.Fatalf("core size = %d, want 16", got)
	}
	if got := unsafe.Offsetof(Core{}.refed); got != 8 {
		t.Fatalf("core.refed offset = %d, want 8", got)
	}
}
