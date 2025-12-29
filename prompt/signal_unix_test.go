//go:build unix

package prompt

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestWinSizeSignalTriggersMainLoopGetsWinSize(t *testing.T) {
	if syscallSIGWINCH == 0 {
		t.Skip("SIGWINCH not supported on this platform")
	}

	// Use a mock reader so we can feed input to stop the prompt later.
	rm := &spyMockReader{
		mockReader:      newMockReader(),
		gotGetWinSize:   make(chan time.Time, 10),
		winSizeToReturn: &WinSize{Col: 66, Row: 44},
	}
	p := newTestPrompt(rm, func(s string) {}, nil)
	done := make(chan struct{})
	go func() {
		p.RunNoExit()
		close(done)
	}()

	// Wait for read goroutine to start
	rm.WaitReady()

	// Drain any prior GetWinSize calls (e.g. from setup) to avoid false positives.
	for {
		select {
		case <-rm.gotGetWinSize:
		default:
			goto drained1
		}
	}
drained1:
	// Allow background goroutines to register signal handlers.
	time.Sleep(50 * time.Millisecond)
	// Send SIGWINCH multiple times to mitigate races where the handler
	// might not have been registered at the instant of the signal.
	go func() {
		for i := 0; i < 20; i++ {
			_ = syscall.Kill(os.Getpid(), syscallSIGWINCH)
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Expect GetWinSize to be called by the main loop
	select {
	case <-rm.gotGetWinSize:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("GetWinSize was not called after SIGWINCH")
	}

	// Exit the prompt by sending Control-D
	ctrlD := findASCIICode(ControlD)
	if ctrlD == nil {
		t.Fatal("could not find ControlD ASCIICode")
	}
	rm.Feed(ctrlD)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not exit in time")
	}

}

func TestWinSizeSignalDuringExecutorHandledAfterExecutor(t *testing.T) {
	if syscallSIGWINCH == 0 {
		t.Skip("SIGWINCH not supported on this platform")
	}

	// Use a mock reader to allow feeding an Enter key to trigger the executor.
	rm := &spyMockReader{
		mockReader:      newMockReader(),
		gotGetWinSize:   make(chan time.Time, 10),
		winSizeToReturn: &WinSize{Col: 55, Row: 21},
	}

	startedExec := make(chan time.Time, 1)
	finishExec := make(chan struct{})

	exec := func(s string) {
		// signal that we've started (timestamp)
		startedExec <- time.Now()
		// block until test signals finish
		<-finishExec
	}

	p := newTestPrompt(rm, exec, nil)
	done := make(chan struct{})
	go func() {
		p.RunNoExit()
		close(done)
	}()

	// Wait for reader goroutine to start
	rm.WaitReady()

	// Drain any prior GetWinSize calls (e.g. from setup) to avoid false positives.
	for {
		select {
		case <-rm.gotGetWinSize:
		default:
			goto drained2
		}
	}
drained2:

	// Feed Enter to trigger executor
	enter := findASCIICode(Enter)
	if enter == nil {
		t.Fatal("could not find Enter ASCIICode")
	}
	rm.Feed(enter)

	// Wait for executor to start and capture the start time
	var startTime time.Time
	select {
	case startTime = <-startedExec:
	case <-time.After(2 * time.Second):
		t.Fatal("executor did not start")
	}

	// Now send SIGWINCH while executor is running. Send multiple signals to
	// improve the odds the handler receives one after it was registered.
	go func() {
		for i := 0; i < 20; i++ {
			_ = syscall.Kill(os.Getpid(), syscallSIGWINCH)
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Ensure GetWinSize is NOT called while executor is running
	select {
	case ts := <-rm.gotGetWinSize:
		// If timestamp falls between start and finish, it's a failure.
		if ts.After(startTime) {
			t.Fatalf("GetWinSize should not be called while executor is running (ts=%v > start=%v)", ts, startTime)
		}
	case <-time.After(100 * time.Millisecond):
		// good: not called during executor
	}

	// Finish executor: allow it to return
	close(finishExec)

	// Now we expect GetWinSize to be called after executor finishes
	select {
	case <-rm.gotGetWinSize:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("GetWinSize was not called after executor finished")
	}

	// Finally, stop prompt by sending Control-D
	ctrlD := findASCIICode(ControlD)
	if ctrlD == nil {
		t.Fatal("could not find ControlD ASCIICode")
	}
	rm.Feed(ctrlD)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not exit in time")
	}
}

// spyMockReader adapts mockReader to count GetWinSize calls.
type spyMockReader struct {
	*mockReader
	gotGetWinSize   chan time.Time
	winSizeToReturn *WinSize
}

func (r *spyMockReader) GetWinSize() *WinSize {
	select {
	case r.gotGetWinSize <- time.Now():
	default:
	}
	return r.winSizeToReturn
}
