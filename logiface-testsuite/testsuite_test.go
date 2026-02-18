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
				Message: ptrString("debug message"),
				Error:   ptrString("debug error"),
				Fields:  map[string]interface{}{"num": 42},
			},
			"level=debug message=\"debug message\" error=\"debug error\" fields={\"num\":42}",
		},
		{
			`level and fields`,
			Event{
				Level:  logiface.LevelWarning,
				Fields: map[string]interface{}{"nested": map[string]interface{}{"again": map[string]interface{}{"key2": "value2"}}},
			},
			"level=warning fields={\"nested\":{\"again\":{\"key2\":\"value2\"}}}",
		},
		{
			`just a level`,
			Event{
				Level:  logiface.LevelError,
				Fields: map[string]interface{}{},
			},
			`level=err`,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			output := tc.event.String()
			if output != tc.output {
				t.Errorf("incorrect output for %#v: expected %q, got %q", tc.event, tc.output, output)
			}
		})
	}
}

func ptrString(s string) *string {
	return &s
}

func TestEventsDiff(t *testing.T) {
	a := Event{
		Level:   logiface.LevelDebug,
		Message: ptrString("debug message"),
		Error:   ptrString("debug error"),
		Fields:  map[string]interface{}{"num": 42},
	}
	b := Event{
		Level:  logiface.LevelWarning,
		Fields: map[string]interface{}{"nested": map[string]interface{}{"again": map[string]interface{}{"key2": "value2"}}},
	}
	if s := EventsDiff(
		[]Event{a, b, b, a, b, a, a},
		[]Event{b, b, a, b, b, b, a},
	); s != "--- expected\n+++ actual\n@@ -1,7 +1,7 @@\n-level=debug message=\"debug message\" error=\"debug error\" fields={\"num\":42}\n level=warning fields={\"nested\":{\"again\":{\"key2\":\"value2\"}}}\n level=warning fields={\"nested\":{\"again\":{\"key2\":\"value2\"}}}\n level=debug message=\"debug message\" error=\"debug error\" fields={\"num\":42}\n level=warning fields={\"nested\":{\"again\":{\"key2\":\"value2\"}}}\n+level=warning fields={\"nested\":{\"again\":{\"key2\":\"value2\"}}}\n+level=warning fields={\"nested\":{\"again\":{\"key2\":\"value2\"}}}\n level=debug message=\"debug message\" error=\"debug error\" fields={\"num\":42}\n-level=debug message=\"debug message\" error=\"debug error\" fields={\"num\":42}\n" {
		t.Errorf("unexpected value: %q\n%s", s, s)
	}
}
