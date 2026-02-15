package logiface

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func ExampleMapFields() {
	// note that the order of keys is not stable (sorted only to make the test pass)
	w, out := sortedLineWriterSplitOnSpace(os.Stdout)
	defer func() {
		_ = w.Close()
		<-out
	}()

	// l is an instance of logiface.Logger
	l := newSimpleLogger(w, false)

	// supports logiface.Builder
	MapFields(l.Notice(), map[string]any{
		`a`: `A`,
		`b`: `B`,
	}).Log(``)

	// supports logiface.Context
	MapFields(l.Clone(), map[string]any{
		`a`: `A`,
		`b`: `B`,
	}).Logger().Alert().
		Log(``)

	// supports fluent chaining
	MapFields(l.Crit().
		Str(`a`, `A1`).
		Str(`b`, `B`), map[string]any{
		`a`: `A2`,
		`c`: `C1`,
		`d`: `D`}).
		Str(`c`, `C2`).
		Str(`e`, `E`).
		Log(``)

	// supports any map with string as the underlying key type
	type Stringish string
	type Mappy map[Stringish]int
	m := Mappy{
		`a`: 1,
		`b`: 2,
	}
	MapFields(l.Build(99), m).Log(``)

	_ = w.Close()
	if err := <-out; err != nil {
		panic(err)
	}

	//output:
	//[notice] a=A b=B
	//[alert] a=A b=B
	//[crit] a=A1 a=A2 b=B c=C1 c=C2 d=D e=E
	//[99] a=1 b=2
}

func TestMapFields_nilMap(t *testing.T) {
	c := newSimpleLogger(io.Discard, false).Clone()
	if len(c.Modifiers) != 0 {
		t.Fatalf("unexpected modifiers: %v", c.Modifiers)
	}
	if v := MapFields(c, map[string]any(nil)); v != c {
		t.Errorf("unexpected return value: %v", v)
	}
	if len(c.Modifiers) != 0 {
		t.Errorf("unexpected modifiers: %v", c.Modifiers)
	}
}

func ExampleArgFields() {
	// l is an instance of logiface.Logger
	l := mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: os.Stdout}),
		mockL.WithLevel(LevelDebug),
	)

	// supports logiface.Builder
	ArgFields(l.Notice(), nil, "a", "A", "b", "B").Log(``)

	// supports logiface.Context
	ArgFields(l.Clone(), nil, "a", "A", "b", "B").Logger().Alert().
		Log(``)

	// supports fluent chaining
	ArgFields(l.Crit().
		Str(`a`, `A1`).
		Str(`b`, `B`), nil, "a", "A2", "c", "C1", "d", "D", "c", "C2", "e", "E").
		Str(`c`, `C3`).
		Str(`f`, `F`).
		Log(``)

	// supports conversion function
	ArgFields(l.Debug(), func(key any) (string, bool) {
		if k, ok := key.(string); ok {
			return k + "_converted", true
		}
		return "", false
	}, "a", "A", "b", "B", 123, "C").
		Log(``)

	// passing an odd number of keys sets the last value to any(nil)
	ArgFields(l.Info(), nil, "a", "A", "b").Log(``)

	//output:
	//[notice] a=A b=B
	//[alert] a=A b=B
	//[crit] a=A1 b=B a=A2 c=C1 d=D c=C2 e=E c=C3 f=F
	//[debug] a_converted=A b_converted=B
	//[info] a=A b=<nil>
}

func TestArgFields_empty(t *testing.T) {
	c := newSimpleLogger(io.Discard, false).Clone()
	if len(c.Modifiers) != 0 {
		t.Fatalf("unexpected modifiers: %v", c.Modifiers)
	}
	if v := ArgFields[any](c, nil); v != c {
		t.Errorf("unexpected return value: %v", v)
	}
	if len(c.Modifiers) != 0 {
		t.Errorf("unexpected modifiers: %v", c.Modifiers)
	}
}

func TestArgFields_conversionError(t *testing.T) {
	l := newSimpleLogger(io.Discard, false)
	b := l.Info()
	if v := ArgFields(b, func(key int) (string, bool) {
		return "", false
	}, 1, 2, 3); v != b {
		t.Errorf("unexpected return value: %v", v)
	}
}

func TestArgFields_oddNumberKeys(t *testing.T) {
	w := &bytes.Buffer{}
	l := newSimpleLogger(w, false)
	ArgFields(l.Notice(), nil, "a", "A", "b").Log(``)
	if got, want := w.String(), "[notice] a=A b=<nil>\n"; got != want {
		t.Errorf("unexpected output: got=%q, want=%q", got, want)
	}
}

