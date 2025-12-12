//go:build unix

package termtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestConsole creates a Console for the helper process using the new API.
func newTestConsole(t *testing.T, args []string, opts ...ConsoleOption) (*Console, error) {
	t.Helper()

	ctx := t.Context()

	// Point to the test binary itself
	cmdName := os.Args[0]
	// Prepend arguments needed to re-exec the test binary as a helper
	fullArgs := append([]string{"-test.run=^TestMain$", "--"}, args...)

	// Merge default options
	allOpts := []ConsoleOption{
		WithCommand(cmdName, fullArgs...),
		WithEnv([]string{"GO_TEST_MODE=helper"}),
		WithDefaultTimeout(30 * time.Second),
	}
	allOpts = append(allOpts, opts...)

	return NewConsole(ctx, allOpts...)
}

func TestConsole_NewTest(t *testing.T) {
	t.Run("successful creation", func(t *testing.T) {
		cp, err := newTestConsole(t, []string{"echo", "ready"})
		require.NoError(t, err)
		defer cp.Close()
		snap := cp.Snapshot()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = cp.Await(ctx, snap, Contains("ready"))
		assert.NoError(t, err)
	})

	t.Run("invalid command path", func(t *testing.T) {
		ctx := context.Background()
		_, err := NewConsole(ctx, WithCommand("/non/existent/command"))
		assert.Error(t, err)
	})

	t.Run("empty command name", func(t *testing.T) {
		ctx := context.Background()
		// Should fail because cmdName is empty
		_, err := NewConsole(ctx, WithCommand(""))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no command specified")
	})

	t.Run("default timeout", func(t *testing.T) {
		cp, err := newTestConsole(t, []string{"echo", "ready"})
		require.NoError(t, err)
		defer cp.Close()
		// Internal timeout should be the default 30s
		assert.Equal(t, 30*time.Second, cp.defaultTimeout)
	})

	t.Run("custom timeout", func(t *testing.T) {
		cp, err := newTestConsole(t, []string{"echo", "ready"}, WithDefaultTimeout(5*time.Second))
		require.NoError(t, err)
		defer cp.Close()
		assert.Equal(t, 5*time.Second, cp.defaultTimeout)
	})

	t.Run("env and dir options", func(t *testing.T) {
		tempDir := t.TempDir()
		cp, err := newTestConsole(t, []string{"pwd"}, WithDir(tempDir))
		require.NoError(t, err)
		defer cp.Close()

		snap := cp.Snapshot()
		_, err = cp.WriteString("pwd\n")
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = cp.Await(ctx, snap, Contains(tempDir))
		assert.NoError(t, err, "Should have used the specified directory")
	})
}

func TestConsole_NewConsole_InvalidOption(t *testing.T) {
	sentinel := errors.New("bad option")
	_, err := NewConsole(context.Background(), consoleOptionImpl(func(*consoleConfig) error { return sentinel }))
	require.ErrorIs(t, err, sentinel)
	require.Contains(t, err.Error(), "failed to apply console option")
}

func TestConsole_Interaction(t *testing.T) {
	cp, err := newTestConsole(t, []string{"interactive"})
	require.NoError(t, err)
	defer cp.Close()
	snap := cp.Snapshot()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = cp.Await(ctx, snap, Contains("Interactive mode ready"))
	require.NoError(t, err)

	t.Run("SendLine and Expect", func(t *testing.T) {
		snap := cp.Snapshot()
		// Test SendLine (which includes a wait and an enter)
		err := cp.SendLine("hello console")
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = cp.Await(ctx, snap, Contains("ECHO: hello console"))
		assert.NoError(t, err)
		assert.Contains(t, cp.String(), "ECHO: hello console")
	})

	t.Run("Send invalid key", func(t *testing.T) {
		err := cp.Send("not-a-key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown key")
	})

	t.Run("Expect timeout description", func(t *testing.T) {
		snap := cp.Snapshot()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		// We expect this to fail
		err := cp.Expect(ctx, snap, Contains("text that will not appear"), "magic text")
		assert.Error(t, err)
		// Verify the error message contains the user description and context
		assert.Contains(t, err.Error(), "expected magic text not found")
		assert.Contains(t, err.Error(), "Output chunk")
	})
}

