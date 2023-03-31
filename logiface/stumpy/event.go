package stumpy

import (
	"encoding/json"
	"fmt"
	"github.com/joeycumines/go-utilpkg/logiface"
	"github.com/joeycumines/go-utilpkg/logiface/stumpy/internal/zljson"
)

type (
	Event struct {
		lvl logiface.Level
		buf []byte
		//lint:ignore U1000 embedded for it's methods
		unimplementedEvent
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

func (x *Event) appendString(val string) {
	x.buf = zljson.AppendString(x.buf, val)
}

func (x *Event) appendInterface(val any) {
	if b, err := json.Marshal(val); err != nil {
		x.appendString(fmt.Sprintf("marshaling error: %v", err))
	} else {
		x.buf = append(x.buf, b...)
	}
}
