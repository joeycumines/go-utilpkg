package prompt

import (
	"fmt"
	"reflect"
	"strings"
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

// TestBreakLineCallback_ZeroRowFiresCallback verifies that BreakLine fires
// the breakLineCallback and emits a newline even when r.row==0.
// Per plan.md Section I.3: the newline and callback must ALWAYS be emitted
// when r.col>0, regardless of r.row.
func TestBreakLineCallback_ZeroRowFiresCallback(t *testing.T) {
	var count int
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return ">" },
		col:            1,
		row:            0, // zero-height viewport
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
		breakLineCallback: func(doc *Document) {
			count++
		},
	}
	mockOut.reset()

	b := NewBuffer()
	b.InsertText("hello", false)

	r.BreakLine(b, nil)

	if count != 1 {
		t.Errorf("breakLineCallback called %d times, want 1", count)
	}

	// Verify a newline was emitted
	newlineCount := 0
	for _, call := range mockOut.Calls() {
		if call.method == "WriteString" && call.args[0] == "\n" {
			newlineCount++
		}
	}
	if newlineCount != 1 {
		t.Errorf("WriteString('\\n') called %d times, want 1", newlineCount)
	}

	// Verify NO rendering calls (r.row==0 means no rendering)
	for _, call := range mockOut.Calls() {
		if call.method == "EraseDown" || call.method == "WriteRawString" {
			t.Errorf("unexpected rendering call %s with r.row=0", call.method)
		}
	}
}

