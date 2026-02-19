//go:build unix

package termtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
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
		if err != nil {
			t.Fatalf("newTestConsole: %v", err)
		}
		defer cp.Close()
		snap := cp.Snapshot()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = cp.Await(ctx, snap, Contains("ready"))
		if err != nil {
			t.Errorf("Await: %v", err)
		}
	})

	t.Run("invalid command path", func(t *testing.T) {
		ctx := context.Background()
		_, err := NewConsole(ctx, WithCommand("/non/existent/command"))
		if err == nil {
			t.Errorf("expected error, got nil")
		}
	})

	t.Run("empty command name", func(t *testing.T) {
		ctx := context.Background()
		// Should fail because cmdName is empty
		_, err := NewConsole(ctx, WithCommand(""))
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "no command specified") {
			t.Errorf("error %q should contain %q", err.Error(), "no command specified")
		}
	})

	t.Run("default timeout", func(t *testing.T) {
		cp, err := newTestConsole(t, []string{"echo", "ready"})
		if err != nil {
			t.Fatalf("newTestConsole: %v", err)
		}
		defer cp.Close()
		// Internal timeout should be the default 30s
		if cp.defaultTimeout != 30*time.Second {
			t.Errorf("defaultTimeout: got %v, want %v", cp.defaultTimeout, 30*time.Second)
		}
	})

	t.Run("custom timeout", func(t *testing.T) {
		cp, err := newTestConsole(t, []string{"echo", "ready"}, WithDefaultTimeout(5*time.Second))
		if err != nil {
			t.Fatalf("newTestConsole: %v", err)
		}
		defer cp.Close()
		if cp.defaultTimeout != 5*time.Second {
			t.Errorf("defaultTimeout: got %v, want %v", cp.defaultTimeout, 5*time.Second)
		}
	})

	t.Run("env and dir options", func(t *testing.T) {
		tempDir := t.TempDir()
		cp, err := newTestConsole(t, []string{"pwd"}, WithDir(tempDir))
		if err != nil {
			t.Fatalf("newTestConsole: %v", err)
		}
		defer cp.Close()

		snap := cp.Snapshot()
		_, err = cp.WriteString("pwd\n")
		if err != nil {
			t.Fatalf("WriteString: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = cp.Await(ctx, snap, Contains(tempDir))
		if err != nil {
			t.Errorf("Await: %v (should have used the specified directory)", err)
		}
	})
}

func TestConsole_NewConsole_InvalidOption(t *testing.T) {
	sentinel := errors.New("bad option")
	_, err := NewConsole(context.Background(), consoleOptionImpl(func(*consoleConfig) error { return sentinel }))
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected error wrapping sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "failed to apply console option") {
		t.Fatalf("error %q should contain %q", err.Error(), "failed to apply console option")
	}
}

