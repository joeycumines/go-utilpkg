//go:build amd64 || arm64

package timerheapdeadline

import (
	"testing"
	"unsafe"
)

var (
	_ [56 - unsafe.Sizeof(timer{})]byte
	_ [unsafe.Sizeof(timer{}) - 56]byte
	_ [0 - unsafe.Offsetof(timer{}.when)]byte
	_ [unsafe.Offsetof(timer{}.when) - 0]byte
	_ [24 - unsafe.Offsetof(timer{}.task)]byte
	_ [unsafe.Offsetof(timer{}.task) - 24]byte
	_ [32 - unsafe.Offsetof(timer{}.id)]byte
	_ [unsafe.Offsetof(timer{}.id) - 32]byte
	_ [40 - unsafe.Offsetof(timer{}.heapIndex)]byte
	_ [unsafe.Offsetof(timer{}.heapIndex) - 40]byte
	_ [48 - unsafe.Offsetof(timer{}.canceled)]byte
	_ [unsafe.Offsetof(timer{}.canceled) - 48]byte
	_ [52 - unsafe.Offsetof(timer{}.nestingLevel)]byte
	_ [unsafe.Offsetof(timer{}.nestingLevel) - 52]byte
	_ [8 - unsafe.Sizeof(timerHeap{}[0])]byte
	_ [unsafe.Sizeof(timerHeap{}[0]) - 8]byte
)

func TestNativeLayout64(t *testing.T) {
	if got := unsafe.Sizeof(timer{}); got != 56 {
		t.Fatalf("timer size = %d, want 56", got)
	}
	for name, got := range map[string]uintptr{
		"when":         unsafe.Offsetof(timer{}.when),
		"task":         unsafe.Offsetof(timer{}.task),
		"id":           unsafe.Offsetof(timer{}.id),
		"heapIndex":    unsafe.Offsetof(timer{}.heapIndex),
		"canceled":     unsafe.Offsetof(timer{}.canceled),
		"nestingLevel": unsafe.Offsetof(timer{}.nestingLevel),
	} {
		want := map[string]uintptr{"when": 0, "task": 24, "id": 32, "heapIndex": 40, "canceled": 48, "nestingLevel": 52}[name]
		if got != want {
			t.Fatalf("timer.%s offset = %d, want %d", name, got, want)
		}
	}
	if got := unsafe.Sizeof(timerHeap{}[0]); got != 8 {
		t.Fatalf("heap element size = %d, want 8", got)
	}
}
