// Package testsuite provides a common test suite for logiface, which operates
// by means of parsing (and validating) the log output.
package testsuite

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	diff "github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/joeycumines/logiface"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

type (
	// Config models the configuration used to initialize the test suite.
	Config[E logiface.Event] struct {
		// LoggerFactory implements initialization of a logger implementation.
		LoggerFactory func(cfg LoggerRequest[E]) LoggerResponse[E]

		// WriteTimeout is the max amount of time that tests should wait for a log to be written.
		WriteTimeout time.Duration

		// AlertCallsOsExit indicates that the logger implementation calls os.Exit, e.g. when mapped to "fatal"
		// (as recommended).
		AlertCallsOsExit bool

		// EmergencyPanics indicates that the logger implementation panics, e.g. when mapped to "panic"
		// (as recommended).
		EmergencyPanics bool

		// LogsEmptyMessage indicates that the logger implementation includes the relevant message field in events
		// even when the message is empty.
		LogsEmptyMessage bool
	}

	// LoggerRequest models a request for a logger from a test.
	LoggerRequest[E logiface.Event] struct {
		// Writer that must be used
		Writer io.Writer

		// Options specified by the test
		Options []logiface.Option[E]
	}

	// LoggerResponse models a response containing a logger for a test.
	LoggerResponse[E logiface.Event] struct {
		// Logger is the configured logger
		Logger *logiface.Logger[E]

		// LevelMapping returns the expected level mapping, after being munged through the logger's native
		// level implementation / configuration (where disabled indicates it won't be logged).
		LevelMapping func(lvl logiface.Level) logiface.Level

		// ParseEvent reads and parses the next event from the stream, returning any read but unused remainder.
		// Returning a nil event indicates EOF.
		ParseEvent func(r io.Reader) (remainder []byte, event *Event)

		// FormatTime is optional, and must be used if logiface.Event.AddTime is implemented
		FormatTime func(t time.Time) any

		// FormatDuration is optional, and must be used if logiface.Event.AddDuration is implemented
		FormatDuration func(d time.Duration) any

		// FormatBase64Bytes is optional, and must be used if logiface.Event.AddBase64Bytes is implemented
		FormatBase64Bytes func(b []byte, enc *base64.Encoding) any

		// FormatInt64 is optional, and must be used if logiface.Event.AddInt64 is implemented
		FormatInt64 func(i int64) any

		// FormatUint64 is optional, and must be used if logiface.Event.AddUint64 is implemented
		FormatUint64 func(u uint64) any
	}

	// TestRequest models the input from a test case, when it requests a logger.
	TestRequest[E logiface.Event] struct {
		// Level specifies the level that the logger should be configured with.
		Level logiface.Level
	}

	// TestResponse models the configured logger etc, provided to a test case.
	TestResponse[E logiface.Event] struct {
		// Logger is the configured logger
		Logger *logiface.Logger[E]

		// LevelMapping returns the expected level mapping, after being munged through the logger's native
		// level implementation / configuration (where disabled indicates it won't be logged).
		LevelMapping func(lvl logiface.Level) logiface.Level

		// Events is the output channel for log events. It is buffered, but may need to be read, for some tests
		// (no current tests require this). It is closed once the writer is closed.
		Events <-chan Event

		// SendEOF indicates that the writer has reached EOF (all logs have been written).
		// This will (indirectly) close Events.
		SendEOF func()

		// ReceiveTimeout is determined by Config.WriteTimeout
		ReceiveTimeout time.Duration

		// FormatTime is a normalizer, used by normalizeEvent.
		FormatTime func(t time.Time) any

		// FormatDuration is a normalizer, used by normalizeEvent.
		FormatDuration func(d time.Duration) any

		// FormatBase64Bytes is a normalizer, used by normalizeEvent.
		FormatBase64Bytes func(b []byte, enc *base64.Encoding) any

		// FormatInt64 is a normalizer, used by normalizeEvent.
		FormatInt64 func(i int64) any

		// FormatUint64 is a normalizer, used by normalizeEvent.
		FormatUint64 func(u uint64) any
	}

	// Event models a parsed log event
	Event struct {
		Level   logiface.Level
		Message *string
		Error   *string
		// Fields is a map of field names to values, and must be the same format encoding/json.Unmarshal.
		// It must include all fields added via the logiface.Event interface, except Message and Error.
		// It must not include any additional fields (e.g. added by the logger, such as timestamp).
		Fields map[string]interface{}
	}

	// base64BytesField models args to the logiface.Event.AddBase64 method.
	base64BytesField struct {
		Data []byte
		Enc  *base64.Encoding
	}
)

