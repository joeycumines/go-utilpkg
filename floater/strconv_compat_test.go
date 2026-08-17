package floater

import (
	"math"
	"math/big"
	"strconv"
	"testing"
)

// interestingFloat64s yields a deterministic, structured set of float64 values
// covering subnormals, every exponent band, edge mantissas, and the special
// values. It is the ground-truth input set for the strconv-compatibility tests.
func interestingFloat64s() []float64 {
	var out []float64
	add := func(v float64) { out = append(out, v) }
	add(0)
	add(math.Copysign(0, -1))
	for e := uint64(0); e <= 2047; e++ {
		mants := []uint64{0, 1, 2, 3, 0x7, 0x123, 0xabcdef, 0x100000, 0x1fffff, 0xfffff}
		if e == 0 || e == 2047 {
			mants = append(mants, 0x55555, 0xaaaaa)
		}
		for _, m := range mants {
			bits := (e << 52) | (m & 0xfffffffffffff)
			add(math.Float64frombits(bits))
			add(math.Float64frombits(bits | (1 << 63))) // negative
		}
	}
	return out
}

// TestAppendDecimalFloat_MatchesStrconv is the authoritative regression test
// that AppendDecimalFloat/Scientific/General produce byte-identical output to
// [strconv.FormatFloat] for every finite, float64-representable input, across
// the 'f', 'e', and 'g' verbs and a range of precisions including -1 (shortest
// round-trip, the case at the heart of golang/go#71245). For non-representable
// values the fallback path is a faithful [math/big.Float].Append passthrough.
func TestAppendDecimalFloat_MatchesStrconv(t *testing.T) {
	type verb struct {
		v    byte
		app  func([]byte, *big.Float, int) []byte
		size int
	}
	verbs := []verb{
		{'f', AppendDecimalFloat, 64},
		{'e', AppendDecimalScientific, 64},
		{'g', AppendDecimalGeneral, 64},
	}
	precs := []int{-1, 0, 1, 3, 6, 12, 17}
	mismatch := 0
	total := 0
	for _, v := range interestingFloat64s() {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		bf := new(big.Float).SetPrec(53).SetFloat64(v)
		if _, acc := bf.Float64(); acc != big.Exact {
			t.Fatalf("setup: %v not float64-exact", v)
		}
		for _, vb := range verbs {
			for _, prec := range precs {
				total++
				got := string(vb.app(nil, bf, prec))
				want := strconv.FormatFloat(v, vb.v, prec, vb.size)
				if got != want {
					mismatch++
					if mismatch <= 20 {
						t.Errorf("divergence v=%g (%c) prec=%d: got=%q want=%q", v, vb.v, prec, got, want)
					}
				}
			}
		}
	}
	if mismatch > 0 {
		t.Errorf("%d/%d divergences from strconv (expected 0)", mismatch, total)
	}
}

// TestAppendDecimalFloat_FallbackPassthrough confirms that for a value NOT
// exactly representable as float64, the formatter falls back to a faithful
// [math/big.Float].Append rendering (same verb and precision).
func TestAppendDecimalFloat_FallbackPassthrough(t *testing.T) {
	bf := new(big.Float).SetPrec(200).SetRat(big.NewRat(1, 3))
	if _, acc := bf.Float64(); acc == big.Exact {
		t.Fatalf("expected non-exact for %v", bf)
	}
	type verb struct {
		v   byte
		app func([]byte, *big.Float, int) []byte
	}
	verbs := []verb{
		{'f', AppendDecimalFloat}, {'e', AppendDecimalScientific}, {'g', AppendDecimalGeneral},
	}
	for _, vb := range verbs {
		for _, prec := range []int{-1, 0, 5, 20} {
			got := string(vb.app(nil, bf, prec))
			want := string(bf.Append(nil, vb.v, prec))
			if got != want {
				t.Errorf("fallback divergence (%c prec=%d): got=%q want=%q", vb.v, prec, got, want)
			}
		}
	}
}

// TestAppendDecimalFloat32_MatchesStrconv is the float32 analogue, including the
// denormal values central to golang/go#71245.
func TestAppendDecimalFloat32_MatchesStrconv(t *testing.T) {
	bad := 0
	total := 0
	for e := uint32(0); e <= 255; e++ {
		for _, m := range []uint32{0, 1, 2, 3, 0x7, 0x123, 0x400000, 0x12345, 0x7fffff} {
			v := float64(math.Float32frombits((e << 23) | (m & 0x7fffff)))
			if math.IsNaN(v) || math.IsInf(v, 0) {
				continue
			}
			bf := new(big.Float).SetPrec(24).SetFloat64(v)
			if _, acc := bf.Float32(); acc != big.Exact {
				continue
			}
			for _, prec := range []int{-1, 0, 1, 3, 6, 9} {
				total++
				got := string(AppendDecimalFloat32(nil, bf, prec))
				want := strconv.FormatFloat(v, 'f', prec, 32)
				if got != want {
					bad++
					if bad <= 20 {
						t.Errorf("f32 divergence v=%g prec=%d: got=%q want=%q", v, prec, got, want)
					}
				}
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d/%d float32 divergences from strconv (expected 0)", bad, total)
	}
}

// TestAppendDecimalFloat_InfAndNil pins the deliberate divergences from
// [strconv.FormatFloat]: infinities render as "Infinity"/"-Infinity" (strconv
// uses "+Inf"/"-Inf") for consistency with [FloatConv], and nil renders "<nil>".
func TestAppendDecimalFloat_InfAndNil(t *testing.T) {
	for _, c := range []struct {
		name string
		f    *big.Float
		want string
	}{
		{"+inf", new(big.Float).SetInf(false), "Infinity"},
		{"-inf", new(big.Float).SetInf(true), "-Infinity"},
		{"nil", nil, "<nil>"},
	} {
		if got := string(AppendDecimalFloat(nil, c.f, -1)); got != c.want {
			t.Errorf("%s: AppendDecimalFloat = %q, want %q", c.name, got, c.want)
		}
	}
}
