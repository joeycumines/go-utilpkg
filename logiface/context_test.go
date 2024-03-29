package logiface

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/joeycumines/logiface/internal/fieldtest"
	"github.com/stretchr/testify/assert"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBuilder_Call_nilReceiver(t *testing.T) {
	var called int
	if ((*Builder[Event])(nil)).Call(func(b *Builder[Event]) {
		if b != nil {
			t.Error()
		}
		called++
	}) != nil {
		t.Error()
	}
	if called != 0 {
		t.Error(called)
	}
}

func TestBuilder_Call_notEnabled(t *testing.T) {
	b := &Builder[Event]{}
	var called int
	if b.Call(func(b *Builder[Event]) {
		if b != nil {
			t.Error()
		}
		called++
	}) != b {
		t.Error()
	}
	if called != 0 {
		t.Error(called)
	}
}

func TestBuilder_Call(t *testing.T) {
	builder := &Builder[Event]{shared: &loggerShared[Event]{}}
	var called int
	if b := builder.Call(func(b *Builder[Event]) {
		if b != builder {
			t.Error(b)
		}
		called++
	}); b != builder {
		t.Error(b)
	}
	if called != 1 {
		t.Error(called)
	}
}

func TestContext_Any_nilReceiver(t *testing.T) {
	if (*Context[*mockEvent])(nil).Any(`key`, `val`) != nil {
		t.Error(`expected nil`)
	}
}

func TestBuilder_Any_nilReceiver(t *testing.T) {
	if (*Builder[*mockEvent])(nil).Any(`key`, `val`) != nil {
		t.Error(`expected nil`)
	}
}

// TestEvent_min tests the minimum set of methods required to implement the Event interface.
func TestEvent_min(t *testing.T) {
	const message = `log called`
	test := func(t *testing.T, log func(l *Logger[*mockSimpleEvent])) {
		t.Helper()
		var buf bytes.Buffer
		l := newSimpleLogger(&buf, true)
		log(l)
		const expected = "[info]\nerr=err called\nfield called with string=val 2\nfield called with bytes=dmFsIDM=\nfield called with time.Time local=2019-05-17T05:07:20.361696123Z\nfield called with time.Time utc=2019-05-17T05:07:20.361696123Z\nfield called with duration=3116139.280723392s\nfield called with int=-51245\nfield called with float32=1e-45\nfield called with unhandled type=-421\nfloat32 called=3.4028235e+38\nint called=9223372036854775807\ninterface called with string=val 4\ninterface called with bool=true\ninterface called with nil=<nil>\nany called with string=val 5\nstr called=val 6\ntime called with local=2021-03-24T13:27:29.876543213Z\ntime called with utc=2020-03-01T00:39:29.456789123Z\ndur called positive=51238123.523458989s\ndur called negative=-51238123.523458989s\ndur called zero=0s\nbase64 called with nil enc=dmFsIDc=\nbase64 called with padding=dmFsIDc=\nbase64 called without padding=dmFsIDc\nbool called=true\nfield called with bool=true\nfloat64 called=1.7976931348623157e+308\nfield called with float64=1.7976931348623157e+308\nint64 called=9223372036854775807\nfield called with int64=9223372036854775807\nuint64 called=18446744073709551615\nfield called with uint64=18446744073709551615\nstringer called=byte stringer 1\nstringer called with nil value=<nil>\nraw json called=[123 34 107 101 121 34 58 34 118 97 108 34 125]\nfield called with raw json=[91 32 102 97 108 115 101 44 32 110 117 108 108 32 93]\nmsg=log called\n"
		if actual := buf.String(); actual != expected {
			t.Errorf("unexpected output: %q\n%s", actual, stringDiff(expected, actual))
		}
	}
	t.Run(`Context`, func(t *testing.T) {
		test(t, func(l *Logger[*mockSimpleEvent]) {
			c := l.Clone()
			fieldtest.FluentObjectTemplate(c)
			c.Logger().Info().Log(message)
		})
	})
	t.Run(`Builder`, func(t *testing.T) {
		test(t, func(l *Logger[*mockSimpleEvent]) {
			b := l.Info()
			fieldtest.FluentObjectTemplate(b)
			b.Log(message)
		})
	})
}

