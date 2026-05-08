package inprocgrpc

import "reflect"

// isNil reports whether the given interface value is nil, including a typed
// nil value of any nilable kind.
func isNil(m any) bool {
	if m == nil {
		return true
	}
	rv := reflect.ValueOf(m)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
