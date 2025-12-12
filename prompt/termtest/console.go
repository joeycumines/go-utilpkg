package termtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creack/pty"
	"github.com/joeycumines/go-prompt"
)

const (
	consoleWaitOnDoneCloseTimeout = time.Second
)

var (
	errConsoleReaderLoopTimeout = errors.New("timeout waiting for console reader loop to exit")
)

// Console represents the user's view of a terminal session.
// It is thread-safe and implements io.Writer, io.StringWriter, and io.Closer.
type Console struct {
	mu             sync.RWMutex
	output         bytes.Buffer
	ptm            *os.File  // PTY Master
	cmd            *exec.Cmd // Underlying command (nil if harness)
	defaultTimeout time.Duration
	cancel         context.CancelFunc // Cancels the reader loop
	done           chan struct{}      // Signals reader loop completion
	closed         bool

	// Process exit management
	waitOnce        sync.Once
	exitCh          chan struct{} // Closed when the process waits successfully
	exitCode        int
	exitErr         error
	closeOnce       sync.Once
	closeErr        error
	waitDoneOnClose bool
}

// Snapshot is an opaque token representing a specific point in the output history.
type Snapshot struct {
	offset int // Byte offset in the internal buffer
}

// syncCounter is used to generate unique sync IDs for the sync protocol.
var syncCounter atomic.Uint64

// NewConsole starts an external process attached to a PTY.
func NewConsole(ctx context.Context, opts ...ConsoleOption) (*Console, error) {
	cfg, err := resolveConsoleOptions(opts)
	if err != nil {
		return nil, err
	}

	testCtx, cancel := context.WithCancel(ctx)

	// Create command
	if cfg.cmdName == "" {
		cancel()
		return nil, fmt.Errorf("no command specified: use WithCommand(cmdName, args...) to specify the command")
	}
	cmd := exec.CommandContext(testCtx, cfg.cmdName, cfg.args...)
	cmd.Env = append(os.Environ(), cfg.env...)
	cmd.Env = append(cmd.Env,
		"TERM=xterm-256color",
		"COLUMNS="+fmt.Sprint(cfg.cols),
		"LINES="+fmt.Sprint(cfg.rows),
	)
	if cfg.dir != "" {
		cmd.Dir = cfg.dir
	}

	ws := &pty.Winsize{Rows: cfg.rows, Cols: cfg.cols}
	ptm, err := pty.StartWithSize(cmd, ws)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start command with pty: %w", err)
	}

	c := newConsole(ptm, cmd, cfg.defaultTimeout, cancel, true)
	return c, nil
}

// newConsole initializes a Console instance from a PTY master.
// Internal use only.
func newConsole(
	ptm *os.File,
	cmd *exec.Cmd,
	timeout time.Duration,
	cancel context.CancelFunc,
	waitDoneOnClose bool,
) *Console {
	c := &Console{
		ptm:             ptm,
		cmd:             cmd,
		defaultTimeout:  timeout,
		cancel:          cancel,
		done:            make(chan struct{}),
		exitCh:          make(chan struct{}),
		waitDoneOnClose: waitDoneOnClose,
	}
	go c.readLoop()
	return c
}

// Snapshot captures the current state of the output buffer.
// MUST be called immediately before an action to establish a baseline for assertions.
func (c *Console) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Snapshot{
		offset: c.output.Len(),
	}
}

// Await blocks until the buffer content generated SINCE the snapshot satisfies the Condition.
// It returns an error if the context is cancelled.
func (c *Console) Await(ctx context.Context, since Snapshot, cond Condition) error {
	// 1. Immediate check to avoid ticker overhead
	if c.checkCondition(since, cond) {
		return nil
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if c.checkCondition(since, cond) {
				return nil
			}
			return ctx.Err()

		case <-ticker.C:
			if c.checkCondition(since, cond) {
				return nil
			}
		}
	}
}

// Expect is a wrapper around Await that provides descriptive error messages.
// It uses the console's default timeout if the context has no deadline.
func (c *Console) Expect(ctx context.Context, since Snapshot, cond Condition, description string) error {
	waitCtx := ctx
	if _, ok := ctx.Deadline(); !ok && c.defaultTimeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, c.defaultTimeout)
		defer cancel()
	}

	if err := c.Await(waitCtx, since, cond); err != nil {
		// Provide a descriptive error including the state of the buffer
		c.mu.RLock()
		outLen := c.output.Len()
		// Grab a tail of the output for debugging context
		start := since.offset
		if start > outLen {
			start = 0
		}
		actual := c.output.String()[start:]
		c.mu.RUnlock()
		return fmt.Errorf("expected %s not found in new output (checked from offset %d, current len %d): %w\nOutput chunk: %q",
			description, since.offset, outLen, err, actual)
	}

	return nil
}

func (c *Console) checkCondition(since Snapshot, cond Condition) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	currentLen := c.output.Len()
	offset := since.offset
	if offset > currentLen {
		offset = 0 // Safety fallback if buffer was reset
	}

	str := c.output.String()[offset:]

	return cond(str)
}

// Write writes raw bytes to the PTY master.
func (c *Console) Write(p []byte) (n int, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed || c.ptm == nil {
		return 0, io.ErrClosedPipe
	}
	return c.ptm.Write(p)
}

// WriteString writes a raw string to the PTY master.
func (c *Console) WriteString(s string) (n int, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed || c.ptm == nil {
		return 0, io.ErrClosedPipe
	}
	return c.ptm.WriteString(s)
}

