//go:build unix

package termtest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt"
	istrings "github.com/joeycumines/go-prompt/strings"
)

func TestPtyReader_OpenAndClose(t *testing.T) {
	ctx := context.Background()
	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}

	// The harness is created but RunPrompt not called yet
	// Just test that we can close without hanging
	closeErrCh := make(chan error, 1)
	go func() {
		closeErrCh <- h.Close()
	}()

	select {
	case err := <-closeErrCh:
		// Close completed - this is what we want
		// Error is acceptable as long as we didn't hang
		t.Logf("Close returned: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Close timed out - likely blocking in ptyReader")
	}
}

func TestHarness_New(t *testing.T) {
	ctx := context.Background()
	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	if h == nil {
		t.Fatalf("expected non-nil harness")
	}

	// Don't call RunPrompt - just test creation and cleanup
	closeErrCh := make(chan error, 1)
	go func() {
		closeErrCh <- h.Close()
	}()

	select {
	case err := <-closeErrCh:
		// Close should return quickly; errors are acceptable when forcing
		// close can race with the reader loop. Only timeouts are failures.
		if err != nil {
			t.Logf("Close returned error (acceptable): %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close timed out")
	}
}

func TestPtyReader_CloseWakesRead(t *testing.T) {
	ctx := context.Background()
	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	// Start the prompt with default executor
	h.RunPrompt(nil)

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Now close - this should work without timeout
	closeErrCh := make(chan error, 1)
	go func() {
		closeErrCh <- h.Close()
	}()

	select {
	case err := <-closeErrCh:
		// Completed - check if error is acceptable
		if err != nil {
			// Some errors are expected during forced close
			t.Logf("Close returned: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close timed out - ptyReader.Close() didn't wake Read()")
	}
}

func TestHarness_Completion(t *testing.T) {
	ctx := context.Background()
	completer := func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		return []prompt.Suggest{{Text: "help"}}, 0, istrings.RuneNumber(1)
	}

	h, err := NewHarness(ctx, WithPromptOptions(prompt.WithPrefix("$ ")))
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	h.RunPrompt(nil, prompt.WithCompleter(completer))

	snap := h.Console().Snapshot()
	if err := h.Console().Await(ctx, snap, Contains("$ ")); err != nil {
		t.Fatalf("Await prompt: %v", err)
	}

	snap = h.Console().Snapshot()
	_, err = h.Console().WriteString("h")
	if err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	_, err = h.Console().WriteString("\t")
	if err != nil {
		t.Fatalf("WriteString tab: %v", err)
	}

	ctx2, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := h.Console().Await(ctx2, snap, Contains("help")); err != nil {
		t.Fatalf("Await completion: %v", err)
	}
}

func TestHarness_Send_Completion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	completer := newTestCompleter("apple", "apricot", "banana")
	h.RunPrompt(nil, prompt.WithCompleter(completer), prompt.WithPrefix("fruit> "))

	snap := h.Console().Snapshot()
	if err := h.Console().Await(ctx, snap, Contains("fruit> ")); err != nil {
		t.Fatalf("Await prompt: %v", err)
	}

	before := h.Console().Snapshot()
	_, err = h.Console().WriteString("ap")
	if err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := h.Console().Await(ctx, before, Contains("ap")); err != nil {
		t.Fatalf("Await ap: %v", err)
	}

	beforeTab := h.Console().Snapshot()

	if err := h.Console().Send("tab"); err != nil {
		t.Fatalf("Send tab: %v", err)
	}

	if err := h.Console().Await(ctx, beforeTab, Contains("apple")); err != nil {
		t.Fatalf("Await apple: %v", err)
	}
	if err := h.Console().Await(ctx, beforeTab, Contains("apricot")); err != nil {
		t.Fatalf("Await apricot: %v", err)
	}

	newOut := h.Console().String()[beforeTab.offset:]
	if strings.Contains(newOut, "banana") {
		t.Errorf("new output should not contain %q", "banana")
	}
}

