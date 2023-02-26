package zerolog

import (
	"errors"
	"github.com/rs/zerolog"
	"io"
	"net"
	"testing"
	"time"
)

// Benchmarks in this file which compare the variants (baseline, generic,
// interface) must have an appropriate structure to support comparison, using
// the benchstat CLI tool. Additionally, all such benchmarks should be added to
// Makefile (BENCHMARK_NAMES variable), to support easy usage.
//
// The VariantBenchmark struct is provided to make it easier to implement the
// appropriate structure, and the Run method can be used to run all three.

/*
Template benchmark:

func Benchmark(b *testing.B) {
	(VariantBenchmark{
		Baseline: func(b *testing.B) {
		},
		Generic: func(b *testing.B) {
		},
		Interface: func(b *testing.B) {
		},
	}).Run(b)
}
*/

var (
	fakeMessage = "Test logging, but use a somewhat realistic message length."
)

// VariantBenchmark models a set of benchmarks testing the relative performance
// of the logiface wrapper implementation.
type VariantBenchmark struct {
	// Baseline should test zerolog directly, without any wrapper.
	Baseline func(b *testing.B)
	// Generic should test logiface using the logger returned by L.New.
	Generic func(b *testing.B)
	// Interface should be identical to Generic, except using the generic logger returned by L.New().Logger()
	Interface func(b *testing.B)
}

// Run runs all three benchmarks as sub-benchmarks of the provided testing.B.
func (x VariantBenchmark) Run(b *testing.B) {
	if x.Baseline != nil {
		b.Run(`variant=baseline`, x.Baseline)
	}
	if x.Generic != nil {
		b.Run(`variant=generic`, x.Generic)
	}
	if x.Interface != nil {
		b.Run(`variant=interface`, x.Interface)
	}
}

func BenchmarkDisabled(b *testing.B) {
	(VariantBenchmark{
		Baseline: func(b *testing.B) {
			logger := zerolog.New(io.Discard).Level(zerolog.Disabled)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().Msg(fakeMessage)
				}
			})
		},
		Generic: func(b *testing.B) {
			logger := L.New(
				L.WithZerolog(zerolog.New(io.Discard)),
				L.WithLevel(L.LevelDisabled()),
			)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().Log(fakeMessage)
				}
			})
		},
		Interface: func(b *testing.B) {
			logger := L.New(
				L.WithZerolog(zerolog.New(io.Discard)),
				L.WithLevel(L.LevelDisabled()),
			).Logger()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().Log(fakeMessage)
				}
			})
		},
	}).Run(b)
}

func BenchmarkInfo(b *testing.B) {
	(VariantBenchmark{
		Baseline: func(b *testing.B) {
			logger := zerolog.New(io.Discard)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().Msg(fakeMessage)
				}
			})
		},
		Generic: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard)))
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().Log(fakeMessage)
				}
			})
		},
		Interface: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard))).Logger()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().Log(fakeMessage)
				}
			})
		},
	}).Run(b)
}

func BenchmarkContextFields(b *testing.B) {
	(VariantBenchmark{
		Baseline: func(b *testing.B) {
			logger := zerolog.New(io.Discard).With().
				Str("string", "four!").
				Time("time", time.Time{}).
				Int("int", 123).
				Float32("float", -2.203230293249593).
				Logger()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().Msg(fakeMessage)
				}
			})
		},
		Generic: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard))).
				Clone().
				Str("string", "four!").
				Time("time", time.Time{}).
				Int("int", 123).
				Float32("float", -2.203230293249593).
				Logger()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().Log(fakeMessage)
				}
			})
		},
		Interface: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard))).
				Logger().
				Clone().
				Str("string", "four!").
				Time("time", time.Time{}).
				Int("int", 123).
				Float32("float", -2.203230293249593).
				Logger()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().Log(fakeMessage)
				}
			})
		},
	}).Run(b)
}

func BenchmarkContextAppend(b *testing.B) {
	(VariantBenchmark{
		Baseline: func(b *testing.B) {
			logger := zerolog.New(io.Discard).With().
				Str("foo", "bar").
				Logger()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.With().Str("bar", "baz")
				}
			})
		},
		Generic: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard))).
				Clone().
				Str("foo", "bar").
				Logger()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Clone().Str("bar", "baz")
				}
			})
		},
		Interface: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard))).
				Logger().
				Clone().
				Str("foo", "bar").
				Logger()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Clone().Str("bar", "baz")
				}
			})
		},
	}).Run(b)
}

