package zerolog

import (
	"errors"
	"github.com/joeycumines/go-utilpkg/logiface"
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
	fakeMessage  = "Test logging, but use a somewhat realistic message length."
	shortMessage = "Test logging."
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
	Variants  []Variant
}

type Variant struct {
	Name      string
	Benchmark func(b *testing.B)
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
	for _, variant := range x.Variants {
		b.Run(`variant=`+variant.Name, variant.Benchmark)
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

//func BenchmarkLogArrayObject(b *testing.B) {
//	obj1 := obj{"a", "b", 2}
//	obj2 := obj{"c", "d", 3}
//	obj3 := obj{"e", "f", 4}
//	logger := zerolog.New(io.Discard)
//	b.ResetTimer()
//	b.ReportAllocs()
//	for i := 0; i < b.N; i++ {
//		arr := zerolog.Arr()
//		arr.Object(&obj1)
//		arr.Object(&obj2)
//		arr.Object(&obj3)
//		logger.Info().Array("objects", arr).Msg("test")
//	}
//}

func BenchmarkLogFieldType_Time(b *testing.B) {
	benchmarkLogFieldType(b, `Time`)
}
func BenchmarkContextFieldType_Time(b *testing.B) {
	benchmarkContextFieldType(b, `Time`)
}

func BenchmarkLogFieldType_Int(b *testing.B) {
	benchmarkLogFieldType(b, `Int`)
}
func BenchmarkContextFieldType_Int(b *testing.B) {
	benchmarkContextFieldType(b, `Int`)
}

func BenchmarkLogFieldType_Float32(b *testing.B) {
	benchmarkLogFieldType(b, `Float32`)
}
func BenchmarkContextFieldType_Float32(b *testing.B) {
	benchmarkContextFieldType(b, `Float32`)
}

func BenchmarkLogFieldType_Err(b *testing.B) {
	benchmarkLogFieldType(b, `Err`)
}
func BenchmarkContextFieldType_Err(b *testing.B) {
	benchmarkContextFieldType(b, `Err`)
}

func BenchmarkLogFieldType_Str(b *testing.B) {
	benchmarkLogFieldType(b, `Str`)
}
func BenchmarkContextFieldType_Str(b *testing.B) {
	benchmarkContextFieldType(b, `Str`)
}

func BenchmarkLogFieldType_Interface(b *testing.B) {
	benchmarkLogFieldType(b, `Interface`)
}
func BenchmarkContextFieldType_Interface(b *testing.B) {
	benchmarkContextFieldType(b, `Interface`)
}

func BenchmarkLogFieldType_InterfaceObject(b *testing.B) {
	benchmarkLogFieldType(b, `Interface(Object)`)
}
func BenchmarkContextFieldType_InterfaceObject(b *testing.B) {
	benchmarkContextFieldType(b, `Interface(Object)`)
}

func BenchmarkLogFieldType_Dur(b *testing.B) {
	benchmarkLogFieldType(b, `Dur`)
}
func BenchmarkContextFieldType_Dur(b *testing.B) {
	benchmarkContextFieldType(b, `Dur`)
}

func BenchmarkLogFieldType_Bool(b *testing.B) {
	benchmarkLogFieldType(b, `Bool`)
}
func BenchmarkContextFieldType_Bool(b *testing.B) {
	benchmarkContextFieldType(b, `Bool`)
}

func BenchmarkLogFieldType_Float64(b *testing.B) {
	benchmarkLogFieldType(b, `Float`)
}
func BenchmarkContextFieldType_Float64(b *testing.B) {
	benchmarkContextFieldType(b, `Float`)
}

func BenchmarkLogFieldType_Int64(b *testing.B) {
	benchmarkLogFieldType(b, `Int64`)
}
func BenchmarkContextFieldType_Int64(b *testing.B) {
	benchmarkContextFieldType(b, `Int64`)
}

func BenchmarkLogFieldType_Uint64(b *testing.B) {
	benchmarkLogFieldType(b, `Uint64`)
}
func BenchmarkContextFieldType_Uint64(b *testing.B) {
	benchmarkContextFieldType(b, `Uint64`)
}

func benchmarkLogFieldType(b *testing.B, typ string) {
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
	types := make(map[string]struct {
		Baseline  func(e *zerolog.Event) *zerolog.Event
		Generic   func(e *logiface.Builder[*Event]) *logiface.Builder[*Event]
		Interface func(e *logiface.Builder[logiface.Event]) *logiface.Builder[logiface.Event]
	})
	// this looks funky because it's an attempt to avoid mutating the original test case as much as possible
	// (to make it easier to propagate updates from upstream / the baseline)
	for k, v := range map[string]func(e *zerolog.Event) *zerolog.Event{
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
	} {
		val := types[k]
		val.Baseline = v
		types[k] = val
	}
	// again, down here to try and make the diff nicer
	float32s := []float32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	int64s := []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	uint64s := []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	for _, v := range []struct {
		Type      string
		Baseline  func(e *zerolog.Event) *zerolog.Event // only if not implemented by the upstream
		Generic   func(e *logiface.Builder[*Event]) *logiface.Builder[*Event]
		Interface func(e *logiface.Builder[logiface.Event]) *logiface.Builder[logiface.Event]
	}{
		{
			Type: `Time`,
			Generic: func(e *logiface.Builder[*Event]) *logiface.Builder[*Event] {
				return e.Time("k", times[0])
			},
			Interface: func(e *logiface.Builder[logiface.Event]) *logiface.Builder[logiface.Event] {
				return e.Time("k", times[0])
			},
		},
		{
			Type: `Int`,
			Generic: func(e *logiface.Builder[*Event]) *logiface.Builder[*Event] {
				return e.Int("k", ints[0])
			},
			Interface: func(e *logiface.Builder[logiface.Event]) *logiface.Builder[logiface.Event] {
				return e.Int("k", ints[0])
			},
		},
		{
			Type: `Float32`,
			Baseline: func(e *zerolog.Event) *zerolog.Event {
				return e.Float32("k", float32s[0])
			},
			Generic: func(e *logiface.Builder[*Event]) *logiface.Builder[*Event] {
				return e.Float32("k", float32s[0])
			},
			Interface: func(e *logiface.Builder[logiface.Event]) *logiface.Builder[logiface.Event] {
				return e.Float32("k", float32s[0])
			},
		},
		{
			Type: `Err`,
			Generic: func(e *logiface.Builder[*Event]) *logiface.Builder[*Event] {
				return e.Err(errs[0])
			},
			Interface: func(e *logiface.Builder[logiface.Event]) *logiface.Builder[logiface.Event] {
				return e.Err(errs[0])
			},
		},
		{
			Type: `Str`,
			Generic: func(e *logiface.Builder[*Event]) *logiface.Builder[*Event] {
				return e.Str("k", strings[0])
			},
			Interface: func(e *logiface.Builder[logiface.Event]) *logiface.Builder[logiface.Event] {
				return e.Str("k", strings[0])
			},
		},
		{
			Type: `Interface`,
			Generic: func(e *logiface.Builder[*Event]) *logiface.Builder[*Event] {
				return e.Interface("k", interfaces[0])
			},
			Interface: func(e *logiface.Builder[logiface.Event]) *logiface.Builder[logiface.Event] {
				return e.Interface("k", interfaces[0])
			},
		},
		{
			Type: `Interface(Object)`,
			Generic: func(e *logiface.Builder[*Event]) *logiface.Builder[*Event] {
				return e.Interface("k", objects[0])
			},
			Interface: func(e *logiface.Builder[logiface.Event]) *logiface.Builder[logiface.Event] {
				return e.Interface("k", objects[0])
			},
		},
		{
			Type: `Dur`,
			Generic: func(e *logiface.Builder[*Event]) *logiface.Builder[*Event] {
				return e.Dur("k", durations[0])
			},
			Interface: func(e *logiface.Builder[logiface.Event]) *logiface.Builder[logiface.Event] {
				return e.Dur("k", durations[0])
			},
		},
		{
			Type: `Bool`,
			Generic: func(e *logiface.Builder[*Event]) *logiface.Builder[*Event] {
				return e.Bool("k", bools[0])
			},
			Interface: func(e *logiface.Builder[logiface.Event]) *logiface.Builder[logiface.Event] {
				return e.Bool("k", bools[0])
			},
		},
		{
			Type: `Float`,
			Generic: func(e *logiface.Builder[*Event]) *logiface.Builder[*Event] {
				return e.Float64("k", floats[0])
			},
			Interface: func(e *logiface.Builder[logiface.Event]) *logiface.Builder[logiface.Event] {
				return e.Float64("k", floats[0])
			},
		},
		{
			Type: `Int64`,
			Baseline: func(e *zerolog.Event) *zerolog.Event {
				return e.Int64("k", int64s[0])
			},
			Generic: func(e *logiface.Builder[*Event]) *logiface.Builder[*Event] {
				return e.Int64("k", int64s[0])
			},
			Interface: func(e *logiface.Builder[logiface.Event]) *logiface.Builder[logiface.Event] {
				return e.Int64("k", int64s[0])
			},
		},
		{
			Type: `Uint64`,
			Baseline: func(e *zerolog.Event) *zerolog.Event {
				return e.Uint64("k", uint64s[0])
			},
			Generic: func(e *logiface.Builder[*Event]) *logiface.Builder[*Event] {
				return e.Uint64("k", uint64s[0])
			},
			Interface: func(e *logiface.Builder[logiface.Event]) *logiface.Builder[logiface.Event] {
				return e.Uint64("k", uint64s[0])
			},
		},
	} {
		if v.Interface == nil || v.Generic == nil {
			b.Fatalf("invalid test case for %q", v.Type)
		}
		val := types[v.Type]
		if v.Baseline == nil {
			if val.Baseline == nil {
				b.Fatalf("unknown type %q", v.Type)
			}
		} else if val.Baseline != nil {
			b.Fatalf("duplicate test case for %q", v.Type)
		} else {
			val.Baseline = v.Baseline
		}
		val.Generic = v.Generic
		val.Interface = v.Interface
		types[v.Type] = val
	}
	val := types[typ]
	if val.Baseline == nil || val.Generic == nil || val.Interface == nil {
		b.Fatalf("unknown type %q", typ)
	}
	(VariantBenchmark{
		Baseline: func(b *testing.B) {
			logger := zerolog.New(io.Discard)
			f := val.Baseline
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					f(logger.Info()).Msg("")
				}
			})
		},
		Generic: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard)))
			f := val.Generic
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					f(logger.Info()).Log("")
				}
			})
		},
		Interface: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard))).Logger()
			f := val.Interface
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					f(logger.Info()).Log("")
				}
			})
		},
	}).Run(b)
}

