package stumpy

import (
	"encoding/json"
	"fmt"
	"github.com/joeycumines/go-utilpkg/logiface"
	"github.com/joeycumines/go-utilpkg/logiface/internal/jsonenc"
)

type (
	Event struct {
		//lint:ignore U1000 embedded for it's methods
		unimplementedEvent

		lvl logiface.Level
		buf []byte
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
	x.appendFieldSeparator()
	x.appendString(key)
	x.buf = append(x.buf, ':')
	x.appendInterface(val)
}

//func (x *Event) AddMessage(msg string) bool {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *Event) AddError(err error) bool {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *Event) AddString(key string, val string) bool {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *Event) AddInt(key string, val int) bool {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *Event) AddFloat32(key string, val float32) bool {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *Event) AddTime(key string, val time.Time) bool {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *Event) AddDuration(key string, val time.Duration) bool {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *Event) AddBase64Bytes(key string, val []byte, enc *base64.Encoding) bool {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *Event) AddBool(key string, val bool) bool {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *Event) AddFloat64(key string, val float64) bool {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *Event) AddInt64(key string, val int64) bool {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *Event) AddUint64(key string, val uint64) bool {
//	//TODO implement me
//	panic("implement me")
//}

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