// TestEvent_max tests an implementation using the complete set of available Event methods.
func TestEvent_max(t *testing.T) {
	const message = `log called`
	test := func(t *testing.T, log func(l *Logger[*mockComplexEvent])) {
		t.Helper()
		w := mockComplexWriter{}
		l := New[*mockComplexEvent](
			WithEventFactory[*mockComplexEvent](EventFactoryFunc[*mockComplexEvent](mockComplexEventFactory)),
			WithWriter[*mockComplexEvent](&w),
		)
		log(l)
		expected := []*mockComplexEvent{
			{
				LevelValue: LevelInformational,
				FieldValues: []mockComplexEventField{
					{
						Type:  `AddError`,
						Value: errors.New(`err called`),
					},
					{
						Type:  `AddString`,
						Key:   "field called with string",
						Value: "val 2",
					},
					{
						Type:  `AddBase64Bytes`,
						Key:   "field called with bytes",
						Value: "dmFsIDM=",
					},
					{
						Type:  `AddTime`,
						Key:   "field called with time.Time local",
						Value: time.Unix(0, 1558069640361696123),
					},
					{
						Type:  `AddTime`,
						Key:   "field called with time.Time utc",
						Value: time.Unix(0, 1558069640361696123).UTC(),
					},
					{
						Type:  "AddDuration",
						Key:   "field called with duration",
						Value: time.Duration(3116139280723392),
					},
					{
						Type:  `AddInt`,
						Key:   "field called with int",
						Value: -51245,
					},
					{
						Type:  `AddFloat32`,
						Key:   "field called with float32",
						Value: float32(math.SmallestNonzeroFloat32),
					},
					{
						Type:  `AddField`,
						Key:   "field called with unhandled type",
						Value: fieldtest.IntDataType(-421),
					},
					{
						Type:  "AddFloat32",
						Key:   `float32 called`,
						Value: float32(math.MaxFloat32),
					},
					{
						Type:  `AddInt`,
						Key:   `int called`,
						Value: math.MaxInt,
					},
					{
						Type:  `AddField`,
						Key:   "interface called with string",
						Value: "val 4",
					},
					{
						Type:  `AddField`,
						Key:   "interface called with bool",
						Value: true,
					},
					{
						Type:  `AddField`,
						Key:   "interface called with nil",
						Value: nil,
					},
					{
						Type:  `AddField`,
						Key:   "any called with string",
						Value: "val 5",
					},
					{
						Type:  `AddString`,
						Key:   "str called",
						Value: "val 6",
					},
					{
						Type:  `AddTime`,
						Key:   "time called with local",
						Value: time.Unix(0, 1616592449876543213),
					},
					{
						Type:  `AddTime`,
						Key:   "time called with utc",
						Value: time.Unix(0, 1583023169456789123).UTC(),
					},
					{
						Type:  `AddDuration`,
						Key:   "dur called positive",
						Value: time.Duration(51238123523458989),
					},
					{
						Type:  `AddDuration`,
						Key:   "dur called negative",
						Value: time.Duration(-51238123523458989),
					},
					{
						Type:  `AddDuration`,
						Key:   "dur called zero",
						Value: time.Duration(0),
					},
					{
						Type:  `AddBase64Bytes`,
						Key:   "base64 called with nil enc",
						Value: "dmFsIDc=",
					},
					{
						Type:  `AddBase64Bytes`,
						Key:   "base64 called with padding",
						Value: "dmFsIDc=",
					},
					{
						Type:  `AddBase64Bytes`,
						Key:   "base64 called without padding",
						Value: "dmFsIDc",
					},
					{
						Type:  `AddBool`,
						Key:   `bool called`,
						Value: true,
					},
					{
						Type:  `AddBool`,
						Key:   `field called with bool`,
						Value: true,
					},
					{
						Type:  `AddFloat64`,
						Key:   `float64 called`,
						Value: float64(math.MaxFloat64),
					},
					{
						Type:  `AddFloat64`,
						Key:   `field called with float64`,
						Value: float64(math.MaxFloat64),
					},
					{
						Type:  `AddInt64`,
						Key:   `int64 called`,
						Value: int64(math.MaxInt64),
					},
					{
						Type:  `AddInt64`,
						Key:   `field called with int64`,
						Value: int64(math.MaxInt64),
					},
					{
						Type:  `AddUint64`,
						Key:   `uint64 called`,
						Value: uint64(math.MaxUint64),
					},
					{
						Type:  `AddUint64`,
						Key:   `field called with uint64`,
						Value: uint64(math.MaxUint64),
					},
					{
						Type:  `AddString`,
						Key:   `stringer called`,
						Value: `byte stringer 1`,
					},
					{
						Type:  `AddString`,
						Key:   `stringer called with nil value`,
						Value: `<nil>`,
					},
					{
						Type:  `AddRawJSON`,
						Key:   `raw json called`,
						Value: json.RawMessage(`{"key":"val"}`),
					},
					{
						Type:  `AddRawJSON`,
						Key:   `field called with raw json`,
						Value: json.RawMessage(`[ false, null ]`),
					},
					{
						Type:  `AddMessage`,
						Value: message,
					},
				},
			},
		}
		assert.Equal(t, w.events, expected)
	}
	t.Run(`Context`, func(t *testing.T) {
		test(t, func(l *Logger[*mockComplexEvent]) {
			c := l.Clone()
			fieldtest.FluentObjectTemplate(c)
			c.Logger().Info().Log(message)
		})
	})
	t.Run(`Builder`, func(t *testing.T) {
		test(t, func(l *Logger[*mockComplexEvent]) {
			b := l.Info()
			fieldtest.FluentObjectTemplate(b)
			b.Log(message)
		})
	})
}

