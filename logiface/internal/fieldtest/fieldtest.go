package fieldtest

import (
	"encoding/base64"
	"errors"
	"math"
	"time"
)

type (
	IntDataType int

	ObjectMethods[T any] interface {
		Field(key string, val any) T
		Any(key string, val any) T
		Base64(key string, b []byte, enc *base64.Encoding) T
		Dur(key string, d time.Duration) T
		Err(err error) T
		Float32(key string, val float32) T
		Int(key string, val int) T
		Interface(key string, val any) T
		Str(key string, val string) T
		Time(key string, t time.Time) T
		Bool(key string, val bool) T
		Float64(key string, val float64) T
		Int64(key string, val int64) T
		Uint64(key string, val uint64) T
	}

	ArrayMethods[T any] interface {
		Field(val any) T
		Any(val any) T
		Base64(b []byte, enc *base64.Encoding) T
		Dur(d time.Duration) T
		Err(err error) T
		Float32(val float32) T
		Int(val int) T
		Interface(val any) T
		Str(val string) T
		Time(t time.Time) T
		Bool(val bool) T
		Float64(val float64) T
		Int64(val int64) T
		Uint64(val uint64) T
	}
)

// FluentObjectTemplate exercises every fluent method that's common between Builder and Context
func FluentObjectTemplate[T ObjectMethods[T]](x T) {
	x.Err(errors.New(`err called`)).
		Field(`field called with string`, `val 2`).
		Field(`field called with bytes`, []byte(`val 3`)).
		Field(`field called with time.Time local`, time.Unix(0, 1558069640361696123)).
		Field(`field called with time.Time utc`, time.Unix(0, 1558069640361696123).UTC()).
		Field(`field called with duration`, time.Duration(3116139280723392)).
		Field(`field called with int`, -51245).
		Field(`field called with float32`, float32(math.SmallestNonzeroFloat32)).
		Field(`field called with unhandled type`, IntDataType(-421)).
		Float32(`float32 called`, float32(math.MaxFloat32)).
		Int(`int called`, math.MaxInt).
		Interface(`interface called with string`, `val 4`).
		Interface(`interface called with bool`, true).
		Interface(`interface called with nil`, nil).
		Any(`any called with string`, `val 5`).
		Str(`str called`, `val 6`).
		Time(`time called with local`, time.Unix(0, 1616592449876543213)).
		Time(`time called with utc`, time.Unix(0, 1583023169456789123).UTC()).
		Dur(`dur called positive`, time.Duration(51238123523458989)).
		Dur(`dur called negative`, time.Duration(-51238123523458989)).
		Dur(`dur called zero`, time.Duration(0)).
		Base64(`base64 called with nil enc`, []byte(`val 7`), nil).
		Base64(`base64 called with padding`, []byte(`val 7`), base64.StdEncoding).
		Base64(`base64 called without padding`, []byte(`val 7`), base64.RawStdEncoding).
		Bool(`bool called`, true).
		Field(`field called with bool`, true).
		Float64(`float64 called`, math.MaxFloat64).
		Field(`field called with float64`, float64(math.MaxFloat64)).
		Int64(`int64 called`, math.MaxInt64).
		Field(`field called with int64`, int64(math.MaxInt64)).
		Uint64(`uint64 called`, math.MaxUint64).
		Field(`field called with uint64`, uint64(math.MaxUint64))
}

func FluentArrayTemplate[T ArrayMethods[T]](x T) {
	x.Err(errors.New(`err called`)).
		Field(`val 2`).
		Field([]byte(`val 3`)).
		Field(time.Unix(0, 1558069640361696123)).
		Field(time.Unix(0, 1558069640361696123).UTC()).
		Field(time.Duration(3116139280723392)).
		Field(-51245).
		Field(float32(math.SmallestNonzeroFloat32)).
		Field(IntDataType(-421)).
		Float32(float32(math.MaxFloat32)).
		Int(math.MaxInt).
		Interface(`val 4`).
		Interface(true).
		Interface(nil).
		Any(`val 5`).
		Str(`val 6`).
		Time(time.Unix(0, 1616592449876543213)).
		Time(time.Unix(0, 1583023169456789123).UTC()).
		Dur(time.Duration(51238123523458989)).
		Dur(time.Duration(-51238123523458989)).
		Dur(time.Duration(0)).
		Base64([]byte(`val 7`), nil).
		Base64([]byte(`val 7`), base64.StdEncoding).
		Base64([]byte(`val 7`), base64.RawStdEncoding).
		Bool(true).
		Field(true).
		Float64(math.MaxFloat64).
		Field(float64(math.MaxFloat64)).
		Int64(math.MaxInt64).
		Field(int64(math.MaxInt64)).
		Uint64(math.MaxUint64).
		Field(uint64(math.MaxUint64))
}