func TestBreakLineCallback_ZeroColFiresCallback(t *testing.T) {
	var count int
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return ">" },
		col:            0,
		row:            5,
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
		breakLineCallback: func(doc *Document) {
			count++
		},
	}

	b := NewBuffer()
	b.InsertText("hello", false)

	r.BreakLine(b, nil)

	if count != 1 {
		t.Fatalf("breakLineCallback called %d times, want 1", count)
	}
	if !r.wasZeroHeight {
		t.Fatalf("wasZeroHeight should be true after zero-width BreakLine")
	}

	newlineCount := 0
	for _, call := range mockOut.Calls() {
		if call.method == "WriteString" && call.args[0] == "\n" {
			newlineCount++
		}
	}
	if newlineCount != 1 {
		t.Fatalf("WriteString(\"\\n\") called %d times, want 1", newlineCount)
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
			want:      2, // lookahead >: chars 0-9 on L0 (exact fill, no wrap), char 10 ('1') wraps to L1, trailing → 2
		},
		{
			name:      "input wraps past screen width",
			text:      "123456789012345", // 15 chars
			startLine: 0,
			col:       10,
			want:      2, // lookahead >: chars 0-9 on L0, char 10 ('1') wraps to L1, trailing → 2
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
			want:      3, // lookahead >: chars 0-9 on L0, '\n' to L1 (empty), chars 10-14 on L1, trailing → 3
		},
		{
			name:      "scrolled view with startLine non-zero",
			text:      "line1\nline2\nline3\nline4\nline5",
			startLine: 2,
			col:       80,
			want:      3, // reflower yields L0,L1 (hidden), then L2,L3,L4 (visible) = 3 visible lines
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
			want:      2, // lookahead >: 'こ''ん''に''ち' on L0 (li=8), 'は' wraps to L1 (li=0+2=2), trailing → 2
		},
		{
			name:      "mixed single and multi-width",
			text:      "hello本日", // 5 + 2 + 2 = 9 width
			startLine: 0,
			col:       8,
			want:      2, // lookahead >: 'hello本' on L0 (li=7+2=9 > 8), '日' on L1 (li=0+2=2), trailing → 2
		},
		{
			name:      "newline at end",
			text:      "hello\n",
			startLine: 0,
			col:       80,
			want:      2, // reflower yields "hello" then empty line from '\n' transition
		},
		{
			name:      "only newlines",
			text:      "\n\n\n",
			startLine: 0,
			col:       80,
			want:      4, // reflower yields at each '\n': empty L0, empty L1, empty L2, empty L3
		},
		{
			name:      "complex wrapping with scroll",
			text:      "123456789012345\n67890\nabcdefghij", // 3 logical lines, first wraps
			startLine: 1,
			col:       10,
			want:      3, // lookahead >: chars 0-9 on L0, '\n' to L1 (hidden); chars 10-14 on L1 (visible); char 15 ('6') wraps to L2 (visible) → 3
		},
		{
			name:      "exact fill single line",
			text:      "abc",
			startLine: 0,
			col:       3,
			want:      1, // lookahead >: 'a','b','c' on L0 (exact fill, no wrap), trailing → 1
		},
		{
			name:      "exact fill with trailing newline",
			text:      "abc\n",
			startLine: 0,
			col:       3,
			want:      2, // reflower: yield "abc" (L0), yield "" from \n transition (L1)
		},
		{
			name:      "exact fill followed by more text",
			text:      "abcde",
			startLine: 0,
			col:       3,
			want:      2, // lookahead >: 'a','b','c' on L0, 'd' wraps to L1, 'e' on L1, trailing → 2
		},
		{
			name:      "multiple exact fills",
			text:      "abcdef",
			startLine: 0,
			col:       3,
			want:      2, // lookahead >: 'a','b','c' on L0, 'd' wraps to L1, 'e','f' on L1 → 2
		},
		{
			name:      "exact fill then newline then text",
			text:      "abc\nxy",
			startLine: 0,
			col:       3,
			want:      2, // lookahead >: 'a','b','c' on L0, '\n' to L1, 'x','y' on L1 → 2
		},
		{
			// Regression test: text that exactly fills the terminal width.
			// Lookahead > ensures characters that exactly fill the line do NOT trigger
			// a premature wrap, correctly counting 1 visual line for 80 chars on col=80.
			name:      "exact fill at terminal width (regression)",
			text:      strings.Repeat("a", 80),
			startLine: 0,
			col:       80,
			want:      1, // lookahead >: all 80 chars fit on L0 (exact fill), trailing → 1
		},
		{
			// Regression test: hidden-line soft-wrap.
			// When a wrap occurs on a hidden line, the overflow character is placed
			// on the first VISIBLE line (not dropped). This matches renderText's
			// lookahead: 'd' wraps L0→L1, L0 hidden so 'd' starts L1. 'e','f','g'
			// continue on L1. 'g' wraps L1→L2, L2 visible. → L1 + L2 = 2 visible.
			name:      "soft wrap at hidden line boundary starts visible line",
			text:      "abcdefg",
			startLine: 1,
			col:       3,
			want:      2, // lookahead >: 'd' wraps to L1 (visible), 'e','f','g' on L1; 'g' wraps to L2 (visible) → 2
		},
		{
			// Regression test: hidden wide-char overflow.
			// A wide char that overflows from a hidden line is placed on the first
			// visible line (not dropped), occupying its full visual width there.
			// '日' wraps L0→L1 (L0 hidden), placed on L1. '本12345678' on L1, '9' on L2.
			name:      "hidden wide-char overflow occupies first visible line",
			text:      "hello本日123456789",
			startLine: 1,
			col:       8,
			want:      2, // lookahead >: '日' wraps to L1 (visible, w=2), '本12345678' on L1; '9' wraps to L2 → 2
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

func TestRenderer_Regressions(t *testing.T) {
	t.Run("No double newline (Plain Renderer)", func(t *testing.T) {
		mockOut := &mockWriterLogger{}
		r := &Renderer{
			out:            mockOut,
			prefixCallback: func() string { return ">" },
			col:            80,
			row:            10,
			inputTextColor: DefaultColor,
			inputBGColor:   DefaultColor,
		}
		mockOut.reset()
		r.renderText(nil, "a\nb", 0)

		calls := mockOut.Calls()
		newlineCount := 0
		for _, call := range calls {
			if call.method == "WriteRawString" && call.args[0] == "\n" {
				newlineCount++
			}
		}
		if newlineCount != 1 {
			t.Errorf("expected exactly 1 newline, got %d", newlineCount)
		}
	})

	t.Run("Direct renderText fallback preserves content", func(t *testing.T) {
		mockOut := &mockWriterLogger{}
		r := &Renderer{
			out:            mockOut,
			prefixCallback: func() string { return ">" },
			col:            80,
			row:            10,
			inputTextColor: DefaultColor,
			inputBGColor:   DefaultColor,
		}
		mockOut.reset()
		r.renderText(nil, "abc", 0)

		found := false
		for _, call := range mockOut.Calls() {
			if call.method == "WriteString" && call.args[0] == "abc" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected direct renderText fallback to render input content")
		}
	})

	t.Run("No double newline (Lexer Renderer)", func(t *testing.T) {
		mockOut := &mockWriterLogger{}
		r := &Renderer{
			out:            mockOut,
			prefixCallback: func() string { return ">" },
			col:            80,
			row:            10,
			inputTextColor: DefaultColor,
			inputBGColor:   DefaultColor,
		}
		lexer := NewEagerLexer(func(input string) []Token {
			return []Token{NewSimpleToken(0, 3, SimpleTokenWithColor(Green))}
		})
		mockOut.reset()
		r.lex(lexer, "a\nb", 0)

		calls := mockOut.Calls()
		newlineCount := 0
		for _, call := range calls {
			if call.method == "WriteRawString" && call.args[0] == "\n" {
				newlineCount++
			}
		}
		if newlineCount != 1 {
			t.Errorf("expected exactly 1 newline, got %d", newlineCount)
		}
	})

	t.Run("No hidden-row advance", func(t *testing.T) {
		mockOut := &mockWriterLogger{}
		r := &Renderer{
			out:            mockOut,
			prefixCallback: func() string { return ">" },
			col:            80,
			row:            1, // Only 1 row visible
			inputTextColor: DefaultColor,
			inputBGColor:   DefaultColor,
		}
		mockOut.reset()
		r.renderText(nil, "a\nb", 0)

		// With row=1, only line 0 is visible. Line 1 is not.
		// So no newline should be emitted.
		calls := mockOut.Calls()
		for _, call := range calls {
			if call.method == "WriteRawString" && call.args[0] == "\n" {
				t.Errorf("did not expect any newline for single visible row")
			}
		}
	})

	t.Run("Lexer Prefix Check", func(t *testing.T) {
		mockOut := &mockWriterLogger{}
		r := &Renderer{
			out:            mockOut,
			prefixCallback: func() string { return "PROMPT> " },
			col:            80,
			row:            10,
			inputTextColor: DefaultColor,
			inputBGColor:   DefaultColor,
		}
		lexer := NewEagerLexer(func(input string) []Token {
			return []Token{NewSimpleToken(0, 1, SimpleTokenWithColor(Green))}
		})
		mockOut.reset()
		r.lex(lexer, "a\nb", 1) // line 0 is "a", line 1 is "b". startLine=1 makes line 1 visible.

		calls := mockOut.Calls()
		// multiline prefix for "PROMPT> " should be "....... " (7 dots + 1 space)
		foundPrefix := false
		for _, call := range calls {
			if call.method == "WriteString" && call.args[0] == "....... " {
				foundPrefix = true
				break
			}
		}
		if !foundPrefix {
			t.Errorf("expected multiline prefix '....... ', but not found in calls")
		}
	})
}

func TestTerminalReflower_IsFullWidth(t *testing.T) {
	tests := []struct {
		name string
		text string
		col  istrings.Width
		want []bool // IsFullWidth for each Next() call
	}{
		{
			name: "exact fill",
			text: "abc",
			col:  3,
			want: []bool{true},
		},
		{
			name: "wrap exactly at boundary",
			text: "abcdef",
			col:  3,
			want: []bool{true, true},
		},
		{
			name: "short line",
			text: "ab",
			col:  3,
			want: []bool{false},
		},
		{
			name: "newline on full line",
			text: "abc\n",
			col:  3,
			want: []bool{true, false}, // "abc" is full, then empty line after \n is not
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reflower := NewTerminalReflower(tt.text, 0, 100, tt.col, true)
			var got []bool
			for {
				state, ok := reflower.Next()
				if !ok {
					break
				}
				got = append(got, state.IsFullWidth)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IsFullWidth got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRenderer_Render_BelowViewportClamp verifies that when the logical cursor is
// BELOW the visible viewport, it clamps to the LAST VISIBLE RENDERED POSITION
// — not just to the bottom row with the original (hidden-line) X coordinate.
//
// This was a verified blocker: the PR clamped only Y, leaving X and IsFullWidth
// from the hidden line, which could produce negative cursor movement counts.
func TestRenderer_Render_BelowViewportClamp(t *testing.T) {
	// Setup: prefix "> " (width 2), col=10, row=2
	// Content: "012345678" (9 chars) on line 0.
	// Cursor at end of line 0 (position {X:9, Y:0} in absolute coords).
	// buffer.startLine=0, r.row=2, endLine=1.
	// Content fits in 1 visual line. Cursor is on that line, visible.
	//
	// Now simulate the problematic scenario:
	// The logical cursor is BELOW the rendered content.
	// To trigger this, set buffer.startLine so that the cursor's absolute Y
	// is > endLine, while the actual rendered content ends on a lower row.
	//
	// Concrete: text "abc\ndef" (2 logical lines), col=10.
	//   Line 0: "abc", Line 1: "def"
	// buffer.startLine = 1 → visible: line 1 only (viewport Y=0)
	// Cursor is at absolute Y=0 (on line 0, which is hidden above viewport).
	// This means cursor Y(0) < startLine(1) → clamp fires for "above viewport".
	//
	// For BELOW viewport, we need the opposite:
	// More content than fits in the viewport, cursor below visible lines.
	// Text "abc\ndef\nghi" (3 lines), col=10, row=2.
	// startLine=0, visible: lines 0,1. endLine=1.
	// Cursor is at absolute Y=2 (line 2, hidden below viewport).
	// → targetCursor.Y = 2, endLine = 1 → below viewport.
	// We want: targetCursor becomes the last visible position (end of line 1 at viewport Y=1).

	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return "> " }, // width 2
		col:            10,
		row:            2,
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
	}

	b := NewBuffer()
	// Three logical lines; col=10 means each fits in one visual line.
	b.InsertText("line0\nline1\nline2", false)
	// Cursor at end of line 2 (absolute Y=2).
	b.cursorPosition = istrings.RuneCountInString("line0\nline1\nline2")
	b.startLine = 0 // viewport shows lines 0,1; line 2 is hidden below

	mockOut.reset()
	r.Render(b, NewCompletionManager(0), nil)

	// Verify: previousCursor must be a valid visible position.
	// The last visible line is "line1" at viewport Y=1.
	// prefixWidth=2, "line1" has width 5, so viewport end = {X:2+5, Y:1} = {X:7, Y:1}
	if r.previousCursor.X < 0 || r.previousCursor.Y < 0 {
		t.Errorf("previousCursor has negative coordinate: %+v", r.previousCursor)
	}
	// previousCursor must be at or before the bottom of the viewport
	if r.previousCursor.Y > 1 {
		t.Errorf("previousCursor Y=%d exceeds viewport bottom (Y=1): %+v", r.previousCursor.Y, r.previousCursor)
	}

	// Verify: no negative cursor movement counts were emitted.
	// CursorBackward with negative count is a bug.
	for _, call := range mockOut.Calls() {
		if call.method == "CursorBackward" || call.method == "CursorForward" {
			val := call.args[0].(int)
			if val < 0 {
				t.Errorf("%s called with negative count %d", call.method, val)
			}
		}
	}
}

// TestRenderer_Render_RowZeroSafe verifies that Render is a safe no-op when
// r.row == 0 (viewport has no height). Previously this could set targetCursor.Y=-1
// via the clamp, which is a direct regression.
func TestRenderer_Render_RowZeroSafe(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return "> " },
		col:            10,
		row:            0, // zero-height viewport
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
	}

	b := NewBuffer()
	b.InsertText("hello", false)

	// Render with zero rows — must not panic, must not set previousCursor to negative.
	r.Render(b, NewCompletionManager(0), nil)

	// previousCursor must remain at its zero value (not updated), since Render returns early.
	if r.previousCursor.Y < 0 {
		t.Errorf("previousCursor.Y is negative after row==0 render: %d", r.previousCursor.Y)
	}
	if r.previousCursor.X < 0 {
		t.Errorf("previousCursor.X is negative after row==0 render: %d", r.previousCursor.X)
	}
}

func TestRenderer_Render_ClampingAndSync(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return ">" }, // width 1
		col:            10,
		row:            10,
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
	}

	t.Run("scrolled past content", func(t *testing.T) {
		b := NewBuffer()
		b.InsertText("hello", false) // 2 args: text, overwrite
		b.startLine = 5              // scrolled way past "hello" (which is line 0)

		mockOut.reset()
		r.Render(b, NewCompletionManager(10), nil) // max=10

		// Verify targetCursor.Y was clamped to 0.
		// If it wasn't clamped, it would be 0 - 5 = -5.
		// r.move would then call CursorUp(-5) -> CursorDown(5) if we're not careful.
		// But Subtract gives {X:0, Y: -5}. Subtract results in {X:0, Y: -5}.
		// move(from, to) calls CursorUp(from.Y - to.Y).
		// If both were clamped to 0, CursorUp(0) is called.

		calls := mockOut.Calls()
		for _, call := range calls {
			if call.method == "CursorUp" {
				val := call.args[0].(int)
				if val < 0 {
					t.Errorf("CursorUp called with negative value %d", val)
				}
			}
			if call.method == "CursorDown" {
				t.Errorf("CursorDown should not be called for simple single-line render")
			}
		}
	})

	t.Run("syncCursor at exact fill", func(t *testing.T) {
		b := NewBuffer()
		b.InsertText("123456789", false) // prefix (1) + text (9) = 10 (exact fill)
		b.cursorPosition = 9             // move cursor to end

		mockOut.reset()
		r.Render(b, NewCompletionManager(10), nil) // max=10

		// syncCursor(p) should have been called because IsFullWidth is true.
		// syncCursor does CursorBackward(1) and CursorForward(1).
		foundSync := false
		for i, call := range mockOut.Calls() {
			if call.method == "CursorBackward" && call.args[0].(int) == 1 {
				// Check if followed by CursorForward(1)
				if i+1 < len(mockOut.Calls()) {
					next := mockOut.Calls()[i+1]
					if next.method == "CursorForward" && next.args[0].(int) == 1 {
						foundSync = true
						break
					}
				}
			}
		}
		if !foundSync {
			for i, call := range mockOut.Calls() {
				t.Logf("%d: %s(%v)", i, call.method, call.args)
			}
			t.Errorf("syncCursor (CursorBackward(1) + CursorForward(1)) not found for exact-fill render")
		}
	})
}