func TestContext_disabledEvent(t *testing.T) {
	c := Context[*mockComplexEvent]{logger: &Logger[*mockComplexEvent]{}}
	if !c.Enabled() {
		t.Fatal()
	}
	fieldtest.FluentObjectTemplate(&c)
	if len(c.Modifiers) == 0 {
		t.Fatal()
	}
	for i, modifier := range c.Modifiers {
		if err := modifier.Modify(&mockComplexEvent{LevelValue: LevelDisabled}); err != ErrDisabled {
			t.Errorf("unexpected error at index %d: %v", i, err)
			continue
		}
		if err := modifier.Modify(&mockComplexEvent{LevelValue: math.MinInt8}); err != ErrDisabled {
			t.Errorf("unexpected error at index %d: %v", i, err)
		}
	}
}

func TestBuilder_disabledEvent(t *testing.T) {
	b := Builder[*mockComplexEvent]{
		Event:  &mockComplexEvent{LevelValue: LevelDisabled},
		shared: &loggerShared[*mockComplexEvent]{},
	}
	if !b.Enabled() {
		t.Fatal()
	}
	fieldtest.FluentObjectTemplate(&b)
}

func TestBuilder_Log_nilReceiver(t *testing.T) {
	(*Builder[*mockComplexEvent])(nil).Log(`message`)
}

func TestBuilder_Logf_nilReceiver(t *testing.T) {
	(*Builder[*mockComplexEvent])(nil).Logf(`%s`, `message`)
}

func TestBuilder_LogFunc_nilReceiver(t *testing.T) {
	(*Builder[*mockComplexEvent])(nil).LogFunc(func() string { return `message` })
}

func TestBuilder_Log_nilShared(t *testing.T) {
	(&Builder[*mockComplexEvent]{}).Log(`message`)
}

func TestBuilder_Logf_nilShared(t *testing.T) {
	(&Builder[*mockComplexEvent]{}).Logf(`%s`, `message`)
}

func TestBuilder_LogFunc_nilShared(t *testing.T) {
	(&Builder[*mockComplexEvent]{}).LogFunc(func() string { return `message` })
}

