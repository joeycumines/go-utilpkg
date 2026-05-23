package gojaeventloop

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

type promiseJobHandoverImplementation struct {
	id            string
	name          string
	install       func(*goeventloop.Loop, *goja.Runtime, *Adapter, promiseJobErrorReporter)
	direct        func(*goeventloop.Loop, *goja.Runtime, *Adapter, promiseJobErrorReporter) goja.PromiseJobEnqueuer
	canonical     bool
	benchmarkable bool
}

var promiseJobHandoverNative = promiseJobHandoverImplementation{
	id: "goja.promise-job.native-internal-queue", name: "NativeInternalQueue",
	install: func(_ *goeventloop.Loop, runtime *goja.Runtime, _ *Adapter, _ promiseJobErrorReporter) {
		runtime.SetPromiseJobEnqueuer(nil)
	},
	canonical: false, benchmarkable: false,
}

var promiseJobHandoverUnobserved = promiseJobHandoverImplementation{
	id: "goja.promise-job.unobserved-per-job-closure", name: "UnobservedPerJobClosure",
	install: func(loop *goeventloop.Loop, runtime *goja.Runtime, adapter *Adapter, report promiseJobErrorReporter) {
		runtime.SetPromiseJobEnqueuer(unobservedPromiseJobEnqueuer(loop, runtime))
	},
	direct: func(loop *goeventloop.Loop, runtime *goja.Runtime, _ *Adapter, _ promiseJobErrorReporter) goja.PromiseJobEnqueuer {
		return unobservedPromiseJobEnqueuer(loop, runtime)
	},
	canonical: false, benchmarkable: true,
}

var promiseJobHandoverObserved = promiseJobHandoverImplementation{
	id: "goja.promise-job.observed-ungated-closure", name: "ObservedUngatedClosure",
	install: func(loop *goeventloop.Loop, runtime *goja.Runtime, _ *Adapter, report promiseJobErrorReporter) {
		runtime.SetPromiseJobEnqueuer(newPromiseJobEnqueuer(loop, runtime, report))
	},
	direct: func(loop *goeventloop.Loop, runtime *goja.Runtime, _ *Adapter, report promiseJobErrorReporter) goja.PromiseJobEnqueuer {
		return newPromiseJobEnqueuer(loop, runtime, report)
	},
	canonical: false, benchmarkable: true,
}

var promiseJobHandoverExitGated = promiseJobHandoverImplementation{
	id: "goja.promise-job.exit-gated-closure", name: "ExitGatedClosure",
	install: func(loop *goeventloop.Loop, runtime *goja.Runtime, adapter *Adapter, report promiseJobErrorReporter) {
		runtime.SetPromiseJobEnqueuer(newPromiseJobEnqueuerWithGate(loop, runtime, report, adapter.exiting.Load))
	},
	direct: func(loop *goeventloop.Loop, runtime *goja.Runtime, adapter *Adapter, report promiseJobErrorReporter) goja.PromiseJobEnqueuer {
		return newPromiseJobEnqueuerWithGate(loop, runtime, report, adapter.exiting.Load)
	},
	canonical: true, benchmarkable: true,
}

func promiseJobHandoverImplementations() []promiseJobHandoverImplementation {
	return []promiseJobHandoverImplementation{
		promiseJobHandoverExitGated,
		promiseJobHandoverNative,
		promiseJobHandoverObserved,
		promiseJobHandoverUnobserved,
	}
}

func promiseJobHandoverBenchmarkImplementations() []promiseJobHandoverImplementation {
	implementations := promiseJobHandoverImplementations()
	result := make([]promiseJobHandoverImplementation, 0, len(implementations))
	for _, implementation := range implementations {
		if implementation.benchmarkable {
			result = append(result, implementation)
		}
	}
	return result
}

func unobservedPromiseJobEnqueuer(loop *goeventloop.Loop, runtime *goja.Runtime) goja.PromiseJobEnqueuer {
	return func(job func()) {
		_ = loop.ScheduleMicrotask(func() {
			_ = runtime.RunPromiseJob(job)
		})
	}
}

