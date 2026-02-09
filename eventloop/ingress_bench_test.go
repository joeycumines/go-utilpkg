package eventloop

import (
	"sync"
	"testing"
)

// Benchmark_chunkedIngress_PushPop benchmarks push/pop on chunkedIngress.
func Benchmark_chunkedIngress_PushPop(b *testing.B) {
	q := newChunkedIngress()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(func() {})
		q.Pop()
	}
}

// Benchmark_chunkedIngress_Push benchmarks push on chunkedIngress.
func Benchmark_chunkedIngress_Push(b *testing.B) {
	q := newChunkedIngress()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(func() {})
	}
}

// Benchmark_chunkedIngress_Pop benchmarks pop on chunkedIngress.
func Benchmark_chunkedIngress_Pop(b *testing.B) {
	q := newChunkedIngress()

	// Pre-fill queue
	for i := 0; i < b.N; i++ {
		q.Push(func() {})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Pop()
	}
}

// Benchmark_chunkedIngress_ParallelWithSync benchmarks parallel push WITH proper synchronization.
func Benchmark_chunkedIngress_ParallelWithSync(b *testing.B) {
	q := newChunkedIngress()
	var mu sync.Mutex

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// In real code, this mutex would be Loop's externalMu
			mu.Lock()
			q.Push(func() {})
			mu.Unlock()
		}
	})
}
