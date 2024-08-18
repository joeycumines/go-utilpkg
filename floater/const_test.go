package floater

import (
	"math"
	"math/big"
	"testing"
)

func TestMaxBigFloat_float64(t *testing.T) {
	const prec = 53
	if big.NewFloat(0).Prec() != prec {
		t.Fatal(`expected 53 precision`)
	}
	v := maxBigFloat(prec, 1024)
	f, acc := v.Float64()
	if acc != big.Exact {
		t.Error(`expected exact accuracy got`, acc)
	}
	if f != math.MaxFloat64 {
		t.Errorf(`got %g, want %g`, f, math.MaxFloat64)
	}
}

func TestMaxBigFloat_float32(t *testing.T) {
	const prec = 24
	v := maxBigFloat(prec, 128)
	f, acc := v.Float32()
	if acc != big.Exact {
		t.Error(`expected exact accuracy got`, acc)
	}
	if f != math.MaxFloat32 {
		t.Errorf(`got %g, want %g`, f, math.MaxFloat32)
	}
}