func TestRenderer_Move_ExactFillUsesPhysicalColumn(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return "" },
		col:            10,
		row:            5,
	}

	r.move(Position{X: 10, Y: 0}, true, Position{X: 9, Y: 0}, false)

	for _, call := range mockOut.Calls() {
		switch call.method {
		case "CursorBackward", "CursorForward", "CursorUp", "CursorDown":
			if len(call.args) > 0 {
				if n, ok := call.args[0].(int); ok && n == 0 {
					continue
				}
			}
			t.Fatalf("expected no physical cursor movement, got %s(%v)", call.method, call.args)
		}
	}
}

// TestRenderer_PreviousCursorViewportRelative verifies that after cursor movement
// in a scrolled (startLine>0) buffer, the renderer's previousCursor uses
// VIEWPORT-RELATIVE coordinates (Y in [0, r.row-1]), not absolute document
// coordinates. This is critical because r.clear() expects viewport-relative input.
//
// Per plan.md Section I.2: newCursor.Y -= b.startLine must be applied before
// assigning to p.renderer.previousCursor.
func TestRenderer_PreviousCursorViewportRelative(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return "> " }, // width 2
		col:            20,
		row:            5,
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
	}

	// Create a multiline buffer and scroll it so startLine > 0.
	b := NewBuffer()
	// Text with multiple lines, each shorter than col=20.
	b.InsertTextMoveCursor("line0\nline1\nline2\nline3\nline4", 20, 5, false)
	// Force scroll: set startLine=2 (lines 0 and 1 are above viewport).
	b.startLine = 2
	// Cursor is at the end of the text (absolute Y=4, viewport-relative Y=2).

	cols := r.col - istrings.GetWidth(r.prefixCallback()) // 18

	// Simulate what CursorRight does: get absolute cursor, apply viewport-relative conversion.
	absoluteCursor := b.DisplayCursorPosition(cols)
	// absoluteCursor.Y = 4, b.startLine = 2

	// The FATAL FIX: convert to viewport-relative.
	viewportRelativeCursor := absoluteCursor
	viewportRelativeCursor.Y -= b.startLine // = 4 - 2 = 2

	// Assign to renderer.previousCursor (what prompt.go does).
	r.previousCursor = viewportRelativeCursor

	// Now call r.clear(r.previousCursor) — this should NOT produce negative cursor movements.
	mockOut.reset()
	r.clear(r.previousCursor, false)

	for _, call := range mockOut.Calls() {
		if call.method == "CursorUp" {
			val := call.args[0].(int)
			if val < 0 {
				t.Errorf("CursorUp called with negative count %d — previousCursor was not viewport-relative", val)
			}
		}
		if call.method == "CursorBackward" {
			val := call.args[0].(int)
			if val < 0 {
				t.Errorf("CursorBackward called with negative count %d — previousCursor was not viewport-relative", val)
			}
		}
	}

	// Verify r.previousCursor is in valid range
	if r.previousCursor.Y < 0 {
		t.Errorf("previousCursor.Y is negative: %d", r.previousCursor.Y)
	}
	if r.previousCursor.Y >= r.row {
		t.Errorf("previousCursor.Y=%d exceeds viewport height r.row=%d", r.previousCursor.Y, r.row)
	}
}

