package gojaeventloop

// Promise job handover benchmarks.
//
// This file benchmarks the "promise job handover" — the path where goja's
// native promise machinery (async/await, native Promise.then) calls our
// enqueuer, which wraps the job in a closure and schedules it as a microtask
// on the event loop:
//
//	runtime.SetPromiseJobEnqueuer(func(job func()) {
//	    _ = loop.ScheduleMicrotask(func() {
//	        _ = runtime.RunPromiseJob(job)
//	    })
//	})
//
// The per-job allocation is the inner closure `func() { ... }` which captures
// `job` and `runtime`, escaping to the heap because it's stored in the
// microtask ring buffer.
//
// === Benchmark Design Rationale ===
//
// 1. BenchmarkNativeAsyncAwaitResolve
//    async/await WITHOUT adapter Bind(). Promise is goja's native.
//    Isolates: goja native promise alloc + enqueuer closure + ScheduleMicrotask
//              + RunPromiseJob + loop round-trip.
//    Confounds: goja's newPromiseReactionJob closure, Promise object alloc,
//               reaction record alloc. These are FIXED costs we can't optimize.
//
// 2. BenchmarkAdapterAsyncAwaitResolve
//    async/await WITH adapter Bind(). Promise is overridden with ChainedPromise.
//    When async/await awaits a Promise.resolve(), the thenable interop path
//    (resolveThenable) adds extra allocations.
//    Isolates: everything in #1 PLUS ChainedPromise interop overhead.
//    The DELTA between #1 and #2 measures the ChainedPromise interop cost.
//
// 3. BenchmarkNativePromiseThenChain
//    Native goja Promise.then chain (without Bind). Each .then() creates a
//    reaction job that goes through the enqueuer.
//    Isolates: 10x enqueuer invocations per iteration (high-throughput test).
//    Confounds: same as #1, but multiplied by chain depth.
//
// 4. BenchmarkPromiseJobEnqueuerOverhead
//    Calls the enqueuer DIRECTLY with a no-op job, bypassing goja's promise
//    machinery entirely. No Promise objects, no reaction records, no
//    newPromiseReactionJob closures.
//    Isolates: JUST the enqueuer closure alloc + ScheduleMicrotask + RunPromiseJob
//              + loop round-trip.
//    THIS IS THE BENCHMARK THAT BEST ISOLATES THE HANDOVER COST.
//
// 5. BenchmarkScheduleMicrotaskBaseline
//    Just loop.ScheduleMicrotask with a no-op function. No enqueuer closure,
//    no RunPromiseJob. This is the FLOOR — the minimum scheduling cost.
//    The DELTA between #4 and #5 measures: enqueuer closure alloc + RunPromiseJob
//    overhead.
//
// === Confounding Factors Addressed ===
//
// - loop.Run(ctx) overhead: minimized by channel-based warmup (no time.Sleep),
//   and by using a single long-lived loop goroutine across all iterations.
// - RACE CONDITION: RunProgram and RunPromiseJob both access the goja runtime,
//   which is NOT goroutine-safe. The async/await benchmarks (#1-#3) use
//   loop.Submit to run RunProgram ON the loop goroutine, ensuring sequential
//   access. The enqueuer overhead benchmarks (#4-#5) don't access the runtime
//   from the test goroutine, so they call the enqueuer directly.
// - Channel signaling overhead: present in all benchmarks as a constant, so
//   deltas between benchmarks are meaningful.
// - GC pressure: goruntime.GC() called before b.ResetTimer() to start clean.
// - Script compilation: pre-compiled with goja.Compile() before the benchmark
//   loop; only RunProgram() is called per iteration (via loop.Submit).
// - Goroutine scheduling: the loop goroutine processes microtasks after the
//   submitted RunProgram task returns, ensuring no concurrent runtime access.