func TestConsole_Interaction(t *testing.T) {
	cp, err := newTestConsole(t, []string{"interactive"})
	if err != nil {
		t.Fatalf("newTestConsole: %v", err)
	}
	defer cp.Close()
	snap := cp.Snapshot()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = cp.Await(ctx, snap, Contains("Interactive mode ready"))
	if err != nil {
		t.Fatalf("Await: %v", err)
	}

	t.Run("SendLine and Expect", func(t *testing.T) {
		snap := cp.Snapshot()
		err := cp.SendLine("hello console")
		if err != nil {
			t.Fatalf("SendLine: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = cp.Await(ctx, snap, Contains("ECHO: hello console"))
		if err != nil {
			t.Errorf("Await: %v", err)
		}
		if !strings.Contains(cp.String(), "ECHO: hello console") {
			t.Errorf("output should contain %q", "ECHO: hello console")
		}
	})

	t.Run("Send invalid key", func(t *testing.T) {
		err := cp.Send("not-a-key")
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "unknown key") {
			t.Errorf("error %q should contain %q", err.Error(), "unknown key")
		}
	})

	t.Run("Expect timeout description", func(t *testing.T) {
		snap := cp.Snapshot()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := cp.Expect(ctx, snap, Contains("text that will not appear"), "magic text")
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		if err != nil {
			if !strings.Contains(err.Error(), "expected magic text not found") {
				t.Errorf("error %q should contain %q", err.Error(), "expected magic text not found")
			}
			if !strings.Contains(err.Error(), "Output chunk") {
				t.Errorf("error %q should contain %q", err.Error(), "Output chunk")
			}
		}
	})
}

func TestConsole_ExpectSince_ExpectNew(t *testing.T) {
	cp, err := newTestConsole(t, []string{"interactive"})
	if err != nil {
		t.Fatalf("newTestConsole: %v", err)
	}
	defer cp.Close()
	snap := cp.Snapshot()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = cp.Await(ctx, snap, Contains("Interactive mode ready"))
	if err != nil {
		t.Fatalf("Await: %v", err)
	}

	snap = cp.Snapshot()
	_, err = cp.WriteString("first\n")
	if err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = cp.Await(ctx, snap, Contains("ECHO: first"))
	if err != nil {
		t.Fatalf("Await: %v", err)
	}

	t.Run("ExpectSince", func(t *testing.T) {
		snap := cp.Snapshot()
		_, err := cp.WriteString("second\n")
		if err != nil {
			t.Fatalf("WriteString: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = cp.Await(ctx, snap, Contains("ECHO: second"))
		if err != nil {
			t.Errorf("Await: %v", err)
		}

		// Should not find old text relative to this snapshot
		ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel2()
		err = cp.Await(ctx2, snap, Contains("ECHO: first"))
		if err == nil {
			t.Errorf("expected error (should not find old text), got nil")
		}
	})

	t.Run("ExpectNew", func(t *testing.T) {
		snap := cp.Snapshot()
		_, err := cp.WriteString("third\n")
		if err != nil {
			t.Fatalf("WriteString: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = cp.Await(ctx, snap, Contains("ECHO: third"))
		if err != nil {
			t.Errorf("Await: %v", err)
		}

		snap = cp.Snapshot()
		_, err = cp.WriteString("fourth\n")
		if err != nil {
			t.Fatalf("WriteString: %v", err)
		}

		ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = cp.Await(ctx, snap, Contains("ECHO: fourth"))
		if err != nil {
			t.Fatalf("Await: %v", err)
		}

		// Now should not find "third" because we take a fresh snapshot
		snap = cp.Snapshot()
		ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel2()
		err = cp.Await(ctx2, snap, Contains("ECHO: third"))
		if err == nil {
			t.Errorf("expected error (should not find old text), got nil")
		}
	})
}

func TestConsole_ExpectExitCode(t *testing.T) {
	t.Run("correct exit code", func(t *testing.T) {
		cp, err := newTestConsole(t, []string{"exit", "17"})
		if err != nil {
			t.Fatalf("newTestConsole: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		exitCode, err := cp.WaitExit(ctx)
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		if exitCode != 17 {
			t.Errorf("exitCode: got %d, want 17", exitCode)
		}
	})

	t.Run("incorrect exit code", func(t *testing.T) {
		cp, err := newTestConsole(t, []string{"exit", "17"})
		if err != nil {
			t.Fatalf("newTestConsole: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		exitCode, err := cp.WaitExit(ctx)
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		if exitCode == 18 {
			t.Errorf("expected exit code != 18, got %d", exitCode)
		}
	})

	t.Run("zero exit code", func(t *testing.T) {
		cp, err := newTestConsole(t, []string{"exit", "0"})
		if err != nil {
			t.Fatalf("newTestConsole: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		exitCode, err := cp.WaitExit(ctx)
		if err != nil {
			t.Errorf("WaitExit: %v", err)
		}
		if exitCode != 0 {
			t.Errorf("exitCode: got %d, want 0", exitCode)
		}
	})

	t.Run("timeout waiting for exit", func(t *testing.T) {
		cp, err := newTestConsole(t, []string{"wait", "2s"})
		if err != nil {
			t.Fatalf("newTestConsole: %v", err)
		}
		defer cp.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err = cp.WaitExit(ctx)
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected DeadlineExceeded, got %v", err)
		}
	})

	t.Run("wait exit on harness", func(t *testing.T) {
		h, err := NewHarness(context.Background())
		if err != nil {
			t.Fatalf("NewHarness: %v", err)
		}
		defer h.Close()

		_, err = h.Console().WaitExit(context.Background())
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "harness mode") {
			t.Errorf("error %q should contain %q", err.Error(), "harness mode")
		}
	})
}

func TestConsole_OutputManagement(t *testing.T) {
	cp, err := newTestConsole(t, []string{"echo", "line 1", "line 2"})
	if err != nil {
		t.Fatalf("newTestConsole: %v", err)
	}
	defer cp.Close()
	snap := cp.Snapshot()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = cp.Await(ctx, snap, Contains("line 2"))
	if err != nil {
		t.Fatalf("Await: %v", err)
	}

	output := cp.String()
	if !strings.Contains(output, "line 1") {
		t.Errorf("output should contain %q", "line 1")
	}
	if !strings.Contains(output, "line 2") {
		t.Errorf("output should contain %q", "line 2")
	}

	if len(cp.String()) <= 0 {
		t.Errorf("expected non-empty string")
	}
}

func TestConsole_WriteSyncAndSendSync(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	h.RunPrompt(nil)

	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = h.Console().WriteSync(ctx, "sync-test")
	if err != nil {
		t.Fatalf("WriteSync: %v", err)
	}

	hCtx, hCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer hCancel()
	if err := h.Console().SendSync(hCtx, "ctrl+j"); err != nil {
		t.Fatalf("SendSync: %v", err)
	}

	err = h.Console().SendSync(hCtx, "invalid-key-name")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestConsole_Await_EdgeCases(t *testing.T) {
	cp, err := newTestConsole(t, []string{"interactive"})
	if err != nil {
		t.Fatalf("newTestConsole: %v", err)
	}
	defer cp.Close()

	// Wait for boot
	if err := cp.Await(context.Background(), cp.Snapshot(), Contains("Interactive mode ready")); err != nil {
		t.Fatalf("Await boot: %v", err)
	}

	t.Run("immediate success", func(t *testing.T) {
		snap := cp.Snapshot()
		cp.WriteString("immediate\n")
		if err := cp.Await(context.Background(), snap, Contains("immediate")); err != nil {
			t.Fatalf("Await: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		err := cp.Await(ctx, snap, Contains("immediate"))
		if err != nil {
			t.Errorf("Await: %v", err)
		}
	})

	t.Run("context already cancelled", func(t *testing.T) {
		snap := cp.Snapshot()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := cp.Await(ctx, snap, Contains("will not happen"))
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})

	t.Run("snapshot isolation", func(t *testing.T) {
		snap1 := cp.Snapshot()
		cp.WriteString("isolation_A\n")
		if err := cp.Await(context.Background(), snap1, Contains("ECHO: isolation_A")); err != nil {
			t.Fatalf("Await: %v", err)
		}

		snap2 := cp.Snapshot()
		cp.WriteString("isolation_B\n")
		if err := cp.Await(context.Background(), snap2, Contains("isolation_B")); err != nil {
			t.Fatalf("Await: %v", err)
		}

		if err := cp.Await(context.Background(), snap1, Contains("isolation_B")); err != nil {
			t.Errorf("Snap1 should see B: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		err := cp.Await(ctx, snap2, Contains("isolation_A"))
		if err == nil {
			t.Errorf("Snap2 should not see output from before it was taken")
		}
	})

	t.Run("out of bounds snapshot", func(t *testing.T) {
		snap := Snapshot{offset: 999999}
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		err := cp.Await(ctx, snap, Contains("Interactive mode ready"))
		if err != nil {
			t.Errorf("Await: %v", err)
		}
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
		if err != nil {
			t.Fatalf("Await returned error: %v", err)
		}
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

	if err := c.WaitIdle(ctx, 30*time.Millisecond); err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}
}

func TestConsole_close_AlreadyClosed_ReturnsNil(t *testing.T) {
	c := &Console{cancel: func() {}}
	c.closed = true
	if err := c.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestConsole_WaitIdle(t *testing.T) {
	newConsole := func(t *testing.T) *Console {
		console, err := newTestConsole(t, []string{"interactive"})
		if err != nil {
			t.Fatalf("newTestConsole: %v", err)
		}
		t.Cleanup(func() {
			_ = console.Close()
		})
		if err := console.Await(context.Background(), console.Snapshot(), Contains("Interactive mode ready")); err != nil {
			t.Fatalf("Await boot: %v", err)
		}
		return console
	}

	t.Run("stable output returns nil", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		err := newConsole(t).WaitIdle(ctx, 50*time.Millisecond)
		if err != nil {
			t.Errorf("WaitIdle: %v", err)
		}
	})

	t.Run("cat", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		c, err := NewConsole(ctx, WithCommand("cat"))
		if err != nil {
			t.Fatalf("NewConsole: %v", err)
		}
		defer c.Close()

		_, err = c.WriteString("foo")
		if err != nil {
			t.Fatalf("WriteString: %v", err)
		}

		err = c.WaitIdle(ctx, 50*time.Millisecond)
		if err != nil {
			t.Errorf("WaitIdle: %v", err)
		}

		if !strings.Contains(c.String(), "foo") {
			t.Errorf("output should contain %q", "foo")
		}
	})
}

func TestConsole_Closed_Operations(t *testing.T) {
	cp, err := newTestConsole(t, []string{"echo", "done"})
	if err != nil {
		t.Fatalf("newTestConsole: %v", err)
	}

	if err := cp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Close again should be idempotent
	if err := cp.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}

	// Write should fail
	_, err = cp.WriteString("fail")
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("WriteString: expected io.ErrClosedPipe, got %v", err)
	}

	// Send should fail
	err = cp.Send("enter")
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("Send: expected io.ErrClosedPipe, got %v", err)
	}

	// String() should still work (buffer is preserved)
	_ = cp.String()
}

func TestConsole_WriteSync_Closed(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	if err := h.Console().Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err = h.Console().WriteSync(context.Background(), "x")
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

func TestConsole_SendLine_Closed(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	if err := h.Console().Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err = h.Console().SendLine("hello")
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

func TestConsole_WaitIdle_EarlyReturnBranches(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	t.Run("ctx already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := h.Console().WaitIdle(ctx, 10*time.Millisecond)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	})

	t.Run("stableDuration under interval", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		err := h.Console().WaitIdle(ctx, 0)
		if err != nil {
			t.Fatalf("WaitIdle: %v", err)
		}
	})
}

func TestConsole_WaitIdle_ContextDeadlineExceeded(t *testing.T) {
	c := &Console{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := c.WaitIdle(ctx, 200*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
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
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "checked from offset") {
		t.Fatalf("error %q should contain %q", err.Error(), "checked from offset")
	}
	if !strings.Contains(err.Error(), "Output chunk") {
		t.Fatalf("error %q should contain %q", err.Error(), "Output chunk")
	}
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
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), errConsoleReaderLoopTimeout.Error()) {
		t.Fatalf("error %q should contain %q", err.Error(), errConsoleReaderLoopTimeout.Error())
	}
	if time.Since(start) < consoleWaitOnDoneCloseTimeout {
		t.Fatalf("expected to wait at least %v", consoleWaitOnDoneCloseTimeout)
	}
}

func TestConsole_checkCondition_BufferResetFallback(t *testing.T) {
	var c Console
	_, _ = c.output.WriteString("abc")
	snap := Snapshot{offset: 999}
	if !c.checkCondition(snap, Contains("a")) {
		t.Fatalf("expected condition to match")
	}
}

func TestConsole_write_StringBufferUsed(t *testing.T) {
	// Tiny sanity to ensure Console.String reads the buffer without deadlocking.
	var c Console
	c.output = *bytes.NewBufferString("xyz")
	_ = c.String()
}

func TestConsole_Concurrency_Regression(t *testing.T) {
	cp, err := newTestConsole(t, []string{"cat"})
	if err != nil {
		t.Fatalf("newTestConsole: %v", err)
	}

	// Note: We don't defer Close() here, we close concurrently in the test.

	var wg sync.WaitGroup
	// Create a cancelable context for the workers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Writer routine: hammers the PTY
	wg.Go(func() {
		for i := range 200 {
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
	})

	// 2. Reader/Awaiter routine: constantly snapshots and checks
	wg.Go(func() {
		for range 50 {
			if ctx.Err() != nil {
				return
			}
			snap := cp.Snapshot()
			// Use a very short timeout - we expect timeouts or success, but NO panics/races
			subCtx, subCancel := context.WithTimeout(ctx, 10*time.Millisecond)
			_ = cp.Await(subCtx, snap, Contains("msg"))
			subCancel()
		}
	})

	// 3. String reader: constantly reads the full buffer
	wg.Go(func() {
		for range 50 {
			if ctx.Err() != nil {
				return
			}
			_ = cp.String()
			time.Sleep(2 * time.Millisecond)
		}
	})

	// 4. Closer: closes the console randomly during execution
	wg.Go(func() {
		time.Sleep(100 * time.Millisecond)
		_ = cp.Close()
		cancel() // Stop other workers
	})

	// Wait for completion
	wg.Wait()

	// Ensure double close at end is fine
	_ = cp.Close()
}
