package eventloop

import (
	"context"
	"testing"
	"time"
)

func BenchmarkAutoExitImmediate(b *testing.B) {
	benchmarkAutoExitImmediate(b, context.Background())
}

func BenchmarkAutoExitImmediateCancelableContext(b *testing.B) {
	ctx := b.Context()
	benchmarkAutoExitImmediate(b, ctx)
}

func benchmarkAutoExitImmediate(b *testing.B, ctx context.Context) {
	watchdog := time.AfterFunc(30*time.Minute, func() {
		panic(b.Name() + " timed out")
	})
	defer watchdog.Stop()

	b.ReportAllocs()
	for b.Loop() {
		loop := New(WithAutoExit(true))
		if err := loop.Run(ctx); err != nil {
			b.Fatalf("Run: %v", err)
		}
		if state := loop.State(); state != StateTerminated {
			b.Fatalf("State = %v, want %v", state, StateTerminated)
		}
	}
}
