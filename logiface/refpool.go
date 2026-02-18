package logiface

import (
	"sync"
)

type (
	refPoolItem struct {
		a any
		b any
	}
)

var (
	refPool = sync.Pool{New: func() any { return new(refPoolItem) }}
)

func refPoolGet() *refPoolItem {
	return refPool.Get().(*refPoolItem)
}

func refPoolPut(item *refPoolItem) {
	*item = refPoolItem{}
	refPool.Put(item)
}