func TestHarness_WaitExit_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	h.RunPrompt(func(s string) { /* do nothing */ })

	snap := h.Console().Snapshot()
	if err := h.Console().Await(ctx, snap, Contains("> ")); err != nil {
		t.Fatalf("Await prompt: %v", err)
	}

	// Should time out because the prompt doesn't exit
	{
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		err = h.WaitExit(ctx)
	}
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestHarness_Close(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	initialSnap := h.Console().Snapshot()
	go func() {
		defer wg.Done()
		h.RunPrompt(func(s string) { /* do nothing */ }, prompt.WithExitChecker(func(s string, b bool) bool { return false }))
	}()

	err = h.Console().Await(ctx, initialSnap, Contains("> "))
	if err != nil {
		t.Fatalf("Await: %v", err)
	}

	err = h.Close()
	if err != nil {
		t.Logf("Close returned: %v", err)
	}

	exitErr := h.waitExitTimeout(2 * time.Second)
	// With the fixed ptyReader, the prompt exits cleanly when Close is
	// called (no longer deadlocks in waitForRead). A nil error indicates
	// a clean shutdown; context.Canceled indicates the old forced path.
	if exitErr != nil && !errors.Is(exitErr, context.Canceled) {
		t.Errorf("expected nil or context.Canceled, got %v", exitErr)
	}

	wg.Wait()
}

func TestRunPrompt_Helper(t *testing.T) {
	runHarnessTest := func(ctx context.Context, fn func(h *Harness) error) error {
		h, err := NewHarness(ctx)
		if err != nil {
			return err
		}
		defer h.Close()

		h.RunPrompt(nil, prompt.WithExitChecker(func(s string, _ bool) bool { return s == "exit" }))

		snap := h.Console().Snapshot()
		if err := h.Console().Await(ctx, snap, Contains("> ")); err != nil {
			return err
		}

		return fn(h)
	}

	t.Run("successful test", func(t *testing.T) {
		ctx := context.Background()
		err := runHarnessTest(ctx, func(h *Harness) error {
			if err := h.Console().SendLine("exit"); err != nil {
				return err
			}
			if err := h.waitExitTimeout(2 * time.Second); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("failing test", func(t *testing.T) {
		ctx := context.Background()
		expectedErr := errors.New("test failed")
		err := runHarnessTest(ctx, func(h *Harness) error { return expectedErr })
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error wrapping sentinel, got %v", err)
		}
	})
}

func TestConsole_NewConsole_External(t *testing.T) {
	// Tests the standalone Console with an external process (echo)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := NewConsole(ctx, WithCommand("sh", "-c", "echo hello world"))
	if err != nil {
		t.Fatalf("NewConsole: %v", err)
	}
	defer c.Close()

	snap := c.Snapshot()
	err = c.Expect(ctx, snap, Contains("hello world"), "waiting for echo output")
	if err != nil {
		t.Fatalf("Expect: %v", err)
	}

	code, err := c.WaitExit(ctx)
	if err != nil {
		t.Fatalf("WaitExit: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code: got %d, want 0", code)
	}
}

func TestConsole_WriteSync_Timeout(t *testing.T) {
	// Tests that WriteSync times out if the sync protocol is not acknowledged
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	// We do NOT run the prompt, so nothing will echo back the sync ACK.

	syncCtx, syncCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer syncCancel()

	err = h.Console().WriteSync(syncCtx, "test")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "expected sync ack") {
		t.Errorf("unexpected error type: %v", err)
	}
}

func TestConsole_Send_Robustness(t *testing.T) {
	ctx := context.Background()
	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	err = h.Console().Send("invalid-key-name")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "unknown key") {
		t.Errorf("error %q should contain %q", err.Error(), "unknown key")
	}

	err = h.Console().Send("enter", "tab")
	if err != nil {
		t.Errorf("Send valid keys: %v", err)
	}
}

func TestConsole_SendSync_Timeout(t *testing.T) {
	// Tests that SendSync times out if the sync protocol is not acknowledged
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	// No prompt running

	syncCtx, syncCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer syncCancel()

	err = h.Console().SendSync(syncCtx, "enter")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestConsole_Environment(t *testing.T) {
	// Verifies that environment variables are correctly passed to the subprocess.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := NewConsole(ctx,
		WithCommand("sh", "-c", "echo $TEST_VAR"),
		WithEnv([]string{"TEST_VAR=foobar"}),
	)
	if err != nil {
		t.Fatalf("NewConsole: %v", err)
	}
	defer c.Close()

	if err := c.Expect(ctx, c.Snapshot(), Contains("foobar"), "environment variable output not found"); err != nil {
		t.Fatalf("Expect: %v", err)
	}
}

func TestConsole_Dir(t *testing.T) {
	// Verifies that the working directory is correctly set for the subprocess.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	// Resolve any potential symlinks to match typical pwd output
	resolvedTmp, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	c, err := NewConsole(ctx,
		WithCommand("sh", "-c", "pwd"),
		WithDir(resolvedTmp),
	)
	if err != nil {
		t.Fatalf("NewConsole: %v", err)
	}
	defer c.Close()

	if err := c.Expect(ctx, c.Snapshot(), Contains(resolvedTmp), "working directory output not found"); err != nil {
		t.Fatalf("Expect: %v", err)
	}
}

func TestHarness_RunPrompt_PanicRecovery(t *testing.T) {
	// Verifies that panics in the prompt/executor are caught and propagated to WaitExit
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	boom := "executor panic explosion"
	h.RunPrompt(func(s string) {
		panic(boom)
	})

	snap := h.Console().Snapshot()
	if err := h.Console().Await(ctx, snap, Contains("> ")); err != nil {
		t.Fatalf("Await prompt: %v", err)
	}

	if err := h.Console().WriteSync(ctx, "trigger"); err != nil {
		t.Fatalf("WriteSync: %v", err)
	}
	if err := h.Console().SendSync(ctx, "enter"); err != nil {
		t.Fatalf("SendSync: %v", err)
	}
	_ = h.Console().Send("enter")

	err = h.WaitExit(ctx)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), boom) {
		t.Errorf("error %q should contain %q", err.Error(), boom)
	}
	if !strings.Contains(err.Error(), "prompt panic") {
		t.Errorf("error %q should contain %q", err.Error(), "prompt panic")
	}
}

