package stumpy

import (
	"encoding/json"
	"errors"
	"github.com/joeycumines/go-utilpkg/logiface"
	"github.com/joeycumines/go-utilpkg/logiface/testsuite"
	"io"
	"math"
	"runtime"
	"testing"
	"time"
)

var testSuiteConfig = testsuite.Config[*Event]{
	LoggerFactory: testSuiteLoggerFactory,
	WriteTimeout:  time.Second * 10,
}

func testSuiteLoggerFactory(req testsuite.LoggerRequest[*Event]) testsuite.LoggerResponse[*Event] {
	var options []logiface.Option[*Event]

	options = append(options, L.WithStumpy(
		WithWriter(req.Writer),
	))

	options = append(options, req.Options...)

	return testsuite.LoggerResponse[*Event]{
		Logger:       L.New(options...),
		LevelMapping: testSuiteLevelMapping,
		ParseEvent:   testSuiteParseEvent,
	}
}

func testSuiteLevelMapping(lvl logiface.Level) logiface.Level {
	return lvl
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
		Level   string  `json:"lvl"`
		Message *string `json:"msg"`
		Error   *string `json:"err"`
	}
	if err := json.Unmarshal(b, &data); err != nil {
		panic(err)
	}

	var fields map[string]interface{}
	if err := json.Unmarshal(b, &fields); err != nil {
		panic(err)
	}
	delete(fields, `lvl`)
	delete(fields, `msg`)
	delete(fields, `err`)

	ev := testsuite.Event{
		Message: data.Message,
		Error:   data.Error,
		Fields:  fields,
		Level:   logiface.LevelDisabled,
	}

	for i := 0; i <= math.MaxInt8; i++ {
		lvl := logiface.Level(i)
		if lvl.String() == data.Level {
			ev.Level = lvl
			break
		}
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
