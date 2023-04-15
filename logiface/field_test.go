package logiface_test

import (
	"bytes"
	"github.com/joeycumines/go-utilpkg/logiface"
	"github.com/joeycumines/go-utilpkg/logiface/internal/fieldtest"
	"github.com/joeycumines/go-utilpkg/logiface-stumpy"
	"math"
	"strings"
	"testing"
)

func fluentObjectTemplate[T interface {
	fieldtest.ObjectMethods[T]
	logiface.Parent[logiface.Event]
	comparable
}](x T) {
	fieldtest.FluentObjectTemplate(x)

	{
		arr1 := logiface.ArrayWithKey[logiface.Event](x, `array using logiface.ArrayWithKey`)

		fieldtest.FluentArrayTemplate(arr1)

		{
			arr2 := logiface.Array[logiface.Event](arr1)

			if v := logiface.SliceArray[logiface.Event](arr2, ``, []float64{1e-6, 1e-7, math.NaN(), math.Inf(1), math.Inf(-1)}); v != arr2 {
				panic(`unexpected return value`)
			}

			if v := logiface.MapObject[logiface.Event](arr2, ``, map[string]interface{}{`k`: math.Inf(-1)}); v != arr2 {
				panic(`unexpected return value`)
			}

			{
				obj := logiface.Object[logiface.Event](arr2)

				fieldtest.FluentObjectTemplate(obj)

				if v := obj.Add(); v != arr2 {
					panic(`unexpected return value`)
				}
			}

			if v := arr2.As(``); v != arr1 {
				panic(`unexpected return value`)
			}
		}

		if v := logiface.MapObject[logiface.Event](arr1, ``, map[string]any(nil)); v != arr1 {
			panic(`unexpected return value`)
		}

		if v := arr1.As(`THIS KEY WILL BE IGNORED`); v != x {
			panic(`unexpected return value`)
		}
	}

	{
		obj1 := logiface.Object[logiface.Event](x)

		if v := logiface.MapObject[logiface.Event](obj1, `nested object`, map[string]any(nil)); v != obj1 {
			panic(`unexpected return value`)
		}

		if v := logiface.SliceArray[logiface.Event](obj1, `nested array`, []any(nil)); v != obj1 {
			panic(`unexpected return value`)
		}

		if v := obj1.As(`object using logiface.Object`); v != x {
			panic(`unexpected return value`)
		}
	}
}

