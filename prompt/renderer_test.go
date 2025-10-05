//go:build !windows

package prompt

import (
	"reflect"
	"syscall"
	"testing"

	istrings "github.com/joeycumines/go-prompt/strings"
)

func TestFormatCompletion(t *testing.T) {
	scenarioTable := []struct {
		scenario      string
		completions   []Suggest
		prefix        string
		suffix        string
		expected      []Suggest
		maxWidth      istrings.Width
		expectedWidth istrings.Width
	}{
		{
			scenario: "",
			completions: []Suggest{
				{Text: "select"},
				{Text: "from"},
				{Text: "insert"},
				{Text: "where"},
			},
			prefix: " ",
			suffix: " ",
			expected: []Suggest{
				{Text: " select "},
				{Text: " from   "},
				{Text: " insert "},
				{Text: " where  "},
			},
			maxWidth:      20,
			expectedWidth: 8,
		},
		{
			scenario: "",
			completions: []Suggest{
				{Text: "select", Description: "select description"},
				{Text: "from", Description: "from description"},
				{Text: "insert", Description: "insert description"},
				{Text: "where", Description: "where description"},
			},
			prefix: " ",
			suffix: " ",
			expected: []Suggest{
				{Text: " select ", Description: " select description "},
				{Text: " from   ", Description: " from description   "},
				{Text: " insert ", Description: " insert description "},
				{Text: " where  ", Description: " where description  "},
			},
			maxWidth:      40,
			expectedWidth: 28,
		},
	}

	for _, s := range scenarioTable {
		ac, width := formatSuggestions(s.completions, s.maxWidth)
		if !reflect.DeepEqual(ac, s.expected) {
			t.Errorf("Should be %#v, but got %#v", s.expected, ac)
		}
		if width != s.expectedWidth {
			t.Errorf("Should be %#v, but got %#v", s.expectedWidth, width)
		}
	}
}

func TestBreakLineCallback(t *testing.T) {
	var i int
	r := NewRenderer()
	r.out = &PosixWriter{
		fd: syscall.Stdin, // "write" to stdin just so we don't mess with the output of the tests
	}
	r.col = 1
	b := NewBuffer()
	r.BreakLine(b, nil)

	if i != 0 {
		t.Errorf("i should initially be 0, before applying a break line callback")
	}

	r.breakLineCallback = func(doc *Document) {
		i++
	}
	r.BreakLine(b, nil)
	r.BreakLine(b, nil)
	r.BreakLine(b, nil)

	if i != 3 {
		t.Errorf("BreakLine callback not called, i should be 3")
	}
}

func TestGetMultilinePrefix(t *testing.T) {
	tests := map[string]struct {
		prefix string
		want   string
	}{
		"single width chars": {
			prefix: ">>",
			want:   "..",
		},
		"double width chars": {
			prefix: "本日",
			want:   "....",
		},
		"trailing spaces and single width chars": {
			prefix: ">!>   ",
			want:   "...   ",
		},
		"trailing spaces and double width chars": {
			prefix: "本日:   ",
			want:   ".....   ",
		},
		"leading spaces and single width chars": {
			prefix: "  ah:   ",
			want:   ".....   ",
		},
		"leading spaces and double width chars": {
			prefix: "  本日:   ",
			want:   ".......   ",
		},
	}

	r := NewRenderer()
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := r.getMultilinePrefix(tc.prefix)
			if tc.want != got {
				t.Errorf("Expected %#v, but got %#v", tc.want, got)
			}
		})
	}
}

func TestRenderer_countInputLines(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		startLine int
		col       istrings.Width
		want      int
	}{
		{
			name:      "empty input",
			text:      "",
			startLine: 0,
			col:       80,
			want:      1,
		},
		{
			name:      "single line shorter than width",
			text:      "hello",
			startLine: 0,
			col:       80,
			want:      1,
		},
		{
			name:      "input wraps exactly to screen width",
			text:      "12345678901234567890", // 20 chars
			startLine: 0,
			col:       10,
			want:      2,
		},
		{
			name:      "input wraps past screen width",
			text:      "123456789012345", // 15 chars
			startLine: 0,
			col:       10,
			want:      2,
		},
		{
			name:      "input with single newline",
			text:      "hello\nworld",
			startLine: 0,
			col:       80,
			want:      2,
		},
		{
			name:      "input with multiple newlines",
			text:      "line1\nline2\nline3",
			startLine: 0,
			col:       80,
			want:      3,
		},
		{
			name:      "wrapped input with newline",
			text:      "12345678901234567890\n12345", // wraps then newline
			startLine: 0,
			col:       10,
			want:      3,
		},
		{
			name:      "scrolled view with startLine non-zero",
			text:      "line1\nline2\nline3\nline4\nline5",
			startLine: 2,
			col:       80,
			want:      3, // lines 2, 3, 4 (zero-indexed: line3, line4, line5)
		},
		{
			name:      "scrolled view beyond content",
			text:      "line1\nline2",
			startLine: 5,
			col:       80,
			want:      1, // Should return at least 1
		},
		{
			name:      "multi-width characters (CJK)",
			text:      "こんにちは", // 5 double-width chars = 10 width
			startLine: 0,
			col:       8,
			want:      2, // Should wrap
		},
		{
			name:      "mixed single and multi-width",
			text:      "hello本日", // 5 + 4 = 9 width, total > col but doesn't trigger wrap
			startLine: 0,
			col:       8,
			want:      1, // lineCharIndex reaches 7, then 9, but never >= 8 BEFORE adding char
		},
		{
			name:      "newline at end",
			text:      "hello\n",
			startLine: 0,
			col:       80,
			want:      2, // The newline moves cursor to a new line, using 2 terminal rows
		},
		{
			name:      "only newlines",
			text:      "\n\n\n",
			startLine: 0,
			col:       80,
			want:      4, // Three newlines create four empty lines (first line + 3 breaks)
		},
		{
			name:      "complex wrapping with scroll",
			text:      "123456789012345\n67890\nabcdefghij", // 3 logical lines, first wraps
			startLine: 1,
			col:       10,
			want:      3, // line 1 (second wrap of first line), line 2, line 3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRenderer()
			r.row = 100 // Set large row to avoid premature cutoff in most tests
			got := r.countInputLines(tt.text, tt.startLine, tt.col)
			if got != tt.want {
				t.Errorf("countInputLines() = %v, want %v", got, tt.want)
			}
		})
	}
}
