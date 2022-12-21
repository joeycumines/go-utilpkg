package logiface

import (
	"fmt"
	"io"
	"strings"
)

type (
	SimpleEvent struct {
		UnimplementedEvent
		level  Level
		msg    string
		fields []SimpleEventField
	}

	SimpleEventField struct {
		Key string
		Val any
	}

	SimpleWriter struct {
		Writer io.Writer
	}
)

var (
	// compile time assertions
	_ Event                = (*SimpleEvent)(nil)
	_ Writer[*SimpleEvent] = (*SimpleWriter)(nil)
)

func SimpleEventFactory(level Level) *SimpleEvent {
	return &SimpleEvent{level: level}
}

func (x *SimpleEvent) Level() Level {
	if x != nil {
		return x.level
	}
	return LevelDisabled
}

func (x *SimpleEvent) AddMessage(msg string) bool {
	x.msg = msg
	return true
}

func (x *SimpleEvent) AddField(key string, val any) {
	x.fields = append(x.fields, SimpleEventField{Key: key, Val: val})
}

func (x *SimpleWriter) Write(event *SimpleEvent) error {
	_, _ = fmt.Fprintf(x.Writer, `[%s]`, event.level.String())
	for _, field := range event.fields {
		_, _ = fmt.Fprintf(x.Writer, ` %s=%v`, field.Key, field.Val)
	}
	if event.msg != `` {
		_, _ = fmt.Fprintf(x.Writer, " %s\n", strings.ReplaceAll(event.msg, "\n", `\n`))
	}
	return nil
}