func TestConsole_ExpectSince_ExpectNew(t *testing.T) {
	cp, err := newTestConsole(t, []string{"interactive"})
	require.NoError(t, err)
	defer cp.Close()
	snap := cp.Snapshot()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = cp.Await(ctx, snap, Contains("Interactive mode ready"))
	require.NoError(t, err)

	snap = cp.Snapshot()
	_, err = cp.WriteString("first\n")
	require.NoError(t, err)
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = cp.Await(ctx, snap, Contains("ECHO: first"))
	require.NoError(t, err)

	t.Run("ExpectSince", func(t *testing.T) {
		snap := cp.Snapshot()
		_, err := cp.WriteString("second\n")
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = cp.Await(ctx, snap, Contains("ECHO: second"))
		assert.NoError(t, err)

		// Should not find old text relative to this snapshot
		ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel2()
		err = cp.Await(ctx2, snap, Contains("ECHO: first"))
		assert.Error(t, err)
	})

	t.Run("ExpectNew", func(t *testing.T) {
		// Snapshot before sending command
		snap := cp.Snapshot()
		_, err := cp.WriteString("third\n")
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = cp.Await(ctx, snap, Contains("ECHO: third"))
		assert.NoError(t, err)

		// Similarly for "fourth"
		snap = cp.Snapshot()
		_, err = cp.WriteString("fourth\n")
		require.NoError(t, err)

		ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = cp.Await(ctx, snap, Contains("ECHO: fourth"))
		require.NoError(t, err)

		// Now should not find "third" because we take a fresh snapshot
		snap = cp.Snapshot()
		ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel2()
		err = cp.Await(ctx2, snap, Contains("ECHO: third"))
		assert.Error(t, err)
	})
}

