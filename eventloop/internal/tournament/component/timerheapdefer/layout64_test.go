//go:build amd64 || arm64

package timerheapdefer

import (
	"testing"
	"unsafe"
)

var (
	_ [72 - unsafe.Sizeof(timer{})]byte
	_ [unsafe.Sizeof(timer{}) - 72]byte
	_ [0 - unsafe.Offsetof(timer{}.when)]byte
	_ [unsafe.Offsetof(timer{}.when) - 0]byte
	_ [24 - unsafe.Offsetof(timer{}.task)]byte
	_ [unsafe.Offsetof(timer{}.task) - 24]byte
	_ [32 - unsafe.Offsetof(timer{}.id)]byte
	_ [unsafe.Offsetof(timer{}.id) - 32]byte
	_ [40 - unsafe.Offsetof(timer{}.earliestTick)]byte
	_ [unsafe.Offsetof(timer{}.earliestTick) - 40]byte
	_ [48 - unsafe.Offsetof(timer{}.heapIndex)]byte
	_ [unsafe.Offsetof(timer{}.heapIndex) - 48]byte
	_ [56 - unsafe.Offsetof(timer{}.canceled)]byte
	_ [unsafe.Offsetof(timer{}.canceled) - 56]byte
	_ [60 - unsafe.Offsetof(timer{}.nestingLevel)]byte
	_ [unsafe.Offsetof(timer{}.nestingLevel) - 60]byte
	_ [64 - unsafe.Offsetof(timer{}.refed)]byte
	_ [unsafe.Offsetof(timer{}.refed) - 64]byte
	_ [8 - unsafe.Sizeof(timerHeap{}[0])]byte
	_ [unsafe.Sizeof(timerHeap{}[0]) - 8]byte
)

func TestNativeLayout64(t *testing.T) {
	if got := unsafe.Sizeof(timer{}); got != 72 {
		t.Fatalf("timer size = %d, want 72", got)
	}
	wants := map[string]uintptr{
		"when": 0, "task": 24, "id": 32, "earliestTick": 40, "heapIndex": 48,
		"canceled": 56, "nestingLevel": 60, "refed": 64,
	}
	values := map[string]uintptr{
		"when": unsafe.Offsetof(timer{}.when), "task": unsafe.Offsetof(timer{}.task),
		"id": unsafe.Offsetof(timer{}.id), "earliestTick": unsafe.Offsetof(timer{}.earliestTick),
		"heapIndex": unsafe.Offsetof(timer{}.heapIndex), "canceled": unsafe.Offsetof(timer{}.canceled),
		"nestingLevel": unsafe.Offsetof(timer{}.nestingLevel), "refed": unsafe.Offsetof(timer{}.refed),
	}
	for name, got := range values {
		if got != wants[name] {
			t.Fatalf("timer.%s offset = %d, want %d", name, got, wants[name])
		}
	}
	if got := unsafe.Sizeof((*timer)(nil)); got != 8 {
		t.Fatalf("heap element size = %d, want 8", got)
	}
}
