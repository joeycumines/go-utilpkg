//go:build amd64 || arm64

package timervaluethree

import (
	"testing"
	"unsafe"
)

var (
	_ [8 - unsafe.Sizeof(Task{})]byte
	_ [unsafe.Sizeof(Task{}) - 8]byte
	_ [0 - unsafe.Offsetof(Task{}.Runnable)]byte
	_ [unsafe.Offsetof(Task{}.Runnable) - 0]byte
	_ [32 - unsafe.Sizeof(timer{})]byte
	_ [unsafe.Sizeof(timer{}) - 32]byte
	_ [0 - unsafe.Offsetof(timer{}.when)]byte
	_ [unsafe.Offsetof(timer{}.when) - 0]byte
	_ [24 - unsafe.Offsetof(timer{}.task)]byte
	_ [unsafe.Offsetof(timer{}.task) - 24]byte
	_ [32 - unsafe.Sizeof(timerHeap{}[0])]byte
	_ [unsafe.Sizeof(timerHeap{}[0]) - 32]byte
)

func TestNativeLayout64(t *testing.T) {
	if got := unsafe.Sizeof(Task{}); got != 8 {
		t.Fatalf("Task size = %d, want 8", got)
	}
	if got := unsafe.Offsetof(Task{}.Runnable); got != 0 {
		t.Fatalf("Task.Runnable offset = %d, want 0", got)
	}
	if got := unsafe.Sizeof(timer{}); got != 32 {
		t.Fatalf("timer size = %d, want 32", got)
	}
	if got := unsafe.Offsetof(timer{}.task); got != 24 {
		t.Fatalf("timer.task offset = %d, want 24", got)
	}
}
