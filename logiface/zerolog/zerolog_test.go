package zerolog

import (
	"bytes"
	"errors"
	"github.com/joeycumines/go-utilpkg/logiface"
	"github.com/rs/zerolog"
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
		h.L = L.New(append([]logiface.Option[*Event]{L.WithZerolog(zerolog.New(&h.B))}, options...)...)
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

		if s := h.B.String(); s != "{\"level\":\"info\",\"message\":\"hello world\"}\n{\"level\":\"warn\",\"message\":\"is warning\"}\n{\"level\":\"error\",\"message\":\"is err\"}\n" {
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

		if s := h.B.String(); s != "{\"level\":\"info\",\"one\":1,\"two\":2,\"three\":3,\"message\":\"hello world\"}\n" {
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

		if s := h.B.String(); s != "{\"level\":\"info\",\"one\":1,\"two\":2,\"three\":3,\"four\":4,\"message\":\"case 1\"}\n{\"level\":\"info\",\"six\":6,\"seven\":7,\"eight\":8,\"message\":\"case 2\"}\n{\"level\":\"info\",\"one\":1,\"two\":2,\"three\":-3,\"five\":5,\"message\":\"case 3\"}\n" {
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

		if s := h.B.String(); s != "{\"level\":\"info\",\"error\":\"some error\",\"message\":\"hello world\"}\n" {
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
