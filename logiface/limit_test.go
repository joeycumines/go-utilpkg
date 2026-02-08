package logiface

import (
	"bytes"
	"io"
	"math"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/go-catrate"
	runtimeutil "github.com/joeycumines/logiface/internal/runtime"
)

func TestCallerSkipPackage(t *testing.T) {
	v := runtimeutil.CallerSkipPackage(``, 0)
	if v.Function != `github.com/joeycumines/logiface.TestCallerSkipPackage` {
		t.Errorf(`unexpected function %q`, v.Function)
	}
	if pkgPath == `` {
		t.Error(`unexpected empty pkgPath`)
	} else if filepath.ToSlash(v.File) != filepath.ToSlash(filepath.Join(pkgPath, `limit_test.go`)) {
		t.Errorf(`unexpected file %q`, v.File)
	}
	if v.Entry == 0 || v.Line == 0 {
		t.Errorf(`unexpected caller: %+v`, v)
	}
}

const (
	categoryRateLimitTestCount  = 20
	categoryRateLimitTestOutput = "[info] i=0 msg=test\n[info] i=1 msg=test\n[info] i=2 msg=test\n[info] i=3 msg=test\n[info] i=4 msg=test\n[info] i=5 msg=test\n[info] i=6 msg=test\n[info] i=7 msg=test\n[info] i=8 msg=test\n[info] i=9 _limited=<stripped> msg=test\n"
)

var (
	categoryRateLimitTestRegex     = regexp.MustCompile(` _limited=map\[category:map\[[^]]+] next:[^ ]+ until:[^ s]+s] `)
	categoryRateLimitTestNormalize = func(output string) string {
		return categoryRateLimitTestRegex.ReplaceAllLiteralString(output, ` _limited=<stripped> `)
	}
	categoryRateLimitTestFactory = func(w io.Writer) *Logger[*mockSimpleEvent] {
		return mockL.New(
			mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
			mockL.WithWriter(&mockSimpleWriter{Writer: w}),
			mockL.WithCategoryRateLimits(map[time.Duration]int{
				time.Millisecond * 300: 10,
			}),
		)
	}
)

func TestBuilder_Limit_callerCategoryRateLimit(t *testing.T) {
	var buf bytes.Buffer

	logger := categoryRateLimitTestFactory(&buf)

	for i := 0; i < categoryRateLimitTestCount; i++ {
		b := logger.Info()
		if b.mode != 0 {
			t.Fatal(b.mode)
		}

		b = b.Limit()
		if b.mode != builderModeCallerCategoryRateLimit {
			t.Fatal(b.mode)
		}

		b.Int(`i`, i).
			Log(`test`)
	}

	output := categoryRateLimitTestNormalize(buf.String())
	if output != categoryRateLimitTestOutput {
		t.Fatalf("unexpected output: %q\n%s", output, output)
	}
}

func TestLogger_CallerCategoryRateLimitModifier(t *testing.T) {
	var buf bytes.Buffer

	logger := categoryRateLimitTestFactory(&buf)

	modifier := logger.CallerCategoryRateLimitModifier()
	if modifier == nil {
		t.Fatal()
	}

	for i := 0; i < categoryRateLimitTestCount; i++ {
		logger.Info().
			Int(`i`, i).
			Modifier(modifier).
			Log(`test`)
	}

	output := categoryRateLimitTestNormalize(buf.String())
	if output != categoryRateLimitTestOutput {
		t.Fatalf("unexpected output: %q\n%s", output, output)
	}
}

func callCatrateAllowCaller(d *loggerShared[*mockEvent], skip int) (caller runtimeutil.Caller, next time.Time, ok bool) {
	return d.catrateAllowCaller(skip)
}

func callCallCatrateAllowCaller(d *loggerShared[*mockEvent], skip int) (caller runtimeutil.Caller, next time.Time, ok bool) {
	return callCatrateAllowCaller(d, skip)
}

