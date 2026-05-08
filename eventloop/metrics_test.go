package eventloop

import "testing"

// BenchmarkMetricsCollection measures the callback-execution component with
// runtime metric collection enabled.
func BenchmarkMetricsCollection(b *testing.B) {
	benchmarkCallbackMetrics(b, true)
}

// BenchmarkNoMetrics measures the same component without metrics.
func BenchmarkNoMetrics(b *testing.B) {
	benchmarkCallbackMetrics(b, false)
}

func benchmarkCallbackMetrics(b *testing.B, enabled bool) {
	var options []LoopOption
	if enabled {
		options = append(options, WithMetrics(true))
	}
	loop := New(options...)
	b.Cleanup(func() {
		if err := loop.Close(); err != nil {
			b.Errorf("Close: %v", err)
		}
	})

	callback := func() {}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		loop.safeExecute(callback)
	}
}
