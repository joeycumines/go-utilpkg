package termtest

import (
	"reflect"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	assert.Equal(t, uint16(40), cfg.rows)
	assert.Equal(t, uint16(100), cfg.cols)
	assert.Equal(t, 10*time.Second, cfg.defaultTimeout)

	// Harness side
	hcfg, err := resolveHarnessOptions([]HarnessOption{WithSize(5, 6), WithDefaultTimeout(7 * time.Second)})
	require.NoError(t, err)
	assert.Equal(t, uint16(5), hcfg.rows)
	assert.Equal(t, uint16(6), hcfg.cols)
	assert.Equal(t, 7*time.Second, hcfg.defaultTimeout)
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
	require.NoError(t, err)

	assert.Equal(t, "bash", cfg.cmdName)
	assert.Equal(t, []string{"-c", "echo hello"}, cfg.args)
	assert.Contains(t, cfg.env, "FOO=bar")
	assert.Equal(t, "/tmp", cfg.dir)
	assert.Equal(t, uint16(10), cfg.rows)
	assert.Equal(t, uint16(20), cfg.cols)
	assert.Equal(t, 5*time.Minute, cfg.defaultTimeout)
}

func TestResolveHarnessOptions_Full(t *testing.T) {
	dummyOpt := prompt.WithPrefix("test")
	opts := []HarnessOption{
		WithSize(50, 150),
		WithDefaultTimeout(1 * time.Second),
		WithPromptOptions(dummyOpt),
	}

	cfg, err := resolveHarnessOptions(opts)
	require.NoError(t, err)

	assert.Equal(t, uint16(50), cfg.rows)
	assert.Equal(t, uint16(150), cfg.cols)
	assert.Equal(t, 1*time.Second, cfg.defaultTimeout)
	assert.Len(t, cfg.promptOptions, 1)
}

func TestOptions_CumulativeBehavior(t *testing.T) {
	t.Run("WithEnv appends", func(t *testing.T) {
		opts := []ConsoleOption{
			WithEnv([]string{"A=1"}),
			WithEnv([]string{"B=2"}),
		}
		cfg, err := resolveConsoleOptions(opts)
		require.NoError(t, err)
		assert.Contains(t, cfg.env, "A=1")
		assert.Contains(t, cfg.env, "B=2")
	})

	t.Run("WithPromptOptions appends", func(t *testing.T) {
		// Since prompt.Option is a function, we can't easily check equality,
		// but we can check the length of the slice.
		opts := []HarnessOption{
			WithPromptOptions(prompt.WithPrefix("a")),
			WithPromptOptions(prompt.WithPrefix("b")),
		}
		cfg, err := resolveHarnessOptions(opts)
		require.NoError(t, err)
		assert.Len(t, cfg.promptOptions, 2)
	})
}

func TestOptions_OverwriteBehavior(t *testing.T) {
	t.Run("WithDir overwrites", func(t *testing.T) {
		opts := []ConsoleOption{
			WithDir("/foo"),
			WithDir("/bar"),
		}
		cfg, err := resolveConsoleOptions(opts)
		require.NoError(t, err)
		assert.Equal(t, "/bar", cfg.dir)
	})

	t.Run("WithSize overwrites", func(t *testing.T) {
		opts := []ConsoleOption{
			WithSize(10, 10),
			WithSize(20, 20),
		}
		cfg, err := resolveConsoleOptions(opts)
		require.NoError(t, err)
		assert.Equal(t, uint16(20), cfg.rows)
		assert.Equal(t, uint16(20), cfg.cols)
	})
}

func TestOptions_EmptyEdgeCases(t *testing.T) {
	t.Run("Empty Command", func(t *testing.T) {
		// NewConsole checks for empty command, but resolveConsoleOptions allows it
		cfg, err := resolveConsoleOptions([]ConsoleOption{WithCommand("")})
		require.NoError(t, err)
		assert.Equal(t, "", cfg.cmdName)
	})

	t.Run("Empty Env", func(t *testing.T) {
		cfg, err := resolveConsoleOptions([]ConsoleOption{WithEnv(nil)})
		require.NoError(t, err)
		assert.Empty(t, cfg.env)
	})
}

func TestResolveConsoleOptions_Error(t *testing.T) {
	sentinel := assert.AnError
	_, err := resolveConsoleOptions([]ConsoleOption{
		consoleOptionImpl(func(*consoleConfig) error { return sentinel }),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to apply console option")
	assert.ErrorIs(t, err, sentinel)
}

func TestResolveHarnessOptions_Error(t *testing.T) {
	sentinel := assert.AnError
	_, err := resolveHarnessOptions([]HarnessOption{
		harnessOptionImpl(func(*harnessConfig) error { return sentinel }),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to apply harness option")
	assert.ErrorIs(t, err, sentinel)
}