func TestLoggerShared_catrateAllowCaller(t *testing.T) {
	{
		old := pkgPath
		defer func() { pkgPath = old }()
	}

	pkgPath = `/some/other/path`

	const dur = time.Minute * 5

	d := &loggerShared[*mockEvent]{catrate: catrate.NewLimiter(map[time.Duration]int{
		dur: 2,
	})}

	caller, next, ok := callCallCatrateAllowCaller(d, 1)
	//t.Logf(`%+v`, caller)
	if !ok {
		t.Error(ok)
	}
	if next != (time.Time{}) {
		t.Error(next)
	}
	if caller.Function != `github.com/joeycumines/logiface.callCallCatrateAllowCaller` {
		t.Error(caller)
	}

	caller, next, ok = callCallCatrateAllowCaller(d, 0)
	//t.Logf(`%+v`, caller)
	if !ok {
		t.Error(ok)
	}
	if next != (time.Time{}) {
		t.Error(next)
	}
	if caller.Function != `github.com/joeycumines/logiface.callCatrateAllowCaller` {
		t.Error(caller)
	}

	caller, next, ok = callCallCatrateAllowCaller(d, 1)
	//t.Logf(`%+v`, caller)
	if !ok {
		t.Error(ok)
	}
	if next == (time.Time{}) || math.Abs(float64(time.Until(next))/float64(dur)-1) > 0.05 {
		t.Error(next)
	}
	if caller.Function != `github.com/joeycumines/logiface.callCallCatrateAllowCaller` {
		t.Error(caller)
	}

	caller, next, ok = callCallCatrateAllowCaller(d, 0)
	//t.Logf(`%+v`, caller)
	if !ok {
		t.Error(ok)
	}
	if next == (time.Time{}) || math.Abs(float64(time.Until(next))/float64(dur)-1) > 0.05 {
		t.Error(next)
	}
	if caller.Function != `github.com/joeycumines/logiface.callCatrateAllowCaller` {
		t.Error(caller)
	}

	caller, next, ok = callCallCatrateAllowCaller(d, 1)
	//t.Logf(`%+v`, caller)
	if ok {
		t.Error(ok)
	}
	if next == (time.Time{}) || math.Abs(float64(time.Until(next))/float64(dur)-1) > 0.05 {
		t.Error(next)
	}
	if caller.Function != `github.com/joeycumines/logiface.callCallCatrateAllowCaller` {
		t.Error(caller)
	}

	caller, next, ok = callCallCatrateAllowCaller(d, 0)
	//t.Logf(`%+v`, caller)
	if ok {
		t.Error(ok)
	}
	if next == (time.Time{}) || math.Abs(float64(time.Until(next))/float64(dur)-1) > 0.05 {
		t.Error(next)
	}
	if caller.Function != `github.com/joeycumines/logiface.callCatrateAllowCaller` {
		t.Error(caller)
	}
}

func TestLoggerShared_catrateAllowCaller_noCaller(t *testing.T) {
	{
		old := pkgPath
		defer func() { pkgPath = old }()
	}
	{
		old := runtimeutilCallerSkipPackage
		defer func() { runtimeutilCallerSkipPackage = old }()
	}

	const skip = 611
	const packagePath = `/some/other/path`
	pkgPath = packagePath
	var count int32
	runtimeutilCallerSkipPackage = func(pkgPath string, i int) runtimeutil.Caller {
		if pkgPath != packagePath {
			t.Error(pkgPath)
		}
		if i != skip+1 {
			t.Error(i)
		}
		if !atomic.CompareAndSwapInt32(&count, 0, 1) {
			t.Error()
		}
		return runtimeutil.Caller{}
	}

	d := &loggerShared[*mockEvent]{}

	caller, next, ok := d.catrateAllowCaller(skip)

	if !atomic.CompareAndSwapInt32(&count, 1, 2) {
		t.Error()
	}

	if caller != (runtimeutil.Caller{}) {
		t.Error(caller)
	}

	if next != (time.Time{}) {
		t.Error(next)
	}

	if !ok {
		t.Error(ok)
	}
}
