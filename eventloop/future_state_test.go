package eventloop

import (
	"math"
	"testing"
)

// The raw promise state encoding is a hand-maintained protocol contract:
// transitional states are negative [Settlement] values (contiguous from -3
// to -1) and every other raw value is a final [Settlement] value. These
// tests pin the exact mapping so a future refactor cannot drift.

func TestPromiseSettlementRawConstants(t *testing.T) {
	if promiseSettlementClaimed != -1 {
		t.Errorf("promiseSettlementClaimed = %d, want -1", promiseSettlementClaimed)
	}
	if promiseFulfilledPublishing != -2 {
		t.Errorf("promiseFulfilledPublishing = %d, want -2", promiseFulfilledPublishing)
	}
	if promiseRejectedPublishing != -3 {
		t.Errorf("promiseRejectedPublishing = %d, want -3", promiseRejectedPublishing)
	}
}

func TestPromiseStateMapping(t *testing.T) {
	cases := []struct {
		value int32
		want  Settlement
	}{
		{math.MinInt32, Settlement(math.MinInt32)},
		{-4, Settlement(-4)},
		{-3, Rejected},
		{-2, Fulfilled},
		{-1, Pending},
		{0, Pending},
		{1, Fulfilled},
		{2, Rejected},
		{3, Settlement(3)},
		{math.MaxInt32, Settlement(math.MaxInt32)},
	}
	for _, c := range cases {
		if got := promiseState(c.value); got != c.want {
			t.Errorf("promiseState(%d) = %v, want %v", c.value, got, c.want)
		}
	}
}

func TestPromisePendingMapping(t *testing.T) {
	cases := []struct {
		value int32
		want  bool
	}{
		{math.MinInt32, false},
		{-4, false},
		{-3, false},
		{-2, false},
		{-1, true},
		{0, true},
		{1, false},
		{2, false},
		{3, false},
		{math.MaxInt32, false},
	}
	for _, c := range cases {
		if got := promisePending(c.value); got != c.want {
			t.Errorf("promisePending(%d) = %v, want %v", c.value, got, c.want)
		}
	}
}
