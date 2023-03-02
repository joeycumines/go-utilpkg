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
	MapFields(l.Notice(), map[string]interface{}{
		`a`: `A`,
		`b`: `B`,
	}).Log(``)

	// supports logiface.Context
	MapFields(l.Clone(), map[string]interface{}{
		`a`: `A`,
		`b`: `B`,
	}).Logger().Alert().
		Log(``)

	// supports fluent chaining
	MapFields(l.Crit().
		Str(`a`, `A1`).
		Str(`b`, `B`), map[string]interface{}{
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
	l := simpleLoggerFactory.New(
		simpleLoggerFactory.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		simpleLoggerFactory.WithWriter(&mockSimpleWriter{Writer: os.Stdout}),
		simpleLoggerFactory.WithLevel(LevelDebug),
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
