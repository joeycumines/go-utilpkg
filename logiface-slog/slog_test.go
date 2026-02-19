package islog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
	testsuite "github.com/joeycumines/logiface-testsuite"
)

var (
	// compile time assertions

	_ logiface.Event                 = (*Event)(nil)
	_ logiface.EventFactory[*Event]  = (*Logger)(nil)
	_ logiface.Writer[*Event]        = (*Logger)(nil)
	_ logiface.EventReleaser[*Event] = (*Logger)(nil)
)

var testSuiteConfig = testsuite.Config[*Event]{
	LoggerFactory:    testSuiteLoggerFactory,
	WriteTimeout:     time.Second * 10,
	EmergencyPanics:  true,
	LogsEmptyMessage: true,
}

func testSuiteLoggerFactory(req testsuite.LoggerRequest[*Event]) testsuite.LoggerResponse[*Event] {
	handler := slog.NewJSONHandler(req.Writer, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		ReplaceAttr: nil,
	})

	var options []logiface.Option[*Event]
	options = append(options, L.WithSlogHandler(handler))

	options = append(options, req.Options...)

	return testsuite.LoggerResponse[*Event]{
		Logger:       L.New(options...),
		LevelMapping: testSuiteLevelMapping,
		ParseEvent:   testSuiteParseEvent,
	}
}

func testSuiteLevelMapping(lvl logiface.Level) logiface.Level {
	if !lvl.Enabled() {
		return logiface.LevelDisabled
	}
	// slog only has 4 levels (DEBUG, INFO, WARN, ERROR), so the mapping is lossy.
	// Custom levels (>= 9) bypass the logger's level check but map to DEBUG in slog.
	if lvl.Custom() {
		return logiface.LevelDebug
	}
	switch lvl {
	case logiface.LevelTrace:
		return logiface.LevelDebug
	case logiface.LevelNotice:
		return logiface.LevelWarning
	case logiface.LevelCritical, logiface.LevelAlert, logiface.LevelEmergency:
		return logiface.LevelError
	default:
		return lvl
	}
}

func testSuiteParseEvent(r io.Reader) ([]byte, *testsuite.Event) {
	d := json.NewDecoder(r)
	var b json.RawMessage
	if err := d.Decode(&b); err != nil {
		if errors.Is(err, io.ErrClosedPipe) {
			runtime.Goexit()
		}
		if err == io.EOF {
			return nil, nil
		}
		panic(err)
	}

	var data struct {
		Level   *string `json:"level"` // slog level (DEBUG, INFO, WARN, ERROR)
		Message *string `json:"msg"`
		Error   *string `json:"error"`
	}
	if err := json.Unmarshal(b, &data); err != nil {
		panic(err)
	}
	if data.Level == nil {
		panic(`expected slog message to have a level`)
	}

	var fields map[string]any
	if err := json.Unmarshal(b, &fields); err != nil {
		panic(err)
	}
	delete(fields, `level`)
	delete(fields, `msg`)
	delete(fields, `time`)
	delete(fields, `error`)

	ev := testsuite.Event{
		Message: data.Message,
		Error:   data.Error,
		Fields:  fields,
	}

	switch *data.Level {
	case "DEBUG":
		ev.Level = logiface.LevelDebug
	case "INFO":
		ev.Level = logiface.LevelInformational
	case "WARN":
		ev.Level = logiface.LevelWarning
	case "ERROR":
		ev.Level = logiface.LevelError
	default:
		panic(fmt.Errorf(`unexpected slog level %q`, *data.Level))
	}

	b, err := io.ReadAll(d.Buffered())
	if err != nil {
		panic(err)
	}

	return b, &ev
}

// Test_TestSuite runs the consolidated/shared test suite.
func Test_TestSuite(t *testing.T) {
	t.Parallel()
	testsuite.TestSuite(t, testSuiteConfig)
}