import (
	"context"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// benchEnv holds the test fixture for a benchmark run.
type benchEnv struct {
	loop    *goeventloop.Loop
	runtime *goja.Runtime
	adapter *Adapter
	wg      sync.WaitGroup
}

// setupBenchEnv creates a loop, runtime, and adapter (optionally bound),
// starts the loop goroutine, and performs a channel-based warmup.
// The bind parameter controls whether adapter.Bind() is called (which
// overrides the global Promise constructor with ChainedPromise).
func setupBenchEnv(b *testing.B, bind bool) *benchEnv {
	b.Helper()

	loop, err := goeventloop.New()
	if err != nil {
		b.Fatalf("failed to create loop: %v", err)
	}

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		b.Fatalf("failed to create adapter: %v", err)
	}

	if bind {
		if err := adapter.Bind(); err != nil {
			b.Fatalf("failed to bind adapter: %v", err)
		}
	}

	env := &benchEnv{
		loop:    loop,
		runtime: rt,
		adapter: adapter,
	}

	// Start the loop goroutine. Run blocks until the loop is shut down.
	env.wg.Go(func() { _ = loop.Run(context.Background()) })

	// Channel-based warmup — no time.Sleep.
	// Confirms the loop goroutine is running and ready to process microtasks.
	warmupDone := make(chan struct{})
	if err := loop.ScheduleMicrotask(func() { close(warmupDone) }); err != nil {
		b.Fatalf("warmup ScheduleMicrotask failed: %v", err)
	}
	select {
	case <-warmupDone:
	case <-time.After(2 * time.Second):
		b.Fatal("warmup timeout: loop did not process microtask")
	}

	return env
}

// teardown shuts down the loop and waits for the goroutine to exit.
func (e *benchEnv) teardown(b *testing.B) {
	b.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.loop.Shutdown(ctx); err != nil {
		b.Fatalf("loop shutdown failed: %v", err)
	}
	e.wg.Wait()
}

// runOnLoop submits fn to the loop goroutine and returns a channel that
// receives fn's error (or nil on success). This is necessary because
// RunProgram and RunPromiseJob both access the goja runtime, which is NOT
// goroutine-safe. By running RunProgram on the loop goroutine via Submit,
// we ensure that RunProgram completes before the loop processes any microtasks
// (which call RunPromiseJob), eliminating the race.
func (e *benchEnv) runOnLoop(fn func() error) <-chan error {
	errCh := make(chan error, 1)
	_ = e.loop.Submit(func() {
		errCh <- fn()
	})
	return errCh
}

// BenchmarkNativeAsyncAwaitResolve measures async/await WITHOUT adapter Bind().
// Promise is goja's native implementation. This isolates the enqueuer handover
// from the ChainedPromise interop path.
//
// Per iteration: 1 async function call -> 1 await -> 1 promise job via enqueuer.
//
// Measures: goja native promise alloc + enqueuer closure alloc + ScheduleMicrotask
//   - loop round-trip + RunPromiseJob + channel signaling.
//
// Confounds: goja's Promise object, reaction record, newPromiseReactionJob closure
//
//	are included but are FIXED costs (not optimizable in adapter.go).
//	loop.Submit overhead is also included (task queue push + wakeup).
func BenchmarkNativeAsyncAwaitResolve(b *testing.B) {
	env := setupBenchEnv(b, false) // no Bind — native Promise
	defer env.teardown(b)

	// Pre-compile the async function definition
	defProgram, err := goja.Compile("define", `
		async function compute() {
			const v = await Promise.resolve(42);
			reportResult(v);
		}
	`, false)
	if err != nil {
		b.Fatalf("failed to compile definition: %v", err)
	}
	if _, err := env.runtime.RunProgram(defProgram); err != nil {
		b.Fatalf("failed to run definition: %v", err)
	}

	// Pre-compile the call expression
	callProgram, err := goja.Compile("call", `compute()`, false)
	if err != nil {
		b.Fatalf("failed to compile call: %v", err)
	}

	// Reusable result channel — avoids per-iteration channel allocation
	resultCh := make(chan int64, 1)
	_ = env.runtime.Set("reportResult", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0).ToInteger()
		return goja.Undefined()
	})

	goruntime.GC() // start with clean heap
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Run RunProgram on the loop goroutine to avoid concurrent runtime access.
		// After RunProgram returns, the loop processes the enqueued microtask
		// (RunPromiseJob), which resumes the async function and calls reportResult.
		errCh := env.runOnLoop(func() error {
			_, err := env.runtime.RunProgram(callProgram)
			return err
		})
		// Phase 1: wait for RunProgram to complete (nil error = success)
		if err := <-errCh; err != nil {
			b.Fatalf("iteration %d: RunProgram failed: %v", i, err)
		}
		// Phase 2: wait for the async result (microtask processed by loop)
		select {
		case v := <-resultCh:
			if v != 42 {
				b.Fatalf("expected 42, got %d", v)
			}
		case <-time.After(5 * time.Second):
			b.Fatalf("iteration %d: timeout waiting for async result", i)
		}
	}
}