func TestPromiseJobHandoverImplementations(t *testing.T) {
	implementations := promiseJobHandoverImplementations()
	if len(implementations) != 4 {
		t.Fatalf("implementation count = %d, want 4", len(implementations))
	}
	if !slices.IsSortedFunc(implementations, func(left, right promiseJobHandoverImplementation) int {
		return compareString(left.id, right.id)
	}) {
		t.Fatal("Promise-job handover implementations are not sorted by stable ID")
	}
	seen := make(map[string]struct{}, len(implementations))
	canonical := 0
	for _, implementation := range implementations {
		if implementation.id == "" || implementation.name == "" || implementation.install == nil {
			t.Errorf("incomplete implementation: %+v", implementation)
		}
		if _, duplicate := seen[implementation.id]; duplicate {
			t.Errorf("duplicate implementation ID %q", implementation.id)
		}
		seen[implementation.id] = struct{}{}
		if implementation.canonical {
			canonical++
		}
		if implementation.id == promiseJobHandoverNative.id {
			if implementation.benchmarkable || implementation.direct != nil {
				t.Error("native internal queue must remain excluded from direct diagnostics")
			}
		} else if !implementation.benchmarkable || implementation.direct == nil {
			t.Errorf("closure implementation %q has invalid capability metadata", implementation.id)
		}
	}
	if canonical != 1 || !promiseJobHandoverExitGated.canonical {
		t.Fatalf("canonical implementation count = %d, want exit-gated only", canonical)
	}
}

func TestPromiseJobHandoverBenchmarkImplementations(t *testing.T) {
	implementations := promiseJobHandoverBenchmarkImplementations()
	if len(implementations) != 3 {
		t.Fatalf("benchmark implementation count = %d, want 3", len(implementations))
	}
	for _, implementation := range implementations {
		if !implementation.benchmarkable || implementation.id == promiseJobHandoverNative.id {
			t.Errorf("benchmark admitted an ineligible diagnostic implementation %+v", implementation)
		}
	}
}

func TestPromiseJobHandoverDirectDiagnosticsRunJobs(t *testing.T) {
	for _, implementation := range promiseJobHandoverImplementations() {
		if implementation.direct == nil {
			continue
		}
		t.Run(implementation.name, func(t *testing.T) {
			loop, err := goeventloop.New()
			if err != nil {
				t.Fatal(err)
			}
			runtime := goja.New()
			adapter, err := New(loop, runtime)
			if err != nil {
				t.Fatal(err)
			}
			runLoopForTest(t, loop)
			errors := make(chan error, 1)
			enqueue := implementation.direct(loop, runtime, adapter, func(err error) { errors <- err })
			done := make(chan struct{}, 1)
			enqueue(func() { done <- struct{}{} })
			select {
			case <-done:
			case err := <-errors:
				t.Fatalf("Promise job failed: %v", err)
			case <-testDeadline(t):
				t.Fatal("Promise job did not run")
			}
		})
	}
}

func compareString(left, right string) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func runLoopForTest(t *testing.T, loop *goeventloop.Loop) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()
	t.Cleanup(func() {
		result := terminateBenchmarkLoop(loop, done, 2*time.Second)
		if result.shutdownErr != nil && !errors.Is(result.shutdownErr, goeventloop.ErrLoopTerminated) {
			t.Errorf("shutdown loop: %v", result.shutdownErr)
		}
		if result.closeErr != nil && !errors.Is(result.closeErr, goeventloop.ErrLoopTerminated) {
			t.Errorf("fallback close loop: %v", result.closeErr)
		}
		if result.runErr != nil {
			t.Errorf("run loop: %v", result.runErr)
		}
	})
}

func testDeadline(t *testing.T) <-chan time.Time {
	t.Helper()
	return time.After(2 * time.Second)
}
