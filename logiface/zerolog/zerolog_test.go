package zerolog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/joeycumines/go-utilpkg/logiface"
	"github.com/joeycumines/go-utilpkg/logiface/testsuite"
	"github.com/rs/zerolog"
	"io"
	"math"
	"os"
	"runtime"
	"testing"
	"time"
)

var testSuiteConfig = testsuite.Config[*Event]{
	LoggerFactory:    testSuiteLoggerFactory,
	WriteTimeout:     time.Second * 10,
	AlertCallsOsExit: true,
	EmergencyPanics:  true,
}

func testSuiteLoggerFactory(req testsuite.LoggerRequest[*Event]) testsuite.LoggerResponse[*Event] {
	zerolog.SetGlobalLevel(math.MinInt8)
	logger := zerolog.New(req.Writer).Level(math.MinInt8)

	var options []logiface.Option[*Event]

	options = append(options, L.WithZerolog(logger))

	options = append(options, req.Options...)

	return testsuite.LoggerResponse[*Event]{
		Logger:         L.New(options...),
		LevelMapping:   testSuiteLevelMapping,
		ParseEvent:     testSuiteParseEvent,
		FormatTime:     testSuiteFormatTime,
		FormatDuration: testSuiteFormatDuration,
		FormatInt64:    testSuiteFormatInt64,
		FormatUint64:   testSuiteFormatUint64,
	}
}

func testSuiteFormatTime(t time.Time) any {
	return t.Format(zerolog.TimeFieldFormat)
}

func testSuiteFormatDuration(d time.Duration) any {
	if zerolog.DurationFieldInteger {
		return float64(d / zerolog.DurationFieldUnit)
	}
	val := float64(d) / float64(zerolog.DurationFieldUnit)
	switch {
	case math.IsNaN(val):
		return `NaN`
	case math.IsInf(val, 1):
		return `+Inf`
	case math.IsInf(val, -1):
		return `-Inf`
	}
	return val
}

func testSuiteFormatInt64(v int64) any {
	return float64(v)
}

func testSuiteFormatUint64(v uint64) any {
	return float64(v)
}

func testSuiteLevelMapping(lvl logiface.Level) logiface.Level {
	if !lvl.Enabled() {
		return logiface.LevelDisabled
	}
	switch lvl {
	case logiface.LevelNotice:
		return logiface.LevelWarning
	case logiface.LevelCritical:
		return logiface.LevelError
	default:
		return lvl
	}
}

func testSuiteParseEvent(r io.Reader) ([]byte, *testsuite.Event) {
	d := json.NewDecoder(r)
	var b json.RawMessage
	if err := d.Decode(&b); err != nil {
		if err == io.EOF {
			return nil, nil
		}
		if errors.Is(err, io.ErrClosedPipe) {
			runtime.Goexit()
		}
		panic(err)
	}

	var data struct {
		Level   *zerolog.Level `json:"level"`
		Message *string        `json:"message"`
		Error   *string        `json:"error"`
	}
	if err := json.Unmarshal(b, &data); err != nil {
		panic(err)
	}
	if data.Level == nil {
		panic(`expected zerolog message to have a level`)
	}

	var fields map[string]interface{}
	if err := json.Unmarshal(b, &fields); err != nil {
		panic(err)
	}
	delete(fields, `level`)
	delete(fields, `message`)
	delete(fields, `error`)

	ev := testsuite.Event{
		Message: data.Message,
		Error:   data.Error,
		Fields:  fields,
	}

	switch *data.Level {
	case zerolog.TraceLevel:
		ev.Level = logiface.LevelTrace

	case zerolog.DebugLevel:
		ev.Level = logiface.LevelDebug

	case zerolog.InfoLevel:
		ev.Level = logiface.LevelInformational

	case zerolog.WarnLevel:
		ev.Level = logiface.LevelWarning

	case zerolog.ErrorLevel:
		ev.Level = logiface.LevelError

	case zerolog.FatalLevel:
		ev.Level = logiface.LevelAlert

	case zerolog.PanicLevel:
		ev.Level = logiface.LevelEmergency

	default:
		if *data.Level < -1 {
			// custom level...
			if lvl := -int(*data.Level) + 7; lvl <= math.MaxInt8 {
				ev.Level = logiface.Level(lvl)
				break
			}
		}
		panic(fmt.Errorf(`unexpected zerolog level %d`, *data.Level))
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
		h.L = L.New(append([]logiface.Option[*Event]{L.WithZerolog(zerolog.New(&h.B))}, options...)...)
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

		if s := h.B.String(); s != "{\"level\":\"info\",\"message\":\"hello world\"}\n{\"level\":\"warn\",\"message\":\"is warning\"}\n{\"level\":\"error\",\"message\":\"is err\"}\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
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

		if s := h.B.String(); s != "{\"level\":\"info\",\"one\":1,\"two\":2,\"three\":3,\"message\":\"hello world\"}\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
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

		if s := h.B.String(); s != "{\"level\":\"info\",\"one\":1,\"two\":2,\"three\":3,\"four\":4,\"message\":\"case 1\"}\n{\"level\":\"info\",\"six\":6,\"seven\":7,\"eight\":8,\"message\":\"case 2\"}\n{\"level\":\"info\",\"one\":1,\"two\":2,\"three\":-3,\"five\":5,\"message\":\"case 3\"}\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
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
	})

	t.Run(`add error`, func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		h.L.Info().
			Err(errors.New(`some error`)).
			Log(`hello world`)

		if s := h.B.String(); s != "{\"level\":\"info\",\"error\":\"some error\",\"message\":\"hello world\"}\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
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
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	})
}

func ExampleLogger_arrayField() {
	logger := L.New(L.WithZerolog(zerolog.New(os.Stdout))).
		Clone().
		Array().
		Field(3).
		Field(4).
		As(`d`).
		Logger()
	logger.Info().
		Str(`a`, `A`).
		Array().
		Field(1).
		Field(2).
		As(`b`).
		Str(`c`, `C`).
		Log(`msg 1`)

	//output:
	//{"level":"info","d":[3,4],"a":"A","b":[1,2],"c":"C","message":"msg 1"}
}
