package tournament

import (
	"testing"
	"time"
)

// TestPanicIsolation verifies that an external task panic is contained and a
// later task executes on the same loop.
func TestPanicIsolation(t *testing.T) {
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			start := time.Now()
			loop, cleanup := startTournamentTestLoop(t, impl)
			before := make(chan struct{})
			panicEntered := make(chan struct{})
			after := make(chan struct{})
			if err := loop.Submit(func() { close(before) }); err != nil {
				t.Fatalf("Submit before panic: %v", err)
			}
			waitTournamentSignal(t, before, "pre-panic callback")
			if err := loop.Submit(func() {
				defer close(panicEntered)
				panic("intentional tournament panic")
			}); err != nil {
				t.Fatalf("Submit panic callback: %v", err)
			}
			waitTournamentSignal(t, panicEntered, "panic callback entry")
			if err := loop.Submit(func() { close(after) }); err != nil {
				t.Fatalf("Submit after panic: %v", err)
			}
			waitTournamentSignal(t, after, "post-panic callback")
			cleanup()
			GetResults().RecordTest(TestResult{
				TestName:       "PanicIsolation",
				VariantID:      impl.VariantID,
				Implementation: impl.Name,
				Passed:         true,
				Duration:       time.Since(start),
			})
		})
	}
}

// TestPanicIsolationMultiple proves exact entry of every panic task and exact
// execution of every interleaved normal task.
func TestPanicIsolationMultiple(t *testing.T) {
	const panicCount = 10
	const normalPerPanic = 10
	const normalCount = panicCount * normalPerPanic
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			start := time.Now()
			loop, cleanup := startTournamentTestLoop(t, impl)
			panicDone := make(chan struct{}, panicCount)
			normalDone := make(chan struct{}, normalCount)
			for range panicCount {
				if err := loop.Submit(func() {
					defer func() { panicDone <- struct{}{} }()
					panic("intentional tournament panic")
				}); err != nil {
					t.Fatalf("Submit panic callback: %v", err)
				}
				for range normalPerPanic {
					if err := loop.Submit(func() { normalDone <- struct{}{} }); err != nil {
						t.Fatalf("Submit normal callback: %v", err)
					}
				}
			}
			waitTournamentCount(t, panicDone, panicCount, "panic callback entries")
			waitTournamentCount(t, normalDone, normalCount, "normal callbacks after panics")
			cleanup()
			GetResults().RecordTest(TestResult{
				TestName:       "PanicIsolationMultiple",
				VariantID:      impl.VariantID,
				Implementation: impl.Name,
				Passed:         true,
				Duration:       time.Since(start),
				Metrics: map[string]any{
					"panic_tasks":  panicCount,
					"normal_tasks": normalCount,
				},
			})
		})
	}
}

// TestPanicIsolationInternal verifies the same containment contract on the
// internal-priority admission path.
func TestPanicIsolationInternal(t *testing.T) {
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			start := time.Now()
			loop, cleanup := startTournamentTestLoop(t, impl)
			panicDone := make(chan struct{})
			after := make(chan struct{})
			if err := loop.SubmitInternal(func() {
				defer close(panicDone)
				panic("intentional internal tournament panic")
			}); err != nil {
				t.Fatalf("SubmitInternal panic callback: %v", err)
			}
			waitTournamentSignal(t, panicDone, "internal panic callback entry")
			if err := loop.SubmitInternal(func() { close(after) }); err != nil {
				t.Fatalf("SubmitInternal after panic: %v", err)
			}
			waitTournamentSignal(t, after, "internal post-panic callback")
			cleanup()
			GetResults().RecordTest(TestResult{
				TestName:       "PanicIsolationInternal",
				VariantID:      impl.VariantID,
				Implementation: impl.Name,
				Passed:         true,
				Duration:       time.Since(start),
			})
		})
	}
}
