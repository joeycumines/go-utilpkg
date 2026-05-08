//go:build !darwin && !linux && (amd64 || arm64)

package pollerbounded

import "unsafe"

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
