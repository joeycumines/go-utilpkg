//go:build windows

package pollerfixedtoken

import (
	"testing"
	"unsafe"
)

var (
	_ [32 - int(unsafe.Sizeof(entry{}))]byte
	_ [int(unsafe.Sizeof(entry{})) - 32]byte
	_ [16 - int(unsafe.Offsetof(entry{}.generation))]byte
	_ [int(unsafe.Offsetof(entry{}.generation)) - 16]byte
	_ [24 - int(unsafe.Offsetof(entry{}.events))]byte
	_ [int(unsafe.Offsetof(entry{}.events)) - 24]byte
	_ [28 - int(unsafe.Offsetof(entry{}.active))]byte
	_ [int(unsafe.Offsetof(entry{}.active)) - 28]byte
)

func TestTableWindowsEntryOffsets(t *testing.T) {
	for name, got := range map[string]uintptr{
		"callback":   unsafe.Offsetof(entry{}.callback),
		"dispatch":   unsafe.Offsetof(entry{}.dispatch),
		"generation": unsafe.Offsetof(entry{}.generation),
		"events":     unsafe.Offsetof(entry{}.events),
		"active":     unsafe.Offsetof(entry{}.active),
	} {
		want := map[string]uintptr{"callback": 0, "dispatch": 8, "generation": 16, "events": 24, "active": 28}[name]
		if got != want {
			t.Errorf("%s offset = %d, want %d", name, got, want)
		}
	}
}
