package stumpy

import (
	"github.com/joeycumines/go-utilpkg/logiface"
	"io"
	"sync"
	"time"
)

type (
	Logger struct {
		//lint:ignore U1000 embedded for it's methods
		unimplementedJSONSupport

		writer io.Writer

		// these are pre-emptively json encoded

		timeField    string
		levelField   string
		messageField string
	}

	//lint:ignore U1000 used to embed without exporting
	unimplementedJSONSupport = logiface.UnimplementedJSONSupport[*Event, *Event, *Event]
)

var (
	eventPool = sync.Pool{New: func() any {
		return &Event{
			buf: make([]byte, 0, 1<<10),
			off: make([]int, 0, 8),
		}
	}}
	timeNow = time.Now
)

func (x *Logger) NewEvent(level logiface.Level) (e *Event) {
	e = eventPool.Get().(*Event)

	e.logger = x

	e.lvl = level

	// note: off isn't reset when added back to the pool
	e.off = e.off[:0]

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
	if cap(e.buf) <= 1<<16 && cap(e.off) <= 1<<13 {
		// clear references that might need to be garbage collected
		e.logger = nil

		eventPool.Put(e)
	}
}

func (x *Logger) NewObject() *Event {
	panic(`stumpy.Logger.NewObject() should never be called`)
}

func (x *Logger) CanAddStartObject() bool { return true }

func (x *Logger) AddStartObject(evt *Event, key string) *Event {
	return x.SetStartObject(evt, key)
}

func (x *Logger) CanSetStartObject() bool { return true }

func (x *Logger) SetStartObject(obj *Event, key string) *Event {
	obj.enterKey(key)
	obj.buf = append(obj.buf, '{')
	return obj
}

func (x *Logger) CanSetStartArray() bool { return true }

func (x *Logger) SetStartArray(obj *Event, key string) *Event {
	obj.enterKey(key)
	obj.buf = append(obj.buf, '[')
	return obj
}

func (x *Logger) CanSetObject() bool { return true }

func (x *Logger) SetObject(obj *Event, key string, val *Event) *Event {
	if obj != val {
		panic(`stumpy.Logger.SetObject() should never be called with a different *Event`)
	}
	obj.exitKey(key)
	obj.buf = append(obj.buf, '}')
	return obj
}

func (x *Logger) CanSetArray() bool { return true }

func (x *Logger) SetArray(obj *Event, key string, val *Event) *Event {
	if obj != val {
		panic(`stumpy.Logger.SetArray() should never be called with a different *Event`)
	}
	obj.exitKey(key)
	obj.buf = append(obj.buf, ']')
	return obj
}

func (x *Logger) AddObject(evt *Event, key string, obj *Event) {
	x.SetObject(evt, key, obj)
}

func (x *Logger) SetField(obj *Event, key string, val any) *Event {
	obj.appendKey(key)
	obj.appendInterface(val)
	return obj
}

func (x *Logger) NewArray() *Event {
	panic(`stumpy.Logger.NewArray() should never be called`)
}

func (x *Logger) CanAddStartArray() bool { return true }

func (x *Logger) AddStartArray(evt *Event, key string) *Event {
	return x.SetStartArray(evt, key)
}

func (x *Logger) CanAppendStartObject() bool { return true }

func (x *Logger) AppendStartObject(arr *Event) *Event {
	arr.appendArraySeparator()
	arr.buf = append(arr.buf, '{')
	return arr
}

func (x *Logger) CanAppendStartArray() bool { return true }

func (x *Logger) AppendStartArray(arr *Event) *Event {
	arr.appendArraySeparator()
	arr.buf = append(arr.buf, '[')
	return arr
}

func (x *Logger) AddArray(evt *Event, key string, arr *Event) {
	x.SetArray(evt, key, arr)
}

func (x *Logger) CanAppendObject() bool { return true }

func (x *Logger) AppendObject(arr *Event, val *Event) *Event {
	if arr != val {
		panic(`stumpy.Logger.AppendObject() should never be called with a different *Event`)
	}
	arr.buf = append(arr.buf, '}')
	return arr
}

func (x *Logger) CanAppendArray() bool { return true }

func (x *Logger) AppendArray(arr *Event, val *Event) *Event {
	if arr != val {
		panic(`stumpy.Logger.AppendArray() should never be called with a different *Event`)
	}
	arr.buf = append(arr.buf, ']')
	return arr
}

func (x *Logger) AppendField(arr *Event, val any) *Event {
	arr.appendArraySeparator()
	arr.appendInterface(val)
	return arr
}