func ExampleSliceArray() {
	logger := mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: os.Stdout, JSON: true}),
		mockL.WithDPanicLevel(LevelEmergency),
	).Logger()

	// it can be used directly
	SliceArray[Event](logger.Info(), `k`, []float64{1, 2, 3, 1e-9, 1e20, 1e21, 1e-6, 1e-7}).Log(``)

	// it can be used with the Call method to aid in formatting, and on other builders
	logger.Info().
		Call(func(b *Builder[Event]) {
			SliceArray[Event](b, `a`, []any{nil, "some value", 1.123, 5})
			SliceArray[Event](b, `b`, []bool{false, false, true})
		}).
		ArrayFunc(`c`, func(b BuilderArray) {
			// NOTE: key must be empty, since it's within an array
			SliceArray[Event](b, ``, []int{-1})
		}).
		ObjectFunc(`d`, func(b BuilderObject) {
			SliceArray[Event](b, `da`, []string{"foo bar"})
		}).
		Log(``)

	// it can also be used context + nested builders
	loggerCtx := logger.Clone().
		Call(func(b *Context[Event]) {
			SliceArray[Event](b, `a`, []any{nil, "some value", 1.123, 5})
			SliceArray[Event](b, `b`, []bool{false, false, true})
		}).
		ArrayFunc(`c`, func(b ContextArray) {
			// NOTE: key must be empty, since it's within an array
			SliceArray[Event](b, ``, []int{-1})
		}).
		ObjectFunc(`d`, func(b ContextObject) {
			SliceArray[Event](b, `da`, []string{"foo bar"})
		}).
		Logger()
	loggerCtx.Info().Log(`fields from context`)

	//output:
	//[info] k=[1,2,3,1e-9,100000000000000000000,1e+21,0.000001,1e-7]
	//[info] a=[null,"some value",1.123,5] b=[false,false,true] c=[[-1]] d={"da":["foo bar"]}
	//[info] a=[null,"some value",1.123,5] b=[false,false,true] c=[[-1]] d={"da":["foo bar"]} msg="fields from context"
}

func TestSliceArray_typedSlice(t *testing.T) {
	type SomeSlice []string
	slice := SomeSlice{"a", "b", "c"}
	var buf bytes.Buffer
	logger := mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: &buf, JSON: true}),
		mockL.WithDPanicLevel(LevelEmergency),
	).Logger()
	SliceArray[Event](logger.Info(), `k`, slice).Log(``)
	if got, want := buf.String(), "[info] k=[\"a\",\"b\",\"c\"]\n"; got != want {
		t.Errorf("unexpected output: got=%q, want=%q", got, want)
	}
}

func ExampleMapObject() {
	logger := mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: os.Stdout, JSON: true}),
		mockL.WithDPanicLevel(LevelEmergency),
	).Logger()

	// it can be used directly
	MapObject[Event](logger.Info(), `k`, map[string]int{"a": 1, "b": 2, "c": 3}).Log(``)

	// it can be used with the Call method to aid in formatting, and on other builders
	logger.Info().
		Call(func(b *Builder[Event]) {
			MapObject[Event](b, `a`, map[string]int{"a": 1, "b": 2, "c": 3})
			MapObject[Event](b, `b`, map[string]any{"a": nil, "b": "some value", "c": 1.123, "d": 5})
			MapObject[Event](b, `c`, map[string]bool{"a": false, "b": false, "c": true})
		}).
		ArrayFunc(`d`, func(b BuilderArray) {
			// NOTE: key must be empty, since it's within an array
			MapObject[Event](b, ``, map[string]any{"a": nil, "b": "some value", "c": 1.123, "d": 5})
		}).
		ObjectFunc(`e`, func(b BuilderObject) {
			MapObject[Event](b, `ea`, map[string]any{"a": nil, "b": "some value", "c": 1.123, "d": 5})
		}).
		Log(``)

	// it can also be used context + nested builders
	loggerCtx := logger.Clone().
		Call(func(b *Context[Event]) {
			MapObject[Event](b, `a`, map[string]int{"a": 1, "b": 2, "c": 3})
			MapObject[Event](b, `b`, map[string]any{"a": nil, "b": "some value", "c": 1.123, "d": 5})
			MapObject[Event](b, `c`, map[string]bool{"a": false, "b": false, "c": true})
		}).
		ArrayFunc(`d`, func(b ContextArray) {
			// NOTE: key must be empty, since it's within an array
			MapObject[Event](b, ``, map[string]any{"a": nil, "b": "some value", "c": 1.123, "d": 5})
		}).
		ObjectFunc(`e`, func(b ContextObject) {
			MapObject[Event](b, `ea`, map[string]any{"a": nil, "b": "some value", "c": 1.123, "d": 5})
		}).
		Logger()
	loggerCtx.Info().Log(`fields from context`)

	//output:
	//[info] k={"a":1,"b":2,"c":3}
	//[info] a={"a":1,"b":2,"c":3} b={"a":null,"b":"some value","c":1.123,"d":5} c={"a":false,"b":false,"c":true} d=[{"a":null,"b":"some value","c":1.123,"d":5}] e={"ea":{"a":null,"b":"some value","c":1.123,"d":5}}
	//[info] a={"a":1,"b":2,"c":3} b={"a":null,"b":"some value","c":1.123,"d":5} c={"a":false,"b":false,"c":true} d=[{"a":null,"b":"some value","c":1.123,"d":5}] e={"ea":{"a":null,"b":"some value","c":1.123,"d":5}} msg="fields from context"
}

func TestMapObject_typedMap(t *testing.T) {
	type SomeString string
	type SomeMap map[SomeString]int
	m := SomeMap{SomeString("a"): 1, SomeString("b"): 2, SomeString("c"): 3}
	var buf bytes.Buffer
	logger := mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: &buf, JSON: true}),
		mockL.WithDPanicLevel(LevelEmergency),
	).Logger()
	MapObject[Event](logger.Info(), `k`, m).Log(``)
	if got, want := buf.String(), "[info] k={\"a\":1,\"b\":2,\"c\":3}\n"; got != want {
		t.Errorf("unexpected output: got=%q, want=%q", got, want)
	}
}

func TestSliceArray_disabled(t *testing.T) {
	var b Builder[Event]
	if v := SliceArray[Event](&b, `k`, []string{"a", "b", "c"}); v != &b {
		t.Error(v)
	}
}

func TestMapObject_disabled(t *testing.T) {
	var b Builder[Event]
	if v := MapObject[Event](&b, `k`, map[string]any{}); v != &b {
		t.Error(v)
	}
}
