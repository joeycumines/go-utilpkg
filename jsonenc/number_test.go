package jsonenc

import (
	"encoding/json"
	"math"
	"math/rand"
	"testing"
)

var float64Tests = []struct {
	Name string
	Val  float64
	Want string
}{
	{
		Name: "Positive integer",
		Val:  1234.0,
		Want: "1234",
	},
	{
		Name: "Negative integer",
		Val:  -5678.0,
		Want: "-5678",
	},
	{
		Name: "Positive decimal",
		Val:  12.3456,
		Want: "12.3456",
	},
	{
		Name: "Negative decimal",
		Val:  -78.9012,
		Want: "-78.9012",
	},
	{
		Name: "Large positive number",
		Val:  123456789.0,
		Want: "123456789",
	},
	{
		Name: "Large negative number",
		Val:  -987654321.0,
		Want: "-987654321",
	},
	{
		Name: "Zero",
		Val:  0.0,
		Want: "0",
	},
	{
		Name: "Smallest positive value",
		Val:  math.SmallestNonzeroFloat64,
		Want: "5e-324",
	},
	{
		Name: "Largest positive value",
		Val:  math.MaxFloat64,
		Want: "1.7976931348623157e+308",
	},
	{
		Name: "Smallest negative value",
		Val:  -math.SmallestNonzeroFloat64,
		Want: "-5e-324",
	},
	{
		Name: "Largest negative value",
		Val:  -math.MaxFloat64,
		Want: "-1.7976931348623157e+308",
	},
	{
		Name: "NaN",
		Val:  math.NaN(),
		Want: `"NaN"`,
	},
	{
		Name: "Infinity",
		Val:  math.Inf(1),
		Want: `"Infinity"`,
	},
	{
		Name: "-Infinity",
		Val:  math.Inf(-1),
		Want: `"-Infinity"`,
	},
	{
		Name: `Clean up e-09 to e-9 case 1`,
		Val:  1e-9,
		Want: "1e-9",
	},
	{
		Name: `Clean up e-09 to e-9 case 2`,
		Val:  -2.236734e-9,
		Want: "-2.236734e-9",
	},
	{
		Name: `Exponent bound case 1e20`,
		Val:  1e20,
		Want: "100000000000000000000",
	},
	{
		Name: `Exponent bound case 1e21`,
		Val:  1e+21,
		Want: "1e+21",
	},
	{
		Name: `Exponent bound case 1e-6`,
		Val:  1e-6,
		Want: "0.000001",
	},
	{
		Name: `Exponent bound case 1e-7`,
		Val:  1e-7,
		Want: "1e-7",
	},
}

func TestAppendFloat64(t *testing.T) {
	for _, tc := range float64Tests {
		t.Run(tc.Name, func(t *testing.T) {
			var b []byte
			b = AppendFloat64(b, tc.Val)
			if s := string(b); tc.Want != s {
				t.Errorf("%q", s)
			}
		})
	}
}

func FuzzAppendFloat64(f *testing.F) {
	for _, tc := range float64Tests {
		f.Add(tc.Val)
	}
	f.Fuzz(func(t *testing.T, val float64) {
		actual := AppendFloat64(nil, val)
		if len(actual) == 0 {
			t.Fatal("empty buffer")
		}

		if actual[0] == '"' {
			switch string(actual) {
			case `"NaN"`:
				if !math.IsNaN(val) {
					t.Fatalf("expected %v got NaN", val)
				}
			case `"Infinity"`:
				if !math.IsInf(val, 1) {
					t.Fatalf("expected %v got +Inf", val)
				}
			case `"-Infinity"`:
				if !math.IsInf(val, -1) {
					t.Fatalf("expected %v got -Inf", val)
				}
			default:
				t.Fatalf("unexpected string: %s", actual)
			}
			return
		}

		if expected, err := json.Marshal(val); err != nil {
			t.Error(err)
		} else if string(actual) != string(expected) {
			t.Errorf("expected %s, got %s", expected, actual)
		}

		var parsed float64
		if err := json.Unmarshal(actual, &parsed); err != nil {
			t.Fatal(err)
		}

		if parsed != val {
			t.Fatalf("expected %v, got %v", val, parsed)
		}
	})
}