func TestBuilder_logEventDisabled(t *testing.T) {
	for _, tc := range [...]struct {
		name string
		log  func(*Builder[*mockComplexEvent])
	}{
		{
			name: `Log`,
			log:  func(b *Builder[*mockComplexEvent]) { b.Log(`message`) },
		},
		{
			name: `Logf`,
			log:  func(b *Builder[*mockComplexEvent]) { b.Logf(`%s`, `message`) },
		},
		{
			name: `LogFunc`,
			log:  func(b *Builder[*mockComplexEvent]) { b.LogFunc(func() string { return `message` }) },
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := loggerShared[*mockComplexEvent]{pool: new(sync.Pool)}
			e := &mockComplexEvent{LevelValue: LevelDisabled}
			b := Builder[*mockComplexEvent]{
				Event:  e,
				shared: &s,
			}
			tc.log(&b)
			if b.Event != nil {
				t.Error(b.Event)
			}
			if len(e.FieldValues) != 0 {
				t.Error(e.FieldValues)
			}
			if b.shared != nil {
				t.Error(b.shared)
			}
			if v := s.pool.Get(); v != &b && v != nil {
				t.Error(v)
			} else if v := s.pool.Get(); v != nil {
				t.Error(v)
			}
		})
	}
}

func TestBuilder_logWritePanicStillReleases(t *testing.T) {
	for _, tc := range [...]struct {
		name string
		log  func(*Builder[*mockComplexEvent])
	}{
		{
			name: `Log`,
			log:  func(b *Builder[*mockComplexEvent]) { b.Log(`message`) },
		},
		{
			name: `Logf`,
			log:  func(b *Builder[*mockComplexEvent]) { b.Logf(`%s`, `message`) },
		},
		{
			name: `LogFunc`,
			log:  func(b *Builder[*mockComplexEvent]) { b.LogFunc(func() string { return `message` }) },
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ev := &mockComplexEvent{}
			s := loggerShared[*mockComplexEvent]{
				pool: new(sync.Pool),
			}
			b := Builder[*mockComplexEvent]{
				Event:  ev,
				shared: &s,
			}
			err := errors.New(`some error`)
			in := make(chan *mockComplexEvent)
			defer close(in)
			out := make(chan struct{})
			var calls int64
			s.writer = NewWriterFunc(func(event *mockComplexEvent) error {
				if b.shared != &s {
					t.Error(b.shared)
				}
				atomic.AddInt64(&calls, 1)
				panic(err)
			})
			s.releaser = NewEventReleaserFunc(func(event *mockComplexEvent) {
				in <- event
				<-out
			})

			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					if r := recover(); r != err {
						t.Error(r)
					}
				}()
				tc.log(&b)
			}()

			time.Sleep(time.Millisecond * 40)
			select {
			case <-done:
				t.Fatal()
			default:
			}

			e := <-in
			if v := atomic.LoadInt64(&calls); v != 1 {
				t.Error(v)
			}
			if e != ev {
				t.Error(e)
			}
			if len(b.Event.FieldValues) != 1 {
				t.Error(b.Event.FieldValues)
			}
			if b.shared != nil {
				t.Error(b.shared)
			}

			// shouldn't be returned to the pool yet
			if v := s.pool.Get(); v != nil {
				t.Error(v)
			}

			out <- struct{}{}

			<-done

			if v := s.pool.Get(); v != &b && v != nil {
				t.Error(v)
			} else if v := s.pool.Get(); v != nil {
				t.Error(v)
			}
		})
	}
}

func TestBuilder_Release_nilReceiver(t *testing.T) {
	(*Builder[*mockComplexEvent])(nil).Release()
}

func TestBuilder_Release_callReleaser(t *testing.T) {
	in := make(chan *mockEvent)
	out := make(chan struct{})
	releaser := NewEventReleaserFunc(func(event *mockEvent) {
		in <- event
		<-out
	})
	ev := &mockEvent{}
	pool := new(sync.Pool)
	shared := &loggerShared[*mockEvent]{
		pool:     pool,
		releaser: releaser,
	}
	builder := &Builder[*mockEvent]{
		Event:  ev,
		shared: shared,
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		builder.Release()
	}()
	if v := <-in; v != ev {
		t.Error(v)
	}
	if builder.shared != nil {
		t.Error()
	}
	if v := pool.Get(); v != nil {
		t.Error(v)
	}
	out <- struct{}{}
	<-done
	close(in)
	close(out)
	if v := pool.Get(); v != builder && v != nil {
		t.Error(v)
	}
	time.Sleep(time.Millisecond * 50)
	if v := pool.Get(); v != nil {
		t.Error(v)
	}
	if builder.Event != nil {
		t.Error(builder)
	}
	if shared.pool != pool || shared.releaser == nil {
		t.Error()
	}
	builder.Release()
	if shared.pool != pool || shared.releaser == nil {
		t.Error()
	}
}