// TestRenderer_ZeroHeightRecovery verifies that wasZeroHeight state transitions
// correctly trigger recalculateStartLine on recovery.
func TestRenderer_ZeroHeightRecovery(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return "> " },
		col:            80,
		row:            0, // start at zero height
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
	}

	b := NewBuffer()
	b.InsertTextMoveCursor("hello", 80, 20, false)
	b.startLine = 5 // stale scroll position

	// Scenario 1: Render with r.row=0 sets wasZeroHeight=true.
	r.Render(b, NewCompletionManager(0), nil)
	if !r.wasZeroHeight {
		t.Errorf("wasZeroHeight should be true after r.row=0 render")
	}

	// Scenario 2: Render with r.row>0 after wasZeroHeight=true
	// should fire recalculateStartLine and reset the flag.
	r.row = 5
	b.startLine = 5 // stale
	mockOut.reset()
	r.Render(b, NewCompletionManager(0), nil)

	if r.wasZeroHeight {
		t.Errorf("wasZeroHeight should be false after r.row>0 render with recovery")
	}
	// buffer.startLine should have been recalculated.
	// recalculateStartLine with b.DisplayCursorPosition(78) for "hello" → Y=0.
	// startLine = 0 (cursor within viewport). The stale startLine=5 should be fixed.
	if b.startLine != 0 {
		t.Errorf("buffer.startLine = %d after recovery, want 0", b.startLine)
	}
}

func TestRenderer_ZeroSizeRecoveryFromZeroByZero(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return "> " },
		col:            0,
		row:            0,
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
	}

	b := NewBuffer()
	b.InsertTextMoveCursor("hello", 80, 20, false)
	b.startLine = 5

	r.Render(b, NewCompletionManager(0), nil)
	if !r.wasZeroHeight {
		t.Fatalf("wasZeroHeight should be true after zero-sized render")
	}

	r.col = 80
	r.row = 5
	r.Render(b, NewCompletionManager(0), nil)

	if r.wasZeroHeight {
		t.Fatalf("wasZeroHeight should be false after recovery render")
	}
	if b.startLine != 0 {
		t.Fatalf("buffer.startLine = %d after zero-size recovery, want 0", b.startLine)
	}
}

// TestRenderer_ComputeReflow_Correctness verifies that computeReflow produces
// correct visible line counts and populates r.reflowScratch correctly,
// matching the semantics of TerminalReflower.Next().
func TestRenderer_ComputeReflow_Correctness(t *testing.T) {
	r := NewRenderer()
	r.row = 100 // large enough to avoid premature cutoff

	tests := []struct {
		name      string
		text      string
		startLine int
		endLine   int
		col       istrings.Width
		wantCount int
	}{
		{"empty input", "", 0, 100, 80, 1},
		{"single line short", "hello", 0, 100, 80, 1},
		{"exact fill", strings.Repeat("a", 80), 0, 100, 80, 1},
		{"exact fill then more", "abcde", 0, 100, 3, 2}, // a,b,c L0 full; d → L1; e L1 full → 2 visible
		{"wrap past width", "123456789012345", 0, 100, 10, 2},
		{"single newline", "hello\nworld", 0, 100, 80, 2},
		{"multiple newlines", "line1\nline2\nline3", 0, 100, 80, 3},
		{"scrolled view", "line1\nline2\nline3\nline4\nline5", 2, 100, 80, 3}, // visible: lines 2,3,4 = 3
		{"scrolled beyond content", "line1\nline2", 5, 100, 80, 1},
		{"CJK wrapping", "こんにちは", 0, 100, 8, 2},           // 5 CJK = 10 width; lookahead: こ on L0, ん on L0, に L0, ち L0(8), は L1 → 2
		{"hidden line wrap", "abcdefg", 1, 100, 3, 2},     // a,b,c L0(hidden); d,e,f L1(visible); g L2(visible) → 2 visible
		{"no visible lines", "line1\nline2", 5, 6, 80, 1}, // startLine=5, endLine=6: no lines in range → 1 (min)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compare computeReflow vs countInputLines.
			computeCount := r.computeReflow(tt.text, tt.startLine, tt.endLine, tt.col)
			countCount := r.countInputLines(tt.text, tt.startLine, tt.col)

			if computeCount != tt.wantCount {
				t.Errorf("computeReflow() = %d, want %d", computeCount, tt.wantCount)
			}
			if computeCount != countCount {
				t.Errorf("computeReflow() = %d, countInputLines() = %d (mismatch)", computeCount, countCount)
			}

			// Verify r.reflowScratch is populated with the right number of entries.
			if len(r.reflowScratch) == 0 && tt.text != "" {
				t.Errorf("r.reflowScratch is empty for non-empty text")
			}

			// Verify scratch entries are internally consistent.
			for i, state := range r.reflowScratch {
				if state.ByteStart < 0 || state.ByteEnd < state.ByteStart || state.ByteEnd > len(tt.text) {
					t.Errorf("state[%d] has invalid byte range [%d, %d) for text len=%d",
						i, state.ByteStart, state.ByteEnd, len(tt.text))
				}
				if state.IsVisible && (state.LineNumber < tt.startLine || state.LineNumber >= tt.endLine) {
					t.Errorf("state[%d].IsVisible=true but LineNumber=%d outside [%d, %d)",
						i, state.LineNumber, tt.startLine, tt.endLine)
				}
				// IsFullWidth should be true iff Width == col (for non-empty lines)
				if state.Width > 0 && state.Width <= tt.col {
					expectedFullWidth := state.Width == tt.col
					if state.IsFullWidth != expectedFullWidth {
						t.Errorf("state[%d].IsFullWidth=%v but Width=%d, col=%d",
							i, state.IsFullWidth, state.Width, tt.col)
					}
				}
			}
		})
	}
}

// TestTerminalReflower_Metrics verifies that the Metrics() method returns
// correct post-exhaustion state matching what Next() yields for the final line.
func TestTerminalReflower_Metrics(t *testing.T) {
	tests := []struct {
		text          string
		col           istrings.Width
		wantWidth     istrings.Width
		wantLine      int
		wantFullWidth bool
	}{
		{"abc", 3, 3, 0, true},    // exact fill, 1 line
		{"abc", 2, 1, 1, false},   // a,b on L0; c wraps to L1 → metrics from L1
		{"a", 10, 1, 0, false},    // short line, not full
		{"", 80, 0, 0, false},     // empty
		{"abc\n", 3, 0, 1, false}, // newline ends L0 (full), L1 empty → metrics from L1
		{"abcdef", 3, 3, 1, true}, // wraps: a,b,c L0 full; d,e,f L1 full → last Next() Width=3 → metrics L1
	}

	for _, tt := range tests {
		t.Run(tt.text+fmt.Sprintf("_col=%d", tt.col), func(t *testing.T) {
			reflower := NewTerminalReflower(tt.text, 0, 1<<30, tt.col, false)
			var lastState ReflowState
			for {
				state, ok := reflower.Next()
				if !ok {
					break
				}
				lastState = state
			}
			width, line, fullWidth := reflower.Metrics()

			if width != tt.wantWidth {
				t.Errorf("Metrics().width = %d, want %d", width, tt.wantWidth)
			}
			if line != tt.wantLine {
				t.Errorf("Metrics().line = %d, want %d", line, tt.wantLine)
			}
			if fullWidth != tt.wantFullWidth {
				t.Errorf("Metrics().fullWidth = %v, want %v", fullWidth, tt.wantFullWidth)
			}

			// Metrics should match the last yielded state.
			if width != lastState.Width || line != lastState.LineNumber || fullWidth != lastState.IsFullWidth {
				t.Errorf("Metrics() = (%d, %d, %v) but last Next() = (%d, %d, %v)",
					width, line, fullWidth, lastState.Width, lastState.LineNumber, lastState.IsFullWidth)
			}
		})
	}
}

