package mocklog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/joeycumines/logiface"
	"io"
	"sort"
)

type (
	Event struct {
		logiface.UnimplementedEvent
		Fields []Field
		Lvl    logiface.Level
	}

	Field struct {
		Val any
		Key string
	}

	Writer struct {
		Writer    io.Writer
		MultiLine bool
		Type      bool
		JSON      bool
	}

	Config struct {
		Writer    io.Writer
		MultiLine bool
		Type      bool
		JSON      bool
	}
)

var (
	L = logiface.LoggerFactory[*Event]{}

	// compile time assertions

	_ logiface.EventFactoryFunc[*Event] = Factory
	_ logiface.Event                    = (*Event)(nil)
	_ logiface.Writer[*Event]           = (*Writer)(nil)
)

func WithMocklog(w *Writer) logiface.Option[*Event] {
	return L.WithOptions(
		L.WithWriter(w),
		L.WithEventFactory(L.NewEventFactoryFunc(Factory)),
	)
}

func Factory(level logiface.Level) *Event {
	return &Event{Lvl: level}
}

func (x *Event) Level() logiface.Level {
	return x.Lvl
}

func (x *Event) AddField(key string, val any) {
	x.Fields = append(x.Fields, Field{Key: key, Val: val})
}

func (x *Writer) Write(event *Event) error {
	if x.Type && x.JSON {
		panic(`cannot use both Type and JSON`)
	}
	_, _ = fmt.Fprintf(x.Writer, `[%s]`, event.Lvl.String())
	for _, field := range event.Fields {
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

type jsonKeyValue struct {
	Value any
	Key   string
}

type jsonKeyValueList []jsonKeyValue

func (k jsonKeyValueList) Len() int {
	return len(k)
}

func (k jsonKeyValueList) Swap(i, j int) {
	k[i], k[j] = k[j], k[i]
}

func (k jsonKeyValueList) Less(i, j int) bool {
	return k[i].Key < k[j].Key
}

func sortKeysForJSONData(data any) any {
	switch v := data.(type) {
	case map[string]any:
		keyValuePairs := make(jsonKeyValueList, 0, len(v))
		for k, val := range v {
			keyValuePairs = append(keyValuePairs, jsonKeyValue{Key: k, Value: sortKeysForJSONData(val)})
		}
		sort.Sort(keyValuePairs)
		return keyValuePairs
	case []any:
		for i, e := range v {
			v[i] = sortKeysForJSONData(e)
		}
		return v
	default:
		return data
	}
}

func sortedKeysJSONMarshal(data any) ([]byte, error) {
	var buffer bytes.Buffer

	switch v := data.(type) {
	case jsonKeyValueList:
		buffer.WriteString("{")
		for i, kv := range v {
			if i > 0 {
				buffer.WriteString(",")
			}
			key, err := json.Marshal(kv.Key)
			if err != nil {
				return nil, err
			}
			buffer.Write(key)
			buffer.WriteString(":")
			value, err := sortedKeysJSONMarshal(kv.Value)
			if err != nil {
				return nil, err
			}
			buffer.Write(value)
		}
		buffer.WriteString("}")
	case []any:
		buffer.WriteString("[")
		for i, e := range v {
			if i > 0 {
				buffer.WriteString(",")
			}
			value, err := sortedKeysJSONMarshal(e)
			if err != nil {
				return nil, err
			}
			buffer.Write(value)
		}
		buffer.WriteString("]")

	default:
		return json.Marshal(data)
	}

	return buffer.Bytes(), nil
}

func sortedJSONMarshal(data any) ([]byte, error) {
	return sortedKeysJSONMarshal(sortKeysForJSONData(data))
}
