package logiface

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	diff "github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"io"
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
		JSON      bool
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
	return x.level
}

func (x *mockSimpleEvent) AddField(key string, val any) {
	x.fields = append(x.fields, mockSimpleEventField{Key: key, Val: val})
}

func (x *mockSimpleWriter) Write(event *mockSimpleEvent) error {
	if x.Type && x.JSON {
		panic(`cannot use both Type and JSON`)
	}
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
		} else if x.JSON {
			b, err := json.Marshal(field.Val)
			if err == nil {
				var v any
				err = json.Unmarshal(b, &v)
				if err == nil {
					b, err = sortedJSONMarshal(v)
				}
			}
			if err != nil {
				panic(err)
			}
			field.Val = string(b)
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

func (x *mockComplexEvent) AddRawJSON(key string, val json.RawMessage) bool {
	x.FieldValues = append(x.FieldValues, mockComplexEventField{Type: `AddRawJSON`, Key: key, Value: val})
	return true
}

func (x *mockComplexEvent) mustEmbedUnimplementedEvent() {}

func (x *mockComplexWriter) Write(event *mockComplexEvent) error {
	x.events = append(x.events, event)
	return nil
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