func benchmarkContextFieldType(b *testing.B, typ string) {
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
	types := make(map[string]struct {
		Baseline  func(c zerolog.Context) zerolog.Context
		Generic   func(e *logiface.Context[*Event]) *logiface.Context[*Event]
		Interface func(e *logiface.Context[logiface.Event]) *logiface.Context[logiface.Event]
	})
	// this looks funky because it's an attempt to avoid mutating the original test case as much as possible
	// (to make it easier to propagate updates from upstream / the baseline)
	for k, v := range map[string]func(c zerolog.Context) zerolog.Context{
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
	} {
		val := types[k]
		val.Baseline = v
		types[k] = val
	}
	// again, down here to try and make the diff nicer
	float32s := []float32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	int64s := []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	uint64s := []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	for _, v := range []struct {
		Type      string
		Baseline  func(c zerolog.Context) zerolog.Context // only if not implemented by the upstream
		Generic   func(c *logiface.Context[*Event]) *logiface.Context[*Event]
		Interface func(c *logiface.Context[logiface.Event]) *logiface.Context[logiface.Event]
	}{
		{
			Type: `Time`,
			Generic: func(c *logiface.Context[*Event]) *logiface.Context[*Event] {
				return c.Time("k", times[0])
			},
			Interface: func(c *logiface.Context[logiface.Event]) *logiface.Context[logiface.Event] {
				return c.Time("k", times[0])
			},
		},
		{
			Type: `Int`,
			Generic: func(c *logiface.Context[*Event]) *logiface.Context[*Event] {
				return c.Int("k", ints[0])
			},
			Interface: func(c *logiface.Context[logiface.Event]) *logiface.Context[logiface.Event] {
				return c.Int("k", ints[0])
			},
		},
		{
			Type: `Float32`,
			Baseline: func(c zerolog.Context) zerolog.Context {
				return c.Float32("k", float32s[0])
			},
			Generic: func(c *logiface.Context[*Event]) *logiface.Context[*Event] {
				return c.Float32("k", float32s[0])
			},
			Interface: func(c *logiface.Context[logiface.Event]) *logiface.Context[logiface.Event] {
				return c.Float32("k", float32s[0])
			},
		},
		{
			Type: `Err`,
			Generic: func(c *logiface.Context[*Event]) *logiface.Context[*Event] {
				return c.Err(errs[0])
			},
			Interface: func(c *logiface.Context[logiface.Event]) *logiface.Context[logiface.Event] {
				return c.Err(errs[0])
			},
		},
		{
			Type: `Str`,
			Generic: func(c *logiface.Context[*Event]) *logiface.Context[*Event] {
				return c.Str("k", strings[0])
			},
			Interface: func(c *logiface.Context[logiface.Event]) *logiface.Context[logiface.Event] {
				return c.Str("k", strings[0])
			},
		},
		{
			Type: `Interface`,
			Generic: func(c *logiface.Context[*Event]) *logiface.Context[*Event] {
				return c.Interface("k", interfaces[0])
			},
			Interface: func(c *logiface.Context[logiface.Event]) *logiface.Context[logiface.Event] {
				return c.Interface("k", interfaces[0])
			},
		},
		{
			Type: `Interface(Object)`,
			Generic: func(c *logiface.Context[*Event]) *logiface.Context[*Event] {
				return c.Interface("k", objects[0])
			},
			Interface: func(c *logiface.Context[logiface.Event]) *logiface.Context[logiface.Event] {
				return c.Interface("k", objects[0])
			},
		},
		{
			Type: `Dur`,
			Generic: func(c *logiface.Context[*Event]) *logiface.Context[*Event] {
				return c.Dur("k", durations[0])
			},
			Interface: func(c *logiface.Context[logiface.Event]) *logiface.Context[logiface.Event] {
				return c.Dur("k", durations[0])
			},
		},
		{
			Type: `Bool`,
			Generic: func(c *logiface.Context[*Event]) *logiface.Context[*Event] {
				return c.Bool("k", bools[0])
			},
			Interface: func(c *logiface.Context[logiface.Event]) *logiface.Context[logiface.Event] {
				return c.Bool("k", bools[0])
			},
		},
		{
			Type: `Float`,
			Generic: func(c *logiface.Context[*Event]) *logiface.Context[*Event] {
				return c.Float64("k", floats[0])
			},
			Interface: func(c *logiface.Context[logiface.Event]) *logiface.Context[logiface.Event] {
				return c.Float64("k", floats[0])
			},
		},
		{
			Type: `Int64`,
			Baseline: func(c zerolog.Context) zerolog.Context {
				return c.Int64("k", int64s[0])
			},
			Generic: func(c *logiface.Context[*Event]) *logiface.Context[*Event] {
				return c.Int64("k", int64s[0])
			},
			Interface: func(c *logiface.Context[logiface.Event]) *logiface.Context[logiface.Event] {
				return c.Int64("k", int64s[0])
			},
		},
		{
			Type: `Uint64`,
			Baseline: func(c zerolog.Context) zerolog.Context {
				return c.Uint64("k", uint64s[0])
			},
			Generic: func(c *logiface.Context[*Event]) *logiface.Context[*Event] {
				return c.Uint64("k", uint64s[0])
			},
			Interface: func(c *logiface.Context[logiface.Event]) *logiface.Context[logiface.Event] {
				return c.Uint64("k", uint64s[0])
			},
		},
	} {
		if v.Interface == nil || v.Generic == nil {
			b.Fatalf("invalid test case for %q", v.Type)
		}
		val := types[v.Type]
		if v.Baseline == nil {
			if val.Baseline == nil {
				b.Fatalf("unknown type %q", v.Type)
			}
		} else if val.Baseline != nil {
			b.Fatalf("duplicate test case for %q", v.Type)
		} else {
			val.Baseline = v.Baseline
		}
		val.Generic = v.Generic
		val.Interface = v.Interface
		types[v.Type] = val
	}
	val := types[typ]
	if val.Baseline == nil || val.Generic == nil || val.Interface == nil {
		b.Fatalf("unknown type %q", typ)
	}
	(VariantBenchmark{
		Baseline: func(b *testing.B) {
			logger := zerolog.New(io.Discard)
			f := val.Baseline
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					l := f(logger.With()).Logger()
					l.Info().Msg("")
				}
			})
		},
		Generic: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard)))
			f := val.Generic
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					l := f(logger.Clone()).Logger()
					l.Info().Log("")
				}
			})
		},
		Interface: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard))).Logger()
			f := val.Interface
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					l := f(logger.Clone()).Logger()
					l.Info().Log("")
				}
			})
		},
	}).Run(b)
}

