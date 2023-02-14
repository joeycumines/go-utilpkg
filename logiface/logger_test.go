package logiface

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
	"time"
)

func TestLogger_simple(t *testing.T) {
	t.Parallel()

	type Harness struct {
		L *Logger[*SimpleEvent]
		B bytes.Buffer
	}

	newHarness := func(t *testing.T, options ...Option[*SimpleEvent]) *Harness {
		var h Harness
		h.L = New(append([]Option[*SimpleEvent]{
			WithEventFactory[*SimpleEvent](EventFactoryFunc[*SimpleEvent](SimpleEventFactory)),
			WithWriter[*SimpleEvent](&SimpleWriter{Writer: &h.B}),
		}, options...)...)
		return &h
	}

	t.Run(`basic log`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			Log(`hello world`)

		h.L.Warning().
			Log(`is warning`)

		h.L.Trace().
			Log(`wont show`)

		h.L.Err().
			Log(`is err`)

		h.L.Debug().
			Log(`wont show`)

		h.L.Emerg().
			Log(`is emerg`)

		if s := h.B.String(); s != "[info] hello world\n[warning] is warning\n[err] is err\n[emerg] is emerg\n" {
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

		if s := h.B.String(); s != "[info] one=1 two=2 three=3 hello world\n" {
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

		if s := h.B.String(); s != "[info] one=1 two=2 three=3 four=4 case 1\n[info] six=6 seven=7 eight=8 case 2\n[info] one=1 two=2 three=-3 five=5 case 3\n" {
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

	t.Run(`field default bytes`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Clone().
			Field(`one`, []byte(`abc`)).
			Field(`two`, []byte(nil)).
			Logger().
			Info().
			Field(`three`, []byte(`hello world`)).
			Field(`four`, []byte{244}).
			Log(`case 1`)

		if s := h.B.String(); s != "[info] one=YWJj two= three=aGVsbG8gd29ybGQ= four=9A== case 1\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`field default bytes`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Clone().
			Field(`one`, []byte(`abc`)).
			Field(`two`, []byte(nil)).
			Logger().
			Info().
			Field(`three`, []byte(`hello world`)).
			Field(`four`, []byte{244}).
			Log(`case 1`)

		if s := h.B.String(); s != "[info] one=YWJj two= three=aGVsbG8gd29ybGQ= four=9A== case 1\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`field default timestamp`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Clone().
			Field(`one`, time.Unix(5, 9400000).Local()).
			Field(`two`, time.Time{}).
			Logger().
			Info().
			Field(`three`, time.Unix(5, 9400000)).
			Log(`case 1`)

		if s := h.B.String(); s != "[info] one=1970-01-01T00:00:05.009400Z two=0001-01-01T00:00:00Z three=1970-01-01T00:00:05.009400Z case 1\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`field default duration`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Clone().
			Field(`one`, time.Hour).
			Field(`zero`, time.Duration(0)).
			Logger().
			Info().
			Field(`two`, time.Second*32-(time.Microsecond*100)).
			Field(`three`, -(time.Second*32 - (time.Microsecond * 100))).
			Log(`case 1`)

		if s := h.B.String(); s != "[info] one=3600s zero=0s two=31.999900s three=-31.999900s case 1\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})
}

func TestLoggerFactory_WithOptions_noOptions(t *testing.T) {
	L.WithOptions()(nil)
}

func TestLoggerFactory_WithOptions_callsAllOptions(t *testing.T) {
	var cfg loggerConfig[Event]
	var out []int
	L.WithOptions(
		func(c *loggerConfig[Event]) {
			if c != &cfg {
				t.Error(`unexpected config`)
			}
			out = append(out, 1)
		},
		func(c *loggerConfig[Event]) {
			if c != &cfg {
				t.Error(`unexpected config`)
			}
			out = append(out, 2)
		},
		func(c *loggerConfig[Event]) {
			if c != &cfg {
				t.Error(`unexpected config`)
			}
			out = append(out, 3)
		},
	)(&cfg)
	if !reflect.DeepEqual(out, []int{1, 2, 3}) {
		t.Errorf(`unexpected output: %v`, out)
	}
}

func TestReverseSlice(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		s := []int{}
		reverseSlice(s)
		if len(s) != 0 {
			t.Errorf("expected empty slice, but got: %v", s)
		}
	})

	t.Run("single element slice", func(t *testing.T) {
		s := []int{1}
		reverseSlice(s)
		if s[0] != 1 {
			t.Errorf("expected [1], but got: %v", s)
		}
	})

	t.Run("even number of elements slice", func(t *testing.T) {
		s := []int{1, 2, 3, 4}
		reverseSlice(s)
		expected := []int{4, 3, 2, 1}
		if len(s) != len(expected) {
			t.Errorf("expected length %d, but got length %d", len(expected), len(s))
		}
		for i := 0; i < len(s); i++ {
			if s[i] != expected[i] {
				t.Errorf("expected %v, but got: %v", expected, s)
			}
		}
	})

	t.Run("odd number of elements slice", func(t *testing.T) {
		s := []int{1, 2, 3, 4, 5}
		reverseSlice(s)
		expected := []int{5, 4, 3, 2, 1}
		if len(s) != len(expected) {
			t.Errorf("expected length %d, but got length %d", len(expected), len(s))
		}
		for i := 0; i < len(s); i++ {
			if s[i] != expected[i] {
				t.Errorf("expected %v, but got: %v", expected, s)
			}
		}
	})
}

func TestLoggerConfig_resolveWriter(t *testing.T) {
	writer1 := &mockWriter[*mockEvent]{}
	writer2 := &mockWriter[*mockEvent]{}
	writer3 := &mockWriter[*mockEvent]{}

	// Test empty writer slice
	config := &loggerConfig[*mockEvent]{}
	writer := config.resolveWriter()
	assert.Nil(t, writer)

	// Test single writer
	config.writer = WriterSlice[*mockEvent]{writer1}
	writer = config.resolveWriter()
	assert.Equal(t, writer1, writer)

	// Test multiple writers
	config.writer = WriterSlice[*mockEvent]{writer1, writer2, writer3}
	writer = config.resolveWriter()
	assert.Equal(t, WriterSlice[*mockEvent]{writer3, writer2, writer1}, writer)
}

func TestLoggerConfig_resolveModifier(t *testing.T) {
	modifier1 := &mockModifier[*mockEvent]{}
	modifier2 := &mockModifier[*mockEvent]{}
	modifier3 := &mockModifier[*mockEvent]{}

	// Test empty modifier slice
	config := &loggerConfig[*mockEvent]{}
	modifier := config.resolveModifier()
	assert.Nil(t, modifier)

	// Test single modifier
	config.modifier = ModifierSlice[*mockEvent]{modifier1}
	modifier = config.resolveModifier()
	assert.Equal(t, modifier1, modifier)

	// Test multiple modifiers
	config.modifier = ModifierSlice[*mockEvent]{modifier1, modifier2, modifier3}
	modifier = config.resolveModifier()
	assert.Equal(t, ModifierSlice[*mockEvent]{modifier3, modifier2, modifier1}, modifier)
}
