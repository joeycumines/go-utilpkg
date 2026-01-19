package eventloop

import (
	"sync"
	"testing"
)

// BenchmarkChunkedIngress_PushPop benchmarks push/pop on ChunkedIngress.
func BenchmarkChunkedIngress_PushPop(b *testing.B) {
	q := NewChunkedIngress()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(Task{Runnable: func() {}})
		q.Pop()
	}
}

// BenchmarkChunkedIngress_Push benchmarks push on ChunkedIngress.
func BenchmarkChunkedIngress_Push(b *testing.B) {
	q := NewChunkedIngress()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(Task{Runnable: func() {}})
	}
}

// BenchmarkChunkedIngress_Pop benchmarks pop on ChunkedIngress.
func BenchmarkChunkedIngress_Pop(b *testing.B) {
	q := NewChunkedIngress()

	// Pre-fill queue
	for i := 0; i < b.N; i++ {
		q.Push(Task{Runnable: func() {}})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Pop()
	}
}

// BenchmarkChunkedIngress_ParallelWithSync benchmarks parallel push WITH proper synchronization.
func BenchmarkChunkedIngress_ParallelWithSync(b *testing.B) {
	q := NewChunkedIngress()
	var mu sync.Mutex

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// In real code, this mutex would be Loop's externalMu
			mu.Lock()
			q.Push(Task{Runnable: func() {}})
			mu.Unlock()
		}
	})
}
