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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPtyReader_OpenAndClose(t *testing.T) {
	// Test that ptyReader can be opened and closed without blocking
	ctx := context.Background()
	h, err := NewHarness(ctx)
	require.NoError(t, err)

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
	require.NoError(t, err)
	assert.NotNil(t, h)

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
	// Test that closing the ptyReader wakes a blocked Read
	ctx := context.Background()
	h, err := NewHarness(ctx)
	require.NoError(t, err)
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
	// Provide a simple completer that always suggests "help" when typing
	completer := func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		return []prompt.Suggest{{Text: "help"}}, 0, istrings.RuneNumber(1)
	}

	h, err := NewHarness(ctx, WithPromptOptions(prompt.WithPrefix("$ ")))
	require.NoError(t, err)
	defer h.Close()

	// Run the prompt with our completer
	h.RunPrompt(nil, prompt.WithCompleter(completer))

	// Wait for prompt to be ready
	snap := h.Console().Snapshot()
	require.NoError(t, h.Console().Await(ctx, snap, Contains("$ ")))

	// Type a single letter and request completions
	snap = h.Console().Snapshot()
	_, err = h.Console().WriteString("h")
	require.NoError(t, err)
	_, err = h.Console().WriteString("\t")
	require.NoError(t, err)

	// The rendered suggestions should include the suggestion text
	ctx2, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	require.NoError(t, h.Console().Await(ctx2, snap, Contains("help")))
}

func TestHarness_Send_Completion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	require.NoError(t, err)
	defer h.Close()

	completer := newTestCompleter("apple", "apricot", "banana")
	h.RunPrompt(nil, prompt.WithCompleter(completer), prompt.WithPrefix("fruit> "))

	// Wait for prompt prefix
	snap := h.Console().Snapshot()
	require.NoError(t, h.Console().Await(ctx, snap, Contains("fruit> ")))

	// Type "ap" and wait for it to appear
	before := h.Console().Snapshot()
	_, err = h.Console().WriteString("ap")
	require.NoError(t, err)
	require.NoError(t, h.Console().Await(ctx, before, Contains("ap")))

	// Capture a snapshot before triggering completion
	beforeTab := h.Console().Snapshot()

	// Trigger completion dropdown (tab)
	require.NoError(t, h.Console().Send("tab"))

	// Check that completion suggestions are visible in NEW output AFTER tab
	require.NoError(t, h.Console().Await(ctx, beforeTab, Contains("apple")))
	require.NoError(t, h.Console().Await(ctx, beforeTab, Contains("apricot")))

	// Ensure "banana" did not appear in the NEW output
	newOut := h.Console().String()[beforeTab.offset:]
	assert.NotContains(t, newOut, "banana")
}

func TestHarness_WaitExit_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	require.NoError(t, err)
	defer h.Close()

	h.RunPrompt(func(s string) { /* do nothing */ })

	snap := h.Console().Snapshot()
	require.NoError(t, h.Console().Await(ctx, snap, Contains("> ")))

	// Should time out because the prompt doesn't exit
	{
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		err = h.WaitExit(ctx)
	}
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}

func TestHarness_Close(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	initialSnap := h.Console().Snapshot()
	go func() {
		defer wg.Done()
		// Run a prompt that never exits on its own
		h.RunPrompt(func(s string) { /* do nothing */ }, prompt.WithExitChecker(func(s string, b bool) bool { return false }))
	}()

	err = h.Console().Await(ctx, initialSnap, Contains("> "))
	require.NoError(t, err)

	// Close the test, which should cancel the context and stop the prompt
	err = h.Close()
	if err != nil {
		t.Logf("Close returned: %v", err)
	}

	// The prompt should exit with a context cancelled error
	exitErr := h.waitExitTimeout(2 * time.Second)
	assert.ErrorIs(t, exitErr, context.Canceled)

	// Check that the prompt's own cleanup was called
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
		assert.NoError(t, err)
	})

	t.Run("failing test", func(t *testing.T) {
		ctx := context.Background()
		expectedErr := errors.New("test failed")
		err := runHarnessTest(ctx, func(h *Harness) error { return expectedErr })
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestConsole_NewConsole_External(t *testing.T) {
	// Tests the standalone Console with an external process (echo)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// echo prints args and exits. We use sh to ensure standard unix behavior.
	c, err := NewConsole(ctx, WithCommand("sh", "-c", "echo hello world"))
	require.NoError(t, err)
	defer c.Close()

	// Wait for output
	snap := c.Snapshot()
	err = c.Expect(ctx, snap, Contains("hello world"), "waiting for echo output")
	require.NoError(t, err)

	// Wait for exit
	code, err := c.WaitExit(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, code)
}

func TestConsole_WriteSync_Timeout(t *testing.T) {
	// Tests that WriteSync times out if the sync protocol is not acknowledged
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	require.NoError(t, err)
	defer h.Close()

	// We do NOT run the prompt, so nothing will echo back the sync ACK.

	// Use a very short timeout for the WriteSync call
	syncCtx, syncCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer syncCancel()

	err = h.Console().WriteSync(syncCtx, "test")
	assert.Error(t, err)
	// The error comes from Expect -> Await -> context.Done
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "expected sync ack"))
}

