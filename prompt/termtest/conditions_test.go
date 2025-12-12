package termtest

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

// Unit tests for normalizeTTYOutput
func TestNormalizeTTYOutput_Unit(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean string",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "strip colors",
			input:    "\x1b[31mred\x1b[0m text",
			expected: "red text",
		},
		{
			name:     "strip cursor movement",
			input:    "a\x1b[C\x1b[2Jb",
			expected: "ab",
		},
		{
			name:     "strip OSC title bell",
			input:    "start\x1b]0;window title\007end",
			expected: "startend",
		},
		{
			name:     "strip OSC title ST",
			input:    "start\x1b]0;window title\x1b\\end",
			expected: "startend",
		},
		{
			name:     "strip carriage return",
			input:    "line1\r\nline2",
			expected: "line1\nline2",
		},
		{
			name:     "incomplete CSI at end",
			input:    "text\x1b[",
			expected: "text",
		},
		{
			name:     "dangling ESC at end",
			input:    "text\x1b",
			expected: "text",
		},
		{
			name:     "charset selection",
			input:    "\x1b(0box\x1b(B",
			expected: "box",
		},
		{
			name:     "simple 2-byte sequence",
			input:    "a\x1bMb", // Reverse index
			expected: "ab",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeTTYOutput(tt.input))
		})
	}
}

// Unit tests for Condition combinators
func TestConditions_Combinators(t *testing.T) {
	t.Run("All", func(t *testing.T) {
		c := All(
			func(s string) bool { return strings.Contains(s, "a") },
			func(s string) bool { return strings.Contains(s, "b") },
		)
		assert.True(t, c("ab"), "should match when all true")
		assert.False(t, c("a"), "should fail when one false")
		assert.False(t, c("c"), "should fail when all false")
	})

	t.Run("Any", func(t *testing.T) {
		c := Any(
			func(s string) bool { return strings.Contains(s, "a") },
			func(s string) bool { return strings.Contains(s, "b") },
		)
		assert.True(t, c("a"), "should match when first true")
		assert.True(t, c("b"), "should match when second true")
		assert.False(t, c("c"), "should fail when all false")
	})

	t.Run("Not", func(t *testing.T) {
		c := Not(Contains("fail"))
		assert.True(t, c("success"), "should be true when inner is false")
		assert.False(t, c("fail"), "should be false when inner is true")
	})

	t.Run("Contains Variants", func(t *testing.T) {
		raw := "foo\x1b[31mbar"
		// Contains (Normalized)
		assert.True(t, Contains("foobar")(raw), "Contains should match normalized")
		assert.True(t, Contains("\x1b")(raw), "Contains should match normalized even with raw chars")
		assert.False(t, Contains("baz")(raw), "Contains should fail when substring not present")
		assert.True(t, Contains("linebreak")(raw+"\r\nlinebreak"), "Contains should ignore CR")

		// ContainsRaw
		assert.True(t, ContainsRaw("\x1b[31m")(raw), "ContainsRaw should match ansi codes")
	})

	t.Run("Matches", func(t *testing.T) {
		re := regexp.MustCompile(`\d+`)
		assert.True(t, Matches(re)("abc 123"), "should match regex")
		assert.False(t, Matches(re)("abc"), "should fail regex")
		// Matches also normalizes
		assert.True(t, Matches(re)("abc \x1b[31m123\x1b[0m"), "should match regex against normalized string")
	})
}

func TestContainsAndRaw(t *testing.T) {
	raw := "\x1b[31mHello\x1b[0m"

	// Raw contains the ANSI escape sequence
	rawCond := ContainsRaw("\x1b[31m")
	if !rawCond(raw) {
		t.Fatalf("expected raw output to contain ANSI code")
	}

	// Normalized contains the plain text
	cond := Contains("Hello")
	if !cond(raw) {
		t.Fatalf("expected normalized output to contain Hello")
	}
}

func TestMatchesAndNormalize(t *testing.T) {
	input := "line1\r\n\x1b[32mGreen\x1b[0m"
	r := regexp.MustCompile(`Green`)
	if !Matches(r)(input) {
		t.Fatalf("expected regex to match normalized output")
	}

	// verify that carriage returns are removed
	normalized := normalizeTTYOutput("foo\rbar")
	assert.Equal(t, "foobar", normalized)
}