var float32Tests = []struct {
	Name string
	Val  float32
	Want string
}{
	{
		Name: "Positive integer",
		Val:  1234.0,
		Want: "1234",
	},
	{
		Name: "Negative integer",
		Val:  -5678.0,
		Want: "-5678",
	},
	{
		Name: "Positive decimal",
		Val:  12.3456,
		Want: "12.3456",
	},
	{
		Name: "Negative decimal",
		Val:  -78.9012,
		Want: "-78.9012",
	},
	{
		Name: "Large positive number",
		Val:  123456789.0,
		Want: "123456790",
	},
	{
		Name: "Large negative number",
		Val:  -987654321.0,
		Want: "-987654340",
	},
	{
		Name: "Zero",
		Val:  0.0,
		Want: "0",
	},
	{
		Name: "Smallest positive value",
		Val:  math.SmallestNonzeroFloat32,
		Want: "1e-45",
	},
	{
		Name: "Largest positive value",
		Val:  math.MaxFloat32,
		Want: "3.4028235e+38",
	},
	{
		Name: "Smallest negative value",
		Val:  -math.SmallestNonzeroFloat32,
		Want: "-1e-45",
	},
	{
		Name: "Largest negative value",
		Val:  -math.MaxFloat32,
		Want: "-3.4028235e+38",
	},
	{
		Name: "NaN",
		Val:  float32(math.NaN()),
		Want: `"NaN"`,
	},
	{
		Name: "Infinity",
		Val:  float32(math.Inf(1)),
		Want: `"Infinity"`,
	},
	{
		Name: "-Infinity",
		Val:  float32(math.Inf(-1)),
		Want: `"-Infinity"`,
	},
	{
		Name: `Clean up e-09 to e-9 case 1`,
		Val:  1e-9,
		Want: "1e-9",
	},
	{
		Name: `Clean up e-09 to e-9 case 2`,
		Val:  -2.236734e-9,
		Want: "-2.236734e-9",
	},
	{
		Name: `Exponent bound case 1e20`,
		Val:  1e20,
		Want: "100000000000000000000",
	},
	{
		Name: `Exponent bound case 1e21`,
		Val:  1e+21,
		Want: "1e+21",
	},
	{
		Name: `Exponent bound case 1e-6`,
		Val:  1e-6,
		Want: "0.000001",
	},
	{
		Name: `Exponent bound case 1e-7`,
		Val:  1e-7,
		Want: "1e-7",
	},
}

func TestAppendFloat32(t *testing.T) {
	for _, tc := range float32Tests {
		t.Run(tc.Name, func(t *testing.T) {
			var b []byte
			b = AppendFloat32(b, tc.Val)
			if s := string(b); tc.Want != s {
				t.Errorf("%q", s)
			}
		})
	}
}

func FuzzAppendFloat32(f *testing.F) {
	for _, tc := range float32Tests {
		f.Add(tc.Val)
	}
	f.Fuzz(func(t *testing.T, val float32) {
		actual := AppendFloat32(nil, val)
		if len(actual) == 0 {
			t.Fatal("empty buffer")
		}

		if actual[0] == '"' {
			val := float64(val)
			switch string(actual) {
			case `"NaN"`:
				if !math.IsNaN(val) {
					t.Fatalf("expected %v got NaN", val)
				}
			case `"Infinity"`:
				if !math.IsInf(val, 1) {
					t.Fatalf("expected %v got +Inf", val)
				}
			case `"-Infinity"`:
				if !math.IsInf(val, -1) {
					t.Fatalf("expected %v got -Inf", val)
				}
			default:
				t.Fatalf("unexpected string: %s", actual)
			}
			return
		}

		if expected, err := json.Marshal(val); err != nil {
			t.Error(err)
		} else if string(actual) != string(expected) {
			t.Errorf("expected %s, got %s", expected, actual)
		}

		var parsed float32
		if err := json.Unmarshal(actual, &parsed); err != nil {
			t.Fatal(err)
		}

		if parsed != val {
			t.Fatalf("expected %v, got %v", val, parsed)
		}
	})
}

func generateFloat32s(n int) []float32 {
	floats := make([]float32, n)
	for i := range n {
		floats[i] = rand.Float32()
	}
	return floats
}

func generateFloat64s(n int) []float64 {
	floats := make([]float64, n)
	for i := range n {
		floats[i] = rand.Float64()
	}
	return floats
}

func BenchmarkAppendFloat32(b *testing.B) {
	floats := append(generateFloat32s(5000), float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1)))
	dst := make([]byte, 0, 128)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, f := range floats {
			dst = AppendFloat32(dst[:0], f)
		}
	}
}

func BenchmarkAppendFloat64(b *testing.B) {
	floats := append(generateFloat64s(5000), math.NaN(), math.Inf(1), math.Inf(-1))
	dst := make([]byte, 0, 128)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, f := range floats {
			dst = AppendFloat64(dst[:0], f)
		}
	}
}