func TestHarness_RunPrompt_MultipleCalls(t *testing.T) {
	// Verifies that calling RunPrompt twice panics (API misuse protection)
	ctx := context.Background()
	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	h.RunPrompt(nil) // First call OK

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		h.RunPrompt(nil) // Second call must panic
	}()
	if !panicked {
		t.Errorf("expected RunPrompt to panic on second call")
	}
}

func TestHarness_Race_Close_RunPrompt(t *testing.T) {
	// Regression test for race conditions between starting the prompt and closing the harness.
	// This loop runs rapidly to increase probability of hitting race windows.
	for i := range 5 {
		func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			h, err := NewHarness(ctx)
			if err != nil {
				t.Fatalf("NewHarness: %v", err)
			}
			// Do NOT defer h.Close() here, we close explicitly below

			var wg sync.WaitGroup
			wg.Go(func() {
				// Artificial delay to misalign timing
				if i%2 == 0 {
					time.Sleep(time.Microsecond * 10)
				}
				// It's acceptable for this to panic if Close happens before RunPrompt finishes initialization,
				// but it should not deadlock or crash the runtime.
				// In this implementation, RunPrompt catches panics, but if Close happened first,
				// the panic might happen before the recovery block is active?
				// Actually, RunPrompt recovery is inside the goroutine it spawns.
				// The main body of RunPrompt checks atomic CAS.
				defer func() { recover() }()
				h.RunPrompt(nil)
			})

			// Concurrent close
			_ = h.Close()
			wg.Wait()
		}()
	}
}

func TestHarness_ExecutorAndExecutedCommands_Coverage(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	// Normal send path (cmdCh <- cmd)
	h.Executor("one")
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		cmds := h.ExecutedCommands()
		if len(cmds) >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for commandWorker to record command")
		}
		time.Sleep(5 * time.Millisecond)
	}

	cmds := h.ExecutedCommands()
	if !sliceContainsStr(cmds, "one") {
		t.Fatalf("expected cmds to contain %q, got %v", "one", cmds)
	}

	// Ensure ExecutedCommands returns a copy.
	cmds[0] = "mutated"
	cmds2 := h.ExecutedCommands()
	if !sliceContainsStr(cmds2, "one") {
		t.Fatalf("expected cmds2 to contain %q, got %v", "one", cmds2)
	}
	if sliceContainsStr(cmds2, "mutated") {
		t.Fatalf("expected cmds2 not to contain %q, got %v", "mutated", cmds2)
	}

	// cmdStop branch: close cmdStop, then call Executor to trigger shutdown behavior.
	h.cmdOnce.Do(func() { close(h.cmdStop) })
	h.Executor("two")
	select {
	case <-h.cmdDone:
		// worker exited
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for commandWorker to exit")
	}
	cmds3 := h.ExecutedCommands()
	if !sliceContainsStr(cmds3, "two") {
		t.Fatalf("expected cmds3 to contain %q, got %v", "two", cmds3)
	}

	// execStop early return path
	h.Executor("three")
	cmds4 := h.ExecutedCommands()
	if sliceContainsStr(cmds4, "three") {
		t.Fatalf("expected cmds4 not to contain %q, got %v", "three", cmds4)
	}
}

