package logiface

import (
	"encoding/base64"
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
		Type      bool
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
	mockL = LoggerFactory[*mockSimpleEvent]{}

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
		if x.Type {
			switch val := field.Val.(type) {
			case []any:
				s := make([]string, len(val))
				for i, v := range val {
					s[i] = fmt.Sprintf(`(%T)%v`, v, v)
				}
				field.Val = s

			default:
				if x.MultiLine {
					_, _ = fmt.Fprintf(x.Writer, "\n%s=(%T)%v", field.Key, field.Val, field.Val)
				} else {
					_, _ = fmt.Fprintf(x.Writer, ` %s=(%T)%v`, field.Key, field.Val, field.Val)
				}
				continue
			}
		}

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

func (x *mockComplexEvent) AddTime(key string, val time.Time) bool {
	x.FieldValues = append(x.FieldValues, mockComplexEventField{Type: `AddTime`, Key: key, Value: val})
	return true
}

func (x *mockComplexEvent) AddDuration(key string, val time.Duration) bool {
	x.FieldValues = append(x.FieldValues, mockComplexEventField{Type: `AddDuration`, Key: key, Value: val})
	return true
}

func (x *mockComplexEvent) AddBase64Bytes(key string, val []byte, enc *base64.Encoding) bool {
	x.FieldValues = append(x.FieldValues, mockComplexEventField{Type: `AddBase64Bytes`, Key: key, Value: enc.EncodeToString(val)})
	return true
}

func (x *mockComplexEvent) AddBool(key string, val bool) bool {
	x.FieldValues = append(x.FieldValues, mockComplexEventField{Type: `AddBool`, Key: key, Value: val})
	return true
}

func (x *mockComplexEvent) AddFloat64(key string, val float64) bool {
	x.FieldValues = append(x.FieldValues, mockComplexEventField{Type: `AddFloat64`, Key: key, Value: val})
	return true
}

func (x *mockComplexEvent) AddInt64(key string, val int64) bool {
	x.FieldValues = append(x.FieldValues, mockComplexEventField{Type: `AddInt64`, Key: key, Value: val})
	return true
}

func (x *mockComplexEvent) AddUint64(key string, val uint64) bool {
	x.FieldValues = append(x.FieldValues, mockComplexEventField{Type: `AddUint64`, Key: key, Value: val})
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
	Time(key string, t time.Time) T
	Dur(key string, d time.Duration) T
	Base64(key string, b []byte, enc *base64.Encoding) T
	Bool(key string, val bool) T
	Float64(key string, val float64) T
	Int64(key string, val int64) T
	Uint64(key string, val uint64) T
}](x T) {
	x.Err(errors.New(`err called`)).
		Field(`field called with string`, `val 2`).
		Field(`field called with bytes`, []byte(`val 3`)).
		Field(`field called with time.Time local`, time.Unix(0, 1558069640361696123)).
		Field(`field called with time.Time utc`, time.Unix(0, 1558069640361696123).UTC()).
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
		Str(`str called`, `val 6`).
		Time(`time called with local`, time.Unix(0, 1616592449876543213)).
		Time(`time called with utc`, time.Unix(0, 1583023169456789123).UTC()).
		Dur(`dur called positive`, time.Duration(51238123523458989)).
		Dur(`dur called negative`, time.Duration(-51238123523458989)).
		Dur(`dur called zero`, time.Duration(0)).
		Base64(`base64 called with nil enc`, []byte(`val 7`), nil).
		Base64(`base64 called with padding`, []byte(`val 7`), base64.StdEncoding).
		Base64(`base64 called without padding`, []byte(`val 7`), base64.RawStdEncoding).
		Bool(`bool called`, true).
		Field(`field called with bool`, true).
		Float64(`float64 called`, math.MaxFloat64).
		Field(`field called with float64`, float64(math.MaxFloat64)).
		Int64(`int64 called`, math.MaxInt64).
		Field(`field called with int64`, int64(math.MaxInt64)).
		Uint64(`uint64 called`, math.MaxUint64).
		Field(`field called with uint64`, uint64(math.MaxUint64))
}

func stringDiff(expected, actual string) string {
	return fmt.Sprint(diff.ToUnified(`expected`, `actual`, expected, myers.ComputeEdits(``, expected, actual)))
}

func newSimpleLogger(w io.Writer, multiLine bool) *Logger[*mockSimpleEvent] {
	return mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: w, MultiLine: multiLine}),
	)
}

func newSimpleLoggerPrintTypes(w io.Writer, multiLine bool) *Logger[*mockSimpleEvent] {
	return mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: w, MultiLine: multiLine, Type: true}),
	)
}