func BenchmarkLogFields(b *testing.B) {
	(VariantBenchmark{
		Baseline: func(b *testing.B) {
			logger := zerolog.New(io.Discard)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().
						Str("string", "four!").
						Time("time", time.Time{}).
						Int("int", 123).
						Float32("float", -2.203230293249593).
						Msg(fakeMessage)
				}
			})
		},
		Generic: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard)))
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().
						Str("string", "four!").
						Time("time", time.Time{}).
						Int("int", 123).
						Float32("float", -2.203230293249593).
						Log(fakeMessage)
				}
			})
		},
		Interface: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard))).Logger()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().
						Str("string", "four!").
						Time("time", time.Time{}).
						Int("int", 123).
						Float32("float", -2.203230293249593).
						Log(fakeMessage)
				}
			})
		},
	}).Run(b)
}

type obj struct {
	Pub  string
	Tag  string `json:"tag"`
	priv int
}

func (o obj) MarshalZerologObject(e *zerolog.Event) {
	e.Str("Pub", o.Pub).
		Str("Tag", o.Tag).
		Int("priv", o.priv)
}

func BenchmarkLogArrayObject(b *testing.B) {
	obj1 := obj{"a", "b", 2}
	obj2 := obj{"c", "d", 3}
	obj3 := obj{"e", "f", 4}
	logger := zerolog.New(io.Discard)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		arr := zerolog.Arr()
		arr.Object(&obj1)
		arr.Object(&obj2)
		arr.Object(&obj3)
		logger.Info().Array("objects", arr).Msg("test")
	}
}

func BenchmarkLogFieldType(b *testing.B) {
	bools := []bool{true, false, true, false, true, false, true, false, true, false}
	ints := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	floats := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	strings := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	durations := []time.Duration{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	times := []time.Time{
		time.Unix(0, 0),
		time.Unix(1, 0),
		time.Unix(2, 0),
		time.Unix(3, 0),
		time.Unix(4, 0),
		time.Unix(5, 0),
		time.Unix(6, 0),
		time.Unix(7, 0),
		time.Unix(8, 0),
		time.Unix(9, 0),
	}
	interfaces := []struct {
		Pub  string
		Tag  string `json:"tag"`
		priv int
	}{
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
	}
	objects := []obj{
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
	}
	errs := []error{errors.New("a"), errors.New("b"), errors.New("c"), errors.New("d"), errors.New("e")}
	types := map[string]func(e *zerolog.Event) *zerolog.Event{
		"Bool": func(e *zerolog.Event) *zerolog.Event {
			return e.Bool("k", bools[0])
		},
		"Bools": func(e *zerolog.Event) *zerolog.Event {
			return e.Bools("k", bools)
		},
		"Int": func(e *zerolog.Event) *zerolog.Event {
			return e.Int("k", ints[0])
		},
		"Ints": func(e *zerolog.Event) *zerolog.Event {
			return e.Ints("k", ints)
		},
		"Float": func(e *zerolog.Event) *zerolog.Event {
			return e.Float64("k", floats[0])
		},
		"Floats": func(e *zerolog.Event) *zerolog.Event {
			return e.Floats64("k", floats)
		},
		"Str": func(e *zerolog.Event) *zerolog.Event {
			return e.Str("k", strings[0])
		},
		"Strs": func(e *zerolog.Event) *zerolog.Event {
			return e.Strs("k", strings)
		},
		"Err": func(e *zerolog.Event) *zerolog.Event {
			return e.Err(errs[0])
		},
		"Errs": func(e *zerolog.Event) *zerolog.Event {
			return e.Errs("k", errs)
		},
		"Time": func(e *zerolog.Event) *zerolog.Event {
			return e.Time("k", times[0])
		},
		"Times": func(e *zerolog.Event) *zerolog.Event {
			return e.Times("k", times)
		},
		"Dur": func(e *zerolog.Event) *zerolog.Event {
			return e.Dur("k", durations[0])
		},
		"Durs": func(e *zerolog.Event) *zerolog.Event {
			return e.Durs("k", durations)
		},
		"Interface": func(e *zerolog.Event) *zerolog.Event {
			return e.Interface("k", interfaces[0])
		},
		"Interfaces": func(e *zerolog.Event) *zerolog.Event {
			return e.Interface("k", interfaces)
		},
		"Interface(Object)": func(e *zerolog.Event) *zerolog.Event {
			return e.Interface("k", objects[0])
		},
		"Interface(Objects)": func(e *zerolog.Event) *zerolog.Event {
			return e.Interface("k", objects)
		},
		"Object": func(e *zerolog.Event) *zerolog.Event {
			return e.Object("k", objects[0])
		},
	}
	logger := zerolog.New(io.Discard)
	b.ResetTimer()
	for name := range types {
		f := types[name]
		b.Run(name, func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					f(logger.Info()).Msg("")
				}
			})
		})
	}
}