func BenchmarkArray_Str(b *testing.B) {
	// corresponding to TestLogger_simple/array_str
	(VariantBenchmark{
		Baseline: func(b *testing.B) {
			logger := zerolog.New(io.Discard)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().
						Array(`k`, zerolog.Arr().
							Str(`a`).
							Str(`b`).
							Str(`c`).
							Str(`d`)).
						Msg(shortMessage)
				}
			})
		},
		Generic: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard)))
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().
						Array().
						Str(`a`).
						Str(`b`).
						Str(`c`).
						Str(`d`).
						As(`k`).
						End().
						Log(shortMessage)
				}
			})
		},
		Interface: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard))).Logger()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().
						Array().
						Str(`a`).
						Str(`b`).
						Str(`c`).
						Str(`d`).
						As(`k`).
						End().
						Log(shortMessage)
				}
			})
		},
	}).Run(b)
}

func BenchmarkNestedArrays(b *testing.B) {
	// corresponding to TestLogger_simple/nested_arrays
	(VariantBenchmark{
		Generic: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard))).
				Clone().
				Array().
				Field(1).
				Field(true).
				Array().
				Field(2).
				Field(false).
				Add().
				Array().
				Field(3).
				Array().
				Field(4).
				Array().
				Field(5).
				Add().
				Add().
				Add().
				As(`arr_1`).
				End().
				Field(`a`, `A`).
				Logger()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Notice().
						Array().
						Field(1).
						Field(true).
						Array().
						Field(2).
						Field(false).
						Add().
						Array().
						Field(3).
						Array().
						Field(4).
						Array().
						Field(5).
						Add().
						Add().
						Add().
						As(`arr_2`).
						End().
						Field(`b`, `B`).
						Log(`msg 1`)
				}
			})
		},
		Interface: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard))).Logger().
				Clone().
				Array().
				Field(1).
				Field(true).
				Array().
				Field(2).
				Field(false).
				Add().
				Array().
				Field(3).
				Array().
				Field(4).
				Array().
				Field(5).
				Add().
				Add().
				Add().
				As(`arr_1`).
				End().
				Field(`a`, `A`).
				Logger()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Notice().
						Array().
						Field(1).
						Field(true).
						Array().
						Field(2).
						Field(false).
						Add().
						Array().
						Field(3).
						Array().
						Field(4).
						Array().
						Field(5).
						Add().
						Add().
						Add().
						As(`arr_2`).
						End().
						Field(`b`, `B`).
						Log(`msg 1`)
				}
			})
		},
	}).Run(b)
}

