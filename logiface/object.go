package logiface

type (
	ObjectBuilder[E Event, P Parent[E]] refPoolItem

	//lint:ignore U1000 it is or will be used
	objectBuilderInterface interface {
		isObjectBuilder() *refPoolItem
		as(key string) any
	}
)

func Object[E Event, P Parent[E]](p P) (obj *ObjectBuilder[E, P]) {
	if p.Enabled() {
		obj = (*ObjectBuilder[E, P])(refPoolGet())
		obj.a = p
		obj.b = p.objNew()
	}
	return
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objNew() any {
	return new(contextFieldData[E])
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objWrite(key string, arr any) {
	o := arr.(*contextFieldData[E])
	o.key = key
	o.shared = x.logger.shared
	x.Modifiers = append(x.Modifiers, ModifierFunc[E](o.object))
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objField(obj any, key string, val any) any {
	o := obj.(*contextFieldData[E])
	o.values = append(o.values, func(shared *loggerShared[E], obj any) any {
		return shared.json.setField(obj, key, val)
	})
	return obj
}

//lint:ignore U1000 it is or will be used
func (x *Context[E]) objObject(obj any, key string, val any) (any, bool) {
	if x.logger.shared.json.iface.CanSetObject() {
		o := obj.(*contextFieldData[E])
		v := val.(*contextFieldData[E])
		o.values = append(o.values, func(shared *loggerShared[E], arr any) any {
			val := shared.json.newObject()
			for _, f := range v.values {
				val = f(shared, val)
			}
			return shared.json.setObject(arr, key, val)
		})
		return obj, true
	}
	return obj, false
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objNew() any {
	return x.shared.json.newObject()
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objWrite(key string, obj any) {
	x.shared.json.addObject(x.Event, key, obj)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objField(obj any, key string, val any) any {
	return x.shared.json.setField(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *Builder[E]) objObject(obj any, key string, val any) (any, bool) {
	if x.shared.json.iface.CanSetObject() {
		return x.shared.json.setObject(obj, key, val), true
	}
	return obj, false
}

// p returns the parent of this object builder
func (x *ObjectBuilder[E, P]) p() (p P) {
	if x != nil && x.a != nil {
		p = x.a.(P)
	}
	return
}

// Enabled returns whether the parent is enabled.
func (x *ObjectBuilder[E, P]) Enabled() bool {
	return x.p().Enabled()
}

func (x *ObjectBuilder[E, P]) Field(key string, val any) *ObjectBuilder[E, P] {
	// TODO better
	if x.Enabled() {
		x.b = x.p().objField(x.b, key, val)
	}
	return x
}

// As writes the object to the parent, using the provided key, and returns the
// parent.
//
// WARNING: References to the receiver must not be retained.
func (x *ObjectBuilder[E, P]) As(key string) (p P) {
	if x.Enabled() {
		p = x.p()
		p.objWrite(key, x.b)
		refPoolPut((*refPoolItem)(x))
	}
	return
}

// Call is provided as a convenience, to facilitate code which uses the
// receiver explicitly, without breaking out of the fluent-style API.
// The provided fn will not be called if not [ObjectBuilder.Enabled].
func (x *ObjectBuilder[E, P]) Call(fn func(a *ObjectBuilder[E, P])) *ObjectBuilder[E, P] {
	if x.Enabled() {
		fn(x)
	}
	return x
}

func (x *ObjectBuilder[E, P]) as(key string) any {
	return x.As(key)
}

func (x *ObjectBuilder[E, P]) isObjectBuilder() *refPoolItem {
	return (*refPoolItem)(x)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) root() *Logger[E] {
	return x.p().root()
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) jsonSupport() iJSONSupport[E] {
	return x.p().jsonSupport()
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objNew() any {
	return x.p().objNew()
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) objWrite(key string, obj any) {
	if !x.jsonSupport().CanSetObject() {
		x.b = x.objField(x.b, key, obj.([]any))
	} else if v, ok := x.objObject(x.b, key, obj); !ok {
		x.root().DPanic().Log(`logiface: implementation disallows writing an object to an object`)
	} else {
		x.b = v
	}
}

func (x *ObjectBuilder[E, P]) objField(obj any, key string, val any) any {
	return x.p().objField(obj, key, val)
}

func (x *ObjectBuilder[E, P]) objObject(obj any, key string, val any) (any, bool) {
	return x.p().objObject(obj, key, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrNew() any {
	return x.p().arrNew()
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrWrite(key string, arr any) {
	x.root().DPanic().Log(`logiface: object builder unexpected called arrWrite`)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrField(arr any, val any) any {
	return x.p().arrField(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrArray(arr, val any) (any, bool) {
	return x.p().arrArray(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrStr(arr any, val string) (any, bool) {
	return x.p().arrStr(arr, val)
}

//lint:ignore U1000 it is or will be used
func (x *ObjectBuilder[E, P]) arrBool(arr any, val bool) (any, bool) {
	return x.p().arrBool(arr, val)
}
