package eventloop

import (
	"runtime"
	"strconv"
	"testing"
)

func BenchmarkJSAdapterRegistrationLiveSet(b *testing.B) {
	for _, liveSet := range []int{64, retainedRegistryHighWater} {
		b.Run(strconv.Itoa(liveSet), func(b *testing.B) {
			benchmarkJSAdapterRegistrationLiveSet(b, liveSet)
		})
	}
}

func benchmarkJSAdapterRegistrationLiveSet(b *testing.B, liveSet int) {
	// Loop construction and terminal disposal remain outside the timer. Each
	// timed batch keeps every prior adapter live, so the measurement exposes
	// registration cost as the weak registry grows to the named live set.
	adapters := make([]*JS, liveSet)
	b.ReportAllocs()
	b.ResetTimer()
	b.StopTimer()
	for completed := 0; completed < b.N; {
		count := min(liveSet, b.N-completed)
		loop := New()
		b.StartTimer()
		for index := range count {
			adapters[index] = NewJS(loop)
		}
		b.StopTimer()
		runtime.KeepAlive(adapters[:count])
		if err := loop.Close(); err != nil {
			b.Fatalf("Close: %v", err)
		}
		clear(adapters[:count])
		completed += count
	}
}