func sliceContainsStr(s []string, v string) bool {
	for _, item := range s {
		if item == v {
			return true
		}
	}
	return false
}

func TestHarness_RunPrompt_NoPTS(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	// Force dupPTS to return nil
	h.ptsMu.Lock()
	h.pts = nil
	h.ptsMu.Unlock()

	h.RunPrompt(nil)
	err = h.WaitExit(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no PTY slave available") {
		t.Fatalf("error %q should contain %q", err.Error(), "no PTY slave available")
	}
}

func TestHarness_dupPTS_EdgeCases(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	// Nil PTS branch
	h.ptsMu.Lock()
	origPTS := h.pts
	h.pts = nil
	h.ptsMu.Unlock()
	orig, dup := h.dupPTS()
	if orig != nil {
		t.Fatalf("expected orig to be nil, got %v", orig)
	}
	if dup != nil {
		t.Fatalf("expected dup to be nil, got %v", dup)
	}

	// Dup failure branch: closed fd triggers syscall.Dup error
	h.ptsMu.Lock()
	h.pts = origPTS
	h.ptsMu.Unlock()
	if err := origPTS.Close(); err != nil {
		t.Fatalf("origPTS.Close: %v", err)
	}
	orig2, dup2 := h.dupPTS()
	if orig2 == nil {
		t.Fatalf("expected orig2 to be non-nil")
	}
	if orig2 != dup2 {
		t.Fatalf("expected orig2 and dup2 to be the same pointer, got %p vs %p", orig2, dup2)
	}
}

func TestHarness_Close_StopAlreadyCalled(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}

	// Pre-stop so Close hits the stop()==false branch.
	_ = h.stop()

	closeErr := h.Close()
	if closeErr != nil {
		// Errors are acceptable here; we're mainly ensuring no deadlock and exercising the branch.
		t.Logf("Close returned: %v", closeErr)
	}
}

func TestHarness_RunPrompt_DupFalse_Branch(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	// Force syscall.Dup to fail (dupPTS returns origPTS==readerFile).
	h.ptsMu.Lock()
	if h.pts == nil {
		t.Fatalf("expected h.pts to be non-nil")
	}
	if err := h.pts.Close(); err != nil {
		t.Fatalf("h.pts.Close: %v", err)
	}
	h.ptsMu.Unlock()

	h.RunPrompt(nil)

	// Prompt should fail quickly; just ensure it doesn't hang.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = h.WaitExit(ctx)
}

func TestHarness_Close_AppendsForcedExitError(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}

	// Ensure RunPrompt fails immediately with a non-canceled error.
	h.ptsMu.Lock()
	h.pts = nil
	h.ptsMu.Unlock()
	h.RunPrompt(nil)

	err = h.Close()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	// This string comes from Harness.Close wrapping the second waitExitTimeout.
	if !strings.Contains(err.Error(), "forced prompt exit") {
		t.Fatalf("error %q should contain %q", err.Error(), "forced prompt exit")
	}
	if !strings.Contains(err.Error(), "no PTY slave available") {
		t.Fatalf("error %q should contain %q", err.Error(), "no PTY slave available")
	}
}

func TestHarness_NewHarness_InvalidOption(t *testing.T) {
	sentinel := errors.New("bad option")
	_, err := NewHarness(context.Background(), harnessOptionImpl(func(*harnessConfig) error { return sentinel }))
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected error to wrap sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "failed to apply harness option") {
		t.Fatalf("error %q should contain %q", err.Error(), "failed to apply harness option")
	}
}

func TestHarness_Executor_CoversSecondCmdStopSelect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Construct a Harness with no commandWorker to ensure cmdCh send isn't ready,
	// so Executor must take the cmdStop branch when it's closed.
	h := &Harness{
		ctx:     ctx,
		cancel:  cancel,
		cmdCh:   make(chan string),
		cmdStop: make(chan struct{}),
		cmdDone: make(chan struct{}),
	}

	done := make(chan struct{})
	go func() {
		h.Executor("x")
		close(done)
	}()

	// Allow the goroutine to block in Executor's second select.
	time.Sleep(10 * time.Millisecond)
	close(h.cmdStop)
	// Ensure the inner select can complete by canceling the context.
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for Executor to return")
	}
}

