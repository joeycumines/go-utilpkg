//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"sync/atomic"
	"unsafe"
)

const sizeOfCacheLine = 128
const sizeOfAtomicUint64 = 8

type FastState struct {
	_ [sizeOfCacheLine]byte
	v atomic.Uint64
	_ [sizeOfCacheLine - sizeOfAtomicUint64]byte
}

func main() {
	s := FastState{}
	fmt.Printf("FastState size: %d bytes\n", unsafe.Sizeof(s))
	fmt.Printf("v offset: %d, size: %d\n", unsafe.Offsetof(s.v), unsafe.Sizeof(s.v))
}
