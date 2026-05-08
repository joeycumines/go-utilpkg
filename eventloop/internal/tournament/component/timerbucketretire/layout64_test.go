//go:build amd64 || arm64

package timerbucketretire

import (
	"testing"
	"unsafe"
)

var (
	_ [112 - unsafe.Sizeof(timer{})]byte
	_ [unsafe.Sizeof(timer{}) - 112]byte
	_ [0 - unsafe.Offsetof(timer{}.when)]byte
	_ [unsafe.Offsetof(timer{}.when) - 0]byte
	_ [24 - unsafe.Offsetof(timer{}.task)]byte
	_ [unsafe.Offsetof(timer{}.task) - 24]byte
	_ [32 - unsafe.Offsetof(timer{}.retire)]byte
	_ [unsafe.Offsetof(timer{}.retire) - 32]byte
	_ [40 - unsafe.Offsetof(timer{}.prev)]byte
	_ [unsafe.Offsetof(timer{}.prev) - 40]byte
	_ [48 - unsafe.Offsetof(timer{}.next)]byte
	_ [unsafe.Offsetof(timer{}.next) - 48]byte
	_ [56 - unsafe.Offsetof(timer{}.list)]byte
	_ [unsafe.Offsetof(timer{}.list) - 56]byte
	_ [64 - unsafe.Offsetof(timer{}.id)]byte
	_ [unsafe.Offsetof(timer{}.id) - 64]byte
	_ [72 - unsafe.Offsetof(timer{}.earliestTick)]byte
	_ [unsafe.Offsetof(timer{}.earliestTick) - 72]byte
	_ [80 - unsafe.Offsetof(timer{}.interval)]byte
	_ [unsafe.Offsetof(timer{}.interval) - 80]byte
	_ [88 - unsafe.Offsetof(timer{}.heapIndex)]byte
	_ [unsafe.Offsetof(timer{}.heapIndex) - 88]byte
	_ [96 - unsafe.Offsetof(timer{}.canceled)]byte
	_ [unsafe.Offsetof(timer{}.canceled) - 96]byte
	_ [100 - unsafe.Offsetof(timer{}.nestingLevel)]byte
	_ [unsafe.Offsetof(timer{}.nestingLevel) - 100]byte
	_ [104 - unsafe.Offsetof(timer{}.nestedClamp)]byte
	_ [unsafe.Offsetof(timer{}.nestedClamp) - 104]byte
	_ [105 - unsafe.Offsetof(timer{}.executing)]byte
	_ [unsafe.Offsetof(timer{}.executing) - 105]byte
	_ [106 - unsafe.Offsetof(timer{}.repeat)]byte
	_ [unsafe.Offsetof(timer{}.repeat) - 106]byte
	_ [108 - unsafe.Offsetof(timer{}.refed)]byte
	_ [unsafe.Offsetof(timer{}.refed) - 108]byte
	_ [64 - unsafe.Sizeof(timerList{})]byte
	_ [unsafe.Sizeof(timerList{}) - 64]byte
	_ [0 - unsafe.Offsetof(timerList{}.deadline)]byte
	_ [unsafe.Offsetof(timerList{}.deadline) - 0]byte
	_ [24 - unsafe.Offsetof(timerList{}.head)]byte
	_ [unsafe.Offsetof(timerList{}.head) - 24]byte
	_ [32 - unsafe.Offsetof(timerList{}.tail)]byte
	_ [unsafe.Offsetof(timerList{}.tail) - 32]byte
	_ [40 - unsafe.Offsetof(timerList{}.key)]byte
	_ [unsafe.Offsetof(timerList{}.key) - 40]byte
	_ [48 - unsafe.Offsetof(timerList{}.heapIndex)]byte
	_ [unsafe.Offsetof(timerList{}.heapIndex) - 48]byte
	_ [56 - unsafe.Offsetof(timerList{}.len)]byte
	_ [unsafe.Offsetof(timerList{}.len) - 56]byte
	_ [8 - unsafe.Sizeof(timerListHeap{}[0])]byte
	_ [unsafe.Sizeof(timerListHeap{}[0]) - 8]byte
)

func TestNativeLayout64(t *testing.T) {
	if got := unsafe.Sizeof(timer{}); got != 112 {
		t.Fatalf("timer size = %d, want 112", got)
	}
	timerWants := map[string]uintptr{
		"when": 0, "task": 24, "retire": 32, "prev": 40, "next": 48, "list": 56,
		"id": 64, "earliestTick": 72, "interval": 80, "heapIndex": 88,
		"canceled": 96, "nestingLevel": 100, "nestedClamp": 104,
		"executing": 105, "repeat": 106, "refed": 108,
	}
	timerValues := map[string]uintptr{
		"when": unsafe.Offsetof(timer{}.when), "task": unsafe.Offsetof(timer{}.task),
		"retire": unsafe.Offsetof(timer{}.retire), "prev": unsafe.Offsetof(timer{}.prev),
		"next": unsafe.Offsetof(timer{}.next), "list": unsafe.Offsetof(timer{}.list),
		"id": unsafe.Offsetof(timer{}.id), "earliestTick": unsafe.Offsetof(timer{}.earliestTick),
		"interval": unsafe.Offsetof(timer{}.interval), "heapIndex": unsafe.Offsetof(timer{}.heapIndex),
		"canceled": unsafe.Offsetof(timer{}.canceled), "nestingLevel": unsafe.Offsetof(timer{}.nestingLevel),
		"nestedClamp": unsafe.Offsetof(timer{}.nestedClamp), "executing": unsafe.Offsetof(timer{}.executing),
		"repeat": unsafe.Offsetof(timer{}.repeat), "refed": unsafe.Offsetof(timer{}.refed),
	}
	for name, got := range timerValues {
		if got != timerWants[name] {
			t.Fatalf("timer.%s offset = %d, want %d", name, got, timerWants[name])
		}
	}
	if got := unsafe.Sizeof(timerList{}); got != 64 {
		t.Fatalf("timerList size = %d, want 64", got)
	}
	listWants := map[string]uintptr{"deadline": 0, "head": 24, "tail": 32, "key": 40, "heapIndex": 48, "len": 56}
	listValues := map[string]uintptr{
		"deadline": unsafe.Offsetof(timerList{}.deadline), "head": unsafe.Offsetof(timerList{}.head),
		"tail": unsafe.Offsetof(timerList{}.tail), "key": unsafe.Offsetof(timerList{}.key),
		"heapIndex": unsafe.Offsetof(timerList{}.heapIndex), "len": unsafe.Offsetof(timerList{}.len),
	}
	for name, got := range listValues {
		if got != listWants[name] {
			t.Fatalf("timerList.%s offset = %d, want %d", name, got, listWants[name])
		}
	}
}