func BenchmarkArray_Bool(b *testing.B) {
	// corresponding to TestLogger_simple/array_bool
	(VariantBenchmark{
		Baseline: func(b *testing.B) {
			logger := zerolog.New(io.Discard)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().
						Array(`k`, zerolog.Arr().
							Bool(true).
							Bool(false).
							Bool(false).
							Bool(true)).
						Msg(shortMessage)
				}
			})
		},
		Generic: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard)))
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().
						Array().
						Bool(true).
						Bool(false).
						Bool(false).
						Bool(true).
						As(`k`).
						End().
						Log(shortMessage)
				}
			})
		},
		Interface: func(b *testing.B) {
			logger := L.New(L.WithZerolog(zerolog.New(io.Discard))).Logger()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().
						Array().
						Bool(true).
						Bool(false).
						Bool(false).
						Bool(true).
						As(`k`).
						End().
						Log(shortMessage)
				}
			})
		},
	}).Run(b)
}

func BenchmarkEventTemplate1_Enabled(b *testing.B) {
	benchmarkEventTemplate(b, 1, true)
}

func BenchmarkEventTemplate1_Disabled(b *testing.B) {
	benchmarkEventTemplate(b, 1, false)
}

func BenchmarkEventTemplate2_Enabled(b *testing.B) {
	benchmarkEventTemplate(b, 2, true)
}

