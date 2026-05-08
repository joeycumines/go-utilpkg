//go:build amd64 || arm64

package timerrefclosureb77

import (
	"testing"
	"unsafe"
)

var (
	_ [64 - unsafe.Sizeof(timer{})]byte
	_ [unsafe.Sizeof(timer{}) - 64]byte
	_ [8 - unsafe.Alignof(timer{})]byte
	_ [unsafe.Alignof(timer{}) - 8]byte
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
	_ [56 - unsafe.Offsetof(timer{}.refed)]byte
	_ [unsafe.Offsetof(timer{}.refed) - 56]byte
)

func TestSourceTimerNodeLayout64(t *testing.T) {
	var value timer
	if got := unsafe.Sizeof(value); got != 64 {
		t.Fatalf("timer size = %d, want 64", got)
	}
	if got := unsafe.Alignof(value); got != 8 {
		t.Fatalf("timer alignment = %d, want 8", got)
	}
	if got := unsafe.Offsetof(value.refed); got != 56 {
		t.Fatalf("timer refed offset = %d, want 56", got)
	}
	offsets := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"when", unsafe.Offsetof(value.when), 0},
		{"task", unsafe.Offsetof(value.task), 24},
		{"id", unsafe.Offsetof(value.id), 32},
		{"heapIndex", unsafe.Offsetof(value.heapIndex), 40},
		{"canceled", unsafe.Offsetof(value.canceled), 48},
		{"nestingLevel", unsafe.Offsetof(value.nestingLevel), 52},
		{"refed", unsafe.Offsetof(value.refed), 56},
	}
	for _, offset := range offsets {
		if offset.got != offset.want {
			t.Fatalf("timer %s offset = %d, want %d", offset.name, offset.got, offset.want)
		}
	}
}