// WriteSync writes data and BLOCKS until the underlying Sync Protocol acknowledges it.
func (c *Console) WriteSync(ctx context.Context, s string) error {
	id := fmt.Sprintf("sync-%d", syncCounter.Add(1))
	req := prompt.BuildSyncRequest(id)
	ack := prompt.BuildSyncAck(id)

	snap := c.Snapshot()

	// Atomically write content + sync request
	if _, err := c.WriteString(s + req); err != nil {
		return err
	}

	cond := ContainsRaw(ack)

	// We use Expect here to handle the timeout logic and error formatting
	if err := c.Expect(ctx, snap, cond, fmt.Sprintf("sync ack %q", id)); err != nil {
		return err
	}

	return nil
}

// Send writes specific control sequences, using the bubbletea friendly key names.
func (c *Console) Send(keys ...string) error {
	for _, k := range keys {
		seq, err := lookupKey(k)
		if err != nil {
			return err
		}
		if _, err := c.WriteString(seq); err != nil {
			return err
		}
		// Brief yield - tries to avoid buffering keys together (WARNING: not deterministic)
		time.Sleep(time.Millisecond)
	}
	return nil
}

// SendSync is the synchronized variant of Send.
func (c *Console) SendSync(ctx context.Context, keys ...string) error {
	for _, k := range keys {
		seq, err := lookupKey(k)
		if err != nil {
			return err
		}
		if err := c.WriteSync(ctx, seq); err != nil {
			return err
		}
	}
	return nil
}

// SendLine sends a line of input followed by an Enter key.
//
// It attempts to wait for output stability before sending Enter to prevent
// "paste" detection, but proceeds regardless of success.
//
// WARNING: The caveats of this method come from [Console.WaitIdle]. See its
// documentation for details and possible alternatives.
func (c *Console) SendLine(input string) error {
	if _, err := c.WriteString(input); err != nil {
		return err
	}

	// It explicitly ignored the error (timeout), proceeding anyway.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// We request 25ms of stability within the 50ms window.
	_ = c.WaitIdle(ctx, 25*time.Millisecond)

	return c.Send("enter")
}

// WaitIdle waits until the output buffer stops changing for stableDuration.
//
// This API is notably non-deterministic, as it depends on timing and output patterns.
// It is recommended to use [Console.Await] or [Console.Expect] wherever possible.
// If those methods are not possible, if the application or library you are testing
// supports explicit synchronization (e.g., via a sync protocol), prefer that instead.
func (c *Console) WaitIdle(ctx context.Context, stableDuration time.Duration) error {
	checkInterval := 5 * time.Millisecond
	requiredChecks := int(stableDuration / checkInterval)
	if requiredChecks < 1 {
		requiredChecks = 1
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	c.mu.RLock()
	initialLen := c.output.Len()
	c.mu.RUnlock()

	stableCount := 0
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
			c.mu.RLock()
			currentLen := c.output.Len()
			c.mu.RUnlock()

			if currentLen == initialLen {
				stableCount++
				if stableCount >= requiredChecks {
					return nil
				}
			} else {
				initialLen = currentLen
				stableCount = 0
			}
		}
	}
}

// WaitExit waits for the underlying command to exit.
func (c *Console) WaitExit(ctx context.Context) (int, error) {
	c.mu.RLock()
	cmd := c.cmd
	c.mu.RUnlock()

	if cmd == nil {
		return -1, fmt.Errorf("no command to wait for (harness mode)")
	}

	// N.B. runs in the background
	c.waitProcess()

	select {
	case <-ctx.Done():
		return -1, ctx.Err()
	case <-c.exitCh:
		// Safe to read results after channel close
		c.mu.RLock()
		defer c.mu.RUnlock()
		return c.exitCode, c.exitErr
	}
}

// waitProcess starts awaiting the process exit in a goroutine, and is performed lazily.
func (c *Console) waitProcess() {
	c.waitOnce.Do(func() {
		go func() {
			// No lock needed here for c.cmd because waitOnce ensures single execution
			// and c.cmd is immutable after creation.
			// However, we must lock to write results to ensure visibility.
			err := c.cmd.Wait()

			c.mu.Lock()
			c.exitErr = err
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					c.exitCode = exitErr.ExitCode()
				} else {
					c.exitCode = -1
				}
			} else {
				c.exitCode = 0
			}
			c.mu.Unlock()

			close(c.exitCh)
		}()
	})
}

// Close terminates the console session.
func (c *Console) Close() error {
	c.closeOnce.Do(func() {
		c.closeErr = errors.New("panic during close")
		c.closeErr = c.close()
	})
	return c.closeErr
}

func (c *Console) close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	c.cancel()

	var errs []error
	if c.ptm != nil {
		if err := c.ptm.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		// N.B. runs in the background
		c.waitProcess()
	}

	if c.waitDoneOnClose {
		select {
		case <-c.done:
		case <-time.After(consoleWaitOnDoneCloseTimeout):
			errs = append(errs, errConsoleReaderLoopTimeout)
		}
	}

	if len(errs) != 0 {
		return fmt.Errorf("close errors: %w", errors.Join(errs...))
	}

	return nil
}

// String returns the accumulated output.
func (c *Console) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.output.String()
}

func (c *Console) readLoop() {
	defer close(c.done)
	buf := make([]byte, 4096)

	for {
		c.mu.RLock()
		ptm := c.ptm
		closed := c.closed
		c.mu.RUnlock()
		if ptm == nil || closed {
			return
		}

		n, err := ptm.Read(buf)
		if n > 0 {
			c.mu.Lock()
			c.output.Write(buf[:n])
			c.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}
