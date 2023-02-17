package logiface

import (
	"errors"
	"fmt"
	diff "github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"io"
	"math"
	"time"
)

type (
	// mockSimpleEvent implements a minimal subset of the Event interface
	mockSimpleEvent struct {
		UnimplementedEvent
		level  Level
		fields []mockSimpleEventField
	}

	mockSimpleEventField struct {
		Key string
		Val any
	}

	mockSimpleWriter struct {
		Writer    io.Writer
		MultiLine bool
	}

	// mockComplexEvent implements the entire Event interface
	// it's used to test the positive path of the switching cases for optional methods
	mockComplexEvent struct {
		FieldValues []mockComplexEventField
		LevelValue  Level
	}

	mockComplexEventField struct {
		Type  string
		Key   string
		Value any
	}

	mockComplexWriter struct {
		events []*mockComplexEvent
	}

	mockEvent struct {
		Event
	}

	mockWriter[E Event] struct {
		Writer[E]
	}

	mockModifier[E Event] struct {
		Modifier[E]
	}

	mockIntDataType int
)

var (
	// compile time assertions
	_ EventFactoryFunc[*mockSimpleEvent]  = mockSimpleEventFactory
	_ Event                               = (*mockSimpleEvent)(nil)
	_ Writer[*mockSimpleEvent]            = (*mockSimpleWriter)(nil)
	_ EventFactoryFunc[*mockComplexEvent] = mockComplexEventFactory
	_ Event                               = (*mockComplexEvent)(nil)
	_ Writer[*mockComplexEvent]           = (*mockComplexWriter)(nil)
)

func mockSimpleEventFactory(level Level) *mockSimpleEvent {
	return &mockSimpleEvent{level: level}
}

func (x *mockSimpleEvent) Level() Level {
	if x != nil {
		return x.level
	}
	return LevelDisabled
}

func (x *mockSimpleEvent) AddField(key string, val any) {
	x.fields = append(x.fields, mockSimpleEventField{Key: key, Val: val})
}

func (x *mockSimpleWriter) Write(event *mockSimpleEvent) error {
	_, _ = fmt.Fprintf(x.Writer, `[%s]`, event.level.String())
	for _, field := range event.fields {
		if x.MultiLine {
			_, _ = fmt.Fprintf(x.Writer, "\n%s=%v", field.Key, field.Val)
		} else {
			_, _ = fmt.Fprintf(x.Writer, ` %s=%v`, field.Key, field.Val)
		}
	}
	_, _ = fmt.Fprintln(x.Writer)
	return nil
}

func mockComplexEventFactory(level Level) *mockComplexEvent {
	return &mockComplexEvent{LevelValue: level}
}

func (x *mockComplexEvent) Level() Level { return x.LevelValue }

func (x *mockComplexEvent) AddField(key string, val any) {
	x.FieldValues = append(x.FieldValues, mockComplexEventField{Type: `AddField`, Key: key, Value: val})
}

func (x *mockComplexEvent) AddMessage(msg string) bool {
	x.FieldValues = append(x.FieldValues, mockComplexEventField{Type: `AddMessage`, Value: msg})
	return true
}

func (x *mockComplexEvent) AddError(err error) bool {
	x.FieldValues = append(x.FieldValues, mockComplexEventField{Type: `AddError`, Value: err})
	return true
}

func (x *mockComplexEvent) AddString(key string, val string) bool {
	x.FieldValues = append(x.FieldValues, mockComplexEventField{Type: `AddString`, Key: key, Value: val})
	return true
}

func (x *mockComplexEvent) AddInt(key string, val int) bool {
	x.FieldValues = append(x.FieldValues, mockComplexEventField{Type: `AddInt`, Key: key, Value: val})
	return true
}

func (x *mockComplexEvent) AddFloat32(key string, val float32) bool {
	x.FieldValues = append(x.FieldValues, mockComplexEventField{Type: `AddFloat32`, Key: key, Value: val})
	return true
}

func (x *mockComplexEvent) mustEmbedUnimplementedEvent() {}

func (x *mockComplexWriter) Write(event *mockComplexEvent) error {
	x.events = append(x.events, event)
	return nil
}

// fluentCallerTemplate exercises every fluent method that's common between Builder and Context
func fluentCallerTemplate[T interface {
	Err(err error) T
	Field(key string, val any) T
	Float32(key string, val float32) T
	Int(key string, val int) T
	Interface(key string, val any) T
	Any(key string, val any) T
	Str(key string, val string) T
}](x T) {
	x.Err(errors.New(`err called`)).
		Field(`field called with string`, `val 2`).
		Field(`field called with bytes`, []byte(`val 3`)).
		Field(`field called with time.Time`, time.Unix(0, 1558069640361696123).Local()).
		Field(`field called with duration`, time.Duration(3116139280723392)).
		Field(`field called with int`, -51245).
		Field(`field called with float32`, float32(math.SmallestNonzeroFloat32)).
		Field(`field called with unhandled type`, mockIntDataType(-421)).
		Float32(`float32 called`, float32(math.MaxFloat32)).
		Int(`int called`, math.MaxInt).
		Interface(`interface called with string`, `val 4`).
		Interface(`interface called with bool`, true).
		Interface(`interface called with nil`, nil).
		Any(`any called with string`, `val 5`).
		Str(`str called`, `val 6`)
}

func stringDiff(expected, actual string) string {
	return fmt.Sprint(diff.ToUnified(`expected`, `actual`, expected, myers.ComputeEdits(``, expected, actual)))
}