func TestConsole_Send_Robustness(t *testing.T) {
	ctx := context.Background()
	h, err := NewHarness(ctx)
	require.NoError(t, err)
	defer h.Close()

	// 1. Invalid key should error
	err = h.Console().Send("invalid-key-name")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown key")

	// 2. Valid keys should succeed (buffered)
	err = h.Console().Send("enter", "tab")
	assert.NoError(t, err)
}

func TestConsole_SendSync_Timeout(t *testing.T) {
	// Tests that SendSync times out if the sync protocol is not acknowledged
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	require.NoError(t, err)
	defer h.Close()

	// No prompt running

	syncCtx, syncCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer syncCancel()

	err = h.Console().SendSync(syncCtx, "enter")
	assert.Error(t, err)
}

func TestConsole_Environment(t *testing.T) {
	// Verifies that environment variables are correctly passed to the subprocess.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := NewConsole(ctx,
		WithCommand("sh", "-c", "echo $TEST_VAR"),
		WithEnv([]string{"TEST_VAR=foobar"}),
	)
	require.NoError(t, err)
	defer c.Close()

	require.NoError(t, c.Expect(ctx, c.Snapshot(), Contains("foobar"), "environment variable output not found"))
}

func TestConsole_Dir(t *testing.T) {
	// Verifies that the working directory is correctly set for the subprocess.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	// Resolve any potential symlinks to match typical pwd output
	resolvedTmp, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	c, err := NewConsole(ctx,
		WithCommand("sh", "-c", "pwd"),
		WithDir(resolvedTmp),
	)
	require.NoError(t, err)
	defer c.Close()

	require.NoError(t, c.Expect(ctx, c.Snapshot(), Contains(resolvedTmp), "working directory output not found"))
}

func TestHarness_RunPrompt_PanicRecovery(t *testing.T) {
	// Verifies that panics in the prompt/executor are caught and propagated to WaitExit
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	require.NoError(t, err)
	defer h.Close()

	// An executor that triggers a panic
	boom := "executor panic explosion"
	h.RunPrompt(func(s string) {
		panic(boom)
	})

	// Wait for prompt to start before trying to send input.
	snap := h.Console().Snapshot()
	require.NoError(t, h.Console().Await(ctx, snap, Contains("> ")))

	// Trigger the panic by sending input
	require.NoError(t, h.Console().WriteSync(ctx, "trigger"))
	require.NoError(t, h.Console().SendSync(ctx, "enter"))
	_ = h.Console().Send("enter")

	// WaitExit should return the panic as an error
	err = h.WaitExit(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), boom)
	assert.Contains(t, err.Error(), "prompt panic")
}

func TestHarness_RunPrompt_MultipleCalls(t *testing.T) {
	// Verifies that calling RunPrompt twice panics (API misuse protection)
	ctx := context.Background()
	h, err := NewHarness(ctx)
	require.NoError(t, err)
	defer h.Close()

	h.RunPrompt(nil) // First call OK

	assert.Panics(t, func() {
		h.RunPrompt(nil) // Second call must panic
	})
}

func TestHarness_Race_Close_RunPrompt(t *testing.T) {
	// Regression test for race conditions between starting the prompt and closing the harness.
	// This loop runs rapidly to increase probability of hitting race windows.
	for i := 0; i < 5; i++ {
		func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			h, err := NewHarness(ctx)
			require.NoError(t, err)
			// Do NOT defer h.Close() here, we close explicitly below

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
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
			}()

			// Concurrent close
			_ = h.Close()
			wg.Wait()
		}()
	}
}

