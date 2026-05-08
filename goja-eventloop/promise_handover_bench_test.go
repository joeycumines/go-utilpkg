package gojaeventloop

// Promise job handover benchmarks measure native Goja async/await and Promise
// reaction jobs, the adapter's canonical exit-gated handover, direct diagnostic
// handover variants, and the event-loop scheduling floor. Bind retains Goja's
// native Promise constructor and always installs the canonical handover.
//
// All direct runtime setup finishes before the loop starts. Once callbacks may
// run, bound product measurements submit through Adapter.Submit and unbound
// component diagnostics use the loop's serialized logical callback owner.

import (
	"context"
	"errors"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// benchEnv holds the test fixture for a benchmark run.
type benchEnv struct {
	loop         *goeventloop.Loop
	runtime      *goja.Runtime
	adapter      *Adapter
	runDone      chan error
	promiseErrCh chan error
	cleanupOnce  sync.Once
	runStarted   bool
	bound        bool
}

func newBenchEnv(tb testing.TB) *benchEnv {
	tb.Helper()

	loop := goeventloop.New()
	env := &benchEnv{
		loop:         loop,
		runDone:      make(chan error, 1),
		promiseErrCh: make(chan error, 1),
	}
	tb.Cleanup(func() { env.teardown(tb) })

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		tb.Fatalf("failed to create adapter: %v", err)
	}
	env.runtime = rt
	env.adapter = adapter
	return env
}

func newDiagnosticBenchEnv(tb testing.TB, implementation promiseJobHandoverImplementation) *benchEnv {
	tb.Helper()
	env := newBenchEnv(tb)
	implementation.install(env.loop, env.runtime, env.adapter, func(err error) {
		select {
		case env.promiseErrCh <- err:
		default:
		}
	})
	return env
}

func newBoundBenchEnv(tb testing.TB) *benchEnv {
	tb.Helper()
	env := newBenchEnv(tb)
	if err := env.adapter.Bind(); err != nil {
		tb.Fatalf("failed to bind adapter: %v", err)
	}
	env.bound = true
	return env
}

// start transfers runtime access after benchmark-specific setup to the
// serialized logical callback owner.
func (e *benchEnv) start(tb testing.TB) {
	tb.Helper()
	if e.runStarted {
		tb.Fatal("benchmark loop started more than once")
	}
	e.runStarted = true
	go func() { e.runDone <- e.loop.Run(context.Background()) }()

	warmupDone := make(chan struct{})
	if err := e.loop.ScheduleMicrotask(func() { close(warmupDone) }); err != nil {
		tb.Fatalf("warmup ScheduleMicrotask failed: %v", err)
	}
	select {
	case <-warmupDone:
	case <-time.After(2 * time.Second):
		tb.Fatal("warmup timeout: loop did not process microtask")
	}
}

// teardown shuts down the loop and waits for the goroutine to exit.
func (e *benchEnv) teardown(tb testing.TB) {
	tb.Helper()
	e.cleanupOnce.Do(func() {
		if !e.runStarted {
			if err := awaitBenchmarkLifecycle(e.loop.Close, 5*time.Second); err != nil && !errors.Is(err, goeventloop.ErrLoopTerminated) {
				tb.Errorf("loop Close failed: %v", err)
			}
			return
		}
		result := terminateBenchmarkLoop(e.loop, e.runDone, 5*time.Second)
		if result.shutdownErr != nil && !errors.Is(result.shutdownErr, goeventloop.ErrLoopTerminated) {
			tb.Errorf("loop Shutdown failed: %v", result.shutdownErr)
		}
		if result.closeErr != nil && !errors.Is(result.closeErr, goeventloop.ErrLoopTerminated) {
			tb.Errorf("loop fallback Close failed: %v", result.closeErr)
		}
		if result.runErr != nil {
			tb.Errorf("loop Run failed: %v", result.runErr)
		}
	})
}

func (e *benchEnv) runOnOwner(fn func(*goja.Runtime) error) <-chan error {
	errCh := make(chan error, 1)
	var err error
	if e.bound {
		err = e.adapter.Submit(func(runtime *goja.Runtime) {
			errCh <- fn(runtime)
		})
	} else {
		err = e.loop.Submit(func() {
			errCh <- fn(e.runtime)
		})
	}
	if err != nil {
		errCh <- err
	}
	return errCh
}

func (e *benchEnv) waitOperation(tb testing.TB, result <-chan error, deadline <-chan time.Time, label string) {
	tb.Helper()
	select {
	case err := <-e.promiseErrCh:
		tb.Fatalf("%s promise job failed: %v", label, err)
	case err := <-result:
		if err != nil {
			tb.Fatalf("%s failed: %v", label, err)
		}
	case <-deadline:
		tb.Fatalf("%s timed out", label)
	}
}

