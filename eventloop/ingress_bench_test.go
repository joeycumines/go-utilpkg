package eventloop

import (
	"testing"
)

// BenchmarkChunkedIngress_PushPop benchmarks push/pop on ChunkedIngress.
func BenchmarkChunkedIngress_PushPop(b *testing.B) {
	q := NewChunkedIngress()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(func() {})
		q.Pop()
	}
}

// BenchmarkChunkedIngress_Push benchmarks push on ChunkedIngress.
func BenchmarkChunkedIngress_Push(b *testing.B) {
	q := NewChunkedIngress()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(func() {})
	}
}

// BenchmarkChunkedIngress_Pop benchmarks pop on ChunkedIngress.
func BenchmarkChunkedIngress_Pop(b *testing.B) {
	q := NewChunkedIngress()

	// Pre-fill queue
	for i := 0; i < b.N; i++ {
		q.Push(func() {})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Pop()
	}
}

// BenchmarkChunkedIngress_Parallel benchmarks parallel push/pop.
func BenchmarkChunkedIngress_Parallel(b *testing.B) {
	q := NewChunkedIngress()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.Push(func() {})
		}
	})
}
