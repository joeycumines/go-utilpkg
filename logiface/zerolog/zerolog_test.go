package zerolog

import (
	"bytes"
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
		h.L = logiface.New(append([]logiface.Option[*Event]{logiface.WithLogger[*Event](&Logger{Z: zerolog.New(&h.B)})}, options...)...)
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
			With(`one`, 1).
			With(`two`, 2).
			With(`three`, 3).
			Log(`hello world`)

		if s := h.B.String(); s != "{\"level\":\"info\",\"one\":1,\"two\":2,\"three\":3,\"message\":\"hello world\"}\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`basic context usage`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		c1 := h.L.Clone().
			With(`one`, 1).
			With(`two`, 2).
			Logger()

		c1.Info().
			With(`three`, 3).
			With(`four`, 4).
			Log(`case 1`)

		h.L.Clone().
			With(`six`, 6).
			Logger().
			Clone().
			With(`seven`, 7).
			Logger().
			Info().
			With(`eight`, 8).
			Log(`case 2`)

		c1.Info().
			With(`three`, -3).
			With(`five`, 5).
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
			With(`one`, 1).
			With(`two`, 2).
			Logger()

		c1.Info().
			With(`three`, 3).
			With(`four`, 4).
			Log(`case 1`)

		h.L.Clone().
			With(`six`, 6).
			Logger().
			Clone().
			With(`seven`, 7).
			Logger().
			Info().
			With(`eight`, 8).
			Log(`case 2`)

		c1.Info().
			With(`three`, -3).
			With(`five`, 5).
			Log(`case 3`)
	})
}
