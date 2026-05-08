//go:build amd64 || arm64

package pollerdynamic

import "unsafe"

var (
	_ [16 - int(unsafe.Sizeof(entry{}))]byte
	_ [int(unsafe.Sizeof(entry{})) - 16]byte
	_ [8 - int(unsafe.Offsetof(entry{}.events))]byte
	_ [int(unsafe.Offsetof(entry{}.events)) - 8]byte
)
