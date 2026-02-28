package logiface

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"
	"time"
)

var (
	_ Option[*mockEvent] = optionFunc[*mockEvent](nil)
)

func TestLogger_simple(t *testing.T) {
	t.Parallel()

	type Harness struct {
		L *Logger[*mockSimpleEvent]
		B bytes.Buffer
	}

	newHarness := func(t *testing.T, options ...Option[*mockSimpleEvent]) *Harness {
		var h Harness
		h.L = New(append([]Option[*mockSimpleEvent]{
			WithEventFactory[*mockSimpleEvent](EventFactoryFunc[*mockSimpleEvent](mockSimpleEventFactory)),
			WithWriter[*mockSimpleEvent](&mockSimpleWriter{Writer: &h.B}),
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

		if s := h.B.String(); s != "[info] msg=hello world\n[warning] msg=is warning\n[err] msg=is err\n[emerg] msg=is emerg\n" {
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

		if s := h.B.String(); s != "[info] one=1 two=2 three=3 msg=hello world\n" {
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

		if s := h.B.String(); s != "[info] one=1 two=2 three=3 four=4 msg=case 1\n[info] six=6 seven=7 eight=8 msg=case 2\n[info] one=1 two=2 three=-3 five=5 msg=case 3\n" {
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

		if s := h.B.String(); s != "[info] one=YWJj two= three=aGVsbG8gd29ybGQ= four=9A== msg=case 1\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`field default time`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Clone().
			Field(`one`, time.Unix(5, 9400000).Local()).
			Field(`two`, time.Time{}).
			Logger().
			Info().
			Field(`three`, time.Unix(5, 9400000)).
			Log(`case 1`)

		if s := h.B.String(); s != "[info] one=1970-01-01T00:00:05.009400Z two=0001-01-01T00:00:00Z three=1970-01-01T00:00:05.009400Z msg=case 1\n" {
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

		if s := h.B.String(); s != "[info] one=3600s zero=0s two=31.999900s three=-31.999900s msg=case 1\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`using Logf`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			Field(`one`, 1).
			Field(`two`, 2).
			Field(`three`, 3).
			Logf(`unstructured a=%d b=%q`, -143, `hello world`)

		if s := h.B.String(); s != "[info] one=1 two=2 three=3 msg=unstructured a=-143 b=\"hello world\"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`using LogFunc`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			Field(`one`, 1).
			Field(`two`, 2).
			Field(`three`, 3).
			LogFunc(func() string {
				return fmt.Sprintf(`unstructured a=%d b=%q`, -143, `hello world`)
			})

		if s := h.B.String(); s != "[info] one=1 two=2 three=3 msg=unstructured a=-143 b=\"hello world\"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})
}

func TestLogger_simpleGeneric(t *testing.T) {
	t.Parallel()

	type Harness struct {
		L *Logger[Event]
		B bytes.Buffer
	}

	// TODO use the other fields to check they're passed down correctly via Logger.Logger
	newHarness := func(t *testing.T, options ...Option[*mockSimpleEvent]) *Harness {
		var h Harness
		l := New(append([]Option[*mockSimpleEvent]{
			WithEventFactory[*mockSimpleEvent](EventFactoryFunc[*mockSimpleEvent](mockSimpleEventFactory)),
			WithWriter[*mockSimpleEvent](&mockSimpleWriter{Writer: &h.B}),
		}, options...)...)
		if l.shared.level != LevelInformational {
			t.Error(l.shared.level)
		}
		h.L = l.Logger()
		if h.L.shared.level != l.shared.level {
			t.Error(h.L.shared.level)
		}
		if h.L.shared.pool != &genericBuilderPool {
			t.Error(h.L.shared.pool)
		}
		if l := h.L.Logger(); l != h.L {
			t.Error(l)
		}
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

		if s := h.B.String(); s != "[info] msg=hello world\n[warning] msg=is warning\n[err] msg=is err\n[emerg] msg=is emerg\n" {
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

		if s := h.B.String(); s != "[info] one=1 two=2 three=3 msg=hello world\n" {
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

		if s := h.B.String(); s != "[info] one=1 two=2 three=3 four=4 msg=case 1\n[info] six=6 seven=7 eight=8 msg=case 2\n[info] one=1 two=2 three=-3 five=5 msg=case 3\n" {
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

		if s := h.B.String(); s != "[info] one=YWJj two= three=aGVsbG8gd29ybGQ= four=9A== msg=case 1\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})

	t.Run(`field default time`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Clone().
			Field(`one`, time.Unix(5, 9400000).Local()).
			Field(`two`, time.Time{}).
			Logger().
			Info().
			Field(`three`, time.Unix(5, 9400000)).
			Log(`case 1`)

		if s := h.B.String(); s != "[info] one=1970-01-01T00:00:05.009400Z two=0001-01-01T00:00:00Z three=1970-01-01T00:00:05.009400Z msg=case 1\n" {
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

		if s := h.B.String(); s != "[info] one=3600s zero=0s two=31.999900s three=-31.999900s msg=case 1\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})
}

func TestLoggerFactory_WithOptions_noOptions(t *testing.T) {
	L.WithOptions().apply(nil)
}

func TestLoggerFactory_WithOptions_callsAllOptions(t *testing.T) {
	var cfg loggerConfig[Event]
	var out []int
	L.WithOptions(
		optionFunc[Event](func(c *loggerConfig[Event]) {
			if c != &cfg {
				t.Error(`unexpected config`)
			}
			out = append(out, 1)
		}),
		optionFunc[Event](func(c *loggerConfig[Event]) {
			if c != &cfg {
				t.Error(`unexpected config`)
			}
			out = append(out, 2)
		}),
		optionFunc[Event](func(c *loggerConfig[Event]) {
			if c != &cfg {
				t.Error(`unexpected config`)
			}
			out = append(out, 3)
		}),
	).apply(&cfg)
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
		for i := range s {
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
		for i := range s {
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
	if writer != nil {
		t.Errorf("expected nil, got %v", writer)
	}

	// Test single writer
	config.writer = WriterSlice[*mockEvent]{writer1}
	writer = config.resolveWriter()
	if writer != writer1 {
		t.Errorf("got %v, want %v", writer, writer1)
	}

	// Test multiple writers
	config.writer = WriterSlice[*mockEvent]{writer1, writer2, writer3}
	writer = config.resolveWriter()
	expected := WriterSlice[*mockEvent]{writer3, writer2, writer1}
	if !reflect.DeepEqual(writer, expected) {
		t.Errorf("got %v, want %v", writer, expected)
	}
	// reflect.DeepEqual doesn't seem to catch the reference equality
	for i, v := range writer.(WriterSlice[*mockEvent]) {
		if v != expected[i] {
			t.Errorf("[%d] expected %p, but got: %p", i, expected[i], v)
		}
	}
}

func TestLoggerConfig_resolveModifier(t *testing.T) {
	modifier1 := &mockModifier[*mockEvent]{}
	modifier2 := &mockModifier[*mockEvent]{}
	modifier3 := &mockModifier[*mockEvent]{}

	// Test empty modifier slice
	config := &loggerConfig[*mockEvent]{}
	modifier := config.resolveModifier()
	if modifier != nil {
		t.Errorf("expected nil, got %v", modifier)
	}

	// Test single modifier
	config.modifier = ModifierSlice[*mockEvent]{modifier1}
	modifier = config.resolveModifier()
	if modifier != modifier1 {
		t.Errorf("got %v, want %v", modifier, modifier1)
	}

	// Test multiple modifiers
	config.modifier = ModifierSlice[*mockEvent]{modifier1, modifier2, modifier3}
	modifier = config.resolveModifier()
	expected := ModifierSlice[*mockEvent]{modifier1, modifier2, modifier3}
	if !reflect.DeepEqual(modifier, expected) {
		t.Errorf("got %v, want %v", modifier, expected)
	}
	// reflect.DeepEqual doesn't seem to catch the reference equality
	for i, v := range modifier.(ModifierSlice[*mockEvent]) {
		if v != expected[i] {
			t.Errorf("[%d] expected %p, but got: %p", i, expected[i], v)
		}
	}
}

func TestLogger_Logger_nilReceiver(t *testing.T) {
	if (*Logger[*mockEvent])(nil).Logger() != nil {
		t.Error(`expected nil`)
	}
}

func TestLogger_Logger_nilShared(t *testing.T) {
	if (&Logger[*mockEvent]{}).Logger() != nil {
		t.Error(`expected nil`)
	}
}

// An example of how to use the non-fluent Log method.
func ExampleLogger_Log() {
	l := newSimpleLogger(os.Stdout, false).Logger()

	if err := l.Log(LevelDisabled, nil); err != ErrDisabled {
		panic(err)
	} else {
		fmt.Printf("disabled level wont log: %v\n", err)
	}

	// note: the method under test is intended for external log integrations
	// (this isn't a real use case)
	with := func(fields ...any) ModifierFunc[Event] {
		return func(e Event) error {
			if len(fields)%2 != 0 {
				return fmt.Errorf("invalid number of fields: %d", len(fields))
			}
			for i := 0; i < len(fields); i += 2 {
				e.AddField(fmt.Sprint(fields[i]), fields[i+1])
			}
			return nil
		}
	}

	fmt.Print(`single modifier provided in the call: `)
	if err := l.Log(LevelNotice, with(
		`a`, `A`,
		`b`, `B`,
	)); err != nil {
		panic(err)
	}

	fmt.Print(`cloned logger modifier + modifier in the call: `)
	if err := l.Clone().Str(`a`, `A1`).Logger().Log(LevelNotice, with(`a`, `A2`)); err != nil {
		panic(err)
	}

	fmt.Print(`just cloned logger modifier: `)
	if err := l.Clone().Str(`a`, `A1`).Logger().Log(LevelNotice, nil); err != nil {
		panic(err)
	}

	fmt.Print(`no logger modifier: `)
	if err := l.Log(LevelNotice, nil); err != nil {
		panic(err)
	}

	fmt.Printf("modifier error: %v\n", l.Log(LevelNotice, with(`willerror`)))

	fmt.Printf("internal modifier error guards provided modifier: %v\n", New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: os.Stdout}),
		mockL.WithModifier(NewModifierFunc(func(e *mockSimpleEvent) error {
			return fmt.Errorf(`some internal modifier error`)
		})),
	).Log(LevelNotice, ModifierFunc[*mockSimpleEvent](func(e *mockSimpleEvent) error {
		panic(`should not be called`)
	})))

	//output:
	//disabled level wont log: logger disabled
	//single modifier provided in the call: [notice] a=A b=B
	//cloned logger modifier + modifier in the call: [notice] a=A1 a=A2
	//just cloned logger modifier: [notice] a=A1
	//no logger modifier: [notice]
	//modifier error: invalid number of fields: 1
	//internal modifier error guards provided modifier: some internal modifier error
}

func TestLogger_Level_nilReceiver(t *testing.T) {
	if v := (*Logger[*mockEvent])(nil).Level(); v != LevelDisabled {
		t.Error(v)
	}
}

func TestLogger_Level_minus100(t *testing.T) {
	l := New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: io.Discard}),
		mockL.WithLevel(-100),
	)
	if !l.Enabled() || l.shared.level != -100 {
		t.Fatal()
	}
	if v := l.Level(); v != LevelDisabled {
		t.Error(v)
	}
}

func TestLogger_Level_crit(t *testing.T) {
	l := New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: io.Discard}),
		mockL.WithLevel(LevelCritical),
	)
	if !l.Enabled() || l.shared.level != LevelCritical {
		t.Fatal()
	}
	if v := l.Level(); v != LevelCritical {
		t.Error(v)
	}
}

func ExampleLogger_Panic() {
	opts := mockL.WithOptions(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: os.Stdout}),
	)

	func() {
		defer func() { fmt.Printf("recover: %v\n", recover()) }()
		New(opts).Panic().Log(`will panic after logging`)
	}()

	fmt.Println(`you should set the message though there is a default string that will be used if you don't`)
	func() {
		defer func() { fmt.Printf("recover: %v\n", recover()) }()
		New(opts).Panic().Str(`some`, `data`).Log(``)
	}()

	func() {
		defer func() { fmt.Printf("recover: %v\n", recover()) }()
		fmt.Println(`will pre-emptively panic if the logger is disabled`)
		New(opts, mockL.WithLevel(LevelDisabled), mockL.WithDPanicLevel(LevelEmergency)).DPanic()
	}()

	//output:
	//[emerg] msg=will panic after logging
	//recover: will panic after logging
	//you should set the message though there is a default string that will be used if you don't
	//[emerg] some=data
	//recover: logiface: panic requested
	//will pre-emptively panic if the logger is disabled
	//recover: logiface: panic requested but logger is disabled
}

func ExampleLogger_DPanic() {
	opts := mockL.WithOptions(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: os.Stdout}),
	)

	New(opts).DPanic().Log(`DPanic uses LevelCritical by default`)

	func() {
		defer func() { fmt.Printf("recover: %v\n", recover()) }()
		New(opts, mockL.WithDPanicLevel(LevelEmergency)).DPanic().Log(`if set to LevelEmergency DPanic behaves per Panic`)
	}()

	func() {
		defer func() { fmt.Printf("recover: %v\n", recover()) }()
		fmt.Println(`like Panic, DPanic at LevelEmergency will pre-emptively panic if the logger is disabled`)
		New(opts, mockL.WithLevel(LevelDisabled), mockL.WithDPanicLevel(LevelEmergency)).DPanic()
	}()

	fmt.Println(`note that if the DPanic level is not LevelEmergency it is possible that neither log or panic will occur`)
	logger := New(opts, mockL.WithLevel(LevelDisabled))
	logger.DPanic().Log(`does nothing as the logger is disabled and the DPanic level is not LevelEmergency`)
	logger = nil
	logger.DPanic().Log(`does nothing for the same reasons as above`)

	//output:
	//[crit] msg=DPanic uses LevelCritical by default
	//[emerg] msg=if set to LevelEmergency DPanic behaves per Panic
	//recover: if set to LevelEmergency DPanic behaves per Panic
	//like Panic, DPanic at LevelEmergency will pre-emptively panic if the logger is disabled
	//recover: logiface: panic requested but logger is disabled
	//note that if the DPanic level is not LevelEmergency it is possible that neither log or panic will occur
}

func TestLogger_DPanic_nilShared(t *testing.T) {
	(&Logger[*mockEvent]{}).DPanic().Log(`does nothing`)
}

func ExampleLogger_Fatal() {
	{
		old := OsExit
		defer func() { OsExit = old }()
	}
	OsExit = func(code int) {
		fmt.Printf("exited with code: %d\n", code)
	}

	l := newSimpleLogger(os.Stdout, false)

	l.Fatal().Log(`will exit after logging`)

	fmt.Println(`will pre-emptively exit if the logger or that log level is disabled`)
	l = nil
	if l.Fatal() != nil {
		panic(`strange...`)
	}

	//output:
	//[alert] msg=will exit after logging
	//exited with code: 1
	//will pre-emptively exit if the logger or that log level is disabled
	//exited with code: 1
}

func TestLogger_Root(t *testing.T) {
	l := newSimpleLogger(io.Discard, false)
	if v := l.Root(); v != l {
		t.Error(v)
	}
	if v := l.Clone().Logger().Root(); v != l {
		t.Error(v)
	}
	if v := l.Clone().Logger().Clone().Logger().Clone().Logger().Root(); v != l {
		t.Error(v)
	}
	g := l.Logger()
	if v := g.Root(); v != g {
		t.Error(v)
	}
	if v := g.Clone().Logger().Root(); v != g {
		t.Error(v)
	}
	if v := g.Clone().Logger().Clone().Logger().Clone().Logger().Root(); v != g {
		t.Error(v)
	}
	if (*Logger[*mockSimpleEvent])(nil).Root() != nil {
		t.Error()
	}
	if (&Logger[*mockSimpleEvent]{}).Root() != nil {
		t.Error()
	}
	if (&Logger[*mockSimpleEvent]{shared: new(loggerShared[*mockSimpleEvent])}).Root() != nil {
		t.Error()
	}
}
