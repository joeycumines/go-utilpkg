package logiface

import (
	"sync"
	"unsafe"
)

type (
	refPoolItem struct {
		a unsafe.Pointer
		b unsafe.Pointer
	}
)

var (
	// used to store pairs of pointers, to avoid allocations - used to extend
	// functionality of existing implementations, using unsafe rather than
	// wrapping them a type which requires an allocation
	refPool = sync.Pool{New: func() interface{} { return new(refPoolItem) }}
)

func refPoolGet() *refPoolItem {
	return refPool.Get().(*refPoolItem)
}

func refPoolPut(item *refPoolItem) {
	*item = refPoolItem{}
	refPool.Put(item)
}
