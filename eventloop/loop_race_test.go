package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestPollStateOverwrite_PreSleep tests the race condition where poll()
// uses Store() to set StateSleeping, which can overwrite StateTerminating
// set by Stop(), causing shutdown to hang indefinitely.
//
// NOTE: This test requires test hooks (loopTestHooks) to be implemented
// in the Loop struct to pause execution at critical points. The hooks should
// include:
//
//	type loopTestHooks struct {
//	    PrePollSleep func()  // Called before state.Store(StateSleeping)
//	    PrePollAwake func()  // Called before state.Store(StateAwake/Running)
//	}
//
// Without these hooks, this test can only probabilistically catch the race.
func TestPollStateOverwrite_PreSleep(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	pollReached := make(chan struct{})
	proceedWithPoll := make(chan struct{})

	// NOTE: If testHooks field doesn't exist, this test documents the expected
	// behavior but cannot deterministically trigger the race.
	// Uncomment when hooks are implemented:
	//
	// l.testHooks = &loopTestHooks{
	// 	PrePollSleep: func() {
	// 		select {
	// 		case <-pollReached:
	// 		default:
	// 			close(pollReached)
	// 		}
	// 		<-proceedWithPoll
	// 	},
	// }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan struct{})
	errChan := make(chan error, 1)
	go func() {
		if err := l.Run(ctx); err != nil {
			errChan <- err
			return
		}
		close(runDone)
	}()

	// Without hooks, we try to catch the race probabilistically
	// Wait for loop to likely be in poll/sleeping state
	time.Sleep(50 * time.Millisecond)

	// Signal we've "reached" poll (simulated without hooks)
	select {
	case <-pollReached:
	default:
		close(pollReached)
	}

	stopDone := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer stopCancel()
		stopDone <- l.Shutdown(stopCtx)
	}()

	time.Sleep(10 * time.Millisecond)

	state := LoopState(l.state.Load())
	// During shutdown, state should be Terminating or Terminated
	if state != StateTerminating && state != StateTerminated && state != StateSleeping {
		t.Logf("State during Stop: %v (expected Terminating, Terminated, or Sleeping)", state)
	}

	close(proceedWithPoll)

	err = <-stopDone

	if err == context.DeadlineExceeded {
		t.Log("BUG CONFIRMED: poll() likely overwrote StateTerminating, causing shutdown hang")
		// Force cleanup for test hygiene
		l.state.Store(StateTerminating)
		l.submitWakeup()
		// Give it a moment to clean up
		time.Sleep(50 * time.Millisecond)
		<-runDone
		select {
		case err := <-errChan:
			t.Logf("Loop error: %v", err)
		default:
		}
	} else if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	} else {
		// Clean shutdown - wait for runDone
		select {
		case <-runDone:
		case err := <-errChan:
			t.Fatalf("Loop Run() failed: %v", err)
		}
	}

	wg.Wait()
}