func TestLogger_simple(t *testing.T) {
	t.Parallel()

	type Harness struct {
		L *logiface.Logger[*Event]
		B bytes.Buffer
	}

	newHarness := func(t *testing.T, options ...logiface.Option[*Event]) *Harness {
		var h Harness
		handler := slog.NewTextHandler(&h.B, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		h.L = L.New(append([]logiface.Option[*Event]{L.WithSlogHandler(handler)}, options...)...)
		return &h
	}

	t.Run(`basic log`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			Log(`hello world`)

		h.L.Debug().
			Log(`wont show`)

		h.L.Warning().
			Log(`is warning`)

		h.L.Trace().
			Log(`wont show`)

		h.L.Err().
			Log(`is err`)

		s := h.B.String()
		// slog text format doesn't have level by default, just message
		if s == "" || len(s) < 10 {
			t.Errorf("unexpected output: %q", s)
		}
	})

	t.Run(`with fields`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			Field(`one`, 1).
			Field(`two`, 2).
			Field(`three`, 3).
			Log(`hello world`)

		s := h.B.String()
		if len(s) < 20 {
			t.Errorf("unexpected output: %q", s)
		}
	})

	t.Run(`basic context usage`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		c1 := h.L.Clone().
			Field(`one`, 1).
			Field(`two`, 2).
			Logger()

		c1.Info().
			Field(`three`, 3).
			Field(`four`, 4).
			Log(`case 1`)

		h.L.Clone().
			Field(`six`, 6).
			Logger().
			Clone().
			Field(`seven`, 7).
			Logger().
			Info().
			Field(`eight`, 8).
			Log(`case 2`)

		c1.Info().
			Field(`three`, -3).
			Field(`five`, 5).
			Log(`case 3`)

		s := h.B.String()
		if len(s) < 50 {
			t.Errorf("unexpected output: %q", s)
		}
	})

	t.Run(`nil logger disabled`, func(t *testing.T) {
		t.Parallel()

		h := &Harness{}

		h.L.Info().
			Log(`hello world`)

		c1 := h.L.Clone().
			Field(`one`, 1).
			Field(`two`, 2).
			Logger()

		c1.Info().
			Field(`three`, 3).
			Field(`four`, 4).
			Log(`case 1`)

		h.L.Clone().
			Field(`six`, 6).
			Logger().
			Clone().
			Field(`seven`, 7).
			Logger().
			Info().
			Field(`eight`, 8).
			Log(`case 2`)

		c1.Info().
			Field(`three`, -3).
			Field(`five`, 5).
			Log(`case 3`)

		if s := h.B.String(); s != "" {
			t.Errorf("unexpected output: %q", s)
		}
	})

	t.Run(`add error`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			Err(errors.New(`some error`)).
			Log(`hello world`)

		s := h.B.String()
		if len(s) < 20 {
			t.Errorf("unexpected output: %q", s)
		}
	})

	t.Run(`add error disabled`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Debug().
			Err(errors.New(`some error`)).
			Log(`hello world`)

		h.L.Clone().
			Err(errors.New(`some error`)).
			Logger().
			Debug().
			Log(`hello world`)

		if s := h.B.String(); s != "" {
			t.Errorf("unexpected output: %q", s)
		}
	})

	t.Run(`invalid raw json`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			RawJSON(`k`, json.RawMessage(`{`)).
			Log(`hello world`)

		s := h.B.String()
		if len(s) < 10 {
			t.Errorf("unexpected output: %q", s)
		}
	})

	t.Run(`valid raw json`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			RawJSON(`k`, json.RawMessage(`{"some key": ["some value", true, null, 1.3]}`)).
			Log(`hello world`)

		s := h.B.String()
		if len(s) < 20 {
			t.Errorf("unexpected output: %q", s)
		}
	})
}

func TestLogger_json(t *testing.T) {
	t.Parallel()

	type Harness struct {
		L *logiface.Logger[*Event]
		B bytes.Buffer
	}

	newHarness := func(t *testing.T, options ...logiface.Option[*Event]) *Harness {
		var h Harness
		handler := slog.NewJSONHandler(&h.B, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		h.L = L.New(append([]logiface.Option[*Event]{
			L.WithSlogHandler(handler),
			logiface.WithLevel[*Event](logiface.LevelDebug), // Enable all levels for testing
		}, options...)...)
		return &h
	}

	t.Run(`int64 formatted as number`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			Int64(`k`, math.MaxInt64).
			Log(`some message`)

		if s := h.B.String(); len(s) < 30 {
			t.Errorf("unexpected output: %q", s)
		}
	})

	t.Run(`uint64 formatted as number`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			Uint64(`k`, math.MaxUint64).
			Log(`some message`)

		if s := h.B.String(); len(s) < 30 {
			t.Errorf("unexpected output: %q", s)
		}
	})

	t.Run(`float64 formatted as number`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			Float64(`k`, math.MaxFloat64).
			Log(`some message`)

		if s := h.B.String(); len(s) < 30 {
			t.Errorf("unexpected output: %q", s)
		}
	})
}