// BenchmarkAdapterAsyncAwaitResolve measures async/await WITH adapter Bind().
// Promise is overridden with ChainedPromise-based implementation.
// When async/await awaits a Promise.resolve(), the thenable interop path
// (resolveThenable) is taken, adding extra allocations.
//
// Per iteration: 1 async function call -> 1 await -> thenable interop + promise job.
//
// Measures: adapter ChainedPromise alloc + resolveThenable + enqueuer closure alloc
//   - ScheduleMicrotask + loop round-trip + RunPromiseJob + channel signaling.
//
// The DELTA between this and BenchmarkNativeAsyncAwaitResolve measures the
// ChainedPromise interop cost (resolveThenable, GojaWrapPromise, etc.).
func BenchmarkAdapterAsyncAwaitResolve(b *testing.B) {
	env := setupBenchEnv(b, true) // with Bind — ChainedPromise
	defer env.teardown(b)

	defProgram, err := goja.Compile("define", `
		async function compute() {
			const v = await Promise.resolve(42);
			reportResult(v);
		}
	`, false)
	if err != nil {
		b.Fatalf("failed to compile definition: %v", err)
	}
	if _, err := env.runtime.RunProgram(defProgram); err != nil {
		b.Fatalf("failed to run definition: %v", err)
	}

	callProgram, err := goja.Compile("call", `compute()`, false)
	if err != nil {
		b.Fatalf("failed to compile call: %v", err)
	}

	resultCh := make(chan int64, 1)
	_ = env.runtime.Set("reportResult", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0).ToInteger()
		return goja.Undefined()
	})

	goruntime.GC()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		errCh := env.runOnLoop(func() error {
			_, err := env.runtime.RunProgram(callProgram)
			return err
		})
		if err := <-errCh; err != nil {
			b.Fatalf("iteration %d: RunProgram failed: %v", i, err)
		}
		select {
		case v := <-resultCh:
			if v != 42 {
				b.Fatalf("expected 42, got %d", v)
			}
		case <-time.After(5 * time.Second):
			b.Fatalf("iteration %d: timeout waiting for async result", i)
		}
	}
}

