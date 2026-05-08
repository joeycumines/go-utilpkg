package eventloop

import "testing"

func TestPercentileIndex(t *testing.T) {
	tests := []struct {
		n    int
		p    int
		want int
	}{
		{n: 1, p: 0, want: 0},
		{n: 1, p: 50, want: 0},
		{n: 1, p: 100, want: 0},
		{n: 2, p: 0, want: 0},
		{n: 2, p: 49, want: 0},
		{n: 2, p: 50, want: 1},
		{n: 2, p: 99, want: 1},
		{n: 2, p: 100, want: 1},
		{n: 5, p: 0, want: 0},
		{n: 5, p: 19, want: 0},
		{n: 5, p: 20, want: 1},
		{n: 5, p: 50, want: 2},
		{n: 5, p: 90, want: 4},
		{n: 5, p: 99, want: 4},
		{n: 5, p: 100, want: 4},
		{n: 100, p: 1, want: 1},
		{n: 100, p: 50, want: 50},
		{n: 100, p: 99, want: 99},
		{n: 100, p: 100, want: 99},
	}
	for _, test := range tests {
		if got := percentileIndex(test.n, test.p); got != test.want {
			t.Errorf("percentileIndex(%d, %d) = %d, want %d", test.n, test.p, got, test.want)
		}
	}
}
