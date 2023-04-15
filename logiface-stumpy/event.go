package stumpy

import (
	"encoding/json"
	"fmt"
	"github.com/joeycumines/go-utilpkg/jsonenc"
	"github.com/joeycumines/go-utilpkg/logiface"
	"strconv"
)

type (
	Event struct {
		//lint:ignore U1000 embedded for it's methods
		unimplementedEvent

		// WARNING: if adding fields consider if they may need to be reset when added back to the pool
		// (e.g. the reference to the logger - slices which are reused and are reset on init are fine)

		logger *Logger
		lvl    logiface.Level
		buf    []byte
		// off is a stack with the index to insert the key content for each nested json object
		// negative values indicate already set keys
		off []int
	}

	//lint:ignore U1000 used to embed without exporting
	unimplementedEvent = logiface.UnimplementedEvent
)

func (x *Event) Level() logiface.Level {
	return x.lvl
}

func (x *Event) AddField(key string, val any) {
	x.appendKey(key)
	x.appendInterface(val)
}
func (*Logger) SetField(obj *Event, key string, val any) *Event {
	obj.appendKey(key)
	obj.appendInterface(val)
	return obj
}
func (*Logger) AppendField(arr *Event, val any) *Event {
	arr.appendArraySeparator()
	arr.appendInterface(val)
	return arr
}

func (x *Event) AddMessage(msg string) bool {
	x.appendFieldSeparator()
	x.buf = append(x.buf, x.logger.messageField...)
	x.buf = append(x.buf, ':')
	x.appendString(msg)
	return true
}

func (x *Event) AddError(err error) bool {
	if err != nil {
		x.appendErrorKey()
		// this seems sensible, even if it's inefficient
		x.appendString(fmt.Sprint(err))
	}
	return true
}
func (*Logger) CanSetError() bool { return true }
func (*Logger) SetError(obj *Event, err error) *Event {
	obj.appendErrorKey()
	// differs from AddError in that it will always set the error field, even if nil
	if err == nil {
		obj.buf = append(obj.buf, `null`...)
	} else {
		obj.appendString(fmt.Sprint(err))
	}
	return obj
}
func (*Logger) CanAppendError() bool { return true }
func (*Logger) AppendError(arr *Event, err error) *Event {
	arr.appendArraySeparator()
	if err == nil {
		arr.buf = append(arr.buf, `null`...)
	} else {
		arr.appendString(fmt.Sprint(err))
	}
	return arr
}

func (x *Event) AddString(key string, val string) bool {
	x.appendKey(key)
	x.appendString(val)
	return true
}
func (*Logger) CanSetString() bool { return true }
func (*Logger) SetString(obj *Event, key string, val string) *Event {
	obj.appendKey(key)
	obj.appendString(val)
	return obj
}
func (*Logger) CanAppendString() bool { return true }
func (*Logger) AppendString(arr *Event, val string) *Event {
	arr.appendArraySeparator()
	arr.appendString(val)
	return arr
}

func (x *Event) AddInt(key string, val int) bool {
	x.appendKey(key)
	x.appendInt(val)
	return true
}
func (*Logger) CanSetInt() bool { return true }
func (*Logger) SetInt(obj *Event, key string, val int) *Event {
	obj.appendKey(key)
	obj.appendInt(val)
	return obj
}
func (*Logger) CanAppendInt() bool { return true }
func (*Logger) AppendInt(arr *Event, val int) *Event {
	arr.appendArraySeparator()
	arr.appendInt(val)
	return arr
}

func (x *Event) AddFloat32(key string, val float32) bool {
	x.appendKey(key)
	x.appendFloat32(val)
	return true
}
func (*Logger) CanSetFloat32() bool { return true }
func (*Logger) SetFloat32(obj *Event, key string, val float32) *Event {
	obj.appendKey(key)
	obj.appendFloat32(val)
	return obj
}
func (*Logger) CanAppendFloat32() bool { return true }
func (*Logger) AppendFloat32(arr *Event, val float32) *Event {
	arr.appendArraySeparator()
	arr.appendFloat32(val)
	return arr
}

func (x *Event) AddBool(key string, val bool) bool {
	x.appendKey(key)
	x.appendBool(val)
	return true
}
func (*Logger) CanSetBool() bool { return true }
func (*Logger) SetBool(obj *Event, key string, val bool) *Event {
	obj.appendKey(key)
	obj.appendBool(val)
	return obj
}
func (*Logger) CanAppendBool() bool { return true }
func (*Logger) AppendBool(arr *Event, val bool) *Event {
	arr.appendArraySeparator()
	arr.appendBool(val)
	return arr
}