func TestEvent_stumpy(t *testing.T) {
	const message = `log called`
	test := func(t *testing.T, obj bool, log func(l *logiface.Logger[logiface.Event])) {
		var buf bytes.Buffer
		l := stumpy.L.New(stumpy.L.WithStumpy(stumpy.WithWriter(&buf), stumpy.WithLevelField(``)), stumpy.L.WithDPanicLevel(stumpy.L.LevelEmergency())).Logger()
		log(l)
		const expected = "{\"err\":\"err called\",\"field called with string\":\"val 2\",\"field called with bytes\":\"dmFsIDM=\",\"field called with time.Time local\":\"2019-05-17T05:07:20.361696123Z\",\"field called with time.Time utc\":\"2019-05-17T05:07:20.361696123Z\",\"field called with duration\":\"3116139.280723392s\",\"field called with int\":-51245,\"field called with float32\":1e-45,\"field called with unhandled type\":-421,\"float32 called\":3.4028235e+38,\"int called\":9223372036854775807,\"interface called with string\":\"val 4\",\"interface called with bool\":true,\"interface called with nil\":null,\"any called with string\":\"val 5\",\"str called\":\"val 6\",\"time called with local\":\"2021-03-24T13:27:29.876543213Z\",\"time called with utc\":\"2020-03-01T00:39:29.456789123Z\",\"dur called positive\":\"51238123.523458989s\",\"dur called negative\":\"-51238123.523458989s\",\"dur called zero\":\"0s\",\"base64 called with nil enc\":\"dmFsIDc=\",\"base64 called with padding\":\"dmFsIDc=\",\"base64 called without padding\":\"dmFsIDc\",\"bool called\":true,\"field called with bool\":true,\"float64 called\":1.7976931348623157e+308,\"field called with float64\":1.7976931348623157e+308,\"int64 called\":\"9223372036854775807\",\"field called with int64\":\"9223372036854775807\",\"uint64 called\":\"18446744073709551615\",\"field called with uint64\":\"18446744073709551615\",\"array using logiface.ArrayWithKey\":[\"err called\",\"val 2\",\"dmFsIDM=\",\"2019-05-17T05:07:20.361696123Z\",\"2019-05-17T05:07:20.361696123Z\",\"3116139.280723392s\",-51245,1e-45,-421,3.4028235e+38,9223372036854775807,\"val 4\",true,null,\"val 5\",\"val 6\",\"2021-03-24T13:27:29.876543213Z\",\"2020-03-01T00:39:29.456789123Z\",\"51238123.523458989s\",\"-51238123.523458989s\",\"0s\",\"dmFsIDc=\",\"dmFsIDc=\",\"dmFsIDc\",true,true,1.7976931348623157e+308,1.7976931348623157e+308,\"9223372036854775807\",\"9223372036854775807\",\"18446744073709551615\",\"18446744073709551615\",[[0.000001,1e-7,\"NaN\",\"Infinity\",\"-Infinity\"],{\"k\":\"-Infinity\"},{\"err\":\"err called\",\"field called with string\":\"val 2\",\"field called with bytes\":\"dmFsIDM=\",\"field called with time.Time local\":\"2019-05-17T05:07:20.361696123Z\",\"field called with time.Time utc\":\"2019-05-17T05:07:20.361696123Z\",\"field called with duration\":\"3116139.280723392s\",\"field called with int\":-51245,\"field called with float32\":1e-45,\"field called with unhandled type\":-421,\"float32 called\":3.4028235e+38,\"int called\":9223372036854775807,\"interface called with string\":\"val 4\",\"interface called with bool\":true,\"interface called with nil\":null,\"any called with string\":\"val 5\",\"str called\":\"val 6\",\"time called with local\":\"2021-03-24T13:27:29.876543213Z\",\"time called with utc\":\"2020-03-01T00:39:29.456789123Z\",\"dur called positive\":\"51238123.523458989s\",\"dur called negative\":\"-51238123.523458989s\",\"dur called zero\":\"0s\",\"base64 called with nil enc\":\"dmFsIDc=\",\"base64 called with padding\":\"dmFsIDc=\",\"base64 called without padding\":\"dmFsIDc\",\"bool called\":true,\"field called with bool\":true,\"float64 called\":1.7976931348623157e+308,\"field called with float64\":1.7976931348623157e+308,\"int64 called\":\"9223372036854775807\",\"field called with int64\":\"9223372036854775807\",\"uint64 called\":\"18446744073709551615\",\"field called with uint64\":\"18446744073709551615\"}],{}],\"object using logiface.Object\":{\"nested object\":{},\"nested array\":[]},\"msg\":\"log called\"}\n"
		actual := buf.String()
		if obj {
			const (
				prefix = `{"k":`
				suffix = "},\"msg\":\"log called\"}\n"
			)
			if !strings.HasPrefix(actual, prefix) || !strings.HasSuffix(actual, suffix) {
				t.Fatalf("unexpected prefix/suffix: %q\n%s", actual, actual)
			}
			actual = actual[len(prefix):len(actual)-len(suffix)] + suffix[1:]
		}
		if actual != expected {
			t.Errorf("unexpected output: %q\nexpected: %s\nactual: %s", actual, expected, actual)
		}
	}
	t.Run(`Context`, func(t *testing.T) {
		test(t, false, func(l *logiface.Logger[logiface.Event]) {
			c := l.Clone()
			fluentObjectTemplate(c)
			c.Logger().Emerg().Log(message)
		})
	})
	t.Run(`Builder`, func(t *testing.T) {
		test(t, false, func(l *logiface.Logger[logiface.Event]) {
			b := l.Emerg()
			fluentObjectTemplate(b)
			b.Log(message)
		})
	})
	t.Run(`BuilderObject`, func(t *testing.T) {
		test(t, true, func(l *logiface.Logger[logiface.Event]) {
			l.Emerg().
				ObjectFunc(`k`, fluentObjectTemplate[logiface.BuilderObject]).
				Log(message)
		})
	})
}
