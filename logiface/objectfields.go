package logiface

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

type (
	// objectFields implements Event in order to use modifierMethods
	objectFields[E Event, P Parent[E]] ObjectBuilder[E, P]
)

func (x *ObjectBuilder[E, P]) methods() modifierMethods[*objectFields[E, P]] {
	return modifierMethods[*objectFields[E, P]]{}
}

func (x *ObjectBuilder[E, P]) fields() *objectFields[E, P] {
	return (*objectFields[E, P])(x)
}

func (x *objectFields[E, P]) builder() *ObjectBuilder[E, P] {
	return (*ObjectBuilder[E, P])(x)
}

// Level is necessary to pass the various guards (they just test [Level.Enabled]).
func (x *objectFields[E, P]) Level() Level {
	if x.builder().Enabled() {
		return LevelEmergency
	}
	return LevelDisabled
}

func (x *objectFields[E, P]) AddField(key string, val any) {
	x.b = x.builder().objField(x.b, key, val)
}

func (x *objectFields[E, P]) AddMessage(msg string) bool {
	panic(`unimplemented`)
}

func (x *objectFields[E, P]) AddString(key string, val string) (ok bool) {
	x.b, ok = x.builder().objString(x.b, key, val)
	return
}

func (x *objectFields[E, P]) AddBool(key string, val bool) (ok bool) {
	x.b, ok = x.builder().objBool(x.b, key, val)
	return
}

func (x *objectFields[E, P]) AddError(err error) (ok bool) {
	x.b, ok = x.builder().objError(x.b, err)
	return
}

func (x *objectFields[E, P]) AddInt(key string, val int) (ok bool) {
	x.b, ok = x.builder().objInt(x.b, key, val)
	return
}

func (x *objectFields[E, P]) AddFloat32(key string, val float32) (ok bool) {
	x.b, ok = x.builder().objFloat32(x.b, key, val)
	return
}

func (x *objectFields[E, P]) AddTime(key string, val time.Time) (ok bool) {
	x.b, ok = x.builder().objTime(x.b, key, val)
	return
}

func (x *objectFields[E, P]) AddDuration(key string, val time.Duration) (ok bool) {
	x.b, ok = x.builder().objDuration(x.b, key, val)
	return
}

func (x *objectFields[E, P]) AddBase64Bytes(key string, val []byte, enc *base64.Encoding) (ok bool) {
	x.b, ok = x.builder().objBase64Bytes(x.b, key, val, enc)
	return
}

func (x *objectFields[E, P]) AddFloat64(key string, val float64) (ok bool) {
	x.b, ok = x.builder().objFloat64(x.b, key, val)
	return
}

func (x *objectFields[E, P]) AddInt64(key string, val int64) (ok bool) {
	x.b, ok = x.builder().objInt64(x.b, key, val)
	return
}

func (x *objectFields[E, P]) AddUint64(key string, val uint64) (ok bool) {
	x.b, ok = x.builder().objUint64(x.b, key, val)
	return
}

func (x *objectFields[E, P]) AddRawJSON(key string, val json.RawMessage) (ok bool) {
	x.b, ok = x.builder().objRawJSON(x.b, key, val)
	return
}

func (x *objectFields[E, P]) AddGroup(string) bool { return false }

func (x *objectFields[E, P]) mustEmbedUnimplementedEvent() {}