func TestHarness_Close_GracefulExitPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	// Prompt exits when user types "exit".
	h.RunPrompt(nil, prompt.WithExitChecker(func(in string, _ bool) bool { return in == "exit" }))
	if err := h.Console().Await(ctx, h.Console().Snapshot(), Contains("> ")); err != nil {
		t.Fatalf("Await prompt: %v", err)
	}
	if err := h.Console().SendLine("exit"); err != nil {
		t.Fatalf("SendLine: %v", err)
	}
	if err := h.waitExitTimeout(10 * time.Second); err != nil {
		t.Fatalf("waitExitTimeout: %v", err)
	}

	// Close after prompt exit should hit the gracefulExitErr==nil path.
	err = h.Close()
	if err != nil {
		t.Logf("Close returned: %v", err)
	}
}

func TestHarness_closePTY_PTSNil(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	h.ptsMu.Lock()
	h.pts = nil
	h.ptsMu.Unlock()

	// Should not panic and should close console.
	_ = h.closePTY()
}

func TestHarness_Executor_ContextDone(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	// Cancel the harness context so Executor should drop the command.
	h.cancel()
	h.Executor("dropped")
	// Ensure it didn't record anything.
	cmds := h.ExecutedCommands()
	if sliceContainsStr(cmds, "dropped") {
		t.Fatalf("expected cmds not to contain %q, got %v", "dropped", cmds)
	}
}

func TestHarness_closePTY_ReturnsErrors(t *testing.T) {
	// Construct a harness with a deliberately bad slave FD to force an error.
	badFD := os.NewFile(^uintptr(0), "bad")
	defer badFD.Close()

	h := &Harness{
		console: &Console{
			cancel:          func() {},
			done:            make(chan struct{}),
			waitDoneOnClose: true,
		},
		pts:    badFD,
		cancel: func() {},
	}

	err := h.closePTY()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// TestCompleter creates a simple completer for testing that filters based on prefix.
func newTestCompleter(suggestions ...string) prompt.Completer {
	return func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		var sug []prompt.Suggest
		text := d.Text
		for _, s := range suggestions {
			// Only include suggestions that match the current input
			if strings.HasPrefix(s, text) || text == "" {
				sug = append(sug, prompt.Suggest{Text: s, Description: "Test suggestion"})
			}
		}
		return sug, 0, istrings.RuneNumber(len(d.Text))
	}
}

func TestHarness_LineWrapRendering(t *testing.T) {
	// Regression test: verify that typed text which exactly fills or wraps
	// the terminal width does not produce spurious blank lines when the
	// prompt re-renders (e.g. after triggering completions).
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	const cols = 30 // narrow terminal
	h, err := NewHarness(ctx,
		WithSize(24, cols),
		WithPromptOptions(prompt.WithPrefix("> ")),
	)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	// Completer that always returns suggestions (regardless of input).
	completer := func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		return []prompt.Suggest{
			{Text: "done", Description: "finish"},
		}, 0, istrings.RuneNumber(len(d.Text))
	}
	h.RunPrompt(nil, prompt.WithCompleter(completer))

	snap := h.Console().Snapshot()
	if err := h.Console().Await(ctx, snap, Contains("> ")); err != nil {
		t.Fatalf("Await prompt: %v", err)
	}

	// Type text that fills the available width exactly.
	// Prefix "> " = 2 chars, so available = 28 chars.
	text := strings.Repeat("x", 28) // exactly fills the line
	before := h.Console().Snapshot()
	if _, err := h.Console().WriteString(text); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := h.Console().Await(ctx, before, Contains(text)); err != nil {
		t.Fatalf("Await text: %v", err)
	}

	// Trigger completions with Tab
	beforeTab := h.Console().Snapshot()
	if err := h.Console().Send("tab"); err != nil {
		t.Fatalf("Send tab: %v", err)
	}
	if err := h.Console().Await(ctx, beforeTab, Contains("done")); err != nil {
		t.Fatalf("Await completion: %v", err)
	}

	// Verify the output is clean: the typed text should still be present
	// and there should be no excessive blank lines (the original bug
	// manifested as many newlines pushed above the prompt).
	full := h.Console().String()
	norm := normalizeTTYOutput(full)
	if !strings.Contains(norm, text) {
		t.Errorf("normalized output missing typed text %q", text)
	}
	// Four consecutive newlines would indicate rendering corruption.
	if strings.Contains(norm, "\n\n\n\n") {
		t.Errorf("output contains excessive blank lines indicating rendering corruption:\n%s", norm)
	}
}