func BenchmarkEventTemplate2_Disabled(b *testing.B) {
	benchmarkEventTemplate(b, 2, false)
}

func BenchmarkEventTemplate3_Enabled(b *testing.B) {
	benchmarkEventTemplate(b, 3, true)
}

func BenchmarkEventTemplate3_Disabled(b *testing.B) {
	benchmarkEventTemplate(b, 3, false)
}

func BenchmarkEventTemplate4_Enabled(b *testing.B) {
	benchmarkEventTemplate(b, 4, true)
}

func BenchmarkEventTemplate4_Disabled(b *testing.B) {
	benchmarkEventTemplate(b, 4, false)
}

func BenchmarkEventTemplate5_Enabled(b *testing.B) {
	benchmarkEventTemplate(b, 5, true)
}

func BenchmarkEventTemplate5_Disabled(b *testing.B) {
	benchmarkEventTemplate(b, 5, false)
}

func benchmarkEventTemplate(b *testing.B, num int, enabled bool) {
	(VariantBenchmark{
		Baseline: func(b *testing.B) {
			if eventTemplates[num-1].Baseline == nil {
				b.Skip(`not implemented`)
			}
			logger := newEventTemplateBaselineLogger(io.Discard, enabled)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					eventTemplates[num-1].Baseline(logger)
				}
			})
		},
		Generic: func(b *testing.B) {
			if eventTemplates[num-1].Generic == nil {
				b.Skip(`not implemented`)
			}
			logger := newEventTemplateGenericLogger(io.Discard, enabled)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					eventTemplates[num-1].Generic(logger)
				}
			})
		},
		Interface: func(b *testing.B) {
			if eventTemplates[num-1].Interface == nil {
				b.Skip(`not implemented`)
			}
			logger := newEventTemplateInterfaceLogger(io.Discard, enabled)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					eventTemplates[num-1].Interface(logger)
				}
			})
		},
		Variants: []Variant{
			{`callForNesting`, func(b *testing.B) {
				if eventTemplates[num-1].CallForNesting == nil {
					b.Skip(`not implemented`)
				}
				logger := newEventTemplateGenericLogger(io.Discard, enabled)
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						eventTemplates[num-1].CallForNesting(logger)
					}
				})
			}},
			{`callForNestingSansChain`, func(b *testing.B) {
				if eventTemplates[num-1].CallForNestingSansChain == nil {
					b.Skip(`not implemented`)
				}
				logger := newEventTemplateGenericLogger(io.Discard, enabled)
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						eventTemplates[num-1].CallForNestingSansChain(logger)
					}
				})
			}},
		},
	}).Run(b)
}
