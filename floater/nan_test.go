package floater

import (
	"math/big"
	"testing"
)

func TestCmp(t *testing.T) {
	for _, tc := range [...]struct {
		name     string
		x, y     *big.Float
		expected int
	}{
		{
			name: `0 and 0`,
			x:    big.NewFloat(0), y: big.NewFloat(0),
			expected: 0,
		},
		{
			name:     `nil and nil`,
			expected: 0,
		},
		{
			name:     `-inf and nil`,
			x:        new(big.Float).SetInf(true),
			expected: 1,
		},
		{
			name:     `nil and -inf`,
			y:        new(big.Float).SetInf(true),
			expected: -1,
		},
		{
			name:     `1 and nil`,
			x:        big.NewFloat(1),
			expected: 1,
		},
		{
			name:     `nil and 1`,
			y:        big.NewFloat(1),
			expected: -1,
		},
		{
			name:     `5 and 10`,
			x:        big.NewFloat(5),
			y:        big.NewFloat(10),
			expected: -1,
		},
		{
			name:     `-5 and -10`,
			x:        big.NewFloat(-5),
			y:        big.NewFloat(-10),
			expected: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual := Cmp(tc.x, tc.y)
			if actual != tc.expected {
				t.Errorf(`got %d, want %d`, actual, tc.expected)
			}
		})
	}
}

func TestCopy(t *testing.T) {
	if v := Copy(nil); v != nil {
		t.Fatal(v)
	}
	a := new(big.Float).SetPrec(123).SetInt64(5324)
	if v := Copy(a); v == a || v.Prec() != 123 || v.Cmp(a) != 0 || v.Cmp(new(big.Float).SetInt64(5324)) != 0 {
		t.Fatal(v)
	}
}
