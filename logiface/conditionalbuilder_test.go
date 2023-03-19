package logiface

import (
	"math"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

var (
	// compile time assertions

	_ ConditionalBuilder[Event] = (*enabledBuilder[Event])(nil)
	_ ConditionalBuilder[Event] = (*disabledBuilder[Event])(nil)
	_ ConditionalBuilder[Event] = (*terminatedBuilder[Event])(nil)
)

func TestDisabledBuilder_nil(t *testing.T) {
	testDisabledBuilder(t, nil)
}

func TestDisabledBuilder_nonNil(t *testing.T) {
	for lvl := math.MinInt8; lvl <= math.MaxInt8; lvl++ {
		b := &Builder[*mockComplexEvent]{
			Event:  &mockComplexEvent{LevelValue: Level(lvl)},
			shared: &loggerShared[*mockComplexEvent]{},
		}
		testDisabledBuilder(t, b)
		if t.Failed() {
			t.Fatalf(`failed on level: %d`, lvl)
		}
	}
}

func testDisabledBuilder(t *testing.T, b *Builder[*mockComplexEvent]) {
	c := ConditionalBuilder[*mockComplexEvent]((*disabledBuilder[*mockComplexEvent])(b))
	fluentCallerTemplate(c)
	c.Call(nil)
	c.Call(func(b *Builder[*mockComplexEvent]) {
		t.Error()
		panic(`shouldnt`)
	})
	if c.Enabled() {
		t.Error()
	}
	if v := c.Builder(); v != b {
		t.Error(v)
	}
	c.isConditionalBuilder()
	if v := c.Else(); v != (*enabledBuilder[*mockComplexEvent])(b) {
		t.Error(v)
	}
	testConditionalBuilderElseIfMethods(t, c, false)
}

func TestTerminatedBuilder_nil(t *testing.T) {
	testTerminatedBuilder(t, nil)
}

func TestTerminatedBuilder_nonNil(t *testing.T) {
	for lvl := math.MinInt8; lvl <= math.MaxInt8; lvl++ {
		b := &Builder[*mockComplexEvent]{
			Event:  &mockComplexEvent{LevelValue: Level(lvl)},
			shared: &loggerShared[*mockComplexEvent]{},
		}
		testTerminatedBuilder(t, b)
		if t.Failed() {
			t.Fatalf(`failed on level: %d`, lvl)
		}
	}
}

func testTerminatedBuilder(t *testing.T, b *Builder[*mockComplexEvent]) {
	c := ConditionalBuilder[*mockComplexEvent]((*terminatedBuilder[*mockComplexEvent])(b))
	fluentCallerTemplate(c)
	c.Call(nil)
	c.Call(func(b *Builder[*mockComplexEvent]) {
		t.Error()
		panic(`shouldnt`)
	})
	if c.Enabled() {
		t.Error()
	}
	if v := c.Builder(); v != b {
		t.Error(v)
	}
	c.isConditionalBuilder()
	if v := c.Else(); v != c {
		t.Error(v)
	} else if v != (*terminatedBuilder[*mockComplexEvent])(b) {
		t.Error(v)
	}
	testConditionalBuilderElseIfMethods(t, c, true)
}

func TestEnabledBuilder_nil(t *testing.T) {
	testEnabledBuilder(t, nil)
}

func TestEnabledBuilder_nonNil(t *testing.T) {
	for lvl := math.MinInt8; lvl <= math.MaxInt8; lvl++ {
		b := &Builder[*mockComplexEvent]{
			Event:  &mockComplexEvent{LevelValue: Level(lvl)},
			shared: &loggerShared[*mockComplexEvent]{},
		}
		testEnabledBuilder(t, b)
		if t.Failed() {
			t.Fatalf(`failed on level: %d`, lvl)
		}
	}
}

func testEnabledBuilder(t *testing.T, b *Builder[*mockComplexEvent]) {
	c := ConditionalBuilder[*mockComplexEvent]((*enabledBuilder[*mockComplexEvent])(b))
	var calls int32
	c.Call(func(b2 *Builder[*mockComplexEvent]) {
		if b2 != b {
			t.Error(b2, b)
		}
		atomic.AddInt32(&calls, 1)
	})
	switch atomic.LoadInt32(&calls) {
	case 0:
		if b != nil {
			t.Error()
		}
	case 1:
		if b == nil {
			t.Error()
		}
	default:
		t.Error()
	}
	if !c.Enabled() {
		t.Error()
	}
	if v := c.Builder(); v != b {
		t.Error(v)
	}
	c.isConditionalBuilder()
	if v := c.Else(); v != (*terminatedBuilder[*mockComplexEvent])(b) {
		t.Error(v)
	}
	testConditionalBuilderElseIfMethods(t, c, true)
}

func testConditionalBuilderElseIfMethods(t *testing.T, c ConditionalBuilder[*mockComplexEvent], terminal bool) {
	b := c.Builder()
	test := func(v ConditionalBuilder[*mockComplexEvent], baseline ConditionalBuilder[*mockComplexEvent]) {
		t.Helper()
		if terminal {
			if v != (*terminatedBuilder[*mockComplexEvent])(b) {
				t.Errorf(`terminal: %T %v`, v, v)
			}
		} else if v != baseline {
			t.Errorf(`baseline: %T %v`, v, v)
		}
	}
	test(c.Else(), (*enabledBuilder[*mockComplexEvent])(b))
	test(c.ElseIf(true), b.If(true))
	test(c.ElseIf(false), b.If(false))
	test(c.ElseIfFunc(nil), b.IfFunc(nil))
	test(c.ElseIfFunc(func() bool { return true }), b.IfFunc(func() bool { return true }))
	test(c.ElseIfFunc(func() bool { return false }), b.IfFunc(func() bool { return false }))
	for lvl := math.MinInt8; lvl <= math.MaxInt8; lvl++ {
		test(c.ElseIfLevel(Level(lvl)), b.IfLevel(Level(lvl)))
	}
	test(c.ElseIfEmerg(), b.IfEmerg())
	test(c.ElseIfAlert(), b.IfAlert())
	test(c.ElseIfCrit(), b.IfCrit())
	test(c.ElseIfErr(), b.IfErr())
	test(c.ElseIfWarning(), b.IfWarning())
	test(c.ElseIfNotice(), b.IfNotice())
	test(c.ElseIfInfo(), b.IfInfo())
	test(c.ElseIfDebug(), b.IfDebug())
	test(c.ElseIfTrace(), b.IfTrace())
}

func TestBuilder_If_nil(t *testing.T) {
	t.Run(`cond=true`, func(t *testing.T) {
		if v := (*Builder[*mockComplexEvent])(nil).If(true); v != (*disabledBuilder[*mockComplexEvent])(nil) {
			t.Error(v)
		}
	})
	t.Run(`cond=false`, func(t *testing.T) {
		if v := (*Builder[*mockComplexEvent])(nil).If(false); v != (*disabledBuilder[*mockComplexEvent])(nil) {
			t.Error(v)
		}
	})
}

func TestBuilder_IfFunc_nil(t *testing.T) {
	t.Run(`cond=nil`, func(t *testing.T) {
		if v := (*Builder[*mockComplexEvent])(nil).IfFunc(nil); v != (*disabledBuilder[*mockComplexEvent])(nil) {
			t.Error(v)
		}
	})
	t.Run(`cond=true`, func(t *testing.T) {
		if v := (*Builder[*mockComplexEvent])(nil).IfFunc(func() bool { return true }); v != (*disabledBuilder[*mockComplexEvent])(nil) {
			t.Error(v)
		}
	})
	t.Run(`cond=false`, func(t *testing.T) {
		if v := (*Builder[*mockComplexEvent])(nil).IfFunc(func() bool { return false }); v != (*disabledBuilder[*mockComplexEvent])(nil) {
			t.Error(v)
		}
	})
}

func TestBuilder_IfLevel_nil(t *testing.T) {
	t.Run(`info`, func(t *testing.T) {
		if v := (*Builder[*mockComplexEvent])(nil).IfLevel(LevelInformational); v != (*disabledBuilder[*mockComplexEvent])(nil) {
			t.Error(v)
		}
	})
	t.Run(`emerg`, func(t *testing.T) {
		if v := (*Builder[*mockComplexEvent])(nil).IfLevel(LevelEmergency); v != (*disabledBuilder[*mockComplexEvent])(nil) {
			t.Error(v)
		}
	})
}

func TestBuilder_IfEmerg_nil(t *testing.T) {
	if v := (*Builder[*mockComplexEvent])(nil).IfEmerg(); v != (*disabledBuilder[*mockComplexEvent])(nil) {
		t.Error(v)
	}
}

func TestBuilder_IfAlert_nil(t *testing.T) {
	if v := (*Builder[*mockComplexEvent])(nil).IfAlert(); v != (*disabledBuilder[*mockComplexEvent])(nil) {
		t.Error(v)
	}
}

func TestBuilder_IfCrit_nil(t *testing.T) {
	if v := (*Builder[*mockComplexEvent])(nil).IfCrit(); v != (*disabledBuilder[*mockComplexEvent])(nil) {
		t.Error(v)
	}
}

func TestBuilder_IfErr_nil(t *testing.T) {
	if v := (*Builder[*mockComplexEvent])(nil).IfErr(); v != (*disabledBuilder[*mockComplexEvent])(nil) {
		t.Error(v)
	}
}

func TestBuilder_IfWarning_nil(t *testing.T) {
	if v := (*Builder[*mockComplexEvent])(nil).IfWarning(); v != (*disabledBuilder[*mockComplexEvent])(nil) {
		t.Error(v)
	}
}

func TestBuilder_IfNotice_nil(t *testing.T) {
	if v := (*Builder[*mockComplexEvent])(nil).IfNotice(); v != (*disabledBuilder[*mockComplexEvent])(nil) {
		t.Error(v)
	}
}

func TestBuilder_IfInfo_nil(t *testing.T) {
	if v := (*Builder[*mockComplexEvent])(nil).IfInfo(); v != (*disabledBuilder[*mockComplexEvent])(nil) {
		t.Error(v)
	}
}

func TestBuilder_IfDebug_nil(t *testing.T) {
	if v := (*Builder[*mockComplexEvent])(nil).IfDebug(); v != (*disabledBuilder[*mockComplexEvent])(nil) {
		t.Error(v)
	}
}

func TestBuilder_IfTrace_nil(t *testing.T) {
	if v := (*Builder[*mockComplexEvent])(nil).IfTrace(); v != (*disabledBuilder[*mockComplexEvent])(nil) {
		t.Error(v)
	}
}

func TestBuilder_If(t *testing.T) {
	b := &Builder[*mockComplexEvent]{
		Event:  &mockComplexEvent{LevelValue: LevelInformational},
		shared: &loggerShared[*mockComplexEvent]{},
	}
	t.Run(`cond=true`, func(t *testing.T) {
		if v := b.If(true); v != (*enabledBuilder[*mockComplexEvent])(b) {
			t.Error(v)
		}
	})
	t.Run(`cond=false`, func(t *testing.T) {
		if v := b.If(false); v != (*disabledBuilder[*mockComplexEvent])(b) {
			t.Error(v)
		}
	})
}

func TestBuilder_IfFunc(t *testing.T) {
	b := &Builder[*mockComplexEvent]{
		Event:  &mockComplexEvent{LevelValue: LevelInformational},
		shared: &loggerShared[*mockComplexEvent]{},
	}
	t.Run(`cond=nil`, func(t *testing.T) {
		if v := b.IfFunc(nil); v != (*disabledBuilder[*mockComplexEvent])(b) {
			t.Error(v)
		}
	})
	t.Run(`cond=true`, func(t *testing.T) {
		if v := b.IfFunc(func() bool { return true }); v != (*enabledBuilder[*mockComplexEvent])(b) {
			t.Error(v)
		}
	})
	t.Run(`cond=false`, func(t *testing.T) {
		if v := b.IfFunc(func() bool { return false }); v != (*disabledBuilder[*mockComplexEvent])(b) {
			t.Error(v)
		}
	})
}

func TestBuilder_IfLevel(t *testing.T) {
	for _, conditionLevel := range [...]struct {
		Level  Level
		Method func(b *Builder[*mockComplexEvent]) ConditionalBuilder[*mockComplexEvent]
	}{
		{
			Level:  LevelEmergency,
			Method: (*Builder[*mockComplexEvent]).IfEmerg,
		},
		{
			Level:  LevelAlert,
			Method: (*Builder[*mockComplexEvent]).IfAlert,
		},
		{
			Level:  LevelCritical,
			Method: (*Builder[*mockComplexEvent]).IfCrit,
		},
		{
			Level:  LevelError,
			Method: (*Builder[*mockComplexEvent]).IfErr,
		},
		{
			Level:  LevelWarning,
			Method: (*Builder[*mockComplexEvent]).IfWarning,
		},
		{
			Level:  LevelNotice,
			Method: (*Builder[*mockComplexEvent]).IfNotice,
		},
		{
			Level:  LevelInformational,
			Method: (*Builder[*mockComplexEvent]).IfInfo,
		},
		{
			Level:  LevelDebug,
			Method: (*Builder[*mockComplexEvent]).IfDebug,
		},
		{
			Level:  LevelTrace,
			Method: (*Builder[*mockComplexEvent]).IfTrace,
		},
	} {
		conditionLevel := conditionLevel
		t.Run(`cond=`+conditionLevel.Level.String(), func(t *testing.T) {
			for loggerLevel := LevelDisabled; loggerLevel <= LevelTrace+1; loggerLevel++ {
				loggerLevel := loggerLevel
				t.Run(`lvl=`+loggerLevel.String(), func(t *testing.T) {
					b := &Builder[*mockComplexEvent]{
						Event: &mockComplexEvent{},
						shared: &loggerShared[*mockComplexEvent]{
							level: loggerLevel,
						},
					}
					v := conditionLevel.Method(b)
					switch {
					case loggerLevel < conditionLevel.Level:
						// logger level is less verbose than the requested level
						if v != (*disabledBuilder[*mockComplexEvent])(b) {
							t.Errorf(`%T`, v)
						}
					default:
						// logger level is at least as verbose as the requested level
						if v != (*enabledBuilder[*mockComplexEvent])(b) {
							t.Errorf(`%T`, v)
						}
					}
					if v != b.IfLevel(conditionLevel.Level) {
						t.Errorf(`%T %T`, v, b.IfLevel(conditionLevel.Level))
					}
				})
			}
		})
	}
}

func ExampleBuilder_IfTrace() {
	sharedOpts := WithOptions(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: os.Stdout}),
	)

	infoLogger := New(sharedOpts).Logger().
		Clone().
		Str(`logger`, `infoLogger`).
		Logger()

	traceLogger := New(sharedOpts, mockL.WithLevel(LevelTrace)).Logger().
		Clone().
		Str(`logger`, `traceLogger`).
		Logger()

	log := func(logger *Logger[Event]) {
		user := struct {
			ID        int64
			Name      string
			Email     string
			CreatedAt time.Time
		}{123, "John Doe", "johndoe@example.com", time.Unix(0, 1676147419539212123).UTC()}

		logger.Info().
			IfTrace().
			Any("user", user).
			Else().
			Int64("user", user.ID).
			Builder().
			Log("user created")
	}

	log(infoLogger)
	log(traceLogger)

	//output:
	//[info] logger=infoLogger user=123 msg=user created
	//[info] logger=traceLogger user={123 John Doe johndoe@example.com 2023-02-11 20:30:19.539212123 +0000 UTC} msg=user created
}