func BenchmarkContextFieldType(b *testing.B) {
	oldFormat := zerolog.TimeFieldFormat
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	defer func() { zerolog.TimeFieldFormat = oldFormat }()
	bools := []bool{true, false, true, false, true, false, true, false, true, false}
	ints := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	floats := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	strings := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	stringer := net.IP{127, 0, 0, 1}
	durations := []time.Duration{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	times := []time.Time{
		time.Unix(0, 0),
		time.Unix(1, 0),
		time.Unix(2, 0),
		time.Unix(3, 0),
		time.Unix(4, 0),
		time.Unix(5, 0),
		time.Unix(6, 0),
		time.Unix(7, 0),
		time.Unix(8, 0),
		time.Unix(9, 0),
	}
	interfaces := []struct {
		Pub  string
		Tag  string `json:"tag"`
		priv int
	}{
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
	}
	objects := []obj{
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
		{"a", "a", 0},
	}
	errs := []error{errors.New("a"), errors.New("b"), errors.New("c"), errors.New("d"), errors.New("e")}
	types := map[string]func(c zerolog.Context) zerolog.Context{
		"Bool": func(c zerolog.Context) zerolog.Context {
			return c.Bool("k", bools[0])
		},
		"Bools": func(c zerolog.Context) zerolog.Context {
			return c.Bools("k", bools)
		},
		"Int": func(c zerolog.Context) zerolog.Context {
			return c.Int("k", ints[0])
		},
		"Ints": func(c zerolog.Context) zerolog.Context {
			return c.Ints("k", ints)
		},
		"Float": func(c zerolog.Context) zerolog.Context {
			return c.Float64("k", floats[0])
		},
		"Floats": func(c zerolog.Context) zerolog.Context {
			return c.Floats64("k", floats)
		},
		"Str": func(c zerolog.Context) zerolog.Context {
			return c.Str("k", strings[0])
		},
		"Strs": func(c zerolog.Context) zerolog.Context {
			return c.Strs("k", strings)
		},
		"Stringer": func(c zerolog.Context) zerolog.Context {
			return c.Stringer("k", stringer)
		},
		"Err": func(c zerolog.Context) zerolog.Context {
			return c.Err(errs[0])
		},
		"Errs": func(c zerolog.Context) zerolog.Context {
			return c.Errs("k", errs)
		},
		"Time": func(c zerolog.Context) zerolog.Context {
			return c.Time("k", times[0])
		},
		"Times": func(c zerolog.Context) zerolog.Context {
			return c.Times("k", times)
		},
		"Dur": func(c zerolog.Context) zerolog.Context {
			return c.Dur("k", durations[0])
		},
		"Durs": func(c zerolog.Context) zerolog.Context {
			return c.Durs("k", durations)
		},
		"Interface": func(c zerolog.Context) zerolog.Context {
			return c.Interface("k", interfaces[0])
		},
		"Interfaces": func(c zerolog.Context) zerolog.Context {
			return c.Interface("k", interfaces)
		},
		"Interface(Object)": func(c zerolog.Context) zerolog.Context {
			return c.Interface("k", objects[0])
		},
		"Interface(Objects)": func(c zerolog.Context) zerolog.Context {
			return c.Interface("k", objects)
		},
		"Object": func(c zerolog.Context) zerolog.Context {
			return c.Object("k", objects[0])
		},
		"Timestamp": func(c zerolog.Context) zerolog.Context {
			return c.Timestamp()
		},
	}
	logger := zerolog.New(io.Discard)
	b.ResetTimer()
	for name := range types {
		f := types[name]
		b.Run(name, func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					l := f(logger.With()).Logger()
					l.Info().Msg("")
				}
			})
		})
	}
}
