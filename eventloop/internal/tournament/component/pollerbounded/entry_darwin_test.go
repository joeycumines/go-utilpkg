//go:build darwin

package pollerbounded

import (
	"testing"
	"unsafe"
)

var (
	_ [48 - int(unsafe.Sizeof(entry{}))]byte
	_ [int(unsafe.Sizeof(entry{})) - 48]byte
	_ [16 - int(unsafe.Offsetof(entry{}.kernelTag))]byte
	_ [int(unsafe.Offsetof(entry{}.kernelTag)) - 16]byte
	_ [24 - int(unsafe.Offsetof(entry{}.generation))]byte
	_ [int(unsafe.Offsetof(entry{}.generation)) - 24]byte
	_ [32 - int(unsafe.Offsetof(entry{}.pollFD))]byte
	_ [int(unsafe.Offsetof(entry{}.pollFD)) - 32]byte
	_ [40 - int(unsafe.Offsetof(entry{}.events))]byte
	_ [int(unsafe.Offsetof(entry{}.events)) - 40]byte
	_ [44 - int(unsafe.Offsetof(entry{}.active))]byte
	_ [int(unsafe.Offsetof(entry{}.active)) - 44]byte
)

func TestTableDarwinEntryOffsets(t *testing.T) {
	for name, got := range map[string]uintptr{
		"callback":    unsafe.Offsetof(entry{}.callback),
		"dispatch":    unsafe.Offsetof(entry{}.dispatch),
		"kernelTag":   unsafe.Offsetof(entry{}.kernelTag),
		"generation":  unsafe.Offsetof(entry{}.generation),
		"pollFD":      unsafe.Offsetof(entry{}.pollFD),
		"events":      unsafe.Offsetof(entry{}.events),
		"active":      unsafe.Offsetof(entry{}.active),
		"internal":    unsafe.Offsetof(entry{}.internal),
		"ownsPollFD":  unsafe.Offsetof(entry{}.ownsPollFD),
		"provisional": unsafe.Offsetof(entry{}.provisional),
	} {
		want := map[string]uintptr{"callback": 0, "dispatch": 8, "kernelTag": 16, "generation": 24, "pollFD": 32, "events": 40, "active": 44, "internal": 45, "provisional": 46, "ownsPollFD": 47}[name]
		if got != want {
			t.Errorf("%s offset = %d, want %d", name, got, want)
		}
	}
}
