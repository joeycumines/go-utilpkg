package jsonenc

import (
	"math"
	"strconv"
)

func AppendFloat32(dst []byte, val float32) []byte {
	return appendFloat(dst, float64(val), 32)
}

func AppendFloat64(dst []byte, val float64) []byte {
	return appendFloat(dst, val, 64)
}

func appendFloat(dst []byte, val float64, bitSize int) []byte {
	// JSON strings are obviously not valid JSON numbers, but JSON numbers do not support NaN or Inf.
	switch {
	case math.IsNaN(val):
		return append(dst, `"NaN"`...)
	case math.IsInf(val, 1):
		return append(dst, `"Infinity"`...)
	case math.IsInf(val, -1):
		return append(dst, `"-Infinity"`...)
	}
	// see also https://cs.opensource.google/go/go/+/refs/tags/go1.20.3:src/encoding/json/encode.go;l=573
	fmt := byte('f')
	// Use float32 comparisons for underlying float32 value to get precise cutoffs right.
	if abs := math.Abs(val); abs != 0 {
		if bitSize == 64 && (abs < 1e-6 || abs >= 1e21) || bitSize == 32 && (float32(abs) < 1e-6 || float32(abs) >= 1e21) {
			fmt = 'e'
		}
	}
	dst = strconv.AppendFloat(dst, val, fmt, -1, bitSize)
	if fmt == 'e' {
		// Clean up e-09 to e-9
		n := len(dst)
		if n >= 4 && dst[n-4] == 'e' && dst[n-3] == '-' && dst[n-2] == '0' {
			dst[n-2] = dst[n-1]
			dst = dst[:n-1]
		}
	}
	return dst
}
