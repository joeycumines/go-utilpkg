package tournament

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
)

// BenchmarkPromisesV2 measures bounded, synchronized workloads. Every measured
// operation starts with fresh promise state, and every scheduled reaction is
// observed before the next operation begins.
func BenchmarkPromisesV2(b *testing.B) {
	for _, impl := range PromiseImplementations() {
		b.Run(impl.Name, func(b *testing.B) {
			b.Run("ChainDepth100EndToEnd", func(b *testing.B) {
				_, js, cleanup := startPromiseBenchmarkLoop(b)
				defer cleanup()
				deadline := time.NewTimer(30 * time.Minute)
				defer deadline.Stop()
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					root, resolve, _ := impl.Factory(js)
					current := root
					for range 100 {
						current = current.Then(identityPromiseValue, nil)
					}
					done := make(chan struct{}, 1)
					tail := current.Then(func(value any) any {
						done <- struct{}{}
						return value
					}, nil)
					resolve(1)
					waitPromiseBenchmarkDeadline(b, done, deadline.C)
					waitPromiseBenchmarkBarrier(b, js, deadline.C)
					runtime.KeepAlive(tail)
				}
			})

			b.Run("ResolvedHandlerEndToEnd", func(b *testing.B) {
				_, js, cleanup := startPromiseBenchmarkLoop(b)
				defer cleanup()
				deadline := time.NewTimer(30 * time.Minute)
				defer deadline.Stop()
				promise, resolve, _ := impl.Factory(js)
				resolve(1)
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					done := make(chan struct{}, 1)
					child := promise.Then(func(value any) any {
						done <- struct{}{}
						return value
					}, nil)
					waitPromiseBenchmarkDeadline(b, done, deadline.C)
					waitPromiseBenchmarkBarrier(b, js, deadline.C)
					runtime.KeepAlive(child)
				}
			})

			b.Run("FanOut100EndToEnd", func(b *testing.B) {
				_, js, cleanup := startPromiseBenchmarkLoop(b)
				defer cleanup()
				deadline := time.NewTimer(30 * time.Minute)
				defer deadline.Stop()
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					promise, resolve, _ := impl.Factory(js)
					done := make(chan struct{}, 100)
					var children [100]Promise
					for index := range children {
						children[index] = promise.Then(func(value any) any {
							done <- struct{}{}
							return value
						}, nil)
					}
					resolve(1)
					for range 100 {
						waitPromiseBenchmarkDeadline(b, done, deadline.C)
					}
					waitPromiseBenchmarkBarrier(b, js, deadline.C)
					runtime.KeepAlive(children)
				}
			})

			b.Run("Race100EndToEnd", func(b *testing.B) {
				if impl.Race == nil {
					b.Skipf("%s has no historical Race combinator", impl.VariantID)
				}
				_, js, cleanup := startPromiseBenchmarkLoop(b)
				defer cleanup()
				deadline := time.NewTimer(30 * time.Minute)
				defer deadline.Stop()
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					promise, settle := impl.Race(js, 100)
					done := make(chan struct{}, 1)
					child := promise.Then(func(value any) any {
						done <- struct{}{}
						return value
					}, nil)
					drained, err := settle()
					if err != nil {
						b.Fatalf("settle race inputs: %v", err)
					}
					waitPromiseBenchmarkDeadline(b, drained, deadline.C)
					waitPromiseBenchmarkDeadline(b, done, deadline.C)
					waitPromiseBenchmarkBarrier(b, js, deadline.C)
					runtime.KeepAlive(child)
				}
			})
		})
	}
}

func identityPromiseValue(value any) any {
	return value
}

func startPromiseBenchmarkLoop(b *testing.B) (*eventloop.Loop, *eventloop.JS, func()) {
	b.Helper()
	loop, err := eventloop.New()
	if err != nil {
		b.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	cleanup := benchmarkLoopCleanup(b, loop, runDone, "promise benchmark")
	b.Cleanup(cleanup)
	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		b.Fatal(err)
	}
	waitPromiseBenchmarkSignal(b, ready)
	js, err := eventloop.NewJS(loop)
	if err != nil {
		b.Fatal(err)
	}
	return loop, js, cleanup
}

func waitPromiseBenchmarkSignal(b *testing.B, signal <-chan struct{}) {
	b.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		b.Fatal("promise benchmark synchronization timed out")
	}
}

func waitPromiseBenchmarkDeadline(t testing.TB, signal <-chan struct{}, deadline <-chan time.Time) {
	t.Helper()
	select {
	case <-signal:
	case <-deadline:
		t.Fatal("promise benchmark synchronization timed out")
	}
}

func waitPromiseBenchmarkBarrier(b *testing.B, js *eventloop.JS, deadline <-chan time.Time) {
	b.Helper()
	drained := make(chan struct{})
	if err := js.QueueMicrotask(func() { close(drained) }); err != nil {
		b.Fatalf("queue benchmark barrier: %v", err)
	}
	waitPromiseBenchmarkDeadline(b, drained, deadline)
}