func TestHarness_ExecutorAndExecutedCommands_Coverage(t *testing.T) {
	h, err := NewHarness(context.Background())
	require.NoError(t, err)
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
	require.Contains(t, cmds, "one")

	// Ensure ExecutedCommands returns a copy.
	cmds[0] = "mutated"
	cmds2 := h.ExecutedCommands()
	require.Contains(t, cmds2, "one")
	require.NotContains(t, cmds2, "mutated")

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
	require.Contains(t, cmds3, "two")

	// execStop early return path
	h.Executor("three")
	cmds4 := h.ExecutedCommands()
	require.NotContains(t, cmds4, "three")
}

func TestHarness_RunPrompt_NoPTS(t *testing.T) {
	h, err := NewHarness(context.Background())
	require.NoError(t, err)
	defer h.Close()

	// Force dupPTS to return nil
	h.ptsMu.Lock()
	h.pts = nil
	h.ptsMu.Unlock()

	h.RunPrompt(nil)
	err = h.WaitExit(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no PTY slave available")
}

func TestHarness_dupPTS_EdgeCases(t *testing.T) {
	h, err := NewHarness(context.Background())
	require.NoError(t, err)
	defer h.Close()

	// Nil PTS branch
	h.ptsMu.Lock()
	origPTS := h.pts
	h.pts = nil
	h.ptsMu.Unlock()
	orig, dup := h.dupPTS()
	require.Nil(t, orig)
	require.Nil(t, dup)

	// Dup failure branch: closed fd triggers syscall.Dup error
	h.ptsMu.Lock()
	h.pts = origPTS
	h.ptsMu.Unlock()
	require.NoError(t, origPTS.Close())
	orig2, dup2 := h.dupPTS()
	require.NotNil(t, orig2)
	require.Same(t, orig2, dup2)
}

func TestHarness_Close_StopAlreadyCalled(t *testing.T) {
	h, err := NewHarness(context.Background())
	require.NoError(t, err)

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
	require.NoError(t, err)
	defer h.Close()

	// Force syscall.Dup to fail (dupPTS returns origPTS==readerFile).
	h.ptsMu.Lock()
	require.NotNil(t, h.pts)
	require.NoError(t, h.pts.Close())
	h.ptsMu.Unlock()

	h.RunPrompt(nil)

	// Prompt should fail quickly; just ensure it doesn't hang.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = h.WaitExit(ctx)
}

func TestHarness_Close_AppendsForcedExitError(t *testing.T) {
	h, err := NewHarness(context.Background())
	require.NoError(t, err)

	// Ensure RunPrompt fails immediately with a non-canceled error.
	h.ptsMu.Lock()
	h.pts = nil
	h.ptsMu.Unlock()
	h.RunPrompt(nil)

	err = h.Close()
	require.Error(t, err)
	// This string comes from Harness.Close wrapping the second waitExitTimeout.
	require.Contains(t, err.Error(), "forced prompt exit")
	require.Contains(t, err.Error(), "no PTY slave available")
}

func TestHarness_NewHarness_InvalidOption(t *testing.T) {
	sentinel := errors.New("bad option")
	_, err := NewHarness(context.Background(), harnessOptionImpl(func(*harnessConfig) error { return sentinel }))
	require.ErrorIs(t, err, sentinel)
	require.Contains(t, err.Error(), "failed to apply harness option")
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, err := NewHarness(ctx)
	require.NoError(t, err)
	defer h.Close()

	// Prompt exits when user types "exit".
	h.RunPrompt(nil, prompt.WithExitChecker(func(in string, _ bool) bool { return in == "exit" }))
	require.NoError(t, h.Console().Await(ctx, h.Console().Snapshot(), Contains("> ")))
	require.NoError(t, h.Console().SendLine("exit"))
	require.NoError(t, h.waitExitTimeout(2*time.Second))

	// Close after prompt exit should hit the gracefulExitErr==nil path.
	err = h.Close()
	if err != nil {
		t.Logf("Close returned: %v", err)
	}
}

func TestHarness_closePTY_PTSNil(t *testing.T) {
	h, err := NewHarness(context.Background())
	require.NoError(t, err)
	defer h.Close()

	h.ptsMu.Lock()
	h.pts = nil
	h.ptsMu.Unlock()

	// Should not panic and should close console.
	_ = h.closePTY()
}

func TestHarness_Executor_ContextDone(t *testing.T) {
	h, err := NewHarness(context.Background())
	require.NoError(t, err)
	defer h.Close()

	// Cancel the harness context so Executor should drop the command.
	h.cancel()
	h.Executor("dropped")
	// Ensure it didn't record anything.
	cmds := h.ExecutedCommands()
	require.NotContains(t, cmds, "dropped")
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
	require.Error(t, err)
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