// TestSuite runs the test suite, using the provided configuration.
func TestSuite[E logiface.Event](t *testing.T, cfg Config[E]) {
	t.Run(`TestLevelMethods`, func(t *testing.T) {
		t.Parallel()
		TestLevelMethods[E](t, cfg)
	})
	t.Run(`TestLoggerLogMethod`, func(t *testing.T) {
		t.Parallel()
		TestLoggerLogMethod[E](t, cfg)
	})
	t.Run(`TestParallel`, func(t *testing.T) {
		t.Parallel()
		TestParallel[E](t, cfg)
	})
}

// RunTest initializes a logger, providing it to the test func.
// The logger will be torn down automatically, after the test func returns.
func (x Config[E]) RunTest(req TestRequest[E], test func(res TestResponse[E])) {
	var options []logiface.Option[E]

	options = append(options, logiface.WithLevel[E](req.Level))

	// the logger writes to a pipe
	r, w := io.Pipe()
	defer r.Close()

	// with events available via a channel
	events := make(chan Event, 1<<10)

	l := x.LoggerFactory(LoggerRequest[E]{
		Writer:  w,
		Options: options,
	})

	// the pipe is read, to facilitate assertion of events + avoid blocking
	done := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		defer close(done)
		defer close(events)
		defer r.Close()

		var (
			b  []byte
			br bytes.Reader
		)

		for {
			remainder, event := l.ParseEvent(io.MultiReader(&br, r))
			if event == nil {
				// EOF
				return
			}

			// send the event...
			select {
			case <-stop:
				return
			case events <- *event:
			}

			// update the buffer...
			switch {
			case len(remainder) == 0:
				// nothing to do
			case br.Len() > 0:
				// append to remainder
				remainder = append(remainder, b[len(b)-br.Len():]...)
				fallthrough
			default:
				// use remainder as next buffer
				b = remainder
				br.Reset(b)
			}
		}
	}()

	test(TestResponse[E]{
		Logger:            l.Logger,
		LevelMapping:      l.LevelMapping,
		Events:            events,
		SendEOF:           func() { _ = w.Close() },
		ReceiveTimeout:    x.WriteTimeout,
		FormatTime:        l.FormatTime,
		FormatDuration:    l.FormatDuration,
		FormatBase64Bytes: l.FormatBase64Bytes,
		FormatInt64:       l.FormatInt64,
		FormatUint64:      l.FormatUint64,
	})

	// stop the reader / parser
	_ = w.Close()
	close(stop)
	<-done
}

func (x Config[E]) HandleEmergencyPanic(t *testing.T, fn func()) {
	var ok bool
	if x.EmergencyPanics {
		defer func() {
			if ok {
				t.Fatal(`expected panic`)
			}
			recover()
		}()
	}
	fn()
	ok = true
}

// SendEOFExpectNoEvents does what it says on the tin, calling SendEOF, then
// receiving from the Events channel, asserting that it is closed, returning
// false if it is not (if t is nil - if it's non-nil it will call t.Fatal).
//
// WARNING: This may not be safe to defer, as exceptional / fatal cases could
// result in a deadlock, e.g. if the Events buffer is full.
func (x TestResponse[E]) SendEOFExpectNoEvents(t *testing.T) bool {
	if t != nil {
		t.Helper()
	}
	x.SendEOF()
	timer := time.NewTimer(x.ReceiveTimeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		if t != nil {
			t.Fatal(`timed out waiting for events to be closed`)
		}
		return false
	case ev, ok := <-x.Events:
		if ok {
			if t != nil {
				t.Fatalf(`unexpected event: %v`, ev)
			}
			return false
		}
	}
	return true
}

