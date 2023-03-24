package logiface

import (
	"encoding/base64"
	"time"
)

type (
	// arrayFields implements Event in order to use modifierMethods
	arrayFields[E Event, P Parent[E]] ArrayBuilder[E, P]
)

func (x *ArrayBuilder[E, P]) methods() modifierMethods[*arrayFields[E, P]] {
	return modifierMethods[*arrayFields[E, P]]{}
}

func (x *ArrayBuilder[E, P]) fields() *arrayFields[E, P] {
	return (*arrayFields[E, P])(x)
}

func (x *arrayFields[E, P]) builder() *ArrayBuilder[E, P] {
	return (*ArrayBuilder[E, P])(x)
}

// Level is necessary to pass the various guards (they just test [Level.Enabled]).
func (x *arrayFields[E, P]) Level() Level {
	if x.builder().Enabled() {
		return LevelEmergency
	}
	return LevelDisabled
}

func (x *arrayFields[E, P]) AddField(_ string, val any) {
	x.b = x.builder().arrField(x.b, val)
}

func (x *arrayFields[E, P]) AddMessage(msg string) bool {
	panic(`unimplemented`)
}

func (x *arrayFields[E, P]) AddError(err error) bool {
	return false
}

func (x *arrayFields[E, P]) AddString(_ string, val string) (ok bool) {
	x.b, ok = x.builder().arrString(x.b, val)
	return
}

func (x *arrayFields[E, P]) AddInt(_ string, val int) (ok bool) {
	x.b, ok = x.builder().arrInt(x.b, val)
	return
}

func (x *arrayFields[E, P]) AddFloat32(_ string, val float32) (ok bool) {
	x.b, ok = x.builder().arrFloat32(x.b, val)
	return
}

func (x *arrayFields[E, P]) AddTime(_ string, val time.Time) (ok bool) {
	x.b, ok = x.builder().arrTime(x.b, val)
	return
}

func (x *arrayFields[E, P]) AddDuration(_ string, val time.Duration) (ok bool) {
	x.b, ok = x.builder().arrDuration(x.b, val)
	return
}

func (x *arrayFields[E, P]) AddBase64Bytes(_ string, val []byte, enc *base64.Encoding) (ok bool) {
	x.b, ok = x.builder().arrBase64Bytes(x.b, val, enc)
	return
}

func (x *arrayFields[E, P]) AddBool(_ string, val bool) (ok bool) {
	x.b, ok = x.builder().arrBool(x.b, val)
	return
}

func (x *arrayFields[E, P]) AddFloat64(_ string, val float64) (ok bool) {
	x.b, ok = x.builder().arrFloat64(x.b, val)
	return
}

func (x *arrayFields[E, P]) AddInt64(_ string, val int64) (ok bool) {
	x.b, ok = x.builder().arrInt64(x.b, val)
	return
}

func (x *arrayFields[E, P]) AddUint64(_ string, val uint64) (ok bool) {
	x.b, ok = x.builder().arrUint64(x.b, val)
	return
}

func (x *arrayFields[E, P]) mustEmbedUnimplementedEvent() {}
