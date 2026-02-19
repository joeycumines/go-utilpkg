package termtest

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt"
)

func TestWithCommand_replacesArgs(t *testing.T) {
	opts := []ConsoleOption{
		WithCommand("bin", "a", "b"),
		WithCommand("bin", "x"),
	}
	cfg, err := resolveConsoleOptions(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.cmdName != "bin" {
		t.Fatalf("expected cmdName 'bin', got %q", cfg.cmdName)
	}
	expected := []string{"x"}
	if !reflect.DeepEqual(cfg.args, expected) {
		t.Fatalf("expected args %v, got %v", expected, cfg.args)
	}
}

func TestOptions_WithSize_And_DefaultTimeout(t *testing.T) {
	// Console side
	cfg, err := resolveConsoleOptions([]ConsoleOption{WithSize(40, 100), WithDefaultTimeout(10 * time.Second)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.rows != 40 {
		t.Errorf("rows: got %d, want %d", cfg.rows, 40)
	}
	if cfg.cols != 100 {
		t.Errorf("cols: got %d, want %d", cfg.cols, 100)
	}
	if cfg.defaultTimeout != 10*time.Second {
		t.Errorf("defaultTimeout: got %v, want %v", cfg.defaultTimeout, 10*time.Second)
	}

	// Harness side
	hcfg, err := resolveHarnessOptions([]HarnessOption{WithSize(5, 6), WithDefaultTimeout(7 * time.Second)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hcfg.rows != 5 {
		t.Errorf("rows: got %d, want %d", hcfg.rows, 5)
	}
	if hcfg.cols != 6 {
		t.Errorf("cols: got %d, want %d", hcfg.cols, 6)
	}
	if hcfg.defaultTimeout != 7*time.Second {
		t.Errorf("defaultTimeout: got %v, want %v", hcfg.defaultTimeout, 7*time.Second)
	}
}

func TestResolveConsoleOptions_Full(t *testing.T) {
	opts := []ConsoleOption{
		WithCommand("bash", "-c", "echo hello"),
		WithEnv([]string{"FOO=bar"}),
		WithDir("/tmp"),
		WithSize(10, 20),
		WithDefaultTimeout(5 * time.Minute),
	}

	cfg, err := resolveConsoleOptions(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.cmdName != "bash" {
		t.Errorf("cmdName: got %q, want %q", cfg.cmdName, "bash")
	}
	if want := []string{"-c", "echo hello"}; !reflect.DeepEqual(cfg.args, want) {
		t.Errorf("args: got %v, want %v", cfg.args, want)
	}
	if !slices.Contains(cfg.env, "FOO=bar") {
		t.Errorf("env should contain FOO=bar, got %v", cfg.env)
	}
	if cfg.dir != "/tmp" {
		t.Errorf("dir: got %q, want %q", cfg.dir, "/tmp")
	}
	if cfg.rows != 10 {
		t.Errorf("rows: got %d, want %d", cfg.rows, 10)
	}
	if cfg.cols != 20 {
		t.Errorf("cols: got %d, want %d", cfg.cols, 20)
	}
	if cfg.defaultTimeout != 5*time.Minute {
		t.Errorf("defaultTimeout: got %v, want %v", cfg.defaultTimeout, 5*time.Minute)
	}
}

func TestResolveHarnessOptions_Full(t *testing.T) {
	dummyOpt := prompt.WithPrefix("test")
	opts := []HarnessOption{
		WithSize(50, 150),
		WithDefaultTimeout(1 * time.Second),
		WithPromptOptions(dummyOpt),
	}

	cfg, err := resolveHarnessOptions(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.rows != 50 {
		t.Errorf("rows: got %d, want %d", cfg.rows, 50)
	}
	if cfg.cols != 150 {
		t.Errorf("cols: got %d, want %d", cfg.cols, 150)
	}
	if cfg.defaultTimeout != 1*time.Second {
		t.Errorf("defaultTimeout: got %v, want %v", cfg.defaultTimeout, 1*time.Second)
	}
	if len(cfg.promptOptions) != 1 {
		t.Errorf("promptOptions length: got %d, want %d", len(cfg.promptOptions), 1)
	}
}

func TestOptions_CumulativeBehavior(t *testing.T) {
	t.Run("WithEnv appends", func(t *testing.T) {
		opts := []ConsoleOption{
			WithEnv([]string{"A=1"}),
			WithEnv([]string{"B=2"}),
		}
		cfg, err := resolveConsoleOptions(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Contains(cfg.env, "A=1") {
			t.Errorf("env should contain A=1, got %v", cfg.env)
		}
		if !slices.Contains(cfg.env, "B=2") {
			t.Errorf("env should contain B=2, got %v", cfg.env)
		}
	})

	t.Run("WithPromptOptions appends", func(t *testing.T) {
		// Since prompt.Option is a function, we can't easily check equality,
		// but we can check the length of the slice.
		opts := []HarnessOption{
			WithPromptOptions(prompt.WithPrefix("a")),
			WithPromptOptions(prompt.WithPrefix("b")),
		}
		cfg, err := resolveHarnessOptions(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.promptOptions) != 2 {
			t.Errorf("promptOptions length: got %d, want %d", len(cfg.promptOptions), 2)
		}
	})
}

func TestOptions_OverwriteBehavior(t *testing.T) {
	t.Run("WithDir overwrites", func(t *testing.T) {
		opts := []ConsoleOption{
			WithDir("/foo"),
			WithDir("/bar"),
		}
		cfg, err := resolveConsoleOptions(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.dir != "/bar" {
			t.Errorf("dir: got %q, want %q", cfg.dir, "/bar")
		}
	})

	t.Run("WithSize overwrites", func(t *testing.T) {
		opts := []ConsoleOption{
			WithSize(10, 10),
			WithSize(20, 20),
		}
		cfg, err := resolveConsoleOptions(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.rows != 20 {
			t.Errorf("rows: got %d, want %d", cfg.rows, 20)
		}
		if cfg.cols != 20 {
			t.Errorf("cols: got %d, want %d", cfg.cols, 20)
		}
	})
}

func TestOptions_EmptyEdgeCases(t *testing.T) {
	t.Run("Empty Command", func(t *testing.T) {
		// NewConsole checks for empty command, but resolveConsoleOptions allows it
		cfg, err := resolveConsoleOptions([]ConsoleOption{WithCommand("")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.cmdName != "" {
			t.Errorf("cmdName: got %q, want empty", cfg.cmdName)
		}
	})

	t.Run("Empty Env", func(t *testing.T) {
		cfg, err := resolveConsoleOptions([]ConsoleOption{WithEnv(nil)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.env) != 0 {
			t.Errorf("env should be empty, got %v", cfg.env)
		}
	})
}

func TestResolveConsoleOptions_Error(t *testing.T) {
	sentinel := errors.New("test error")
	_, err := resolveConsoleOptions([]ConsoleOption{
		consoleOptionImpl(func(*consoleConfig) error { return sentinel }),
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to apply console option") {
		t.Errorf("error %q should contain %q", err.Error(), "failed to apply console option")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected error to wrap sentinel, got %v", err)
	}
}

func TestResolveHarnessOptions_Error(t *testing.T) {
	sentinel := errors.New("test error")
	_, err := resolveHarnessOptions([]HarnessOption{
		harnessOptionImpl(func(*harnessConfig) error { return sentinel }),
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to apply harness option") {
		t.Errorf("error %q should contain %q", err.Error(), "failed to apply harness option")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected error to wrap sentinel, got %v", err)
	}
}
