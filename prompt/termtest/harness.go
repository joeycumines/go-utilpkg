package termtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/joeycumines/go-prompt"
)

// Harness manages an in-process PTY pair and app lifecycle.
type Harness struct {
	// PTY management
	console *Console // The unified interaction surface
	pts     *os.File // PTY Slave (used by go-prompt)
	ptsMu   sync.Mutex

	// Prompt lifecycle
	ctx        context.Context
	cancel     context.CancelFunc
	promptCh   chan error
	promptErr  error
	promptMu   sync.Mutex
	promptDone bool
	stopCh     chan struct{}
	stopOnce   sync.Once
	doneCh     chan struct{}
	runStarted atomic.Bool
	closeOnce  sync.Once
	closeErr   error

	// Command capture
	cmdMu    sync.RWMutex
	cmdCh    chan string
	cmdBuf   []string
	cmdStop  chan struct{}
	cmdDone  chan struct{}
	cmdOnce  sync.Once
	execMu   sync.Mutex
	execStop bool

	cfg *harnessConfig
}

// NewHarness creates a new test harness for in-process testing.
func NewHarness(ctx context.Context, opts ...HarnessOption) (*Harness, error) {
	cfg, err := resolveHarnessOptions(opts)
	if err != nil {
		return nil, err
	}

	testCtx, cancel := context.WithCancel(ctx)

	// Create PTY pair
	ptm, pts, err := pty.Open()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to open pty: %w", err)
	}

	// Ensure reasonable window size
	_ = pty.Setsize(ptm, &pty.Winsize{Rows: cfg.rows, Cols: cfg.cols})

	// Create the Console wrapper around the PTY master
	// Note: We pass waitDoneOnClose=false because we handle the wait manually
	// in Harness.Close() to strictly preserve original shutdown ordering.
	console := newConsole(ptm, nil, cfg.defaultTimeout, cancel, false)

	h := &Harness{
		console:  console,
		pts:      pts,
		ctx:      testCtx,
		cancel:   cancel,
		promptCh: make(chan error, 1),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
		cmdCh:    make(chan string),
		cmdStop:  make(chan struct{}),
		cmdDone:  make(chan struct{}),
		cfg:      cfg,
	}

	go h.commandWorker()

	return h, nil
}

// Console returns the underlying console for interaction.
func (h *Harness) Console() *Console {
	return h.console
}

func (h *Harness) commandWorker() {
	defer close(h.cmdDone)
	for {
		select {
		case <-h.ctx.Done():
			return
		case cmd, ok := <-h.cmdCh:
			if !ok {
				return
			}
			h.cmdMu.Lock()
			h.cmdBuf = append(h.cmdBuf, cmd)
			h.cmdMu.Unlock()
		}
	}
}

// Executor is the command handler that records executed commands.
func (h *Harness) Executor(cmd string) {
	h.execMu.Lock()
	defer h.execMu.Unlock()

	if h.execStop {
		return
	}

	// Prefer stop behavior deterministically (avoid select randomness when
	// both cmdCh and cmdStop are ready). WARNING: This is technically STILL a
	// race condition, so we compromise by using a timeout (non-deterministic).
	select {
	case <-h.cmdStop:
		h.execStop = true
		defer close(h.cmdCh)
		select {
		case <-time.After(time.Millisecond * 500):
		case <-h.ctx.Done():
		case h.cmdCh <- cmd:
		}
		return
	default:
	}

	select {
	case <-h.ctx.Done():
	case h.cmdCh <- cmd:
	case <-h.cmdStop:
		h.execStop = true
		defer close(h.cmdCh)
		select {
		case <-h.ctx.Done():
		case h.cmdCh <- cmd:
		}
	}
}

// ExecutedCommands returns a slice of all commands executed so far.
func (h *Harness) ExecutedCommands() []string {
	h.cmdMu.RLock()
	defer h.cmdMu.RUnlock()
	return append([]string(nil), h.cmdBuf...)
}