func TestConsole_ExpectExitCode(t *testing.T) {
	t.Run("correct exit code", func(t *testing.T) {
		cp, err := newTestConsole(t, []string{"exit", "17"})
		require.NoError(t, err)
		// No need to defer close, WaitExit waits for termination
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		exitCode, err := cp.WaitExit(ctx)
		// We expect an ExitError because code is non-zero, but WaitExit returns the code directly
		assert.Error(t, err)
		assert.Equal(t, 17, exitCode)
	})

	t.Run("incorrect exit code", func(t *testing.T) {
		cp, err := newTestConsole(t, []string{"exit", "17"})
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		exitCode, err := cp.WaitExit(ctx)
		assert.Error(t, err)
		assert.NotEqual(t, 18, exitCode, "expected exit code 18, got %d", exitCode)
	})

	t.Run("zero exit code", func(t *testing.T) {
		cp, err := newTestConsole(t, []string{"exit", "0"})
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		exitCode, err := cp.WaitExit(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 0, exitCode)
	})

	t.Run("timeout waiting for exit", func(t *testing.T) {
		cp, err := newTestConsole(t, []string{"wait", "2s"})
		require.NoError(t, err)
		defer cp.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err = cp.WaitExit(ctx)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("wait exit on harness", func(t *testing.T) {
		// Harness mode doesn't have an underlying cmd to wait for
		h, err := NewHarness(context.Background())
		require.NoError(t, err)
		defer h.Close()

		_, err = h.Console().WaitExit(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "harness mode")
	})
}

func TestConsole_OutputManagement(t *testing.T) {
	cp, err := newTestConsole(t, []string{"echo", "line 1", "line 2"})
	require.NoError(t, err)
	defer cp.Close()
	snap := cp.Snapshot()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = cp.Await(ctx, snap, Contains("line 2"))
	require.NoError(t, err)

	output := cp.String()
	// Output from helper process will have newlines
	assert.Contains(t, output, "line 1")
	assert.Contains(t, output, "line 2")

	assert.True(t, len(cp.String()) > 0)
}

func TestConsole_WriteSyncAndSendSync(t *testing.T) {
	h, err := NewHarness(context.Background())
	require.NoError(t, err)
	defer h.Close()

	// Start prompt with default executor and sync protocol enabled
	h.RunPrompt(nil)

	// Wait briefly for prompt to start
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// We expect to be able to successfully write sync requests
	err = h.Console().WriteSync(ctx, "sync-test")
	require.NoError(t, err)

	// And sending keys synchronously should also work (e.g., enter)
	hCtx, hCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer hCancel()
	require.NoError(t, h.Console().SendSync(hCtx, "ctrl+j"))

	// Test invalid key in SendSync
	err = h.Console().SendSync(hCtx, "invalid-key-name")
	assert.Error(t, err)
}

func TestConsole_Await_EdgeCases(t *testing.T) {
	cp, err := newTestConsole(t, []string{"interactive"})
	require.NoError(t, err)
	defer cp.Close()

	// Wait for boot
	require.NoError(t, cp.Await(context.Background(), cp.Snapshot(), Contains("Interactive mode ready")))

	t.Run("immediate success", func(t *testing.T) {
		snap := cp.Snapshot()
		cp.WriteString("immediate\n")
		// Wait for it to appear first
		require.NoError(t, cp.Await(context.Background(), snap, Contains("immediate")))

		// Now Await again on the SAME snapshot - should return nil immediately (fast path)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		err := cp.Await(ctx, snap, Contains("immediate"))
		assert.NoError(t, err)
	})

	t.Run("context already cancelled", func(t *testing.T) {
		snap := cp.Snapshot()
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := cp.Await(ctx, snap, Contains("will not happen"))
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("snapshot isolation", func(t *testing.T) {
		// Verify that a snapshot taken *after* output exists doesn't see that output
		// relative to itself.
		snap1 := cp.Snapshot()
		cp.WriteString("isolation_A\n")
		require.NoError(t, cp.Await(context.Background(), snap1, Contains("ECHO: isolation_A")))

		snap2 := cp.Snapshot()
		cp.WriteString("isolation_B\n")
		require.NoError(t, cp.Await(context.Background(), snap2, Contains("isolation_B")))

		// Verify cross-talk
		assert.NoError(t, cp.Await(context.Background(), snap1, Contains("isolation_B")), "Snap1 sees B")

		// Snap2 should NOT see A (it was in the past)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		err := cp.Await(ctx, snap2, Contains("isolation_A"))
		assert.Error(t, err, "Snap2 should not see output from before it was taken")
	})

	t.Run("out of bounds snapshot", func(t *testing.T) {
		// Artificially create a snapshot with a huge offset
		snap := Snapshot{offset: 999999}
		// Await should fallback to offset 0 and search everything
		// We expect it to find existing content "Interactive mode ready"
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		err := cp.Await(ctx, snap, Contains("Interactive mode ready"))
		assert.NoError(t, err)
	})
}

func TestConsole_Await_ContextDoneButConditionSatisfied(t *testing.T) {
	// Exercise the ctx.Done branch that still returns nil if condition is met.
	c := &Console{}

	// Snapshot at current (empty) output.
	snap := c.Snapshot()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- c.Await(ctx, snap, Contains("x"))
	}()

	// Ensure the Await goroutine has a chance to enter the loop.
	time.Sleep(2 * time.Millisecond)

	c.mu.Lock()
	_, _ = c.output.WriteString("x")
	c.mu.Unlock()
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for Await to return")
	}
}

func TestConsole_WaitIdle_ResetOnChange(t *testing.T) {
	// Exercise the branch where WaitIdle detects output changes and resets stableCount.
	c := &Console{}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go func() {
		// Make output change once after WaitIdle starts.
		time.Sleep(20 * time.Millisecond)
		c.mu.Lock()
		_, _ = c.output.WriteString("change")
		c.mu.Unlock()
	}()

	require.NoError(t, c.WaitIdle(ctx, 30*time.Millisecond))
}

func TestConsole_close_AlreadyClosed_ReturnsNil(t *testing.T) {
	c := &Console{cancel: func() {}}
	c.closed = true
	require.NoError(t, c.close())
}

func TestConsole_WaitIdle(t *testing.T) {
	// We need a process that outputs intermittently
	newConsole := func(t *testing.T) *Console {
		console, err := newTestConsole(t, []string{"interactive"})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = console.Close()
		})
		require.NoError(t, console.Await(context.Background(), console.Snapshot(), Contains("Interactive mode ready")))
		return console
	}

	t.Run("stable output returns nil", func(t *testing.T) {
		// The console is currently doing nothing
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		err := newConsole(t).WaitIdle(ctx, 50*time.Millisecond)
		assert.NoError(t, err)
	})

	t.Run("cat", func(t *testing.T) {
		// This is timing dependent, so we use loose checks.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		c, err := NewConsole(ctx, WithCommand("cat")) // cat echoes everything
		require.NoError(t, err)
		defer c.Close()

		// Write something
		_, err = c.WriteString("foo")
		require.NoError(t, err)

		// Wait for it to stabilize
		err = c.WaitIdle(ctx, 50*time.Millisecond)
		assert.NoError(t, err)

		// Ensure we got the output
		assert.Contains(t, c.String(), "foo")
	})
}

func TestConsole_Closed_Operations(t *testing.T) {
	cp, err := newTestConsole(t, []string{"echo", "done"})
	require.NoError(t, err)

	// Close explicitly
	require.NoError(t, cp.Close())

	// Close again should be idempotent
	assert.NoError(t, cp.Close())

	// Write should fail
	_, err = cp.WriteString("fail")
	assert.ErrorIs(t, err, io.ErrClosedPipe)

	// Send should fail
	err = cp.Send("enter")
	assert.ErrorIs(t, err, io.ErrClosedPipe)

	// String() should still work (buffer is preserved)
	_ = cp.String()
}

