package termtest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLookupKey(t *testing.T) {
	for _, tt := range []struct {
		name      string
		input     string
		want      string
		wantErr   bool
		wantError string
	}{
		// --- Control Characters ---
		{
			name:  "Ctrl+C lowercase",
			input: "ctrl+c",
			want:  "\x03",
		},
		{
			name:  "Ctrl+D lowercase",
			input: "ctrl+d",
			want:  "\x04",
		},
		{
			name:  "Ctrl+Z lowercase",
			input: "ctrl+z",
			want:  "\x1a",
		},

		// --- Case Insensitivity Checks ---
		{
			name:  "Ctrl+C uppercase",
			input: "CTRL+C",
			want:  "\x03",
		},
		{
			name:  "Ctrl+D mixed case",
			input: "Ctrl+D",
			want:  "\x04",
		},

		// --- Special Keys & Aliases ---
		{
			name:  "Escape (full word)",
			input: "escape",
			want:  "\x1b",
		},
		{
			name:  "Escape (alias)",
			input: "esc",
			want:  "\x1b",
		},
		{
			name:  "Escape (mixed case alias)",
			input: "EsC",
			want:  "\x1b",
		},
		{
			name:  "Tab",
			input: "tab",
			want:  "\t",
		},
		{
			name:  "Enter",
			input: "enter",
			want:  "\r",
		},
		{
			name:  "Line feed (Ctrl+J)",
			input: "ctrl+j",
			want:  "\n",
		},
		{
			name:  "Backspace",
			input: "backspace",
			want:  "\x7f",
		},

		// --- Navigation (ANSI Escape Codes) ---
		{
			name:  "Up Arrow",
			input: "up",
			want:  "\x1b[A",
		},
		{
			name:  "Down Arrow",
			input: "down",
			want:  "\x1b[B",
		},
		{
			name:  "Right Arrow",
			input: "right",
			want:  "\x1b[C",
		},
		{
			name:  "Left Arrow",
			input: "left",
			want:  "\x1b[D",
		},
		{
			name:  "Right Arrow uppercase",
			input: "RIGHT",
			want:  "\x1b[C",
		},

		// --- Negative Cases (Errors) ---
		{
			name:      "Unknown key",
			input:     "spacebar",
			want:      "",
			wantErr:   true,
			wantError: "unknown key: spacebar",
		},
		{
			name:      "Empty string",
			input:     "",
			want:      "",
			wantErr:   true,
			wantError: "unknown key: ",
		},
		{
			name:      "Whitespace only",
			input:     "   ",
			want:      "",
			wantErr:   true,
			wantError: "unknown key:    ",
		},
		{
			name:      "Near miss (missing hyphen)",
			input:     "ctrlc",
			want:      "",
			wantErr:   true,
			wantError: "unknown key: ctrlc",
		},
		{
			name:      "Preserves case in error message",
			input:     "UnknownKey",
			want:      "",
			wantErr:   true,
			wantError: "unknown key: UnknownKey",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lookupKey(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("lookupKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err.Error() != tt.wantError {
				t.Errorf("lookupKey() error message = %q, want %q", err.Error(), tt.wantError)
			}

			if got != tt.want {
				t.Errorf("lookupKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func FuzzLookupKey(f *testing.F) {
	// Seed corpus with known keys and edge cases
	f.Add("enter")
	f.Add("ctrl+c")
	f.Add("shift+up")
	f.Add("invalid-key-random")
	f.Add("")
	f.Add("   ")
	f.Add("\x1b")

	f.Fuzz(func(t *testing.T, key string) {
		seq, err := lookupKey(key)

		// Invariant 1: No panics (implicitly covered by Fuzz execution)

		if err == nil {
			// Invariant 2: If no error, sequence must not be empty
			if seq == "" {
				t.Errorf("lookupKey(%q) returned empty sequence with no error", key)
			}

			// Invariant 3: Output consistency
			// If we look it up again, it should be identical
			seq2, err2 := lookupKey(key)
			assert.NoError(t, err2)
			assert.Equal(t, seq, seq2)
		} else {
			// Invariant 4: Error messages should be descriptive
			assert.Contains(t, err.Error(), "unknown key")
		}

		// Invariant 5: Normalization
		// lookupKey is case-insensitive (based on implementation of strings.ToLower)
		// So "ENTER" and "enter" should behave identically.
		seqUpper, errUpper := lookupKey(strings.ToUpper(key))
		if err == nil {
			assert.NoError(t, errUpper)
			assert.Equal(t, seq, seqUpper)
		} else {
			assert.Error(t, errUpper)
		}
	})
}