func (e *benchEnv) waitPromiseResult(tb testing.TB, resultCh <-chan int64, deadline <-chan time.Time, label string) int64 {
	tb.Helper()
	select {
	case err := <-e.promiseErrCh:
		tb.Fatalf("%s promise job failed: %v", label, err)
		return 0
	case v := <-resultCh:
		return v
	case <-deadline:
		tb.Fatalf("%s result timed out", label)
		return 0
	}
}

func (e *benchEnv) waitIterationTail(tb testing.TB, deadline <-chan time.Time, label string) {
	tb.Helper()
	done, err := e.scheduleIterationTail()
	if err != nil {
		tb.Fatalf("%s checkpoint admission failed: %v", label, err)
	}
	select {
	case err := <-e.promiseErrCh:
		tb.Fatalf("%s promise job failed: %v", label, err)
	case <-done:
	case <-deadline:
		tb.Fatalf("%s checkpoint timed out", label)
	}
	select {
	case err := <-e.promiseErrCh:
		tb.Fatalf("%s late promise job failed: %v", label, err)
	default:
	}
}

func (e *benchEnv) scheduleIterationTail() (<-chan struct{}, error) {
	done := make(chan struct{})
	if err := e.loop.ScheduleMicrotaskCheckpoint(func() { close(done) }); err != nil {
		return nil, err
	}
	return done, nil
}

func (e *benchEnv) waitDone(tb testing.TB, done <-chan struct{}, deadline <-chan time.Time, label string) {
	tb.Helper()
	select {
	case err := <-e.promiseErrCh:
		tb.Fatalf("%s promise job failed: %v", label, err)
	case <-done:
	case <-deadline:
		tb.Fatalf("%s timed out", label)
	}
}

func TestPromiseJobBenchmarkIterationTailWaitsJobReturn(t *testing.T) {
	env := newDiagnosticBenchEnv(t, promiseJobHandoverExitGated)
	env.start(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var returned atomic.Bool
	enqueue := promiseJobHandoverExitGated.direct(env.loop, env.runtime, env.adapter, func(err error) {
		select {
		case env.promiseErrCh <- err:
		default:
		}
	})
	enqueue(func() {
		close(entered)
		<-release
		returned.Store(true)
	})
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	env.waitDone(t, entered, deadline.C, "promise job entry")
	tail, err := env.scheduleIterationTail()
	if err != nil {
		t.Fatalf("schedule iteration tail: %v", err)
	}
	select {
	case <-tail:
		t.Fatal("iteration tail ran before Promise job returned")
	default:
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-env.promiseErrCh:
		t.Fatalf("Promise job failed: %v", err)
	case <-tail:
	case <-deadline.C:
		t.Fatal("iteration tail did not run after Promise job returned")
	}
	if !returned.Load() {
		t.Fatal("iteration tail ran before Promise job return was published")
	}
}

// BenchmarkNativeAsyncAwaitResolve measures an unbound component fixture using
// Goja's native Promise implementation and the canonical handover directly.
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
	benchmarkNativeAsyncAwaitResolve(b, promiseJobHandoverExitGated)
}

func benchmarkNativeAsyncAwaitResolve(b *testing.B, implementation promiseJobHandoverImplementation) {
	env := newDiagnosticBenchEnv(b, implementation)

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
	env.start(b)

	goruntime.GC() // start with clean heap
	b.ReportAllocs()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	b.ResetTimer()

	for range b.N {
		// The serialized callback owner runs the program before its queued
		// Promise job resumes the async function.
		errCh := env.runOnOwner(func(runtime *goja.Runtime) error {
			_, err := runtime.RunProgram(callProgram)
			return err
		})
		// Phase 1: wait for RunProgram to complete (nil error = success)
		env.waitOperation(b, errCh, deadline.C, "native async/await RunProgram")
		// Phase 2: wait for the async result (microtask processed by loop).
		v := env.waitPromiseResult(b, resultCh, deadline.C, "native async/await")
		if v != 42 {
			b.Fatalf("expected 42, got %d", v)
		}
		env.waitIterationTail(b, deadline.C, "native async/await")
	}
	b.StopTimer()
}

// BenchmarkAdapterAsyncAwaitResolve measures the bound product path: Goja's
// native Promise constructor and the canonical exit-gated handover installed
// atomically by Bind.
func BenchmarkAdapterAsyncAwaitResolve(b *testing.B) {
	benchmarkAdapterAsyncAwaitResolve(b)
}

func benchmarkAdapterAsyncAwaitResolve(b *testing.B) {
	env := newBoundBenchEnv(b)

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
	env.start(b)

	goruntime.GC()
	b.ReportAllocs()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	b.ResetTimer()

	for range b.N {
		errCh := env.runOnOwner(func(runtime *goja.Runtime) error {
			_, err := runtime.RunProgram(callProgram)
			return err
		})
		env.waitOperation(b, errCh, deadline.C, "adapter async/await RunProgram")
		v := env.waitPromiseResult(b, resultCh, deadline.C, "adapter async/await")
		if v != 42 {
			b.Fatalf("expected 42, got %d", v)
		}
		env.waitIterationTail(b, deadline.C, "adapter async/await")
	}
	b.StopTimer()
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
	benchmarkNativePromiseThenChain(b, promiseJobHandoverExitGated)
}