func TestBuilder_Release_noReleaser(t *testing.T) {
	ev := &mockEvent{}
	pool := new(sync.Pool)
	shared := &loggerShared[*mockEvent]{
		pool: pool,
	}
	builder := &Builder[*mockEvent]{
		Event:  ev,
		shared: shared,
	}
	builder.Release()
	if builder.shared != nil {
		t.Error()
	}
	if builder.Event != nil {
		t.Error(builder)
	}
	if v := pool.Get(); v != builder && v != nil {
		t.Error(v)
	}
	time.Sleep(time.Millisecond * 50)
	if v := pool.Get(); v != nil {
		t.Error(v)
	}
	if shared.pool != pool || shared.releaser != nil {
		t.Error()
	}
	builder.Release()
	if shared.pool != pool || shared.releaser != nil {
		t.Error()
	}
}

func ExampleContext_Call_arrayField() {
	type E = *mockSimpleEvent
	var logger *Logger[E] = newSimpleLogger(os.Stdout, false)

	// while not a necessary part of the Array API, this pattern makes things easier to manage
	subLogger := logger.Clone().
		Call(func(b *Context[E]) { Array[E](b).Field(1).Field(2).Field(3).As(`arr_field`) }).
		Str(`str_field`, `str_value`).
		Logger()

	subLogger.Info().Log(`log msg`)

	//output:
	//[info] arr_field=[1 2 3] str_field=str_value msg=log msg
}

func TestContext_Call_disabled(t *testing.T) {
	(*Context[*mockSimpleEvent])(nil).Call(func(b *Context[*mockSimpleEvent]) { t.Error() })
}

func TestBuilder_root(t *testing.T) {
	if (*Builder[*mockSimpleEvent])(nil).Root() != nil {
		t.Error()
	}
	if (&Builder[*mockSimpleEvent]{}).Root() != nil {
		t.Error()
	}
	if (&Builder[*mockSimpleEvent]{shared: &loggerShared[*mockSimpleEvent]{}}).Root() != nil {
		t.Error()
	}
	var l Logger[*mockSimpleEvent]
	if (&Builder[*mockSimpleEvent]{shared: &loggerShared[*mockSimpleEvent]{root: &l}}).Root() != &l {
		t.Error()
	}
}

func TestContext_root(t *testing.T) {
	if (*Context[*mockSimpleEvent])(nil).Root() != nil {
		t.Error()
	}
	if (&Context[*mockSimpleEvent]{}).Root() != nil {
		t.Error()
	}
	if (&Context[*mockSimpleEvent]{logger: &Logger[*mockSimpleEvent]{}}).Root() != nil {
		t.Error()
	}
	if (&Context[*mockSimpleEvent]{logger: &Logger[*mockSimpleEvent]{shared: &loggerShared[*mockSimpleEvent]{}}}).Root() != nil {
		t.Error()
	}
	var l Logger[*mockSimpleEvent]
	if (&Context[*mockSimpleEvent]{logger: &Logger[*mockSimpleEvent]{shared: &loggerShared[*mockSimpleEvent]{root: &l}}}).Root() != &l {
		t.Error()
	}
}

func TestBuilder_Modifier_nilReceiver(t *testing.T) {
	if v := (*Builder[*mockEvent])(nil).Modifier(ModifierFunc[*mockEvent](func(event *mockEvent) error {
		t.Error()
		return nil
	})); v != nil {
		t.Error(v)
	}
}

func TestBuilder_Modifier_nilModifier(t *testing.T) {
	b := &Builder[*mockEvent]{shared: new(loggerShared[*mockEvent])}
	if v := b.Modifier(nil); v != b {
		t.Error(v)
	}
}

