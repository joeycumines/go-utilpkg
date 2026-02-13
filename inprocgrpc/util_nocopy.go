package inprocgrpc

import (
	"reflect"
)

// shallowCopy performs a shallow copy from in to out: *out = *in.
// Both out and in must be pointers to the same type.
func shallowCopy(out, in any) {
	valIn := reflect.ValueOf(in)
	valOut := reflect.ValueOf(out)
	if valIn.Kind() == reflect.Ptr {
		valIn = valIn.Elem()
	}
	if valOut.Kind() == reflect.Ptr {
		valOut = valOut.Elem()
	}
	valOut.Set(valIn)
}