// BenchmarkAdapterPromiseThenChain measures the corresponding bound product
// path with the canonical handover selected by Bind.
func BenchmarkAdapterPromiseThenChain(b *testing.B) {
	benchmarkPromiseThenChain(b, newBoundBenchEnv(b), "adapter")
}

func benchmarkNativePromiseThenChain(b *testing.B, implementation promiseJobHandoverImplementation) {
	benchmarkPromiseThenChain(b, newDiagnosticBenchEnv(b, implementation), "native")
}

func benchmarkPromiseThenChain(b *testing.B, env *benchEnv, label string) {

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
	env.start(b)

	goruntime.GC()
	b.ReportAllocs()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	b.ResetTimer()

	for range b.N {
		errCh := env.runOnOwner(func(runtime *goja.Runtime) error {
			_, err := runtime.RunProgram(callProgram)
			return err
		})
		env.waitOperation(b, errCh, deadline.C, label+" promise chain RunProgram")
		v := env.waitPromiseResult(b, resultCh, deadline.C, label+" promise chain")
		if v != int64(chainDepth) {
			b.Fatalf("expected %d, got %d", chainDepth, v)
		}
		env.waitIterationTail(b, deadline.C, label+" promise chain")
	}
	b.StopTimer()
}

// BenchmarkPromiseJobEnqueuerOverhead isolates the enqueuer handover cost by
// calling the enqueuer directly with a no-op job, bypassing goja's promise
// machinery. Reported allocations and latency include direct handover,
// ScheduleMicrotask, RunPromiseJob, callback-owner transfer, and signaling.
func BenchmarkPromiseJobEnqueuerOverhead(b *testing.B) {
	benchmarkPromiseJobEnqueuerOverhead(b, promiseJobHandoverExitGated)
}

func benchmarkPromiseJobEnqueuerOverhead(b *testing.B, implementation promiseJobHandoverImplementation) {
	env := newDiagnosticBenchEnv(b, implementation)

	rt := env.runtime
	loop := env.loop
	enqueuer := implementation.direct(loop, rt, env.adapter, func(err error) {
		select {
		case env.promiseErrCh <- err:
		default:
		}
	})

	// Reusable completion channel — avoids per-iteration channel allocation
	done := make(chan struct{}, 1)
	env.start(b)

	goruntime.GC()
	b.ReportAllocs()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	b.ResetTimer()

	for range b.N {
		enqueuer(func() {
			select {
			case done <- struct{}{}:
			default:
			}
		})
		env.waitDone(b, done, deadline.C, "promise job enqueuer")
		env.waitIterationTail(b, deadline.C, "promise job enqueuer")
	}
	b.StopTimer()
}

// BenchmarkScheduleMicrotaskBaseline is the scheduling floor. It omits Promise
// creation, handover wrapping, and RunPromiseJob.
func BenchmarkScheduleMicrotaskBaseline(b *testing.B) {
	env := newDiagnosticBenchEnv(b, promiseJobHandoverExitGated)

	done := make(chan struct{}, 1)
	env.start(b)

	goruntime.GC()
	b.ReportAllocs()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	b.ResetTimer()

	for range b.N {
		if err := env.loop.ScheduleMicrotask(func() {
			select {
			case done <- struct{}{}:
			default:
			}
		}); err != nil {
			b.Fatalf("ScheduleMicrotask failed: %v", err)
		}
		env.waitDone(b, done, deadline.C, "ScheduleMicrotask baseline")
		env.waitIterationTail(b, deadline.C, "ScheduleMicrotask baseline")
	}
	b.StopTimer()
}

func BenchmarkPromiseJobHandoverNativeAsyncAwaitResolve(b *testing.B) {
	for _, implementation := range promiseJobHandoverBenchmarkImplementations() {
		b.Run(implementation.id, func(b *testing.B) {
			benchmarkNativeAsyncAwaitResolve(b, implementation)
		})
	}
}

func BenchmarkPromiseJobHandoverAdapterAsyncAwaitResolve(b *testing.B) {
	b.Run(promiseJobHandoverExitGated.id, benchmarkAdapterAsyncAwaitResolve)
}

func BenchmarkPromiseJobHandoverNativePromiseThenChain(b *testing.B) {
	for _, implementation := range promiseJobHandoverBenchmarkImplementations() {
		b.Run(implementation.id, func(b *testing.B) {
			benchmarkNativePromiseThenChain(b, implementation)
		})
	}
}

func BenchmarkPromiseJobHandoverEnqueuerOverhead(b *testing.B) {
	for _, implementation := range promiseJobHandoverBenchmarkImplementations() {
		if implementation.direct == nil {
			continue
		}
		b.Run(implementation.id, func(b *testing.B) {
			benchmarkPromiseJobEnqueuerOverhead(b, implementation)
		})
	}
}
