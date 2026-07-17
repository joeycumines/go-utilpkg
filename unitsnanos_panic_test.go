package floater

import (
	"math"
	"testing"
)

// TestAppendUnitsNanos_OutOfRangePanics pins the precondition that nanos must
// be in [-999999999, 999999999]: out-of-contract values (notably
// math.MinInt32, whose negation overflows int32 and previously produced
// garbage like "-0.00000000-2147483648") now panic with a clear message,
// matching the validation in [UnitsNanosToRat] and surfacing the caller bug
// instead of silently corrupting a monetary value.
func TestAppendUnitsNanos_OutOfRangePanics(t *testing.T) {
	for _, nanos := range []int32{math.MinInt32, 1000000000, -1000000000, math.MaxInt32} {
		t.Run("", func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("AppendUnitsNanos(0, %d) did not panic", nanos)
				}
			}()
			_ = AppendUnitsNanos(nil, 0, nanos)
		})
	}
}

// TestAppendUnitsNanos_BoundaryInRange confirms the boundary values
// ±999999999 are accepted (do not panic) and format correctly.
func TestAppendUnitsNanos_BoundaryInRange(t *testing.T) {
	for _, c := range []struct {
		units int64
		nanos int32
		want  string
	}{
		{0, 999999999, "0.999999999"},
		{0, -999999999, "-0.999999999"},
		{5, 999999999, "5.999999999"},
		{-5, -999999999, "-5.999999999"},
		{0, 1, "0.000000001"},
		{0, -1, "-0.000000001"},
	} {
		if got := string(AppendUnitsNanos(nil, c.units, c.nanos)); got != c.want {
			t.Errorf("AppendUnitsNanos(%d,%d) = %q, want %q", c.units, c.nanos, got, c.want)
		}
	}
}

// TestAppendUnitsNanos_SignMismatchPanics pins that when both units and nanos
// are non-zero they must share the same sign (matching [UnitsNanosToRat]'s
// validation). Previously AppendUnitsNanos silently normalised the sign, so
// (1,-1) and (1,1) both produced "1.000000001" — a silent collision of distinct
// (invalid-Money) states. It now panics to surface the caller bug. Cases where
// either is zero remain valid (the sign is taken from the non-zero side).
func TestAppendUnitsNanos_SignMismatchPanics(t *testing.T) {
	for _, c := range []struct {
		units int64
		nanos int32
	}{
		{1, -1},
		{-1, 1},
		{5, -999999999},
		{-5, 999999999},
		{math.MaxInt64, -1},
		{math.MinInt64, 1},
	} {
		t.Run("", func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("AppendUnitsNanos(%d,%d) did not panic on sign mismatch", c.units, c.nanos)
				}
			}()
			_ = AppendUnitsNanos(nil, c.units, c.nanos)
		})
	}
}

// TestAppendUnitsNanos_SignConsistentNoPanic confirms the sign-mismatch guard
// does not fire for legitimate sign-consistent inputs (incl. either side zero).
func TestAppendUnitsNanos_SignConsistentNoPanic(t *testing.T) {
	for _, c := range []struct {
		units int64
		nanos int32
	}{
		{1, 1}, {-1, -1}, {0, 1}, {0, -1}, {1, 0}, {-1, 0}, {0, 0},
	} {
		// must not panic
		_ = AppendUnitsNanos(nil, c.units, c.nanos)
	}
}