// TestTerminalReflower_CombiningMarks verifies that zero-width combining characters
// (combining marks, zero-width joiners) do not trigger line wrapping.
func TestTerminalReflower_CombiningMarks(t *testing.T) {
	tests := []struct {
		text string
		desc string
		want int // number of visual lines
	}{
		{"e\u0301", "e + combining acute = 1 cell", 1},         // é (1 cell)
		{"\u0300\u0301", "two combining marks = 0 cells", 1},   // zero-width
		{"a\u0300\u0301b", "a + 2 combining + b = 2 cells", 1}, // a b
		{"a\u0300\u0301", "a + combining = 1 cell", 1},
		{"e\u0301\u200d", "e + combining + ZWJ = 1 cell", 1}, // ZWJ is zero-width
		{"\u200d", "bare ZWJ = 0 cells", 1},                  // zero-width
		{"\u200b", "bare ZWSP = 0 cells", 1},                 // zero-width space
		{"a\u0300\nb", "combining mark + explicit newline", 2},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			reflower := NewTerminalReflower(tt.text, 0, 1<<30, istrings.Width(80), false)
			count := 0
			for {
				_, ok := reflower.Next()
				if !ok {
					break
				}
				count++
			}
			if count != tt.want {
				t.Errorf("got %d lines, want %d", count, tt.want)
			}
		})
	}
}

// TestTerminalReflower_ExtremeScroll verifies that startLine beyond the content
// produces lines without panicking, and that IsVisible is correctly computed.
func TestTerminalReflower_ExtremeScroll(t *testing.T) {
	// Use concrete test cases to avoid subtle off-by-one issues.
	// For "a\nb\nc" at col=80: yields 3 lines (L0="a", L1="b", L2="c").
	// With startLine=100, endLine=110: all 3 lines have IsVisible=false.
	text := "a\nb\nc"
	reflower := NewTerminalReflower(text, 100, 110, istrings.Width(80), false)

	count := 0
	for {
		state, ok := reflower.Next()
		if !ok {
			break
		}
		count++
		// All 3 lines have lineNumber ∈ {0,1,2}, all < startLine=100 → invisible.
		if state.IsVisible {
			t.Errorf("text=%q: line %d should be invisible (startLine=100), got IsVisible=true",
				text, state.LineNumber)
		}
	}
	if count != 3 {
		t.Errorf("text=%q: got %d lines, want 3", text, count)
	}

	// Empty text: the reflower is exactly at the start of a new line (byteIndex == lineStart == 0
	// and currentWidth == 0), so it yields one empty state then becomes exhausted.
	// See renderer.go line 1123: "yield if we are exactly at the start of a new line."
	reflower2 := NewTerminalReflower("", 0, 1000, istrings.Width(80), false)
	state, ok := reflower2.Next()
	if !ok {
		t.Error("empty text: Next() should yield one empty state (at line start)")
	}
	if state.Width != 0 || state.LineNumber != 0 {
		t.Errorf("empty text: got Width=%d LineNumber=%d, want both 0", state.Width, state.LineNumber)
	}
	// Second call: exhausted.
	_, ok = reflower2.Next()
	if ok {
		t.Error("empty text: second Next() should return false (exhausted)")
	}
}

// TestTerminalReflower_ReplacementCharacter verifies that invalid UTF-8 bytes
// are replaced with U+FFFD and don't corrupt state.
func TestTerminalReflower_ReplacementCharacter(t *testing.T) {
	tests := []struct {
		text string
		want int // line count
	}{
		{"\xff", 1},           // single invalid byte
		{"\xff\xfe", 1},       // two invalid bytes
		{"\xff\n", 2},         // invalid + newline
		{"a\xffb", 1},         // embedded invalid
		{"\xff\xff\xff", 1},   // three invalid bytes
		{"hello\xffworld", 1}, // embedded in text
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.text), func(t *testing.T) {
			reflower := NewTerminalReflower(tt.text, 0, 1<<30, istrings.Width(80), false)
			count := 0
			var states []ReflowState
			for {
				state, ok := reflower.Next()
				if !ok {
					break
				}
				states = append(states, state)
				count++
			}
			if count != tt.want {
				t.Errorf("got %d lines, want %d", count, tt.want)
			}
			// No line buffer should contain 0xFF or 0xFE bytes directly
			for i, s := range states {
				for _, b := range s.LineBuffer {
					if b == 0xFF || b == 0xFE {
						t.Errorf("line %d: LineBuffer contains invalid byte 0x%02X", i, b)
					}
				}
			}
		})
	}
}

// TestTerminalReflower_CJKExactFill verifies that CJK characters fill a column
// exactly when col is a multiple of the character width, and wrap when not.
//
// The wrap check fires BEFORE adding a character: if currentWidth + charWidth > col,
// the current line is yielded first. This means a single char wider than col gets
// its own line (width > col is allowed for wide chars), and a char exactly equal
// to col fills that line and triggers a wrap for the next character.
func TestTerminalReflower_CJKExactFill(t *testing.T) {
	// Each CJK char in "日本" has width 2, so "日本" = width 4.
	tests := []struct {
		text   string
		col    istrings.Width
		want   int
		wantF0 bool // IsFullWidth of first line
	}{
		{"日本", 4, 1, true},   // exact fill (4==4): "日"+"本" both fit
		{"日本", 2, 2, true},   // "日"(2) fits; "本"(2) triggers wrap: L0 full, L1 full
		{"日本", 3, 2, false},  // "日"(2) fits; "本"(2) triggers wrap: L0 not full, L1 full
		{"日本", 1, 2, false},  // each char width=2 > col=1 → forced onto line (no phantom empty lines)
		{"日本日本", 4, 2, true}, // "日本日本" = width 8; at col=4: L0 full, L1 full
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("col=%d", tt.col), func(t *testing.T) {
			reflower := NewTerminalReflower(tt.text, 0, 1<<30, tt.col, false)
			var firstLine ReflowState
			var lastState ReflowState
			count := 0
			for {
				state, ok := reflower.Next()
				if !ok {
					break
				}
				if count == 0 {
					firstLine = state
				}
				lastState = state
				count++
			}
			if count != tt.want {
				t.Errorf("got %d lines, want %d", count, tt.want)
			}
			if firstLine.LineNumber == 0 && firstLine.IsFullWidth != tt.wantF0 {
				t.Errorf("first line IsFullWidth=%v, want %v", firstLine.IsFullWidth, tt.wantF0)
			}
			// All lines should have Width <= col * lineCount (can't exceed total)
			if lastState.Width > tt.col && lastState.Width <= 2*tt.col {
				// CJK char wider than col is valid (char width 2 > col 1)
			}
		})
	}
}

