package testsuite

import (
	"github.com/joeycumines/logiface"
	"testing"
)

func TestEvent_String(t *testing.T) {
	for _, tc := range []struct {
		name   string
		event  Event
		output string
	}{
		{
			`with all`,
			Event{
				Level:   logiface.LevelDebug,
				Message: new("debug message"),
				Error:   new("debug error"),
				Fields:  map[string]any{"num": 42},
			},
			"level=debug message=\"debug message\" error=\"debug error\" fields={\"num\":42}",
		},
		{
			`level and fields`,
			Event{
				Level:  logiface.LevelWarning,
				Fields: map[string]any{"nested": map[string]any{"again": map[string]any{"key2": "value2"}}},
			},
			"level=warning fields={\"nested\":{\"again\":{\"key2\":\"value2\"}}}",
		},
		{
			`just a level`,
			Event{
				Level:  logiface.LevelError,
				Fields: map[string]any{},
			},
			`level=err`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := tc.event.String()
			if output != tc.output {
				t.Errorf("incorrect output for %#v: expected %q, got %q", tc.event, tc.output, output)
			}
		})
	}
}

//go:fix inline
func ptrString(s string) *string {
	return new(s)
}

func TestEventsDiff(t *testing.T) {
	a := Event{
		Level:   logiface.LevelDebug,
		Message: new("debug message"),
		Error:   new("debug error"),
		Fields:  map[string]any{"num": 42},
	}
	b := Event{
		Level:  logiface.LevelWarning,
		Fields: map[string]any{"nested": map[string]any{"again": map[string]any{"key2": "value2"}}},
	}
	if s := EventsDiff(
		[]Event{a, b, b, a, b, a, a},
		[]Event{b, b, a, b, b, b, a},
	); s != "--- expected\n+++ actual\n@@ -1,7 +1,7 @@\n-level=debug message=\"debug message\" error=\"debug error\" fields={\"num\":42}\n level=warning fields={\"nested\":{\"again\":{\"key2\":\"value2\"}}}\n level=warning fields={\"nested\":{\"again\":{\"key2\":\"value2\"}}}\n level=debug message=\"debug message\" error=\"debug error\" fields={\"num\":42}\n level=warning fields={\"nested\":{\"again\":{\"key2\":\"value2\"}}}\n+level=warning fields={\"nested\":{\"again\":{\"key2\":\"value2\"}}}\n+level=warning fields={\"nested\":{\"again\":{\"key2\":\"value2\"}}}\n level=debug message=\"debug message\" error=\"debug error\" fields={\"num\":42}\n-level=debug message=\"debug message\" error=\"debug error\" fields={\"num\":42}\n" {
		t.Errorf("unexpected value: %q\n%s", s, s)
	}
}
