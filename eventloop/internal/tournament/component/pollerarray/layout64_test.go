//go:build amd64 || arm64

package pollerarray

import "unsafe"

var (
	_ [16 - int(unsafe.Sizeof(entry{}))]byte
	_ [int(unsafe.Sizeof(entry{})) - 16]byte
	_ [3264 - int(unsafe.Offsetof(Table{}.entries))]byte
	_ [int(unsafe.Offsetof(Table{}.entries)) - 3264]byte
	_ [1_051_848 - int(unsafe.Sizeof(Table{}))]byte
	_ [int(unsafe.Sizeof(Table{})) - 1_051_848]byte
)
