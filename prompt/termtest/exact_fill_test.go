//go:build unix

package termtest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt"
	istrings "github.com/joeycumines/go-prompt/strings"
)

func TestExactFill_PTY(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Set a fixed terminal size
	cols, rows := uint16(20), uint16(10)
	h, err := NewHarness(ctx, WithSize(rows, cols))
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	// Simple executor that just returns
	executor := func(string) {}

	// Custom prompt with a fixed prefix
	h.RunPrompt(executor, prompt.WithPrefix("> ")) // prefix length 2

	// Wait for prompt to appear
	snap := h.Console().Snapshot()
	if err := h.Console().Await(ctx, snap, Contains("> ")); err != nil {
		t.Fatalf("Await prompt: %v", err)
	}

	// 1. Test exact fill: prefix(2) + 18 chars = 20 (exact fill)
	input := strings.Repeat("a", 18)
	if _, err := h.Console().Write([]byte(input)); err != nil {
		t.Fatalf("Write input: %v", err)
	}

	// Wait for input to be rendered
	if err := h.Console().Await(ctx, snap, Contains("> "+input)); err != nil {
		t.Fatalf("Await input: %v", err)
	}

	// 2. Test completion window after exact fill
	h.Close()
	h, _ = NewHarness(ctx, WithSize(rows, cols))
	defer h.Close()

	completer := func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		return []prompt.Suggest{{Text: "suggestion"}}, 0, 0
	}

	h.RunPrompt(executor,
		prompt.WithPrefix("> "),
		prompt.WithCompleter(completer),
	)

	snap = h.Console().Snapshot()
	if _, err := h.Console().Write([]byte(input)); err != nil {
		t.Fatalf("Write exact fill: %v", err)
	}
	if err := h.Console().Await(ctx, snap, Contains("> "+input)); err != nil {
		t.Fatalf("Await exact fill: %v", err)
	}

	if err := h.Console().Await(ctx, snap, Contains("suggestion")); err != nil {
		t.Fatalf("Await suggestion: %v", err)
	}

	// 3. Test Enter after exact fill
	if _, err := h.Console().Write([]byte("\r")); err != nil {
		t.Fatalf("Write Enter: %v", err)
	}

	// Prompt should appear on next line
	if err := h.Console().Await(ctx, snap, Contains("> ")); err != nil {
		t.Fatalf("Await next prompt: %v", err)
	}
}
