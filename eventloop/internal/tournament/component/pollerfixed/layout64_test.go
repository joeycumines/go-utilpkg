//go:build amd64 || arm64

package pollerfixed

import "unsafe"

var (
	_ [32 - int(unsafe.Sizeof(entry{}))]byte
	_ [int(unsafe.Sizeof(entry{})) - 32]byte
	_ [16 - int(unsafe.Offsetof(entry{}.generation))]byte
	_ [int(unsafe.Offsetof(entry{}.generation)) - 16]byte
	_ [24 - int(unsafe.Offsetof(entry{}.events))]byte
	_ [int(unsafe.Offsetof(entry{}.events)) - 24]byte
)