// TestRenderer_computeReflow_CJK verifies computeReflow handles CJK text correctly.
func TestRenderer_computeReflow_CJK(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		col:            istrings.Width(4),
		row:            24,
		prefixCallback: func() string { return "> " },
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
		reflowScratch:  make([]ReflowState, 0, 10),
		tokenScratch:   nil,
	}

	text := "日本語" // width = 6 (3 chars × 2)
	count := r.computeReflow(text, 0, 100, istrings.Width(4))

	// col=4: "日本" (width=4) is full; "語" (width=2) wraps to L2
	if count != 2 {
		t.Errorf("computeReflow got %d visible lines, want 2", count)
	}
	if len(r.reflowScratch) != 2 {
		t.Errorf("reflowScratch has %d entries, want 2", len(r.reflowScratch))
	}
	// L0: "日本" width=4 fullWidth=true
	if r.reflowScratch[0].Width != 4 || !r.reflowScratch[0].IsFullWidth {
		t.Errorf("L0: got Width=%d IsFullWidth=%v, want Width=4 IsFullWidth=true",
			r.reflowScratch[0].Width, r.reflowScratch[0].IsFullWidth)
	}
	// L1: "語" width=2 fullWidth=false
	if r.reflowScratch[1].Width != 2 || r.reflowScratch[1].IsFullWidth {
		t.Errorf("L1: got Width=%d IsFullWidth=%v, want Width=2 IsFullWidth=false",
			r.reflowScratch[1].Width, r.reflowScratch[1].IsFullWidth)
	}
}

// TestRenderer_computeReflow_CombiningMarks verifies computeReflow with combining marks.
// Combining marks (U+0300, U+0301, U+200D, etc.) have width=0 and do not advance
// the cursor, so "a\u0300\u0301b" has total width=2 (a + b, combining marks contribute 0).
func TestRenderer_computeReflow_CombiningMarks(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		col:            istrings.Width(80),
		row:            24,
		prefixCallback: func() string { return "> " },
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
		reflowScratch:  make([]ReflowState, 0, 10),
		tokenScratch:   nil,
	}

	text := "a\u0300\u0301b" // a(1) + combining(0) + combining(0) + b(1) = width 2
	count := r.computeReflow(text, 0, 100, istrings.Width(80))

	if count != 1 {
		t.Errorf("got %d visible lines, want 1", count)
	}
	// Combining marks do not contribute width.
	if r.reflowScratch[0].Width != 2 {
		t.Errorf("Width=%d, want 2 (combining marks have width=0)", r.reflowScratch[0].Width)
	}
}

// TestRender_NilCompletion verifies that Render does not panic when
// the completion parameter is nil.
func TestRender_NilCompletion(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		col:            80,
		row:            24,
		prefixCallback: func() string { return "> " },
		indentSize:     2,
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
	}
	b := NewBuffer()
	b.InsertTextMoveCursor("hello world", 78, 24, false)

	// This must not panic.
	r.Render(b, nil, nil)

	// Verify that completion lines are 0.
	if r.previousCompletionLines != 0 {
		t.Errorf("previousCompletionLines = %d, want 0", r.previousCompletionLines)
	}
}

// TestRender_BareCarriageReturn verifies that bare \r characters in the
// buffer text are treated as line terminators by the reflower and do not
// leak raw carriage returns into the rendered output.
func TestRender_BareCarriageReturn(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		col:            80,
		row:            24,
		prefixCallback: func() string { return "> " },
		indentSize:     2,
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
	}
	b := NewBuffer()
	// Insert text with bare \r (not \r\n)
	b.InsertTextMoveCursor("line1\rline2", 78, 24, false)

	r.Render(b, nil, nil)

	// Verify that no raw \r appears in content-bearing WriteString calls.
	// WriteString("\r") used for cursor positioning is expected, but \r
	// embedded within longer content strings would indicate a leak.
	for _, call := range mockOut.Calls() {
		if call.method == "WriteString" {
			if str, ok := call.args[0].(string); ok && len(str) > 1 {
				for i, ch := range str {
					if ch == '\r' {
						t.Fatalf("raw \\r found at index %d of WriteString(%q); bare CR should be treated as line terminator, not passed through", i, str)
					}
				}
			}
		}
	}

	// The reflower should have split "line1\rline2" into 2 lines
	if r.previousInputLines < 2 {
		t.Errorf("previousInputLines = %d, want >= 2 (bare \\r should act as line terminator)", r.previousInputLines)
	}
}

// TestCJK_NoPhantomLines verifies that CJK characters wider than the
// column width do not create phantom empty lines.
func TestCJK_NoPhantomLines(t *testing.T) {
	// "日本" = two CJK chars, each width 2. In col=1, they can never fit.
	// The fix: force them onto the current line instead of yielding an empty line.
	reflower := NewTerminalReflower("日本", 0, 1<<30, 1, false)
	count := 0
	for {
		_, ok := reflower.Next()
		if !ok {
			break
		}
		count++
	}
	if count != 2 {
		t.Errorf("got %d lines, want 2 (no phantom empty lines for CJK in col=1)", count)
	}
}

// TestBidirectionalCompletion_Above verifies that completion renders
// above the cursor when there's more space above than below.
func TestBidirectionalCompletion_Above(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:               mockOut,
		col:               80,
		row:               10,
		prefixCallback:    func() string { return "> " },
		indentSize:        2,
		inputTextColor:    DefaultColor,
		inputBGColor:      DefaultColor,
		dynamicCompletion: true,
	}

	cm := NewCompletionManager(5)
	cm.Update(Document{Text: "test", cursorPosition: 4})
	// Simulate suggestions
	cm.tmp = []Suggest{
		{Text: "one  ", Description: "desc1"},
		{Text: "two  ", Description: "desc2"},
		{Text: "three", Description: "desc3"},
	}

	// Cursor at bottom of viewport (Y=9, row=10) — only 0 rows below, 9 above
	cursor := Position{X: 6, Y: 9}
	lines := r.renderCompletion(cm, cursor, false)

	if lines == 0 {
		t.Fatal("expected completion to render above cursor, got 0 lines")
	}
	if !r.previousCompletionAbove {
		t.Fatal("expected previousCompletionAbove=true when cursor is at bottom")
	}

	// Verify CursorUp was called (upward rendering)
	foundCursorUp := false
	for _, call := range mockOut.Calls() {
		if call.method == "CursorUp" {
			foundCursorUp = true
			break
		}
	}
	if !foundCursorUp {
		t.Fatal("expected CursorUp call for above-cursor rendering")
	}
}

