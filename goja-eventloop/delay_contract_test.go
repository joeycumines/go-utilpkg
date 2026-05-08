package gojaeventloop

import (
	"math"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func newDelayContractAdapter(t *testing.T) (*goeventloop.Loop, *goja.Runtime, *Adapter) {
	t.Helper()
	loop := goeventloop.New()
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return loop, runtime, adapter
}

func TestDelayDurationConversion(t *testing.T) {
	_, runtime, adapter := newDelayContractAdapter(t)
	tests := []struct {
		name  string
		value goja.Value
		want  time.Duration
	}{
		{name: "omitted", value: goja.Undefined(), want: 0},
		{name: "undefined", value: goja.Undefined(), want: 0},
		{name: "null", value: goja.Null(), want: 0},
		{name: "false", value: runtime.ToValue(false), want: 0},
		{name: "true", value: runtime.ToValue(true), want: time.Millisecond},
		{name: "empty string", value: runtime.ToValue(""), want: 0},
		{name: "numeric string", value: runtime.ToValue("12.9"), want: 12 * time.Millisecond},
		{name: "NaN", value: runtime.ToValue(math.NaN()), want: 0},
		{name: "negative infinity", value: runtime.ToValue(math.Inf(-1)), want: 0},
		{name: "negative", value: runtime.ToValue(-4.9), want: 0},
		{name: "negative zero", value: runtime.ToValue(math.Copysign(0, -1)), want: 0},
		{name: "positive zero", value: runtime.ToValue(0), want: 0},
		{name: "fraction", value: runtime.ToValue(4.9), want: 4 * time.Millisecond},
		{
			name:  "maximum",
			value: runtime.ToValue(float64(maxDelayMilliseconds)),
			want:  time.Duration(maxDelayMilliseconds) * time.Millisecond,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got time.Duration
			if exception := runtime.Try(func() { got = adapter.delayDuration(test.value) }); exception != nil {
				t.Fatalf("delayDuration: %v", exception)
			}
			if got != test.want {
				t.Fatalf("delayDuration = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDelayDurationRangeError(t *testing.T) {
	_, runtime, adapter := newDelayContractAdapter(t)
	constructorValue, ok := runtime.Intrinsic(goja.IntrinsicRangeErrorConstructor)
	if !ok {
		t.Fatal("RangeError intrinsic unavailable")
	}
	constructor := constructorValue.ToObject(runtime)
	for _, test := range []struct {
		name  string
		value goja.Value
	}{
		{name: "maximum plus one", value: runtime.ToValue(float64(maxDelayMilliseconds + 1))},
		{name: "positive infinity", value: runtime.ToValue(math.Inf(1))},
	} {
		t.Run(test.name, func(t *testing.T) {
			exception := runtime.Try(func() { adapter.delayDuration(test.value) })
			if exception == nil {
				t.Fatal("delayDuration did not throw")
			}
			object := exception.Value().ToObject(runtime)
			if !runtime.InstanceOf(object, constructor) {
				t.Fatalf("exception is not RangeError: %v", exception)
			}
			if got, want := object.Get("message").String(),
				"delay must not exceed 9223372036854 milliseconds"; got != want {
				t.Fatalf("RangeError message = %q, want %q", got, want)
			}
		})
	}
}

func TestDelayDurationAbruptConversion(t *testing.T) {
	_, runtime, adapter := newDelayContractAdapter(t)
	value, err := runtime.RunString(`
		globalThis.delayCoercionCalls = 0;
		globalThis.delayCoercionHint = "";
		globalThis.delayCoercionValue = {
			[Symbol.toPrimitive](hint) {
				delayCoercionCalls++;
				delayCoercionHint = hint;
				return 7.8;
			},
		};
		delayCoercionValue;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := adapter.delayDuration(value); got != 7*time.Millisecond {
		t.Fatalf("coerced duration = %v, want 7ms", got)
	}
	if got := runtime.Get("delayCoercionCalls").ToInteger(); got != 1 {
		t.Fatalf("coercion calls = %d, want 1", got)
	}
	if got := runtime.Get("delayCoercionHint").String(); got != "number" {
		t.Fatalf("coercion hint = %q, want number", got)
	}

	abrupt, err := runtime.RunString(`
		globalThis.delayAbrupt = {};
		({ [Symbol.toPrimitive]() { throw delayAbrupt; } });
	`)
	if err != nil {
		t.Fatal(err)
	}
	exception := runtime.Try(func() { adapter.delayDuration(abrupt) })
	if exception == nil || !exception.Value().SameAs(runtime.Get("delayAbrupt")) {
		t.Fatalf("abrupt conversion exception = %v", exception)
	}
}

func TestDelayDurationTypeErrors(t *testing.T) {
	_, runtime, adapter := newDelayContractAdapter(t)
	constructorValue, ok := runtime.Intrinsic(goja.IntrinsicTypeErrorConstructor)
	if !ok {
		t.Fatal("TypeError intrinsic unavailable")
	}
	constructor := constructorValue.ToObject(runtime)
	for _, expression := range []string{`Symbol("delay")`, `1n`, `Object(1n)`} {
		t.Run(expression, func(t *testing.T) {
			value, err := runtime.RunString(expression)
			if err != nil {
				t.Fatal(err)
			}
			exception := runtime.Try(func() { adapter.delayDuration(value) })
			if exception == nil || !runtime.InstanceOf(exception.Value(), constructor) {
				t.Fatalf("exception = %v, want TypeError", exception)
			}
		})
	}
}