func TestBuilder_Modifier_callModifier(t *testing.T) {
	ev := &mockEvent{}
	shared := &loggerShared[*mockEvent]{
		pool: new(sync.Pool),
	}
	builder := &Builder[*mockEvent]{
		Event:  ev,
		shared: shared,
	}
	var called bool
	builder.Modifier(ModifierFunc[*mockEvent](func(event *mockEvent) error {
		if event != ev {
			t.Error(event)
		}
		called = true
		return nil
	}))
	if !called {
		t.Error()
	}
	if v := shared.pool.Get(); v != nil {
		t.Error(v)
	}
}

func TestBuilder_Modifier_error(t *testing.T) {
	var buf bytes.Buffer
	logger := mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: &buf}),
	)
	var called bool
	if v := logger.Info().Modifier(ModifierFunc[*mockSimpleEvent](func(event *mockSimpleEvent) error {
		if event == nil {
			t.Error(`nil event`)
		}
		called = true
		return errors.New(`test`)
	})); v != nil {
		t.Error(v)
	}
	if !called {
		t.Error()
	}
	if v := buf.String(); v != `[crit] err=test msg=modifier error`+"\n" {
		t.Errorf(`unexpected output: %s`, v)
	}
}

func TestBuilder_Modifier_callReleaser(t *testing.T) {
	for _, sentinel := range [...]error{
		ErrDisabled,
		ErrLimited,
		fmt.Errorf(`wrapped: %w`, ErrDisabled),
		fmt.Errorf(`wrapped: %w`, ErrLimited),
	} {
		sentinel := sentinel
		t.Run(sentinel.Error(), func(t *testing.T) {
			in := make(chan *mockEvent)
			out := make(chan struct{})
			releaser := NewEventReleaserFunc(func(event *mockEvent) {
				in <- event
				<-out
			})
			ev := &mockEvent{}
			pool := new(sync.Pool)
			shared := &loggerShared[*mockEvent]{
				pool:     pool,
				releaser: releaser,
			}
			builder := &Builder[*mockEvent]{
				Event:  ev,
				shared: shared,
			}
			done := make(chan struct{})
			go func() {
				var called bool
				var success bool
				defer func() {
					if !called || !success {
						t.Error(called, success, recover())
					}
					close(done)
				}()
				if v := builder.Modifier(ModifierFunc[*mockEvent](func(event *mockEvent) error {
					called = true
					return sentinel
				})); v != nil {
					t.Error(v)
				}
				success = true
			}()
			if v := <-in; v != ev {
				t.Error(v)
			}
			if builder.shared != nil {
				t.Error()
			}
			if v := pool.Get(); v != nil {
				t.Error(v)
			}
			out <- struct{}{}
			<-done
			close(in)
			close(out)
			if v := pool.Get(); v != builder && v != nil {
				t.Error(v)
			}
			time.Sleep(time.Millisecond * 50)
			if v := pool.Get(); v != nil {
				t.Error(v)
			}
			if builder.Event != nil {
				t.Error(builder)
			}
		})
	}
}

func TestContext_Modifier_nilReceiver(t *testing.T) {
	if v := (*Context[*mockEvent])(nil).Modifier(ModifierFunc[*mockEvent](func(event *mockEvent) error {
		t.Error()
		return nil
	})); v != nil {
		t.Error(v)
	}
}

func TestContext_Modifier_nilModifier(t *testing.T) {
	c := &Context[*mockEvent]{logger: &Logger[*mockEvent]{}}
	if v := c.Modifier(nil); v != c {
		t.Error(c)
	}
}

func TestContext_Modifier_appendsToSlice(t *testing.T) {
	c := &Context[*mockEvent]{logger: &Logger[*mockEvent]{}}
	var called bool
	expected := new(mockEvent)
	if v := c.Modifier(ModifierFunc[*mockEvent](func(event *mockEvent) error {
		if event != expected {
			t.Error(event)
		}
		called = true
		return nil
	})); v != c {
		t.Error(v)
	}
	if called {
		t.Error()
	}
	if len(c.Modifiers) != 1 {
		t.Error(c.Modifiers)
	}
	if err := c.Modifiers.Modify(expected); err != nil {
		t.Error(err)
	}
	if !called {
		t.Error()
	}
}
