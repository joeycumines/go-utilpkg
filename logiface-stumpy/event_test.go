package stumpy

import (
	"bytes"
	"errors"
	"github.com/joeycumines/go-utilpkg/logiface"
	"testing"
)

var (
	// compile time assertions

	_ logiface.Event = (*Event)(nil)
)

func TestLogger_fieldTypes(t *testing.T) {
	t.Parallel()

	type Harness struct {
		L *logiface.Logger[*Event]
		B bytes.Buffer
	}

	newHarness := func(t *testing.T, options ...logiface.Option[*Event]) *Harness {
		var h Harness
		h.L = L.New(append([]logiface.Option[*Event]{L.WithStumpy(WithWriter(&h.B), WithLevelField(``))}, options...)...)
		return &h
	}

	t.Run(`message`, func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.L.Info().Log(`some message`)
		if s := h.B.String(); s != `{"msg":"some message"}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`any`, func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.L.Info().
			Any(`a`, true).
			Object().Any(`ba`, true).As(`b`).End().
			Array().Any(true).As(`c`).End().
			Log(``)
		if s := h.B.String(); s != `{"a":true,"b":{"ba":true},"c":[true]}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`error`, func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.L.Info().
			Err(nil).
			Err(errors.New(`err 1`)).
			Object().Err(nil).Err(errors.New(`err 2`)).As(`obj`).End().
			Array().Err(nil).Err(errors.New(`err 3`)).As(`arr`).End().
			Log(``)
		if s := h.B.String(); s != `{"err":"err 1","obj":{"err":null,"err":"err 2"},"arr":[null,"err 3"]}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`string`, func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.L.Info().
			Str(`a`, `A`).
			Object().Str(`ba`, `BA`).As(`b`).End().
			Array().Str(`CA`).As(`c`).End().
			Log(``)
		if s := h.B.String(); s != `{"a":"A","b":{"ba":"BA"},"c":["CA"]}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`int`, func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.L.Info().
			Int(`a`, 1).
			Object().Int(`ba`, 2).As(`b`).End().
			Array().Int(3).As(`c`).End().
			Log(``)
		if s := h.B.String(); s != `{"a":1,"b":{"ba":2},"c":[3]}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`float32`, func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.L.Info().
			Float32(`a`, 1.1).
			Object().Float32(`ba`, 2.2).As(`b`).End().
			Array().Float32(3.3).As(`c`).End().
			Log(``)
		if s := h.B.String(); s != `{"a":1.1,"b":{"ba":2.2},"c":[3.3]}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`float64`, func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.L.Info().
			Float64(`a`, 1.1).
			Object().Float64(`ba`, 2.2).As(`b`).End().
			Array().Float64(3.3).As(`c`).End().
			Log(``)
		if s := h.B.String(); s != `{"a":1.1,"b":{"ba":2.2},"c":[3.3]}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`bool`, func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.L.Info().
			Bool(`a`, true).
			Object().Bool(`ba`, false).As(`b`).End().
			Array().Bool(true).As(`c`).End().
			Log(``)
		if s := h.B.String(); s != `{"a":true,"b":{"ba":false},"c":[true]}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`int64`, func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.L.Info().
			Int64(`a`, 1).
			Object().Int64(`ba`, 2).As(`b`).End().
			Array().Int64(3).As(`c`).End().
			Log(``)
		if s := h.B.String(); s != `{"a":"1","b":{"ba":"2"},"c":["3"]}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`uint64`, func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.L.Info().
			Uint64(`a`, 1).
			Object().Uint64(`ba`, 2).As(`b`).End().
			Array().Uint64(3).As(`c`).End().
			Log(``)
		if s := h.B.String(); s != `{"a":"1","b":{"ba":"2"},"c":["3"]}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})
}
