package inprocgrpc

import "reflect"

// isNil reports whether the given interface value is nil (either untyped nil
// or a nil pointer).
func isNil(m any) bool {
	if m == nil {
		return true
	}
	rv := reflect.ValueOf(m)
	return rv.Kind() == reflect.Ptr && rv.IsNil()
}