func TestHarness_LineWrapOverflow(t *testing.T) {
	// Test rendering when text overflows past the terminal width (wraps).
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	const cols = 25
	h, err := NewHarness(ctx,
		WithSize(24, cols),
		WithPromptOptions(prompt.WithPrefix("$ ")),
	)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	completer := func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		return []prompt.Suggest{
			{Text: "result", Description: "test"},
		}, 0, istrings.RuneNumber(len(d.Text))
	}
	h.RunPrompt(nil, prompt.WithCompleter(completer))

	snap := h.Console().Snapshot()
	if err := h.Console().Await(ctx, snap, Contains("$ ")); err != nil {
		t.Fatalf("Await prompt: %v", err)
	}

	// Prefix "$ " = 2 chars, available = 23. Type 30 chars to force a wrap.
	text := strings.Repeat("y", 30)
	before := h.Console().Snapshot()
	if _, err := h.Console().WriteString(text); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := h.Console().Await(ctx, before, Contains(text)); err != nil {
		t.Fatalf("Await text: %v", err)
	}

	// Trigger completion with Tab
	beforeTab := h.Console().Snapshot()
	if err := h.Console().Send("tab"); err != nil {
		t.Fatalf("Send tab: %v", err)
	}
	if err := h.Console().Await(ctx, beforeTab, Contains("result")); err != nil {
		t.Fatalf("Await completion: %v", err)
	}

	full := h.Console().String()
	norm := normalizeTTYOutput(full)
	if !strings.Contains(norm, text) {
		t.Errorf("normalized output missing typed text %q", text)
	}
	if strings.Contains(norm, "\n\n\n\n") {
		t.Errorf("output contains excessive blank lines indicating rendering corruption:\n%s", norm)
	}
}