func ExampleBuilder_degreesOfLogVerbosity() {
	log := func(logger *Logger[Event]) {
		user := struct {
			ID   int
			Name string
			Role string
		}{123, "Some Guy", "admin"}

		entity := struct {
			ID     int
			Name   string
			Type   string
			Status string
		}{456, "example entity", "document", "active"}

		logger.Warning().
			Int(`user_id`, user.ID).
			Int(`entity_id`, entity.ID).
			IfTrace().
			Any(`user`, user).
			Any(`entity`, entity).
			ElseIfNotice().
			Str(`entity_type`, entity.Type).
			Str(`user_role`, user.Role).
			Builder().
			Log("access denied")
	}

	for lvl := LevelEmergency; lvl <= LevelTrace; lvl++ {
		log(New(
			mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
			mockL.WithWriter(&mockSimpleWriter{Writer: os.Stdout}),
			mockL.WithLevel(lvl),
		).Logger().Clone().
			Str(`loggerLevel`, lvl.String()).
			Logger())
	}

	//output:
	//[warning] loggerLevel=warning user_id=123 entity_id=456 msg=access denied
	//[warning] loggerLevel=notice user_id=123 entity_id=456 entity_type=document user_role=admin msg=access denied
	//[warning] loggerLevel=info user_id=123 entity_id=456 entity_type=document user_role=admin msg=access denied
	//[warning] loggerLevel=debug user_id=123 entity_id=456 entity_type=document user_role=admin msg=access denied
	//[warning] loggerLevel=trace user_id=123 entity_id=456 user={123 Some Guy admin} entity={456 example entity document active} msg=access denied
}