func (x *Event) AddFloat64(key string, val float64) bool {
	x.appendKey(key)
	x.appendFloat64(val)
	return true
}
func (*Logger) CanSetFloat64() bool { return true }
func (*Logger) SetFloat64(obj *Event, key string, val float64) *Event {
	obj.appendKey(key)
	obj.appendFloat64(val)
	return obj
}
func (*Logger) CanAppendFloat64() bool { return true }
func (*Logger) AppendFloat64(arr *Event, val float64) *Event {
	arr.appendArraySeparator()
	arr.appendFloat64(val)
	return arr
}

func (x *Event) AddInt64(key string, val int64) bool {
	x.appendKey(key)
	x.appendInt64(val)
	return true
}
func (*Logger) CanSetInt64() bool { return true }
func (*Logger) SetInt64(obj *Event, key string, val int64) *Event {
	obj.appendKey(key)
	obj.appendInt64(val)
	return obj
}
func (*Logger) CanAppendInt64() bool { return true }
func (*Logger) AppendInt64(arr *Event, val int64) *Event {
	arr.appendArraySeparator()
	arr.appendInt64(val)
	return arr
}

func (x *Event) AddUint64(key string, val uint64) bool {
	x.appendKey(key)
	x.appendUint64(val)
	return true
}
func (*Logger) CanSetUint64() bool { return true }
func (*Logger) SetUint64(obj *Event, key string, val uint64) *Event {
	obj.appendKey(key)
	obj.appendUint64(val)
	return obj
}
func (*Logger) CanAppendUint64() bool { return true }
func (*Logger) AppendUint64(arr *Event, val uint64) *Event {
	arr.appendArraySeparator()
	arr.appendUint64(val)
	return arr
}

func (x *Event) appendFieldSeparator() {
	if x.buf[len(x.buf)-1] != '{' {
		x.buf = append(x.buf, ',')
	}
}

func (x *Event) appendArraySeparator() {
	if x.buf[len(x.buf)-1] != '[' {
		x.buf = append(x.buf, ',')
	}
}

func (x *Event) appendString(val string) {
	x.buf = jsonenc.AppendString(x.buf, val)
}

func (x *Event) appendInt(val int) {
	x.buf = strconv.AppendInt(x.buf, int64(val), 10)
}

func (x *Event) appendFloat64(val float64) {
	x.buf = jsonenc.AppendFloat64(x.buf, val)
}

func (x *Event) appendFloat32(val float32) {
	x.buf = jsonenc.AppendFloat32(x.buf, val)
}

func (x *Event) appendBool(val bool) {
	x.buf = strconv.AppendBool(x.buf, val)
}

func (x *Event) appendInt64(val int64) {
	x.buf = append(x.buf, '"')
	x.buf = strconv.AppendInt(x.buf, val, 10)
	x.buf = append(x.buf, '"')
}

func (x *Event) appendUint64(val uint64) {
	x.buf = append(x.buf, '"')
	x.buf = strconv.AppendUint(x.buf, val, 10)
	x.buf = append(x.buf, '"')
}

func (x *Event) insertStringContent(index int, value string) {
	if value == `` {
		return
	}
	n := len(x.buf)
	x.buf = jsonenc.InsertStringContent(x.buf, index, value)
	n = len(x.buf) - n
	for i := len(x.off) - 1; i >= 0; i-- {
		if x.off[i] < 0 {
			continue
		}
		if x.off[i] <= index {
			break
		}
		x.off[i] += n
	}
}

func (x *Event) appendInterface(val any) {
	if b, err := json.Marshal(val); err != nil {
		x.appendString(fmt.Sprintf("marshaling error: %v", err))
	} else {
		x.buf = append(x.buf, b...)
	}
}

func (x *Event) appendKey(key string) {
	x.appendFieldSeparator()
	x.appendString(key)
	x.buf = append(x.buf, ':')
}

func (x *Event) appendErrorKey() {
	x.appendFieldSeparator()
	x.buf = append(x.buf, x.logger.errorField...)
	x.buf = append(x.buf, ':')
}

func (x *Event) enterKey(key string) {
	x.appendFieldSeparator()
	if key == `` {
		// off is the index where the key should be inserted
		x.off = append(x.off, len(x.buf)+1)
		x.buf = append(x.buf, '"', '"', ':')
	} else {
		// key already set, so off is -1
		x.off = append(x.off, -1)
		x.appendString(key)
		x.buf = append(x.buf, ':')
	}
}

func (x *Event) exitKey(key string) {
	if index := x.off[len(x.off)-1]; index >= 0 {
		x.insertStringContent(index, key)
	}
	x.off = x.off[:len(x.off)-1]
}