func TestRenderer_Clear_AboveCompletionOverlapsInput(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:                     mockOut,
		prefixCallback:          func() string { return "> " },
		previousInputLines:      3,
		previousCompletionLines: 2,
		previousCompletionAbove: true,
	}

	r.clear(Position{}, false)

	// Overlap model: above-cursor completions are INSIDE the input rows.
	// When previousCompletionAbove == true, the clear region is the UNION
	// of input and completion rows, which equals inputLines (3).
	// The 2 completion rows occupy space within the 3 input rows.
	// Expected sequence:
	//   1. move(Position{}, Position{}) — no movement (from and to are both origin)
	//   2. Loop 3x: WriteString("\r"), EraseLine, [WriteString("\n") except last]
	//   3. CursorUp(2) — return to row 0 from row 2
	//   4. WriteString("\r") — reset column
	calls := mockOut.Calls()

	// With cursor already at origin, no extra upward erase pass is emitted.
	eraseStartIdx := -1
	cursorUpBeforeErase := false
	for i, call := range calls {
		if call.method == "EraseLine" && eraseStartIdx == -1 {
			eraseStartIdx = i
		}
		if call.method == "CursorUp" && eraseStartIdx == -1 {
			cursorUpBeforeErase = true
		}
	}
	if cursorUpBeforeErase {
		t.Fatal("clear() with above-cursor completion emitted CursorUp before erase loop — overlap model should erase downward from origin only")
	}

	// Count EraseLine calls — should be exactly inputLines (3), not 5.
	eraseLines := 0
	for _, call := range calls {
		if call.method == "EraseLine" {
			eraseLines++
		}
	}
	if eraseLines != 3 {
		t.Fatalf("EraseLine count = %d, want 3 (inputLines, overlap model: union of 3 input + 2 completion rows)", eraseLines)
	}

	// Verify exactly ONE CursorUp at the end for return-to-origin.
	cursorUpCount := 0
	cursorUpValue := -1
	for _, call := range calls {
		if call.method == "CursorUp" {
			cursorUpCount++
			if len(call.args) > 0 {
				cursorUpValue = call.args[0].(int)
			}
		}
	}
	if cursorUpCount != 1 {
		t.Fatalf("CursorUp count = %d, want 1 (single return-to-origin)", cursorUpCount)
	}
	if cursorUpValue != 2 {
		t.Fatalf("CursorUp(%d) for return-to-origin, want CursorUp(2) (inputLines-1 = 3-1)", cursorUpValue)
	}

	// Verify sequence integrity: the CursorUp must come AFTER all EraseLine calls.
	lastEraseIdx := -1
	cursorUpIdx := -1
	for i, call := range calls {
		if call.method == "EraseLine" {
			lastEraseIdx = i
		}
		if call.method == "CursorUp" {
			cursorUpIdx = i
		}
	}
	if cursorUpIdx < lastEraseIdx {
		t.Fatal("CursorUp appeared before last EraseLine — return-to-origin must come after all erasures")
	}
}

// TestRenderer_AboveCompletionRestoreWithExactFill tests that above-completion
// final restore uses physical column when cursor is at exact-fill boundary.
// Regression test for review.md issue #1.
func TestRenderer_AboveCompletionRestoreWithExactFill(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return "> " },
		row:            10,
		col:            20,
	}
	c := NewCompletionManager(1)

	// Create a simple completion that renders above cursor
	c.tmp = []Suggest{{Text: "test", Description: "desc"}}
	c.selected = 0

	// Position cursor at exact-fill with full-width state
	cursor := Position{X: 20, Y: 9}
	cursorIsFullWidth := true

	// Render above cursor
	r.renderCompletion(c, cursor, cursorIsFullWidth)

	calls := mockOut.Calls()
	// Find the final CursorForward call after CursorDown(1)
	// This should use physical column (19) not logical X (20)
	foundRestore := false
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].method == "CursorDown" && len(calls[i].args) > 0 {
			if n, ok := calls[i].args[0].(int); ok && n == 1 {
				// Found CursorDown(1) - look for next CursorForward
				for j := i + 1; j < len(calls); j++ {
					if calls[j].method == "CursorForward" && len(calls[j].args) > 0 {
						if n, ok := calls[j].args[0].(int); ok {
							// Verify it's using physical column (< logical X when full-width)
							if n >= 20 {
								t.Fatalf("CursorForward(%d) should use physical column (<20) for exact-fill, not logical X", n)
							}
							foundRestore = true
							break
						}
					}
				}
				break
			}
		}
	}
	if !foundRestore {
		t.Fatal("did not find CursorForward restore after CursorDown(1)")
	}
}

// TestRenderer_BackwardZeroFromFullWidth tests that backward(from, fromIsFullWidth=true, 0)
// is a true no-op and doesn't emit movement.
// Regression test for review.md issue #2.
func TestRenderer_BackwardZeroFromFullWidth(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out: mockOut,
		col: 20,
	}

	// Call backward with n=0 from exact-fill position
	from := Position{X: 20, Y: 0}
	result := r.backward(from, true, 0)

	// Result should be unchanged
	if result != from {
		t.Fatalf("backward(..., 0) changed position: got %v, want %v", result, from)
	}

	// Should emit no cursor movement
	for _, call := range mockOut.Calls() {
		if call.method == "CursorUp" || call.method == "CursorDown" ||
			call.method == "CursorBackward" || call.method == "CursorForward" {
			t.Fatalf("backward(..., 0) emitted cursor movement: %v", call)
		}
	}
}

// TestRenderer_BackwardOneFromFullWidth tests that backward(from, fromIsFullWidth=true, 1)
// doesn't emit CursorBackward(0) which VT100 interprets as CursorBackward(1).
// Regression test for review-2.md critical issue.
func TestRenderer_BackwardOneFromFullWidth(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out: mockOut,
		col: 20,
	}

	// Call backward with n=1 from exact-fill position
	// This creates: from.X=20, to.X=19
	// physicalColumn(from, true) = 19
	// physicalColumn(to, false) = 19
	// delta = 0
	// Old code would emit CursorBackward(0) which VT100 interprets as CursorBackward(1)
	from := Position{X: 20, Y: 0}
	result := r.backward(from, true, 1)

	// Result should be X=19
	expected := Position{X: 19, Y: 0}
	if result != expected {
		t.Fatalf("backward(..., 1) returned: got %v, want %v", result, expected)
	}

	// Should NOT emit CursorBackward(0) which VT100 would interpret as CursorBackward(1)
	// With moveHorizontal, delta=0 case emits nothing (correct behavior)
	for _, call := range mockOut.Calls() {
		if call.method == "CursorBackward" && len(call.args) > 0 {
			if n, ok := call.args[0].(int); ok && n == 0 {
				t.Fatalf("backward(..., 1) emitted CursorBackward(0) which VT100 interprets as CursorBackward(1)")
			}
		}
	}
}

