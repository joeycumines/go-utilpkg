package eventloop

import (
	"fmt"
	"testing"
)

// BenchmarkIngressPop benchmarks the popLocked operation at various queue depths.
// This verifies O(1) complexity - ns/op should be constant regardless of depth.
//
// SUCCESS CRITERIA: Depth-10000 must have roughly same ns/op as Depth-100
func BenchmarkIngressPop(b *testing.B) {
	counts := []int{100, 1000, 10000, 100000}

	for _, count := range counts {
		b.Run(fmt.Sprintf("Depth-%d", count), func(b *testing.B) {
			q := &IngressQueue{}
			for i := 0; i < count; i++ {
				q.Push(Task{Runnable: func() {}})
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if q.Length() == 0 {
					b.StopTimer()
					for k := 0; k < count; k++ {
						q.Push(Task{Runnable: func() {}})
					}
					b.StartTimer()
				}
				_, _ = q.popLocked()
			}
		})
	}
}