func TestHarness_EnterVsCtrlJ(t *testing.T) {
	// Verify that ExecuteOnEnterCallback can distinguish Enter from Ctrl+J
	// via Prompt.EnterKeyPressed(). Enter sends \r which is preserved as
	// ControlM; Ctrl+J sends \n which maps to Enter.
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	type callbackResult struct {
		enterKey bool
		text     string
	}
	resultCh := make(chan callbackResult, 10)

	// Callback that records whether EnterKeyPressed() returns true,
	// then always executes (submits).
	enterCallback := func(p *prompt.Prompt, indentSize int) (int, bool) {
		resultCh <- callbackResult{
			enterKey: p.EnterKeyPressed(),
			text:     p.Buffer().Text(),
		}
		return 0, true
	}

	h.RunPrompt(func(s string) {},
		prompt.WithPrefix("$ "),
		prompt.WithExecuteOnEnterCallback(enterCallback),
	)

	snap := h.Console().Snapshot()
	if err := h.Console().Await(ctx, snap, Contains("$ ")); err != nil {
		t.Fatalf("Await prompt: %v", err)
	}

	// Test 1: type "hello" then press Enter (\r) → EnterKeyPressed
	if err := h.Console().WriteSync(ctx, "hello"); err != nil {
		t.Fatalf("WriteSync: %v", err)
	}
	if err := h.Console().SendSync(ctx, "enter"); err != nil {
		t.Fatalf("SendSync enter: %v", err)
	}

	select {
	case r := <-resultCh:
		if !r.enterKey {
			t.Errorf("Enter key: EnterKeyPressed() = false, want true")
		}
		if r.text != "hello" {
			t.Errorf("Enter key: text = %q, want %q", r.text, "hello")
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for Enter callback")
	}

	// Wait for next prompt
	snap = h.Console().Snapshot()
	if err := h.Console().Await(ctx, snap, Contains("$ ")); err != nil {
		t.Fatalf("Await second prompt: %v", err)
	}

	// Test 2: type "world" then press Ctrl+J (\n) → NOT EnterKeyPressed
	if err := h.Console().WriteSync(ctx, "world"); err != nil {
		t.Fatalf("WriteSync: %v", err)
	}
	if err := h.Console().SendSync(ctx, "ctrl+j"); err != nil {
		t.Fatalf("SendSync ctrl+j: %v", err)
	}

	select {
	case r := <-resultCh:
		if r.enterKey {
			t.Errorf("Ctrl+J: EnterKeyPressed() = true, want false")
		}
		if r.text != "world" {
			t.Errorf("Ctrl+J: text = %q, want %q", r.text, "world")
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for Ctrl+J callback")
	}
}

func TestHarness_FreeformMultiline(t *testing.T) {
	// Exercises freeform text input: Ctrl+J inserts a newline (execute=false),
	// Enter submits the full multiline buffer (execute=true).
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	type callbackResult struct {
		text string
	}
	resultCh := make(chan callbackResult, 10)

	// Freeform mode callback: Ctrl+J inserts newline, Enter submits.
	freeformCallback := func(p *prompt.Prompt, indentSize int) (int, bool) {
		if p.EnterKeyPressed() {
			resultCh <- callbackResult{text: p.Buffer().Text()}
			return 0, true
		}
		// Ctrl+J: insert newline, no indent
		return 0, false
	}

	h.RunPrompt(func(s string) {},
		prompt.WithPrefix("$ "),
		prompt.WithExecuteOnEnterCallback(freeformCallback),
	)

	snap := h.Console().Snapshot()
	if err := h.Console().Await(ctx, snap, Contains("$ ")); err != nil {
		t.Fatalf("Await prompt: %v", err)
	}

	// Type first line
	if err := h.Console().WriteSync(ctx, "line one"); err != nil {
		t.Fatalf("WriteSync line one: %v", err)
	}

	// Ctrl+J to insert newline (should NOT submit)
	if err := h.Console().SendSync(ctx, "ctrl+j"); err != nil {
		t.Fatalf("SendSync ctrl+j: %v", err)
	}

	// Verify no submission yet
	select {
	case r := <-resultCh:
		t.Fatalf("unexpected submission after Ctrl+J: %q", r.text)
	default:
	}

	// Type second line
	if err := h.Console().WriteSync(ctx, "line two"); err != nil {
		t.Fatalf("WriteSync line two: %v", err)
	}

	// Another Ctrl+J for a third line
	if err := h.Console().SendSync(ctx, "ctrl+j"); err != nil {
		t.Fatalf("SendSync ctrl+j 2: %v", err)
	}

	if err := h.Console().WriteSync(ctx, "line three"); err != nil {
		t.Fatalf("WriteSync line three: %v", err)
	}

	// Enter to submit the full multiline input
	if err := h.Console().SendSync(ctx, "enter"); err != nil {
		t.Fatalf("SendSync enter: %v", err)
	}

	select {
	case r := <-resultCh:
		want := "line one\nline two\nline three"
		if r.text != want {
			t.Errorf("submitted text = %q, want %q", r.text, want)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for submission")
	}
}

func TestHarness_ModeSwitching(t *testing.T) {
	// Demonstrates dynamic mode switching based on input prefix.
	// Default mode: freeform (Ctrl+J inserts newline, Enter submits).
	// Bang mode ("!" prefix): Enter submits immediately (no freeform).
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	type callbackResult struct {
		text string
	}
	resultCh := make(chan callbackResult, 10)

	modeCallback := func(p *prompt.Prompt, indentSize int) (int, bool) {
		text := p.Buffer().Text()
		if strings.HasPrefix(text, "!") {
			// Bang mode: always execute on any Enter/Ctrl+J
			resultCh <- callbackResult{text: text}
			return 0, true
		}
		// Freeform mode: Enter submits, Ctrl+J inserts newline
		if p.EnterKeyPressed() {
			resultCh <- callbackResult{text: text}
			return 0, true
		}
		return 0, false
	}

	h.RunPrompt(func(s string) {},
		prompt.WithPrefix("$ "),
		prompt.WithExecuteOnEnterCallback(modeCallback),
	)

	snap := h.Console().Snapshot()
	if err := h.Console().Await(ctx, snap, Contains("$ ")); err != nil {
		t.Fatalf("Await prompt: %v", err)
	}

	// Test 1: Freeform mode — multiline input with Ctrl+J
	if err := h.Console().WriteSync(ctx, "hello"); err != nil {
		t.Fatalf("WriteSync: %v", err)
	}
	if err := h.Console().SendSync(ctx, "ctrl+j"); err != nil {
		t.Fatalf("SendSync ctrl+j: %v", err)
	}

	// No submission yet
	select {
	case r := <-resultCh:
		t.Fatalf("unexpected submission in freeform mode: %q", r.text)
	default:
	}

	if err := h.Console().WriteSync(ctx, "world"); err != nil {
		t.Fatalf("WriteSync: %v", err)
	}
	if err := h.Console().SendSync(ctx, "enter"); err != nil {
		t.Fatalf("SendSync enter: %v", err)
	}

	select {
	case r := <-resultCh:
		want := "hello\nworld"
		if r.text != want {
			t.Errorf("freeform text = %q, want %q", r.text, want)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for freeform submission")
	}

	// Wait for next prompt
	snap = h.Console().Snapshot()
	if err := h.Console().Await(ctx, snap, Contains("$ ")); err != nil {
		t.Fatalf("Await second prompt: %v", err)
	}

	// Test 2: Bang mode — "!" prefix causes immediate submit on Enter
	if err := h.Console().WriteSync(ctx, "!ls -la"); err != nil {
		t.Fatalf("WriteSync bang: %v", err)
	}
	if err := h.Console().SendSync(ctx, "enter"); err != nil {
		t.Fatalf("SendSync enter: %v", err)
	}

	select {
	case r := <-resultCh:
		want := "!ls -la"
		if r.text != want {
			t.Errorf("bang text = %q, want %q", r.text, want)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for bang submission")
	}

	// Wait for next prompt
	snap = h.Console().Snapshot()
	if err := h.Console().Await(ctx, snap, Contains("$ ")); err != nil {
		t.Fatalf("Await third prompt: %v", err)
	}

	// Test 3: Bang mode with Ctrl+J — also submits immediately
	if err := h.Console().WriteSync(ctx, "!echo hi"); err != nil {
		t.Fatalf("WriteSync bang 2: %v", err)
	}
	if err := h.Console().SendSync(ctx, "ctrl+j"); err != nil {
		t.Fatalf("SendSync ctrl+j: %v", err)
	}

	select {
	case r := <-resultCh:
		want := "!echo hi"
		if r.text != want {
			t.Errorf("bang ctrl+j text = %q, want %q", r.text, want)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for bang ctrl+j submission")
	}
}

func TestHarness_MultilineCursorNavigation(t *testing.T) {
	// Verifies that Up/Down arrow keys navigate between lines in a
	// multiline buffer, and that edits apply to the correct line.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	type callbackResult struct {
		text string
	}
	resultCh := make(chan callbackResult, 10)

	freeformCallback := func(p *prompt.Prompt, indentSize int) (int, bool) {
		if p.EnterKeyPressed() {
			resultCh <- callbackResult{text: p.Buffer().Text()}
			return 0, true
		}
		return 0, false
	}

	h.RunPrompt(func(s string) {},
		prompt.WithPrefix("$ "),
		prompt.WithExecuteOnEnterCallback(freeformCallback),
	)

	snap := h.Console().Snapshot()
	if err := h.Console().Await(ctx, snap, Contains("$ ")); err != nil {
		t.Fatalf("Await prompt: %v", err)
	}

	// Build a 3-line buffer: "aaa" / "bbb" / "ccc"
	if err := h.Console().WriteSync(ctx, "aaa"); err != nil {
		t.Fatalf("WriteSync: %v", err)
	}
	if err := h.Console().SendSync(ctx, "ctrl+j"); err != nil {
		t.Fatalf("SendSync: %v", err)
	}
	if err := h.Console().WriteSync(ctx, "bbb"); err != nil {
		t.Fatalf("WriteSync: %v", err)
	}
	if err := h.Console().SendSync(ctx, "ctrl+j"); err != nil {
		t.Fatalf("SendSync: %v", err)
	}
	if err := h.Console().WriteSync(ctx, "ccc"); err != nil {
		t.Fatalf("WriteSync: %v", err)
	}

	// Navigate up to "bbb" and type "X"
	if err := h.Console().SendSync(ctx, "up"); err != nil {
		t.Fatalf("SendSync up: %v", err)
	}
	if err := h.Console().WriteSync(ctx, "X"); err != nil {
		t.Fatalf("WriteSync X: %v", err)
	}

	// Navigate up to "aaa" and type "Y"
	if err := h.Console().SendSync(ctx, "up"); err != nil {
		t.Fatalf("SendSync up: %v", err)
	}
	if err := h.Console().WriteSync(ctx, "Y"); err != nil {
		t.Fatalf("WriteSync Y: %v", err)
	}

	// Submit with Enter
	if err := h.Console().SendSync(ctx, "enter"); err != nil {
		t.Fatalf("SendSync enter: %v", err)
	}

	select {
	case r := <-resultCh:
		want := "aaaY\nbbbX\nccc"
		if r.text != want {
			t.Errorf("submitted text = %q, want %q", r.text, want)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for submission")
	}
}
