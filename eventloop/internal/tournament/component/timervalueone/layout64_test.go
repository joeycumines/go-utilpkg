//go:build amd64 || arm64

package timervalueone

import (
	"testing"
	"unsafe"
)

var (
	_ [24 - unsafe.Sizeof(SafeTask{})]byte
	_ [unsafe.Sizeof(SafeTask{}) - 24]byte
	_ [0 - unsafe.Offsetof(SafeTask{}.Fn)]byte
	_ [unsafe.Offsetof(SafeTask{}.Fn) - 0]byte
	_ [8 - unsafe.Offsetof(SafeTask{}.ID)]byte
	_ [unsafe.Offsetof(SafeTask{}.ID) - 8]byte
	_ [16 - unsafe.Offsetof(SafeTask{}.Lane)]byte
	_ [unsafe.Offsetof(SafeTask{}.Lane) - 16]byte
	_ [48 - unsafe.Sizeof(timer{})]byte
	_ [unsafe.Sizeof(timer{}) - 48]byte
	_ [0 - unsafe.Offsetof(timer{}.when)]byte
	_ [unsafe.Offsetof(timer{}.when) - 0]byte
	_ [24 - unsafe.Offsetof(timer{}.task)]byte
	_ [unsafe.Offsetof(timer{}.task) - 24]byte
	_ [48 - unsafe.Sizeof(timerHeap{}[0])]byte
	_ [unsafe.Sizeof(timerHeap{}[0]) - 48]byte
)

func TestNativeLayout64(t *testing.T) {
	if got := unsafe.Sizeof(SafeTask{}); got != 24 {
		t.Fatalf("SafeTask size = %d, want 24", got)
	}
	for name, got := range map[string]uintptr{
		"Fn": unsafe.Offsetof(SafeTask{}.Fn), "ID": unsafe.Offsetof(SafeTask{}.ID),
		"Lane": unsafe.Offsetof(SafeTask{}.Lane),
	} {
		want := map[string]uintptr{"Fn": 0, "ID": 8, "Lane": 16}[name]
		if got != want {
			t.Fatalf("SafeTask.%s offset = %d, want %d", name, got, want)
		}
	}
	if got := unsafe.Sizeof(timer{}); got != 48 {
		t.Fatalf("timer size = %d, want 48", got)
	}
	if got := unsafe.Offsetof(timer{}.task); got != 24 {
		t.Fatalf("timer.task offset = %d, want 24", got)
	}
}