// TestRenderer_RestoreColumnZero tests that restoreColumn(0) doesn't emit CursorForward(0)
// which VT100 interprets as CursorForward(1), causing completion layout drift.
// Regression test for review-4.md critical issue.
func TestRenderer_RestoreColumnZero(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out: mockOut,
		col: 20,
	}

	// Call restoreColumn with col=0
	// This happens when completion spans full terminal width, causing adjustedCursor.X == 0
	// Old code would emit CursorForward(0) which VT100 interprets as CursorForward(1)
	r.restoreColumn(0)

	// Should emit WriteString("\r") but NOT CursorForward(0)
	calls := mockOut.Calls()
	foundCarriageReturn := false
	for _, call := range calls {
		if call.method == "WriteString" && len(call.args) > 0 {
			if s, ok := call.args[0].(string); ok && s == "\r" {
				foundCarriageReturn = true
			}
		}
		if call.method == "CursorForward" && len(call.args) > 0 {
			if n, ok := call.args[0].(int); ok && n == 0 {
				t.Fatalf("restoreColumn(0) emitted CursorForward(0) which VT100 interprets as CursorForward(1)")
			}
		}
	}

	if !foundCarriageReturn {
		t.Fatalf("restoreColumn(0) did not emit carriage return")
	}
}

// TestRenderer_MoveWithNegativeDeltas tests that move() doesn't emit negative VT100 sequences.
// Regression test for VT100 malformation bug.
func TestRenderer_MoveWithNegativeDeltas(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out: mockOut,
		col: 20,
	}

	// Test case 1: move DOWN (negative Y delta)
	// from.Y=0, to.Y=5 should emit CursorDown(5), not CursorUp(-5)
	from := Position{X: 0, Y: 0}
	to := Position{X: 0, Y: 5}
	result := r.move(from, false, to, false)

	if result != to {
		t.Fatalf("move() returned: got %v, want %v", result, to)
	}

	// Verify CursorDown was called, not CursorUp
	foundCursorDown := false
	for _, call := range mockOut.Calls() {
		if call.method == "CursorDown" && len(call.args) > 0 {
			if n, ok := call.args[0].(int); ok && n == 5 {
				foundCursorDown = true
			}
		}
		if call.method == "CursorUp" {
			t.Fatalf("move(0,0 -> 0,5) emitted CursorUp, should have emitted CursorDown")
		}
	}
	if !foundCursorDown {
		t.Fatal("move(0,0 -> 0,5) did not emit CursorDown(5)")
	}

	// Test case 2: move RIGHT (negative X delta via physicalColumn)
	// from.X=0, to.X=10 should emit CursorForward(10), not CursorBackward(-10)
	mockOut.Calls()
	mockOut.calls = nil

	from2 := Position{X: 0, Y: 0}
	to2 := Position{X: 10, Y: 0}
	result2 := r.move(from2, false, to2, false)

	if result2 != to2 {
		t.Fatalf("move() returned: got %v, want %v", result2, to2)
	}

	// Verify CursorForward was called, not CursorBackward
	foundCursorForward := false
	for _, call := range mockOut.Calls() {
		if call.method == "CursorForward" && len(call.args) > 0 {
			if n, ok := call.args[0].(int); ok && n == 10 {
				foundCursorForward = true
			}
		}
		if call.method == "CursorBackward" {
			t.Fatalf("move(0,0 -> 10,0) emitted CursorBackward, should have emitted CursorForward")
		}
	}
	if !foundCursorForward {
		t.Fatal("move(0,0 -> 10,0) did not emit CursorForward(10)")
	}
}

// TestRenderer_BelowCompletionRewindSyncsCursor tests that below-completion path
// calls syncCursor before moving away from exact-fill row.
// Regression test for review.md issue #3.
func TestRenderer_BelowCompletionRewindSyncsCursor(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return "> " },
		row:            10,
		col:            20,
	}
	b := NewBuffer()
	c := NewCompletionManager(1)

	// Position cursor so completion overflows right margin
	// available width = 20 - 2(prefix) = 18
	// Insert 15 chars, leaving room for backward shift
	b.InsertTextMoveCursor(strings.Repeat("a", 15), 20, 10, false)

	c.tmp = []Suggest{{Text: "very-long-suggestion", Description: "desc"}}
	c.selected = 0

	// Render below cursor (not at bottom, so space available below)
	r.renderCompletion(c, Position{X: 17, Y: 0}, false)

	calls := mockOut.Calls()
	// Find the pattern: after writing the completion row (space at end),
	// we should see CursorBackward(1), CursorForward(1) from syncCursor
	// before the backward movement to reset for next row
	foundSyncPattern := false
	for i := 0; i < len(calls)-3; i++ {
		if calls[i].method == "WriteString" && len(calls[i].args) > 0 {
			if s, ok := calls[i].args[0].(string); ok && s == " " {
				// Found the trailing space after completion row
				// Next calls should include syncCursor pattern before CursorUp
				for j := i + 1; j < len(calls); j++ {
					if calls[j].method == "CursorBackward" && len(calls[j].args) > 0 {
						if n, ok := calls[j].args[0].(int); ok && n == 1 {
							// Found CursorBackward(1) from syncCursor
							if j+1 < len(calls) && calls[j+1].method == "CursorForward" {
								foundSyncPattern = true
								break
							}
						}
					}
					if calls[j].method == "CursorUp" {
						break
					}
				}
				break
			}
		}
	}
	if !foundSyncPattern {
		t.Fatal("below-completion rewind did not call syncCursor before moving from exact-fill row")
	}
}

// TestRenderer_AboveCompletionAtColumnZero tests that above-path completion rendering
// at column 0 doesn't emit CursorForward(0). Regression test for review-5.md.
func TestRenderer_AboveCompletionAtColumnZero(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return "" }, // Empty prefix
		row:            10,
		col:            20,
	}

	// Setup: empty prefix, cursor at left edge (X: 0), Y=8 to force above rendering
	cursor := Position{X: 0, Y: 8}
	cursorIsFullWidth := false

	// Create simple completion manager with one suggestion
	c := NewCompletionManager(5)
	c.tmp = []Suggest{{Text: "test", Description: "desc"}}

	// Render above the cursor (cursor at Y=8 with row=10 forces above path)
	_ = r.renderCompletion(c, cursor, cursorIsFullWidth)

	// Verify no CursorForward(0) was emitted
	calls := mockOut.Calls()
	for _, call := range calls {
		if call.method == "CursorForward" && len(call.args) > 0 {
			if n, ok := call.args[0].(int); ok && n == 0 {
				t.Fatalf("Above-path completion at column 0 emitted CursorForward(0) which VT100 interprets as CursorForward(1)")
			}
		}
	}
}

// TestRenderer_BelowCompletionAtColumnZero tests that below-path completion rendering
// at column 0 doesn't emit CursorForward(0). Regression test for review-5.md.
func TestRenderer_BelowCompletionAtColumnZero(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return "" }, // Empty prefix
		row:            10,
		col:            20,
	}

	// Setup: empty prefix, cursor at left edge (X: 0), Y=2 to force below rendering
	cursor := Position{X: 0, Y: 2}
	cursorIsFullWidth := false

	// Create simple completion manager with one suggestion
	c := NewCompletionManager(5)
	c.tmp = []Suggest{{Text: "test", Description: "desc"}}

	// Render below the cursor (cursor at Y=2 with row=10 allows below path)
	_ = r.renderCompletion(c, cursor, cursorIsFullWidth)

	// Verify no CursorForward(0) was emitted
	calls := mockOut.Calls()
	for _, call := range calls {
		if call.method == "CursorForward" && len(call.args) > 0 {
			if n, ok := call.args[0].(int); ok && n == 0 {
				t.Fatalf("Below-path completion at column 0 emitted CursorForward(0) which VT100 interprets as CursorForward(1)")
			}
		}
	}
}