// BenchmarkNativePromiseThenChain measures a chain of native goja Promise.then()
// calls WITHOUT adapter Bind(). Each .then() creates a reaction job that goes
// through the enqueuer.
//
// Per iteration: 1 resolved promise + chainDepth .then() calls = chainDepth
// enqueuer invocations. This is a high-throughput test that stresses the
// enqueuer path.
//
// Measures: chainDepth x (reaction record alloc + enqueuer closure alloc +
//
//	ScheduleMicrotask + RunPromiseJob) + loop round-trips.
//
// The chainDepth constant can be adjusted to scale the enqueuer workload.
func BenchmarkNativePromiseThenChain(b *testing.B) {
	env := setupBenchEnv(b, false) // no Bind — native Promise
	defer env.teardown(b)

	const chainDepth = 10

	// Build: Promise.resolve(0).then(x=>x+1).then(x=>x+1)...then(reportResult)
	var jsCode strings.Builder
	jsCode.WriteString("Promise.resolve(0)")
	for range chainDepth {
		jsCode.WriteString(".then(x => x + 1)")
	}
	jsCode.WriteString(".then(reportResult)")

	defProgram, err := goja.Compile("define", `
		function runChain() {
			`+jsCode.String()+`;
		}
	`, false)
	if err != nil {
		b.Fatalf("failed to compile: %v", err)
	}
	if _, err := env.runtime.RunProgram(defProgram); err != nil {
		b.Fatalf("failed to run definition: %v", err)
	}

	callProgram, err := goja.Compile("call", `runChain()`, false)
	if err != nil {
		b.Fatalf("failed to compile call: %v", err)
	}

	resultCh := make(chan int64, 1)
	_ = env.runtime.Set("reportResult", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0).ToInteger()
		return goja.Undefined()
	})

	goruntime.GC()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		errCh := env.runOnLoop(func() error {
			_, err := env.runtime.RunProgram(callProgram)
			return err
		})
		if err := <-errCh; err != nil {
			b.Fatalf("iteration %d: RunProgram failed: %v", i, err)
		}
		select {
		case v := <-resultCh:
			if v != int64(chainDepth) {
				b.Fatalf("expected %d, got %d", chainDepth, v)
			}
		case <-time.After(5 * time.Second):
			b.Fatalf("iteration %d: timeout waiting for chain result", i)
		}
	}
}

// BenchmarkPromiseJobEnqueuerOverhead isolates the enqueuer handover cost by
// calling the enqueuer directly with a no-op job, bypassing goja's promise
// machinery (no Promise objects, reaction records, or newPromiseReactionJob
// closures). It replicates the exact closure pattern shipped by the adapter
// (newPromiseJobEnqueuer in adapter.go), so it measures what the production
// code actually pays per promise job.
//
// Measures: closure alloc (1 heap object) + ScheduleMicrotask + RunPromiseJob
//   - loop round-trip.
//
// The DELTA between this and BenchmarkScheduleMicrotaskBaseline measures the
// enqueuer closure allocation + RunPromiseJob overhead — the cost the closure
// pattern adds on top of bare microtask scheduling. A queue+drainOne variant
// that eliminates the per-job closure alloc was evaluated and rejected: it
// introduces a secondary FIFO queue that is architecturally inconsistent with
// the rest of the codebase (every ScheduleMicrotask caller uses a closure),
// for a marginal ~1.5% reduction in async/await allocations.
func BenchmarkPromiseJobEnqueuerOverhead(b *testing.B) {
	env := setupBenchEnv(b, false) // enqueuer is set by New() regardless of Bind
	defer env.teardown(b)

	// Replicate the exact enqueuer closure from adapter.go
	rt := env.runtime
	loop := env.loop
	enqueuer := func(job func()) {
		_ = loop.ScheduleMicrotask(func() {
			_ = rt.RunPromiseJob(job)
		})
	}

	// Reusable completion channel — avoids per-iteration channel allocation
	done := make(chan struct{}, 1)

	goruntime.GC()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		enqueuer(func() {
			select {
			case done <- struct{}{}:
			default:
			}
		})
		<-done
	}
}

// BenchmarkScheduleMicrotaskBaseline measures JUST the ScheduleMicrotask path
// without the enqueuer closure wrapping or RunPromiseJob. This is the floor —
// the minimum possible cost for scheduling work on the loop.
//
// The DELTA between BenchmarkPromiseJobEnqueuerOverhead and this benchmark
// measures: enqueuer closure allocation + RunPromiseJob overhead.
//
// Per iteration: 1 ScheduleMicrotask call -> 1 microtask -> 1 callback.
func BenchmarkScheduleMicrotaskBaseline(b *testing.B) {
	env := setupBenchEnv(b, false)
	defer env.teardown(b)

	done := make(chan struct{}, 1)

	goruntime.GC()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = env.loop.ScheduleMicrotask(func() {
			select {
			case done <- struct{}{}:
			default:
			}
		})
		<-done
	}
}