func TestConsole_WriteSync_Closed(t *testing.T) {
	h, err := NewHarness(context.Background())
	require.NoError(t, err)
	defer h.Close()

	require.NoError(t, h.Console().Close())
	err = h.Console().WriteSync(context.Background(), "x")
	require.ErrorIs(t, err, io.ErrClosedPipe)
}

func TestConsole_SendLine_Closed(t *testing.T) {
	h, err := NewHarness(context.Background())
	require.NoError(t, err)
	defer h.Close()

	require.NoError(t, h.Console().Close())
	err = h.Console().SendLine("hello")
	require.ErrorIs(t, err, io.ErrClosedPipe)
}

func TestConsole_WaitIdle_EarlyReturnBranches(t *testing.T) {
	h, err := NewHarness(context.Background())
	require.NoError(t, err)
	defer h.Close()

	t.Run("ctx already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := h.Console().WaitIdle(ctx, 10*time.Millisecond)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("stableDuration under interval", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		err := h.Console().WaitIdle(ctx, 0)
		require.NoError(t, err)
	})
}

func TestConsole_WaitIdle_ContextDeadlineExceeded(t *testing.T) {
	c := &Console{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := c.WaitIdle(ctx, 200*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestConsole_Expect_OutOfBoundsSnapshotFormatting(t *testing.T) {
	var c Console
	c.defaultTimeout = 0
	c.done = make(chan struct{})
	c.cancel = func() {}
	_, _ = c.output.WriteString("hello")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Expect(ctx, Snapshot{offset: 999999}, Contains("nope"), "nope")
	require.Error(t, err)
	// Specifically ensure it didn't panic, and it included formatting context.
	require.Contains(t, err.Error(), "checked from offset")
	require.Contains(t, err.Error(), "Output chunk")
}

func TestConsole_close_TimesOutWaitingForDone(t *testing.T) {
	// Exercise the timeout branch in Console.close without relying on OS-level PTY behavior.
	c := &Console{
		cancel:          func() {},
		done:            make(chan struct{}),
		waitDoneOnClose: true,
	}

	start := time.Now()
	err := c.close()
	require.Error(t, err)
	require.Contains(t, err.Error(), errConsoleReaderLoopTimeout.Error())
	// Keep this loose: just ensure it actually waited roughly the timeout.
	require.GreaterOrEqual(t, time.Since(start), consoleWaitOnDoneCloseTimeout)
}

func TestConsole_checkCondition_BufferResetFallback(t *testing.T) {
	// Covers the offset>len fallback path in checkCondition.
	var c Console
	_, _ = c.output.WriteString("abc")
	snap := Snapshot{offset: 999}
	require.True(t, c.checkCondition(snap, Contains("a")))
}

func TestConsole_write_StringBufferUsed(t *testing.T) {
	// Tiny sanity to ensure Console.String reads the buffer without deadlocking.
	var c Console
	c.output = *bytes.NewBufferString("xyz")
	_ = c.String()
}

func TestConsole_Concurrency_Regression(t *testing.T) {
	// Enhanced regression test for race conditions during high-throughput I/O
	// and concurrent Snapping/Awaiting/Closing.
	cp, err := newTestConsole(t, []string{"cat"}) // cat echoes everything back
	require.NoError(t, err)

	// Note: We don't defer Close() here, we close concurrently in the test.

	var wg sync.WaitGroup
	// Create a cancelable context for the workers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Writer routine: hammers the PTY
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			if ctx.Err() != nil {
				return
			}
			// Use WriteString directly
			_, err := cp.WriteString(fmt.Sprintf("msg-%d\n", i))
			if err != nil {
				// If closed, that's expected
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	// 2. Reader/Awaiter routine: constantly snapshots and checks
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			if ctx.Err() != nil {
				return
			}
			snap := cp.Snapshot()
			// Use a very short timeout - we expect timeouts or success, but NO panics/races
			subCtx, subCancel := context.WithTimeout(ctx, 10*time.Millisecond)
			_ = cp.Await(subCtx, snap, Contains("msg"))
			subCancel()
		}
	}()

	// 3. String reader: constantly reads the full buffer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			if ctx.Err() != nil {
				return
			}
			_ = cp.String()
			time.Sleep(2 * time.Millisecond)
		}
	}()

	// 4. Closer: closes the console randomly during execution
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		_ = cp.Close()
		cancel() // Stop other workers
	}()

	// Wait for completion
	wg.Wait()

	// Ensure double close at end is fine
	_ = cp.Close()
}
