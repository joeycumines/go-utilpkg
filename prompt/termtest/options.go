package termtest

import (
	"fmt"
	"time"

	"github.com/joeycumines/go-prompt"
)

// ConsoleOption configures a process-based Console.
type ConsoleOption interface {
	applyConsole(*consoleConfig) error
}

// HarnessOption configures an in-process Harness.
type HarnessOption interface {
	applyHarness(*harnessConfig) error
}

// SharedOption is a return type for options compatible with BOTH factories.
type SharedOption interface {
	ConsoleOption
	HarnessOption
}

// consoleConfig holds configuration for Console.
type consoleConfig struct {
	rows           uint16
	cols           uint16
	defaultTimeout time.Duration
	env            []string
	dir            string
	cmdName        string
	args           []string
}

// harnessConfig holds configuration for Harness.
type harnessConfig struct {
	rows           uint16
	cols           uint16
	defaultTimeout time.Duration
	promptOptions  []prompt.Option
}

// sharedOptionImpl implements SharedOption.
type sharedOptionImpl struct {
	applyConsoleFunc func(*consoleConfig) error
	applyHarnessFunc func(*harnessConfig) error
}

func (s *sharedOptionImpl) applyConsole(c *consoleConfig) error {
	return s.applyConsoleFunc(c)
}

func (s *sharedOptionImpl) applyHarness(c *harnessConfig) error {
	return s.applyHarnessFunc(c)
}

// consoleOptionImpl implements ConsoleOption.
type consoleOptionImpl func(*consoleConfig) error

func (f consoleOptionImpl) applyConsole(c *consoleConfig) error {
	return f(c)
}

// harnessOptionImpl implements HarnessOption.
type harnessOptionImpl func(*harnessConfig) error

func (f harnessOptionImpl) applyHarness(c *harnessConfig) error {
	return f(c)
}

// --- Shared Options ---

// WithSize sets the PTY dimensions. Default is 24x80.
func WithSize(rows, cols uint16) SharedOption {
	return &sharedOptionImpl{
		applyConsoleFunc: func(c *consoleConfig) error {
			c.rows = rows
			c.cols = cols
			return nil
		},
		applyHarnessFunc: func(c *harnessConfig) error {
			c.rows = rows
			c.cols = cols
			return nil
		},
	}
}

// WithDefaultTimeout sets the default timeout for Await operations.
func WithDefaultTimeout(d time.Duration) SharedOption {
	return &sharedOptionImpl{
		applyConsoleFunc: func(c *consoleConfig) error {
			c.defaultTimeout = d
			return nil
		},
		applyHarnessFunc: func(c *harnessConfig) error {
			c.defaultTimeout = d
			return nil
		},
	}
}

// --- Console-Specific Options ---

// WithEnv appends to the default environment.
func WithEnv(env []string) ConsoleOption {
	return consoleOptionImpl(func(c *consoleConfig) error {
		c.env = append(c.env, env...)
		return nil
	})
}

// WithDir sets the working directory.
func WithDir(path string) ConsoleOption {
	return consoleOptionImpl(func(c *consoleConfig) error {
		c.dir = path
		return nil
	})
}

// WithCommand configures the command name/path and arguments to execute.
func WithCommand(cmdName string, args ...string) ConsoleOption {
	return consoleOptionImpl(func(c *consoleConfig) error {
		c.cmdName = cmdName
		// WARNING: replace, do not append
		c.args = args
		return nil
	})
}

// --- Harness-Specific Options ---

// WithPromptOptions passes options directly to the underlying go-prompt instance.
func WithPromptOptions(opts ...prompt.Option) HarnessOption {
	return harnessOptionImpl(func(c *harnessConfig) error {
		c.promptOptions = append(c.promptOptions, opts...)
		return nil
	})
}

func resolveConsoleOptions(opts []ConsoleOption) (*consoleConfig, error) {
	cfg := &consoleConfig{
		rows:           24,
		cols:           80,
		defaultTimeout: 30 * time.Second,
	}
	for _, opt := range opts {
		if err := opt.applyConsole(cfg); err != nil {
			return nil, fmt.Errorf("failed to apply console option: %w", err)
		}
	}
	return cfg, nil
}

func resolveHarnessOptions(opts []HarnessOption) (*harnessConfig, error) {
	cfg := &harnessConfig{
		rows:           24,
		cols:           80,
		defaultTimeout: 30 * time.Second,
	}
	for _, opt := range opts {
		if err := opt.applyHarness(cfg); err != nil {
			return nil, fmt.Errorf("failed to apply harness option: %w", err)
		}
	}
	return cfg, nil
}
