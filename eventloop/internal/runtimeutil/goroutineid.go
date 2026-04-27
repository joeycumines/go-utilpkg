package runtimeutil

import (
	"sync"

	"github.com/joeycumines/goroutineid"
)

var bufPool = sync.Pool{New: func() any {
	return new(make([]byte, 64))
}}

func GoroutineID() (ID int64) {
	ID = goroutineid.Fast()
	if ID == -1 {
		buf := bufPool.Get().(*[]byte)
		ID = goroutineid.Slow(*buf)
		bufPool.Put(buf)
	}
	return
}
