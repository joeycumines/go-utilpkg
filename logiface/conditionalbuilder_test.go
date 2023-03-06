package logiface

import (
	"sync/atomic"
	"testing"
)

func TestDisabledBuilder_nil(t *testing.T) {
	testDisabledBuilder(t, nil)
}

func TestDisabledBuilder_nonNil(t *testing.T) {
	b := &Builder[*mockComplexEvent]{
		Event:  &mockComplexEvent{LevelValue: LevelInformational},
		shared: &loggerShared[*mockComplexEvent]{},
	}
	testDisabledBuilder(t, b)
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
	c.private()
}

func TestEnabledBuilder_nil(t *testing.T) {
	testEnabledBuilder(t, nil)
}

func TestEnabledBuilder_nonNil(t *testing.T) {
	b := &Builder[*mockComplexEvent]{
		Event:  &mockComplexEvent{LevelValue: LevelInformational},
		shared: &loggerShared[*mockComplexEvent]{},
	}
	testEnabledBuilder(t, b)
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
	c.private()
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
