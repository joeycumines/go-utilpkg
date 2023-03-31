package stumpy

import (
	"github.com/joeycumines/go-utilpkg/logiface"
	"io"
	"sync"
	"time"
)

type (
	Logger struct {
		writer     io.Writer
		timeField  string
		levelField string
	}
)

var (
	eventPool = sync.Pool{New: func() any { return new(Event) }}
	timeNow   = time.Now
)

func (x *Logger) NewEvent(level logiface.Level) (e *Event) {
	e = eventPool.Get().(*Event)

	e.lvl = level

	// note: buf isn't reset when added back to the pool
	e.buf = append(e.buf[:0], '{')

	if x.timeField != `` {
		e.buf = append(e.buf, x.timeField...)
		e.buf = append(e.buf, ':', '"')
		e.buf = append(e.buf, formatTime(timeNow())...)
		e.buf = append(e.buf, '"')
	}

	if x.levelField != `` {
		e.appendFieldSeparator()
		e.buf = append(e.buf, x.levelField...)
		e.buf = append(e.buf, ':', '"')
		e.buf = append(e.buf, level.String()...)
		e.buf = append(e.buf, '"')
	}

	return
}

func (x *Logger) Write(event *Event) (err error) {
	event.buf = append(event.buf, '}', '\n')
	_, err = x.writer.Write(event.buf)
	return
}

func (x *Logger) ReleaseEvent(e *Event) {
	// sync.Pool depends on each item consuming roughly the same amount of memory
	if cap(e.buf) <= 1<<16 {
		eventPool.Put(e)
	}
}
