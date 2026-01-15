package eventloop

import (
	"sync"
	"sync/atomic"
	"testing"
)

// =============================================================================
// REGRESSION TESTS FOR POLLER RACE CONDITIONS
//
// These tests are designed to FAIL on buggy code to prove the existence of
// the defects documented in scratch.md.
// =============================================================================

// TestPoller_Init_Race proves the initPoller CAS race condition.
//
// DEFECT #1 (CRITICAL): initPoller() CAS Returns Before Init() Completes
//
// The bug: initPoller uses atomic CAS to ensure only one thread initializes,
// but it incorrectly assumes that losing the CAS race means initialization is complete.
//
// Race Scenario:
//  1. Goroutine A calls initPoller. Wins CAS. Sets initialized to true.
//     Begins p.p.Init() (syscall, takes non-zero time).
//  2. Goroutine B calls initPoller. Loses CAS. Returns nil immediately.
//  3. Goroutine B proceeds to call RegisterFD.
//  4. CRASH: RegisterFD runs against a FastPoller that hasn't finished Init()
//
// FIX: Replace atomic.Bool + CompareAndSwap with sync.Once
//
// RUN: go test -v -count=100 -run TestPoller_Init_Race
func TestPoller_Init_Race(t *testing.T) {
	for i := 0; i < 100; i++ {
		p := &ioPoller{}
		start := make(chan struct{})
		var wg sync.WaitGroup
		var failures atomic.Int32
		var initErrors atomic.Int32

		for g := 0; g < 4; g++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				<-start

				if err := p.initPoller(); err != nil {
					initErrors.Add(1)
					return
				}

				defer func() {
					if r := recover(); r != nil {
						failures.Add(1)
						t.Logf("Goroutine %d panicked: %v", id, r)
					}
				}()

				err := p.p.RegisterFD(100+id, EventRead, func(IOEvents) {})
				if err != nil {
					t.Logf("Goroutine %d RegisterFD error: %v", id, err)
				}
			}(g)
		}

		close(start)
		wg.Wait()

		if failures.Load() > 0 {
			t.Fatalf("Iteration %d: Race condition detected! %d goroutines panicked",
				i, failures.Load())
		}

		_ = p.closePoller()
	}
}

// TestIOPollerClosedDataRace proves that the closed field is now atomic.
//
// DEFECT #2 (CRITICAL): Data Race on ioPoller.closed - NOW FIXED
//
// The bug was: The closed field was a non-atomic bool with concurrent access.
// The fix: closed is now atomic.Bool using Store(true)/Load()
//
// This test verifies that concurrent reads of the closed field do not race
// with writes. We test the correct pattern: one goroutine closes while
// others check the closed state.
//
// RUN: go test -race -v -count=10 -run TestIOPollerClosedDataRace
func TestIOPollerClosedDataRace(t *testing.T) {
	for i := 0; i < 100; i++ {
		// Create and initialize a poller first (valid usage pattern)
		var poller ioPoller
		if err := poller.initPoller(); err != nil {
			t.Fatalf("Failed to init poller: %v", err)
		}

		var wg sync.WaitGroup
		start := make(chan struct{})

		// Multiple goroutines checking closed state
		for g := 0; g < 4; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for j := 0; j < 100; j++ {
					// These reads of closed should not race with the writer
					_ = poller.closed.Load()
				}
			}()
		}

		// One goroutine closing the poller
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = poller.closePoller()
		}()

		close(start)
		wg.Wait()
	}

	t.Log("Test completed - run with -race flag to verify no data race on closed field")
}
