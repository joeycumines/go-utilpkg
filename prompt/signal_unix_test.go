//go:build unix

package prompt

import (
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestWinSizeSignalTriggersMainLoopGetsWinSize(t *testing.T) {
	if syscallSIGWINCH == 0 {
		t.Skip("SIGWINCH not supported on this platform")
	}

	// Use a mock reader so we can feed input to stop the prompt later.
	rm := newSpyMockReader(WinSize{Col: 66, Row: 44})
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
	// Send SIGWINCH until the main loop observes it. Retrying until the
	// effect is observed removes any dependence on how quickly the handler
	// goroutine registered itself, without a fixed sleep, and stops the
	// sprayer immediately so it cannot outlive the test and signal later ones.
	stopSpray := make(chan struct{})
	sprayDone := make(chan struct{})
	go func() {
		defer close(sprayDone)
		for {
			_ = syscall.Kill(os.Getpid(), syscallSIGWINCH)
			select {
			case <-stopSpray:
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}()

	// Expect GetWinSize to be called by the main loop
	select {
	case <-rm.gotGetWinSize:
		// success
	case <-time.After(2 * time.Second):
		close(stopSpray)
		<-sprayDone
		t.Fatal("GetWinSize was not called after SIGWINCH")
	}
	close(stopSpray)
	<-sprayDone

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
	rm := newSpyMockReader(WinSize{Col: 55, Row: 21})

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

	// Now send SIGWINCH while the executor is running. The sprayer must keep
	// firing until the executor is released, otherwise the "not called during
	// executor" window below could pass merely because no signal arrived. It
	// is stopped via defer so it cannot outlive the test and signal later tests.
	stopSpray := make(chan struct{})
	sprayDone := make(chan struct{})
	defer func() {
		close(stopSpray)
		<-sprayDone
	}()
	go func() {
		defer close(sprayDone)
		for {
			_ = syscall.Kill(os.Getpid(), syscallSIGWINCH)
			select {
			case <-stopSpray:
				return
			case <-time.After(10 * time.Millisecond):
			}
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

// TestWinSizeSignalRelayoutsViewportOnResize verifies that a SIGWINCH-driven
// resize re-anchors the input viewport so the cursor stays visible.
//
// The buffer holds five logical lines (L0..L4) with the cursor on L4. The
// terminal first reports a tall window where all lines fit, then shrinks to a
// single row. Only a viewport relayout (resetting and recomputing the buffer's
// start line for the new geometry) leaves L4 as the single visible line; a
// viewport left anchored at the top would keep showing L0 and push the cursor
// off screen.
func TestWinSizeSignalRelayoutsViewportOnResize(t *testing.T) {
	if syscallSIGWINCH == 0 {
		t.Skip("SIGWINCH not supported on this platform")
	}

	// Resizable reader: starts tall, then reports a single row.
	rm := newSpyMockReader(WinSize{Col: 40, Row: 24})

	p := newTestPrompt(rm, func(string) {}, nil)
	logger := &mockWriterLogger{}
	p.renderer.out = logger
	p.renderer.col = 40
	p.renderer.row = 24

	done := make(chan struct{})
	go func() {
		defer func() { _ = rm.Close() }()
		p.RunNoExit()
		close(done)
	}()

	rm.WaitReady()

	// Drain setup's GetWinSize call so the signal below is what we observe.
	for {
		select {
		case <-rm.gotGetWinSize:
		default:
			goto drained3
		}
	}
drained3:

	// Five logical lines, cursor ends on L4.
	rm.Feed([]byte("L0\nL1\nL2\nL3\nL4"))

	if !awaitWriteAfter(logger, 0, "L4", 2*time.Second) {
		t.Fatalf("initial render of the pasted lines did not happen within timeout")
	}

	// Everything rendered so far; only inspect calls from here on.
	mark := len(logger.Calls())

	// Shrink to a single row so only one line can be visible. The main loop
	// must rescale the viewport to keep the cursor line visible.
	rm.SetWinSize(WinSize{Col: 40, Row: 1})

	stopSpray := make(chan struct{})
	sprayDone := make(chan struct{})
	defer func() {
		close(stopSpray)
		<-sprayDone
	}()
	go func() {
		defer close(sprayDone)
		for {
			_ = syscall.Kill(os.Getpid(), syscallSIGWINCH)
			select {
			case <-stopSpray:
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}()

	select {
	case <-rm.gotGetWinSize:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("GetWinSize was not called after SIGWINCH")
	}

	// The resize triggers a render for the new geometry; the cursor line L4
	// must be the line it shows.
	if !awaitWriteAfter(logger, mark, "L4", 2*time.Second) {
		t.Fatalf("post-resize render did not draw the cursor line within timeout")
	}

	// A viewport still anchored at the top would instead redraw L0.
	for _, c := range logger.Calls()[mark:] {
		if c.method != "WriteString" {
			continue
		}
		if s, ok := c.args[0].(string); ok && strings.Contains(s, "L0") {
			t.Fatalf("post-resize render drew %q: viewport was not re-anchored on the cursor line", s)
		}
	}

	// Stop the prompt. ControlD only exits on an empty buffer, so clear the
	// pasted lines with ControlC first.
	ctrlC := findASCIICode(ControlC)
	if ctrlC == nil {
		t.Fatal("could not find ControlC ASCIICode")
	}
	rm.Feed(ctrlC)

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

// awaitWriteAfter polls the writer log until a WriteString call at or after
// index mark contains want, or the timeout expires.
func awaitWriteAfter(logger *mockWriterLogger, mark int, want string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		calls := logger.Calls()
		if mark > len(calls) {
			mark = len(calls)
		}
		for _, c := range calls[mark:] {
			if c.method != "WriteString" {
				continue
			}
			if s, ok := c.args[0].(string); ok && strings.Contains(s, want) {
				return true
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// spyMockReader adapts mockReader to count GetWinSize calls. The reported
// window size may be changed while the prompt loop is running, so it is
// guarded by winSizeMu.
type spyMockReader struct {
	*mockReader
	gotGetWinSize chan time.Time
	winSizeMu     sync.Mutex
	winSize       WinSize
}

// newSpyMockReader builds a spyMockReader that reports win.
func newSpyMockReader(win WinSize) *spyMockReader {
	return &spyMockReader{
		mockReader:    newMockReader(),
		gotGetWinSize: make(chan time.Time, 10),
		winSize:       win,
	}
}

func (r *spyMockReader) GetWinSize() *WinSize {
	select {
	case r.gotGetWinSize <- time.Now():
	default:
	}
	r.winSizeMu.Lock()
	defer r.winSizeMu.Unlock()
	ws := r.winSize
	return &ws
}

// SetWinSize changes the size reported to the running prompt.
func (r *spyMockReader) SetWinSize(ws WinSize) {
	r.winSizeMu.Lock()
	r.winSize = ws
	r.winSizeMu.Unlock()
}