func (x TestResponse[E]) ReceiveEvent() (ev Event, ok bool) {
	timer := time.NewTimer(x.ReceiveTimeout)
	defer timer.Stop()
	select {
	case <-timer.C:
	case ev, ok = <-x.Events:
	}
	return
}

func (x Event) String() string {
	var b strings.Builder
	var args []interface{}
	b.WriteString(`level=%s`)
	args = append(args, x.Level)
	if x.Message != nil {
		b.WriteString(` message=%q`)
		args = append(args, *x.Message)
	}
	if x.Error != nil {
		b.WriteString(` error=%q`)
		args = append(args, *x.Error)
	}
	if len(x.Fields) != 0 {
		b.WriteString(` fields=%s`)
		if b, err := json.Marshal(x.Fields); err != nil {
			args = append(args, err)
		} else {
			args = append(args, b)
		}
	}
	return fmt.Sprintf(b.String(), args...)
}

func (x Event) Equal(ev Event) bool {
	if x.Level != ev.Level {
		return false
	}

	if x.Message != nil {
		if ev.Message == nil {
			return false
		}
		if *x.Message != *ev.Message {
			return false
		}
	} else if ev.Message != nil {
		return false
	}

	if x.Error != nil {
		if ev.Error == nil {
			return false
		}
		if *x.Error != *ev.Error {
			return false
		}
	} else if ev.Error != nil {
		return false
	}

	if len(x.Fields) != len(ev.Fields) {
		return false
	}

	for k, v1 := range x.Fields {
		v2, ok := ev.Fields[k]
		if !ok {
			return false
		}
		if !reflect.DeepEqual(v1, v2) {
			return false
		}
	}

	return true
}

func EventsEqual(a, b []Event) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}

func EventsDiff(expected, actual []Event) string {
	var s strings.Builder
	for _, ev := range expected {
		s.WriteString(ev.String())
		s.WriteByte('\n')
	}
	expectedStr := s.String()
	s.Reset()
	for _, ev := range actual {
		s.WriteString(ev.String())
		s.WriteByte('\n')
	}
	return fmt.Sprint(diff.ToUnified(`expected`, `actual`, expectedStr, myers.ComputeEdits(``, expectedStr, s.String())))
}

// normalizeEvent is used to convert templates into comparable values.
func normalizeEvent[E logiface.Event](cfg Config[E], tr TestResponse[E], ev Event) Event {
	ev.Level = tr.LevelMapping(ev.Level)
	if ev.Message == nil && cfg.LogsEmptyMessage {
		ev.Message = new(string)
	}
	var normalizeFields func(v any) any
	normalizeFields = func(v any) any {
		switch v := v.(type) {
		case []interface{}:
			for i, val := range v {
				v[i] = normalizeFields(val)
			}
		case map[string]interface{}:
			for k, val := range v {
				v[k] = normalizeFields(val)
			}
		case time.Time:
			if tr.FormatTime != nil {
				return tr.FormatTime(v)
			}
		case time.Duration:
			if tr.FormatDuration != nil {
				return tr.FormatDuration(v)
			}
		case base64BytesField:
			if tr.FormatBase64Bytes != nil {
				return tr.FormatBase64Bytes(v.Data, v.Enc)
			}
		case int64:
			if tr.FormatInt64 != nil {
				return tr.FormatInt64(v)
			}
		case uint64:
			if tr.FormatUint64 != nil {
				return tr.FormatUint64(v)
			}
		}
		return v
	}
	ev.Fields = normalizeFields(ev.Fields).(map[string]any)
	return ev
}
