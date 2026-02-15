package logiface_test

import (
	"github.com/joeycumines/stumpy"
	"math"
	"sync/atomic"
	"testing"
	"time"
)

type nopWriter struct {
	writeCount uint64
}

func (s *nopWriter) WriteCount() uint64 {
	return atomic.LoadUint64(&s.writeCount)
}

func (s *nopWriter) Write(p []byte) (int, error) {
	atomic.AddUint64(&s.writeCount, 1)
	return len(p), nil
}

func BenchmarkBuilder_Limit_callerCategoryRateLimit(b *testing.B) {
	windowConfigs := []struct {
		name    string
		limits  map[time.Duration]int
		wantMax uint64
	}{
		{"NoLimit", nil, math.MaxUint64},
		{"5MinLimit", map[time.Duration]int{time.Minute * 5: 150}, 150},
		{"10MinLimit", map[time.Duration]int{time.Minute * 10: 200}, 200},
		{"25MinLimit", map[time.Duration]int{time.Minute * 25: 250}, 250},
		{"AllLimits", map[time.Duration]int{
			time.Minute * 5:  150,
			time.Minute * 10: 200,
			time.Minute * 25: 250,
		}, 150},
	}

	for _, config := range windowConfigs {
		b.Run(config.name, func(b *testing.B) {
			var writer nopWriter
			logger := stumpy.L.New(
				stumpy.L.WithStumpy(stumpy.WithWriter(&writer)),
				stumpy.L.WithCategoryRateLimits(config.limits),
			)

			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					logger.Info().
						Limit().
						Log("The quick brown fox jumps over the lazy dog")
				}
			})

			b.StopTimer()

			if n := writer.WriteCount(); n > config.wantMax || (len(config.limits) == 0 && n != uint64(b.N)) {
				b.Fatalf("b.N=%d: unexpected write count: %d", b.N, n)
			}
		})
	}
}