func TestCollapseWhitespace(t *testing.T) {
	s := "a\t b\n  c   d"
	got := collapseWhitespace(s)
	assert.Equal(t, "a b c d", got)
}

func TestCollapseWhitespace_NBSP(t *testing.T) {
	// Regression test for non-breaking space (\u00A0) handling
	s := "foo\u00A0\u00A0bar"
	got := collapseWhitespace(s)
	assert.Equal(t, "foo bar", got)
}

func TestAll_AllTrue_EvaluatesAllAndReturnsTrue(t *testing.T) {
	called := []bool{false, false, false}
	conds := []Condition{
		func(_ string) bool { called[0] = true; return true },
		func(_ string) bool { called[1] = true; return true },
		func(_ string) bool { called[2] = true; return true },
	}
	c := All(conds...)
	if !c("any") {
		t.Fatalf("expected All to return true when all conds return true")
	}
	for i, v := range called {
		if !v {
			t.Fatalf("expected condition %d to be called", i)
		}
	}
}

func TestAll_FirstFalse_StopsEarlyAndReturnsFalse(t *testing.T) {
	called := []bool{false, false, false}
	conds := []Condition{
		func(_ string) bool { called[0] = true; return true },
		func(_ string) bool { called[1] = true; return false },
		func(_ string) bool { called[2] = true; return true },
	}
	c := All(conds...)
	if c("any") {
		t.Fatalf("expected All to return false when a cond returns false")
	}
	if !called[0] || !called[1] {
		t.Fatalf("expected first two conditions to have been called")
	}
	if called[2] {
		t.Fatalf("expected third condition NOT to have been called due to short-circuit")
	}
}

func TestAll_Empty_ReturnsTrue(t *testing.T) {
	c := All()
	if !c("anything") {
		t.Fatalf("expected All() with no conditions to return true")
	}
}

func TestAny_AllFalse_EvaluatesAllAndReturnsFalse(t *testing.T) {
	called := []bool{false, false, false}
	conds := []Condition{
		func(_ string) bool { called[0] = true; return false },
		func(_ string) bool { called[1] = true; return false },
		func(_ string) bool { called[2] = true; return false },
	}
	c := Any(conds...)
	if c("x") {
		t.Fatalf("expected Any to return false when all conds return false")
	}
	for i, v := range called {
		if !v {
			t.Fatalf("expected condition %d to be called", i)
		}
	}
}

func TestAny_FirstTrue_StopsEarlyAndReturnsTrue(t *testing.T) {
	called := []bool{false, false, false}
	conds := []Condition{
		func(_ string) bool { called[0] = true; return false },
		func(_ string) bool { called[1] = true; return true },
		func(_ string) bool { called[2] = true; return false },
	}
	c := Any(conds...)
	if !c("x") {
		t.Fatalf("expected Any to return true when a cond returns true")
	}
	if !called[0] || !called[1] {
		t.Fatalf("expected first two conditions to have been called")
	}
	if called[2] {
		t.Fatalf("expected third condition NOT to have been called due to short-circuit")
	}
}

func TestAny_Empty_ReturnsFalse(t *testing.T) {
	c := Any()
	if c("anything") {
		t.Fatalf("expected Any() with no conditions to return false")
	}
}

func TestNot_InvertsResult(t *testing.T) {
	counter := 0
	cond := func(_ string) bool { counter++; return true }
	not := Not(cond)
	if not("x") {
		t.Fatalf("expected Not to invert the true result to false")
	}
	if counter != 1 {
		t.Fatalf("expected inner condition to be called exactly once, got %d", counter)
	}

	counter = 0
	condFalse := func(_ string) bool { counter++; return false }
	notFalse := Not(condFalse)
	if !notFalse("x") {
		t.Fatalf("expected Not to invert the false result to true")
	}
	if counter != 1 {
		t.Fatalf("expected inner condition to be called exactly once, got %d", counter)
	}
}

