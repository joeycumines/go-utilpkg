package logrus

import (
	"bytes"
	"errors"
	"github.com/joeycumines/go-utilpkg/logiface"
	"github.com/sirupsen/logrus"
	"testing"
)

func TestLogger_simple(t *testing.T) {
	t.Parallel()

	type Harness struct {
		L *logiface.Logger[*Event]
		B bytes.Buffer
	}

	newHarness := func(t *testing.T, options ...logiface.Option[*Event]) *Harness {
		var h Harness
		l := logrus.New()
		l.Formatter = &logrus.TextFormatter{
			DisableColors:    true,
			DisableTimestamp: true,
		}
		l.Out = &h.B
		h.L = L.New(append([]logiface.Option[*Event]{L.WithLogrus(l)}, options...)...)
		return &h
	}

	t.Run(`basic log`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			Log(`hello world`)

		h.L.Debug().
			Log(`wont show`)

		h.L.Warning().
			Log(`is warning`)

		h.L.Trace().
			Log(`wont show`)

		h.L.Err().
			Log(`is err`)

		if s := h.B.String(); s != "level=info msg=\"hello world\"\nlevel=warning msg=\"is warning\"\nlevel=error msg=\"is err\"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`with fields`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			Field(`one`, 1).
			Field(`two`, 2).
			Field(`three`, 3).
			Log(`hello world`)

		if s := h.B.String(); s != "level=info msg=\"hello world\" one=1 three=3 two=2\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`basic context usage`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		c1 := h.L.Clone().
			Field(`one`, 1).
			Field(`two`, 2).
			Logger()

		c1.Info().
			Field(`three`, 3).
			Field(`four`, 4).
			Log(`case 1`)

		h.L.Clone().
			Field(`six`, 6).
			Logger().
			Clone().
			Field(`seven`, 7).
			Logger().
			Info().
			Field(`eight`, 8).
			Log(`case 2`)

		c1.Info().
			Field(`three`, -3).
			Field(`five`, 5).
			Log(`case 3`)

		if s := h.B.String(); s != "level=info msg=\"case 1\" four=4 one=1 three=3 two=2\nlevel=info msg=\"case 2\" eight=8 seven=7 six=6\nlevel=info msg=\"case 3\" five=5 one=1 three=-3 two=2\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`nil logger disabled`, func(t *testing.T) {
		t.Parallel()

		h := &Harness{}

		h.L.Info().
			Log(`hello world`)

		c1 := h.L.Clone().
			Field(`one`, 1).
			Field(`two`, 2).
			Logger()

		c1.Info().
			Field(`three`, 3).
			Field(`four`, 4).
			Log(`case 1`)

		h.L.Clone().
			Field(`six`, 6).
			Logger().
			Clone().
			Field(`seven`, 7).
			Logger().
			Info().
			Field(`eight`, 8).
			Log(`case 2`)

		c1.Info().
			Field(`three`, -3).
			Field(`five`, 5).
			Log(`case 3`)
	})

	t.Run(`add error`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			Err(errors.New(`some error`)).
			Log(`hello world`)

		if s := h.B.String(); s != "level=info msg=\"hello world\" error=\"some error\"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`add error disabled`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Debug().
			Err(errors.New(`some error`)).
			Log(`hello world`)

		h.L.Clone().
			Err(errors.New(`some error`)).
			Logger().
			Debug().
			Log(`hello world`)

		if s := h.B.String(); s != "" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})
}