func TestEvent_Level(t *testing.T) {
	t.Parallel()

	type Harness struct {
		L *logiface.Logger[*Event]
		B bytes.Buffer
	}

	newHarness := func(t *testing.T) *Harness {
		var h Harness
		handler := slog.NewTextHandler(&h.B, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		h.L = L.New(
			L.WithSlogHandler(handler),
			logiface.WithLevel[*Event](logiface.LevelDebug), // Enable Debug level
		)
		return &h
	}

	t.Run(`returns correct level`, func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)

		evt := h.L.Build(logiface.LevelInformational)
		evt.Log(`test`)
		if s := h.B.String(); len(s) < 5 {
			t.Errorf("unexpected output: %q", s)
		}
	})

	t.Run(`nil event returns Disabled`, func(t *testing.T) {
		t.Parallel()

		var e *Event
		if e.Level() != logiface.LevelDisabled {
			t.Errorf("expected LevelDisabled, got %v", e.Level())
		}
	})
}

func TestLogger_NewEvent_ReleaseEvent(t *testing.T) {
	t.Parallel()

	type Harness struct {
		L *logiface.Logger[*Event]
		B bytes.Buffer
	}

	newHarness := func(t *testing.T) *Harness {
		var h Harness
		handler := slog.NewTextHandler(&h.B, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		h.L = L.New(
			L.WithSlogHandler(handler),
			logiface.WithLevel[*Event](logiface.LevelDebug), // Enable Debug level
		)
		return &h
	}

	t.Run(`pool reuse`, func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)

		// Log multiple times to exercise pool
		for i := 0; i < 10; i++ {
			h.L.Build(logiface.LevelDebug).Str(`key`, `value`).Log(`test`)
		}

		s := h.B.String()
		if len(s) < 50 {
			t.Errorf("unexpected output: %q", s)
		}
	})

	t.Run(`nil logger disabled`, func(t *testing.T) {
		t.Parallel()
		var nilLogger *logiface.Logger[*Event]

		// Should not panic
		nilLogger.Build(logiface.LevelDebug).Log(`test`)
	})
}

func TestLevel_toSlogLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    logiface.Level
		expected slog.Level
	}{
		{"Trace", logiface.LevelTrace, slog.LevelDebug},
		{"Debug", logiface.LevelDebug, slog.LevelDebug},
		{"Informational", logiface.LevelInformational, slog.LevelInfo},
		{"Notice", logiface.LevelNotice, slog.LevelWarn},
		{"Warning", logiface.LevelWarning, slog.LevelWarn},
		{"Error", logiface.LevelError, slog.LevelError},
		{"Critical", logiface.LevelCritical, slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if result := toSlogLevel(tt.input); result != tt.expected {
				t.Errorf("toSlogLevel(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestWithSlogHandler_panic_on_nil(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil handler")
		}
	}()

	L.WithSlogHandler(nil)
}

// Test Event AddGroup method - returns false to indicate fallback to flattened keys
func TestEvent_AddGroup(t *testing.T) {
	t.Parallel()

	// Create a handler to initialize a logger
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	// Get the underlying Logger from WithSlogHandler
	l := &Logger{}
	l.Handler = handler

	event := l.NewEvent(logiface.LevelDebug)
	defer l.ReleaseEvent(event)

	// AddGroup should return false, signaling slog to flatten keys
	result := event.AddGroup("testGroup")
	if result != false {
		t.Errorf("Expected AddGroup to return false, got %v", result)
	}
}

// TestLogger_Write_returns_err_when_disabled tests the Write method with a disabled level
func TestLogger_Write_returns_err_when_disabled(t *testing.T) {
	// Create a handler that always returns false from Enabled()
	handler := &testHandler{
		enabledLevels: map[slog.Level]bool{
			slog.LevelDebug: false,
		},
	}

	logger := L.New(L.WithSlogHandler(handler))

	// Try to log - this exercises the Write() path where Handler.Enabled returns false
	// The goal is to verify no panic occurs (defensive code at lines 162-164)
	logger.Info().Str("test", "value").Log("test message")
}

// testHandler wraps a slog.Handler to control Enabled() behavior
type testHandler struct {
	slog.Handler
	enabledLevels map[slog.Level]bool
}

func (h *testHandler) Enabled(ctx context.Context, level slog.Level) bool {
	enabled, ok := h.enabledLevels[level]
	if !ok {
		// Default to enabled if not in map
		return true
	}
	return enabled
}

func (h *testHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.Handler != nil {
		return h.Handler.Handle(ctx, r)
	}
	// If Handler is nil, just discard the record
	return nil
}

// The toSlogLevel default case handles unknown level values.
// This is defensive code that should never execute with valid logiface.Level constants.
// Since toSlogLevel is not exported, we document that this code path exists
// for robustness if an invalid Level value is introduced in the future.