func TestComposedConditions_AnyAllNot(t *testing.T) {
	// Test a composed example: Not(All( cond1, Any(cond2, cond3) ))
	c1Called := false
	c2Called := false
	c3Called := false

	c1 := func(_ string) bool { c1Called = true; return true }
	c2 := func(_ string) bool { c2Called = true; return false }
	c3 := func(_ string) bool { c3Called = true; return true }

	composed := Not(All(c1, Any(c2, c3)))
	// All(c1, Any(c2, c3)) -> All(true, Any(false, true)) -> All(true, true) -> true
	// Not(true) -> false
	if composed("x") {
		t.Fatalf("expected composed condition to evaluate to false")
	}
	if !c1Called || !c2Called || !c3Called {
		t.Fatalf("expected all inner conditions to be called during evaluation")
	}
}

// FuzzNormalizeTTYOutput verifies the robustness of the ANSI stripping state machine.
// It ensures no panics occur and that invariants (like no raw ESC remaining) hold true.
func FuzzNormalizeTTYOutput(f *testing.F) {
	// Seed corpus with interesting ANSI sequences
	seeds := []string{
		"",
		"simple text",
		"\x1b[31mred\x1b[0m",        // Color
		"\x1b[?25h",                 // CSI private mode
		"\x1b]0;Title\007",          // OSC Title
		"\x1b]0;Title\x1b\\",        // OSC Title with ST terminator
		"line\r\n",                  // CRLF
		"\x1b(A",                    // G0 Set
		"\x1b[1;2;3mCombined\x1b[m", // SGR
		"\x1b[",                     // Incomplete CSI
		"\x1b]",                     // Incomplete OSC
		"\x1b",                      // Dangling ESC
		"\x1b[31;100",               // Truncated CSI
		"foo\x1b[?25hbar",           // Mixed
		"\x1b[>0;10;1c",             // Device Attributes
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		output := normalizeTTYOutput(input)

		// Invariant 1: Valid UTF-8 strings should generally remain valid (mostly).
		// We don't strictly enforce this if the input wasn't valid, but it's good to check.
		if utf8.ValidString(input) && !utf8.ValidString(output) {
			t.Logf("Warning: Valid input produced invalid output. Input: %q, Output: %q", input, output)
		}

		// Invariant 2: Output should never contain a raw Escape character (\x1b).
		// The normalizer is designed to strip or process all ESC sequences.
		if strings.ContainsRune(output, '\x1b') {
			t.Errorf("Normalized output contains ESC character. Input: %q, Output: %q", input, output)
		}

		// Invariant 3: Output should never contain a Carriage Return (\r).
		if strings.ContainsRune(output, '\r') {
			t.Errorf("Normalized output contains CR character. Input: %q, Output: %q", input, output)
		}

		// Invariant 4: Output length should generally be <= Input length.
		// (Since we are stripping sequences and CRs, not expanding).
		if len(output) > len(input) {
			t.Errorf("Normalized output grew in size. Input len: %d, Output len: %d", len(input), len(output))
		}

		// Invariant 5: Idempotency. Normalizing a normalized string should be a no-op.
		output2 := normalizeTTYOutput(output)
		if output != output2 {
			t.Errorf("Not idempotent: %q -> %q", output, output2)
		}
	})
}

func FuzzCollapseWhitespace(f *testing.F) {
	seeds := []string{
		"a b",
		"a  b",
		" a \t b \n ",
		"foo\u00A0bar", // NBSP seed
		"",
		"  ",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		out := collapseWhitespace(s)

		// Property 1: Output should not contain double spaces
		if strings.Contains(out, "  ") {
			t.Errorf("output contains double spaces: %q", out)
		}

		// Property 2: Output should not contain raw whitespace characters (tabs, newlines, NBSP)
		// strings.Fields handles all Unicode whitespace.
		if strings.ContainsAny(out, "\t\n\r\u00A0") {
			t.Errorf("output contains raw whitespace: %q", out)
		}

		// Property 3: Idempotency
		out2 := collapseWhitespace(out)
		if out != out2 {
			t.Errorf("not idempotent: %q -> %q", out, out2)
		}
	})
}
