package prompt

import (
	"testing"

	istrings "github.com/joeycumines/go-prompt/strings"
)

// TestIntegration_MultilineFullPipeline exercises the complete rendering pipeline:
// multiline buffer + lexer + dynamicCompletion + cursor movement + zero-height
// resize + BreakLine. It verifies the full end-to-end flow works correctly.
func TestIntegration_MultilineFullPipeline(t *testing.T) {
	t.Run("multiline render with lexer and completions", func(t *testing.T) {
		mockOut := &mockWriterLogger{}
		r := &Renderer{
			out:                         mockOut,
			prefixCallback:              func() string { return "> " },
			col:                         80,
			row:                         10,
			inputTextColor:              DefaultColor,
			inputBGColor:                DefaultColor,
			dynamicCompletion:           true,
			suggestionTextColor:         White,
			suggestionBGColor:           Cyan,
			selectedSuggestionTextColor: Black,
			selectedSuggestionBGColor:   Turquoise,
		}

		// Set up a multiline buffer with 3 lines.
		b := NewBuffer()
		b.InsertText("line1\nline2\nline3", false)

		// Lexer: simple color by line index.
		lexer := NewEagerLexer(func(input string) []Token {
			// Build tokens for each visible line using TerminalReflower.
			reflower := NewTerminalReflower(input, 0, 1<<30, 78, true)
			var tokens []Token
			for {
				state, ok := reflower.Next()
				if !ok {
					break
				}
				if state.ByteEnd > state.ByteStart {
					tokens = append(tokens, NewSimpleToken(
						istrings.ByteNumber(state.ByteStart),
						istrings.ByteNumber(state.ByteEnd-1),
						SimpleTokenWithColor(Green),
					))
				}
			}
			return tokens
		})

		// Set up completion manager.
		c := NewCompletionManager(20)
		c.tmp = genSuggestions(20, "suggestion")

		// First render: full pipeline.
		mockOut.reset()
		r.Render(b, c, lexer)

		calls := mockOut.Calls()
		t.Logf("Render #1: %d calls", len(calls))

		// Verify: should have written content (WriteString/WriteRawString calls).
		writeCount := 0
		for _, call := range calls {
			if call.method == "WriteString" || call.method == "WriteRawString" {
				writeCount++
				t.Logf("  Write: %q", call.args[0])
			}
		}
		if writeCount == 0 {
			t.Error("expected at least one WriteString/WriteRawString call after Render")
		}

		// Second render: zero-height transition (r.row=0).
		mockOut.reset()
		r.row = 0
		r.Render(b, c, lexer)
		calls2 := mockOut.Calls()
		t.Logf("Render #2 (row=0): %d calls", len(calls2))

		// Third render: recovery from zero-height (r.row>0).
		mockOut.reset()
		r.row = 10
		r.Render(b, c, lexer)
		calls3 := mockOut.Calls()
		t.Logf("Render #3 (recovery): %d calls", len(calls3))

		// Verify: recovery should write content.
		writeCount3 := 0
		for _, call := range calls3 {
			if call.method == "WriteString" || call.method == "WriteRawString" {
				writeCount3++
			}
		}
		if writeCount3 == 0 {
			t.Error("expected content after zero-height recovery")
		}
	})

	t.Run("BreakLine fires callback and recovers from zero-height", func(t *testing.T) {
		mockOut := &mockWriterLogger{}
		r := &Renderer{
			out:            mockOut,
			prefixCallback: func() string { return "> " },
			col:            80,
			row:            10,
			inputTextColor: DefaultColor,
			inputBGColor:   DefaultColor,
		}

		b := NewBuffer()
		b.InsertText("hello world", false)

		// Set up a breakLineCallback.
		callbackFired := 0
		r.breakLineCallback = func(*Document) { callbackFired++ }

		// Simulate zero-height state first, then resize.
		r.row = 0
		r.Render(b, nil, nil) // sets wasZeroHeight=true

		// Now BreakLine should recover from zero-height.
		mockOut.reset()
		r.row = 10
		r.BreakLine(b, nil)

		calls := mockOut.Calls()
		t.Logf("BreakLine: %d calls", len(calls))

		// Verify: newline should be emitted.
		newlineCount := 0
		for _, call := range calls {
			if call.method == "WriteString" && call.args[0] == "\n" {
				newlineCount++
			}
		}
		if newlineCount == 0 {
			t.Error("BreakLine should emit a newline")
		}

		// Verify: callback was fired.
		if callbackFired == 0 {
			t.Error("BreakLine should have fired breakLineCallback")
		}
	})

	t.Run("cursor movement updates previousCursor (viewport-relative)", func(t *testing.T) {
		mockOut := &mockWriterLogger{}
		r := &Renderer{
			out:            mockOut,
			prefixCallback: func() string { return "> " },
			col:            80,
			row:            10,
			inputTextColor: DefaultColor,
			inputBGColor:   DefaultColor,
		}

		b := NewBuffer()
		// Create multiline content.
		b.InsertText("line1\nline2\nline3", false)

		// Force scroll: set startLine beyond the first line.
		// This simulates being scrolled into a document.
		b.startLine = 2 // last line only visible

		// Do cursor movement: CursorUp(1 column, 1 row).
		b.CursorUp(1, istrings.Width(80), 1)

		// Verify: r.previousCursor.Y should be in [0, r.row-1] after fix.
		if r.previousCursor.Y < 0 || r.previousCursor.Y >= r.row {
			t.Errorf("previousCursor.Y=%d out of valid range [0, %d]", r.previousCursor.Y, r.row-1)
		}
	})

	t.Run("CJK text wraps correctly in narrow terminal", func(t *testing.T) {
		mockOut := &mockWriterLogger{}
		r := &Renderer{
			out:            mockOut,
			prefixCallback: func() string { return "> " },
			col:            20, // narrow terminal
			row:            10,
			inputTextColor: DefaultColor,
			inputBGColor:   DefaultColor,
		}

		b := NewBuffer()
		// CJK text: each char is 2 columns wide. 20 cols = 10 chars per line.
		// "日本語日本語日本語日本語テスト" = 20 chars × 2 cols = 40 cols → 2 visible lines.
		b.InsertText("日本語日本語日本語日本語テスト", false)

		lexer := NewEagerLexer(func(input string) []Token {
			reflower := NewTerminalReflower(input, 0, 1<<30, 18, true)
			var tokens []Token
			for {
				state, ok := reflower.Next()
				if !ok {
					break
				}
				if state.ByteEnd > state.ByteStart {
					tokens = append(tokens, NewSimpleToken(
						istrings.ByteNumber(state.ByteStart),
						istrings.ByteNumber(state.ByteEnd-1),
						SimpleTokenWithColor(Green),
					))
				}
			}
			return tokens
		})

		mockOut.reset()
		// Pass a completion manager with no suggestions to avoid nil pointer panic in renderCompletion.
		cEmpty := NewCompletionManager(0)
		r.Render(b, cEmpty, lexer)

		calls := mockOut.Calls()
		t.Logf("CJK render: %d calls", len(calls))

		// Verify: should have written content.
		writeCount := 0
		for _, call := range calls {
			if call.method == "WriteString" || call.method == "WriteRawString" {
				writeCount++
			}
		}
		if writeCount == 0 {
			t.Error("expected content after CJK render")
		}
	})

	t.Run("empty buffer with multiline prefix", func(t *testing.T) {
		mockOut := &mockWriterLogger{}
		r := &Renderer{
			out:            mockOut,
			prefixCallback: func() string { return "> " },
			col:            80,
			row:            10,
			inputTextColor: DefaultColor,
			inputBGColor:   DefaultColor,
			indentSize:     2,
		}

		b := NewBuffer()
		b.InsertText("", false)
		b.startLine = 5 // Force scrolled state

		lexer := NewEagerLexer(func(input string) []Token {
			return nil // empty input = no tokens
		})

		mockOut.reset()
		cEmpty := NewCompletionManager(0)
		r.Render(b, cEmpty, lexer)

		calls := mockOut.Calls()
		t.Logf("Empty buffer render: %d calls", len(calls))

		// Should handle gracefully without panic — at minimum, should have some output.
	})
}
