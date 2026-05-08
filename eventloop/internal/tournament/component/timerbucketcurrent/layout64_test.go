//go:build amd64 || arm64

package timerbucketcurrent

import (
	"testing"
	"unsafe"
)

var (
	_ [120 - unsafe.Sizeof(timer{})]byte
	_ [unsafe.Sizeof(timer{}) - 120]byte
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
	_ [64 - unsafe.Offsetof(timer{}.publication)]byte
	_ [unsafe.Offsetof(timer{}.publication) - 64]byte
	_ [72 - unsafe.Offsetof(timer{}.id)]byte
	_ [unsafe.Offsetof(timer{}.id) - 72]byte
	_ [80 - unsafe.Offsetof(timer{}.scheduledTick)]byte
	_ [unsafe.Offsetof(timer{}.scheduledTick) - 80]byte
	_ [88 - unsafe.Offsetof(timer{}.interval)]byte
	_ [unsafe.Offsetof(timer{}.interval) - 88]byte
	_ [96 - unsafe.Offsetof(timer{}.heapIndex)]byte
	_ [unsafe.Offsetof(timer{}.heapIndex) - 96]byte
	_ [104 - unsafe.Offsetof(timer{}.canceled)]byte
	_ [unsafe.Offsetof(timer{}.canceled) - 104]byte
	_ [108 - unsafe.Offsetof(timer{}.deferTick)]byte
	_ [unsafe.Offsetof(timer{}.deferTick) - 108]byte
	_ [109 - unsafe.Offsetof(timer{}.executing)]byte
	_ [unsafe.Offsetof(timer{}.executing) - 109]byte
	_ [110 - unsafe.Offsetof(timer{}.repeat)]byte
	_ [unsafe.Offsetof(timer{}.repeat) - 110]byte
	_ [112 - unsafe.Offsetof(timer{}.refed)]byte
	_ [unsafe.Offsetof(timer{}.refed) - 112]byte
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
	if got := unsafe.Sizeof(timer{}); got != 120 {
		t.Fatalf("timer size = %d, want 120", got)
	}
	timerWants := map[string]uintptr{
		"when": 0, "task": 24, "retire": 32, "prev": 40, "next": 48, "list": 56,
		"publication": 64, "id": 72, "scheduledTick": 80, "interval": 88,
		"heapIndex": 96, "canceled": 104, "deferTick": 108,
		"executing": 109, "repeat": 110, "refed": 112,
	}
	timerValues := map[string]uintptr{
		"when": unsafe.Offsetof(timer{}.when), "task": unsafe.Offsetof(timer{}.task),
		"retire": unsafe.Offsetof(timer{}.retire), "prev": unsafe.Offsetof(timer{}.prev),
		"next": unsafe.Offsetof(timer{}.next), "list": unsafe.Offsetof(timer{}.list),
		"publication": unsafe.Offsetof(timer{}.publication), "id": unsafe.Offsetof(timer{}.id),
		"scheduledTick": unsafe.Offsetof(timer{}.scheduledTick), "interval": unsafe.Offsetof(timer{}.interval),
		"heapIndex": unsafe.Offsetof(timer{}.heapIndex), "canceled": unsafe.Offsetof(timer{}.canceled),
		"deferTick": unsafe.Offsetof(timer{}.deferTick), "executing": unsafe.Offsetof(timer{}.executing),
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
