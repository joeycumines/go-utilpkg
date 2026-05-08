//go:build linux

package pollerbounded

import (
	"testing"
	"unsafe"
)

var (
	_ [48 - int(unsafe.Sizeof(entry{}))]byte
	_ [int(unsafe.Sizeof(entry{})) - 48]byte
	_ [16 - int(unsafe.Offsetof(entry{}.generation))]byte
	_ [int(unsafe.Offsetof(entry{}.generation)) - 16]byte
	_ [24 - int(unsafe.Offsetof(entry{}.pollFD))]byte
	_ [int(unsafe.Offsetof(entry{}.pollFD)) - 24]byte
	_ [32 - int(unsafe.Offsetof(entry{}.events))]byte
	_ [int(unsafe.Offsetof(entry{}.events)) - 32]byte
	_ [36 - int(unsafe.Offsetof(entry{}.active))]byte
	_ [int(unsafe.Offsetof(entry{}.active)) - 36]byte
	_ [39 - int(unsafe.Offsetof(entry{}.kernelActive))]byte
	_ [int(unsafe.Offsetof(entry{}.kernelActive)) - 39]byte
)

func TestTableLinuxEntryOffsets(t *testing.T) {
	for name, got := range map[string]uintptr{
		"callback":     unsafe.Offsetof(entry{}.callback),
		"dispatch":     unsafe.Offsetof(entry{}.dispatch),
		"generation":   unsafe.Offsetof(entry{}.generation),
		"pollFD":       unsafe.Offsetof(entry{}.pollFD),
		"events":       unsafe.Offsetof(entry{}.events),
		"active":       unsafe.Offsetof(entry{}.active),
		"kernelActive": unsafe.Offsetof(entry{}.kernelActive),
		"ownsPollFD":   unsafe.Offsetof(entry{}.ownsPollFD),
	} {
		want := map[string]uintptr{"callback": 0, "dispatch": 8, "generation": 16, "pollFD": 24, "events": 32, "active": 36, "kernelActive": 39, "ownsPollFD": 40}[name]
		if got != want {
			t.Errorf("%s offset = %d, want %d", name, got, want)
		}
	}
}