// RunPrompt runs a go-prompt instance with the test PTY.
func (h *Harness) RunPrompt(executor func(string), options ...prompt.Option) {
	if !h.runStarted.CompareAndSwap(false, true) {
		panic("RunPrompt can only be called once per Harness instance")
	}

	if executor == nil {
		executor = h.Executor
	}

	go func() {
		defer close(h.doneCh)

		var resultOnce sync.Once
		sendResult := func(err error) {
			resultOnce.Do(func() {
				h.promptCh <- err
			})
		}

		defer func() {
			h.cmdOnce.Do(func() {
				close(h.cmdStop)
			})
			h.execMu.Lock()
			if !h.execStop {
				h.execStop = true
				close(h.cmdCh)
			}
			h.execMu.Unlock()
			<-h.cmdDone
		}()

		defer func() {
			if r := recover(); r != nil {
				sendResult(fmt.Errorf("prompt panic: %v", r))
			}
		}()

		// Duplicate slave FD for independent lifecycle
		origPTS, readerFile := h.dupPTS()
		if readerFile == nil {
			sendResult(fmt.Errorf("no PTY slave available"))
			return
		}

		dup := readerFile != origPTS
		owned := true
		defer func() {
			if dup && owned {
				_ = readerFile.Close()
			}
		}()

		testOptions := []prompt.Option{
			prompt.WithReader(newPTYReader(readerFile)),
			prompt.WithWriter(&ptyWriter{origPTS}),
			prompt.WithExecuteOnEnterCallback(func(p *prompt.Prompt, indentSize int) (int, bool) {
				return 0, true
			}),
			prompt.WithSyncProtocol(true),
			prompt.WithExitChecker(func(in string, breakline bool) bool { return false }),
			prompt.WithGracefulClose(true),
		}

		testOptions = append(testOptions, h.cfg.promptOptions...)
		testOptions = append(testOptions, options...)

		var p *prompt.Prompt
		safeExecutor := executor
		if safeExecutor != nil {
			safeExecutor = func(in string) {
				defer func() {
					if r := recover(); r != nil {
						sendResult(fmt.Errorf("prompt panic: %v", r))
						if p != nil {
							p.Close()
						}
					}
				}()
				executor(in)
			}
		}

		p = prompt.New(safeExecutor, testOptions...)
		owned = false

		ctx, cancel := context.WithCancel(h.ctx)
		defer cancel()

		go func() {
			select {
			case <-ctx.Done():
			case <-h.stopCh:
			}
			p.Close()
		}()

		p.Run()
		sendResult(nil)
	}()
}

func (h *Harness) dupPTS() (orig, dup *os.File) {
	h.ptsMu.Lock()
	defer h.ptsMu.Unlock()
	if h.pts == nil {
		return nil, nil
	}
	dupFD, err := syscall.Dup(int(h.pts.Fd()))
	if err != nil {
		return h.pts, h.pts
	}
	return h.pts, os.NewFile(uintptr(dupFD), h.pts.Name()+"-dup")
}

// WaitExit waits for the prompt to exit and returns any error.
func (h *Harness) WaitExit(ctx context.Context) error {
	h.promptMu.Lock()
	defer h.promptMu.Unlock()

	if h.promptDone {
		return h.promptErr
	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	case h.promptErr = <-h.promptCh:
		h.promptDone = true
		return h.promptErr

	case <-h.ctx.Done():
		h.promptDone = true
		h.promptErr = h.ctx.Err()
		return h.promptErr
	}
}

// TODO: consider removing this or adding a more idiomatic graceful shutdown mechanism.
func (h *Harness) stop() (stopped bool) {
	h.stopOnce.Do(func() {
		close(h.stopCh)
		stopped = true
	})
	return stopped
}

func (h *Harness) Close() error {
	h.closeOnce.Do(func() {
		h.closeErr = errors.New("panic during close")

		var errs []error
		if h.stop() {
			gracefulExitErr := h.waitExitTimeout(time.Millisecond * 500)

			if err := h.closePTY(); err != nil {
				errs = append(errs, fmt.Errorf("error closing PTY: %w", err))
			}

			if gracefulExitErr != nil {
				if err := h.waitExitTimeout(time.Second * 2); err != nil && !errors.Is(err, context.Canceled) {
					errs = append(errs, fmt.Errorf("error waiting for forced prompt exit: %w", err))
				}
			}
		} else if err := h.closePTY(); err != nil {
			errs = append(errs, fmt.Errorf("error closing PTY: %w", err))
		}

		// This is in this location specifically to fix macOS hangs, see console.go.
		select {
		case <-h.console.done:
		case <-time.After(consoleWaitOnDoneCloseTimeout):
			errs = append(errs, errConsoleReaderLoopTimeout)
		}

		h.closeErr = errors.Join(errs...)
	})

	return h.closeErr
}

func (h *Harness) waitExitTimeout(d time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return h.WaitExit(ctx)
}

func (h *Harness) closePTY() error {
	defer h.cancel()

	h.ptsMu.Lock()
	defer h.ptsMu.Unlock()

	var errs []error

	// Explicitly close the Slave (pts) BEFORE the Master (console/ptm).
	// This ensures that the read loop on the master side receives the correct
	// EOF/EIO signal. Inverting this order causes race conditions and hangs.

	// Close slave
	if h.pts != nil {
		if err := h.pts.Close(); err != nil {
			if !strings.Contains(err.Error(), "file already closed") {
				errs = append(errs, fmt.Errorf("failed to close pts: %w", err))
			}
		}
		h.pts = nil
	}

	// Close console (Master)
	if err := h.console.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close console: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}

	return nil
}
