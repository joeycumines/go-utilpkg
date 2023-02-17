package logiface

import (
	"bytes"
	"errors"
	"github.com/stretchr/testify/assert"
	"math"
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
	if called != 1 {
		t.Error(called)
	}
}

func TestBuilder_Call(t *testing.T) {
	builder := &Builder[Event]{}
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
	test := func(log func(l *Logger[*mockSimpleEvent])) {
		t.Helper()
		var buf bytes.Buffer
		l := New[*mockSimpleEvent](
			WithEventFactory[*mockSimpleEvent](EventFactoryFunc[*mockSimpleEvent](mockSimpleEventFactory)),
			WithWriter[*mockSimpleEvent](&mockSimpleWriter{Writer: &buf, MultiLine: true}),
		)
		log(l)
		const expected = "[info]\nerr=err called\nfield called with string=val 2\nfield called with bytes=dmFsIDM=\nfield called with time.Time=2019-05-17T05:07:20.361696123Z\nfield called with duration=3116139.280723392s\nfield called with int=-51245\nfield called with float32=1e-45\nfield called with unhandled type=-421\nfloat32 called=3.4028235e+38\nint called=9223372036854775807\ninterface called with string=val 4\ninterface called with bool=true\ninterface called with nil=<nil>\nany called with string=val 5\nstr called=val 6\nmsg=log called\n"
		if actual := buf.String(); actual != expected {
			t.Errorf("unexpected output: %q\n%s", actual, stringDiff(expected, actual))
		}
	}
	t.Run(`Context`, func(t *testing.T) {
		test(func(l *Logger[*mockSimpleEvent]) {
			c := l.Clone()
			fluentCallerTemplate(c)
			c.Logger().Info().Log(message)
		})
	})
	t.Run(`Builder`, func(t *testing.T) {
		test(func(l *Logger[*mockSimpleEvent]) {
			b := l.Info()
			fluentCallerTemplate(b)
			b.Log(message)
		})
	})
}

// TestEvent_max tests an implementation using the complete set of available Event methods.
func TestEvent_max(t *testing.T) {
	const message = `log called`
	test := func(log func(l *Logger[*mockComplexEvent])) {
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
						Type:  `AddString`,
						Key:   "field called with bytes",
						Value: "dmFsIDM=",
					},
					{
						Type:  `AddString`,
						Key:   "field called with time.Time",
						Value: "2019-05-17T05:07:20.361696123Z",
					},
					{
						Type:  "AddString",
						Key:   "field called with duration",
						Value: "3116139.280723392s",
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
						Value: mockIntDataType(-421),
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
						Type:  `AddMessage`,
						Value: message,
					},
				},
			},
		}
		assert.Equal(t, w.events, expected)
	}
	t.Run(`Context`, func(t *testing.T) {
		test(func(l *Logger[*mockComplexEvent]) {
			c := l.Clone()
			fluentCallerTemplate(c)
			c.Logger().Info().Log(message)
		})
	})
	t.Run(`Builder`, func(t *testing.T) {
		test(func(l *Logger[*mockComplexEvent]) {
			b := l.Info()
			fluentCallerTemplate(b)
			b.Log(message)
		})
	})
}

func TestContext_disabledEvent(t *testing.T) {
	c := Context[*mockComplexEvent]{logger: &Logger[*mockComplexEvent]{}}
	if !c.ok() {
		t.Fatal()
	}
	fluentCallerTemplate(&c)
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
	if !b.ok() {
		t.Fatal()
	}
	fluentCallerTemplate(&b)
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
			b := Builder[*mockComplexEvent]{
				Event:  &mockComplexEvent{LevelValue: LevelDisabled},
				shared: &s,
			}
			tc.log(&b)
			if len(b.Event.FieldValues) != 0 {
				t.Error(b.Event.FieldValues)
			}
			if b.shared != nil {
				t.Error(b.shared)
			}
			if v := s.pool.Get(); v != &b {
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
			s.writer = WriterFunc[*mockComplexEvent](func(event *mockComplexEvent) error {
				if b.shared != &s {
					t.Error(b.shared)
				}
				atomic.AddInt64(&calls, 1)
				panic(err)
			})
			s.releaser = EventReleaserFunc[*mockComplexEvent](func(event *mockComplexEvent) {
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

			if v := s.pool.Get(); v != &b {
				t.Error(v)
			} else if v := s.pool.Get(); v != nil {
				t.Error(v)
			}
		})
	}
}
