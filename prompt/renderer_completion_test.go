package prompt

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	istrings "github.com/joeycumines/go-prompt/strings"
)

// genSuggestions generates n suggestions in the format used by the existing tests:
// "item-text-N" with description "item-desc-N".
func genSuggestions(n int, prefix string) []Suggest {
	suggestions := make([]Suggest, n)
	for i := range n {
		suggestions[i] = Suggest{
			Text:        fmt.Sprintf("%s-text-%d", prefix, i),
			Description: fmt.Sprintf("%s-desc-%d", prefix, i),
		}
	}
	return suggestions
}

// mockWriterLogger captures all calls to the Writer interface for verification.
type mockWriterLogger struct {
	Writer
	mu    sync.Mutex
	calls []mockCall
}

// mockCall represents a single method call, for log-style mocking calls.
type mockCall struct {
	method string
	args   []any
}

func (m *mockWriterLogger) reset() {
	m.mu.Lock()
	m.calls = []mockCall{}
	m.mu.Unlock()
}

func (m *mockWriterLogger) addCall(method string, args ...any) {
	m.mu.Lock()
	m.calls = append(m.calls, mockCall{method: method, args: args})
	m.mu.Unlock()
}

// Calls returns a snapshot copy of the logged calls for safe concurrent inspection.
func (m *mockWriterLogger) Calls() []mockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := make([]mockCall, len(m.calls))
	copy(c, m.calls)
	return c
}

// Writer interface implementation
func (m *mockWriterLogger) Write(p []byte) (int, error) {
	m.addCall("Write", string(p))
	return len(p), nil
}
func (m *mockWriterLogger) WriteString(s string) (int, error) {
	m.addCall("WriteString", s)
	return len(s), nil
}
func (m *mockWriterLogger) SetColor(fg, bg Color, bold bool) {
	m.addCall("SetColor", fg, bg, bold)
}
func (m *mockWriterLogger) SetDisplayAttributes(fg, bg Color, attrs ...DisplayAttribute) {
	m.addCall("SetDisplayAttributes", fg, bg, attrs)
}
func (m *mockWriterLogger) CursorGoTo(row, col int) { m.addCall("CursorGoTo", row, col) }
func (m *mockWriterLogger) CursorUp(n int)          { m.addCall("CursorUp", n) }
func (m *mockWriterLogger) CursorDown(n int)        { m.addCall("CursorDown", n) }
func (m *mockWriterLogger) CursorForward(n int)     { m.addCall("CursorForward", n) }
func (m *mockWriterLogger) CursorBackward(n int)    { m.addCall("CursorBackward", n) }
func (m *mockWriterLogger) EraseDown()              { m.addCall("EraseDown") }
func (m *mockWriterLogger) EraseScreen()            { m.addCall("EraseScreen") }
func (m *mockWriterLogger) EraseStartOfLine()       { m.addCall("EraseStartOfLine") }
func (m *mockWriterLogger) EraseEndOfLine()         { m.addCall("EraseEndOfLine") }
func (m *mockWriterLogger) EraseLine()              { m.addCall("EraseLine") }
func (m *mockWriterLogger) ShowCursor()             { m.addCall("ShowCursor") }
func (m *mockWriterLogger) HideCursor()             { m.addCall("HideCursor") }
func (m *mockWriterLogger) SaveCursor()             { m.addCall("SaveCursor") }
func (m *mockWriterLogger) UnSaveCursor()           { m.addCall("UnSaveCursor") }
func (m *mockWriterLogger) AskForCPR()              { m.addCall("AskForCPR") }
func (m *mockWriterLogger) Flush() error            { m.addCall("Flush"); return nil }
func (m *mockWriterLogger) SetTitle(title string)   { m.addCall("SetTitle", title) }
func (m *mockWriterLogger) ClearTitle()             { m.addCall("ClearTitle") }
func (m *mockWriterLogger) ScrollUp()               { m.addCall("ScrollUp") }
func (m *mockWriterLogger) ScrollDown()             { m.addCall("ScrollDown") }
func (m *mockWriterLogger) WriteRaw(data []byte)    { m.addCall("WriteRaw", string(data)) }
func (m *mockWriterLogger) WriteRawString(data string) {
	m.addCall("WriteRawString", data)
}

func TestRenderer_renderCompletion(t *testing.T) {
	// clampedCursorForTest mirrors the clamping logic from renderer.go Render():
	// computes the clamped viewport cursor from the renderer/buffer state.
	clampedCursorForTest := func(r *Renderer, buf *Buffer) (Position, bool) {
		prefix := r.prefixCallback()
		prefixWidth := istrings.GetWidth(prefix)
		col := r.col - prefixWidth
		endLine := buf.startLine + r.row - 1

		displayPos, displayIsFullWidth := buf.DisplayCursorPositionFullWidth(col)

		if displayPos.Y < buf.startLine {
			return Position{X: prefixWidth, Y: 0}, false
		}
		if displayPos.Y > endLine {
			visCursor, visFullWidth := positionAtEndOfStringLine(buf.Text(), col, endLine)
			if visCursor.Y < buf.startLine {
				return Position{X: prefixWidth, Y: 0}, false
			}
			return Position{
				X: visCursor.X + prefixWidth,
				Y: visCursor.Y - buf.startLine,
			}, visFullWidth
		}
		// Cursor is within the visible range. The full-width state holds when either:
		//  - the display position itself was at the line boundary (displayIsFullWidth), OR
		//  - after adding the prefix, the cursor lands exactly at r.col (right-margin case).
		cursorIsFullWidth := displayIsFullWidth || (displayPos.X+prefixWidth == r.col)
		return Position{
			X: displayPos.X + prefixWidth,
			Y: displayPos.Y - buf.startLine,
		}, cursorIsFullWidth
	}

	// Helper function to create a slice of suggestions
	genSuggestions := func(n int, prefix string) []Suggest {
		s := make([]Suggest, n)
		for i := range n {
			s[i] = Suggest{
				Text:        fmt.Sprintf("%s-text-%d", prefix, i),
				Description: fmt.Sprintf("%s-desc-%d", prefix, i),
			}
		}
		return s
	}

	// Default renderer with predictable colors and size for testing
	defaultRenderer := func() *Renderer {
		return &Renderer{
			out:                          &mockWriterLogger{},
			prefixCallback:               func() string { return "> " },
			row:                          20,
			col:                          80,
			suggestionTextColor:          White,
			suggestionBGColor:            Cyan,
			selectedSuggestionTextColor:  Black,
			selectedSuggestionBGColor:    Turquoise,
			descriptionTextColor:         Black,
			descriptionBGColor:           Turquoise,
			selectedDescriptionTextColor: White,
			selectedDescriptionBGColor:   Cyan,
			scrollbarThumbColor:          DarkGray,
			scrollbarBGColor:             Cyan,
		}
	}

	testCases := []struct {
		name          string
		setup         func() (*Renderer, *Buffer, *CompletionManager)
		expectedCalls []mockCall
	}{
		{
			name: "No Suggestions",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer()
				b := NewBuffer()
				c := NewCompletionManager(5)
				c.tmp = []Suggest{} // Empty suggestions
				return r, b, c
			},
			expectedCalls: []mockCall{}, // Should do nothing
		},
		{
			name: "Basic Rendering - No Selection",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer()
				b := NewBuffer()
				b.InsertTextMoveCursor("hel", 80, 20, false)
				c := NewCompletionManager(5)
				c.tmp = []Suggest{
					{Text: "hello", Description: "greeting"},
					{Text: "help", Description: "command"},
				}
				c.selected = -1 // Nothing selected
				return r, b, c
			},
			expectedCalls: []mockCall{
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"CursorUp", []any{2}},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{5}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" hello "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" greeting "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{18}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{5}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" help  "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" command  "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{18}},
				{"CursorUp", []any{2}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
			},
		},
		{
			name: "Basic Rendering - With Selection",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer()
				b := NewBuffer()
				c := NewCompletionManager(5)
				c.tmp = []Suggest{
					{Text: "hello", Description: "greeting"},
					{Text: "help", Description: "command"},
				}
				c.selected = 1 // "help" is selected
				return r, b, c
			},
			expectedCalls: []mockCall{
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"CursorUp", []any{2}},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" hello "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" greeting "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{18}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{2}},
				{"SetColor", []any{Black, Turquoise, true}}, // Selected colors
				{"WriteString", []any{" help  "}},
				{"SetColor", []any{White, Cyan, false}}, // Selected colors
				{"WriteString", []any{" command  "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{18}},
				{"CursorUp", []any{2}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
			},
		},
		{
			name: "Layout - Truncation",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer()
				r.col = 30 // Smaller terminal width
				b := NewBuffer()
				c := NewCompletionManager(5)
				c.tmp = []Suggest{
					{Text: "a-very-long-suggestion-text", Description: "a-very-long-description"},
				}
				c.selected = 0
				return r, b, c
			},
			expectedCalls: []mockCall{
				{"WriteRawString", []any{"\n"}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{2}},
				{"SetColor", []any{Black, Turquoise, true}},
				{"WriteString", []any{" a-very-long-suggestion... "}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{""}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{1}},
				{"CursorForward", []any{1}},
				{"CursorBackward", []any{27}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
			},
		},
		{
			name: "Layout - Multi-width Characters",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer()
				b := NewBuffer()
				c := NewCompletionManager(5)
				c.tmp = []Suggest{
					{Text: "你好", Description: "世界"},
				}
				c.selected = 0
				// Text: " 你好 " (1+4+1=6 width), Description: " 世界 " (1+4+1=6 width)
				return r, b, c
			},
			expectedCalls: []mockCall{
				{"WriteRawString", []any{"\n"}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{2}},
				{"SetColor", []any{Black, Turquoise, true}},
				{"WriteString", []any{" 你好 "}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" 世界 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{13}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
			},
		},
		{
			name: "Layout - Overflow Shift Left",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer() // col=80
				b := NewBuffer()
				// Position cursor far to the right
				b.InsertTextMoveCursor(strings.Repeat("a", 70), 80, 20, false) // cursor at 70 + 2(prefix) = 72
				c := NewCompletionManager(5)
				c.tmp = []Suggest{{Text: "suggestion", Description: "description"}} // width = (1+10+1) + (1+11+1) + 1 = 12 + 13 + 1 = 26
				c.selected = -1
				// x=72, width=26. x+width = 98. r.col=80. Overflow by 18. Shift left by 18.
				return r, b, c
			},
			expectedCalls: []mockCall{
				{"WriteRawString", []any{"\n"}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{54}}, // Aligns to shifted cursor (72-18=54), NOT original 72
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" suggestion "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" description "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{1}},
				{"CursorForward", []any{1}},
				{"CursorBackward", []any{25}},
				{"CursorForward", []any{18}}, // Restore cursor
				{"CursorUp", []any{1}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
			},
		},
		{
			name: "Scrolling - Pagination and Vertical Scroll",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer()
				b := NewBuffer()
				c := NewCompletionManager(4) // max = 4
				c.tmp = genSuggestions(10, "item")
				c.verticalScroll = 5 // Scrolled down
				c.selected = 6       // "item-text-6", which is visible at index 1
				return r, b, c
			},
			expectedCalls: []mockCall{
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"CursorUp", []any{4}},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-5 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-5 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorBackward", []any{27}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{Black, Turquoise, true}}, {"WriteString", []any{" item-text-6 "}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-desc-6 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorBackward", []any{27}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-7 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-7 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorBackward", []any{27}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-8 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-8 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorBackward", []any{27}},
				{"CursorUp", []any{4}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
			},
		},
		{
			name: "Scrolling - Selected Item Above Window",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer()
				b := NewBuffer()
				c := NewCompletionManager(5)
				c.tmp = genSuggestions(10, "item")
				c.verticalScroll = 2
				c.selected = 0 // Selected is not visible
				return r, b, c
			},
			// selected = 0 should force verticalScroll to 0 so item 0 is shown and highlighted.
			expectedCalls: []mockCall{
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"CursorUp", []any{5}},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{2}},
				{"SetColor", []any{Black, Turquoise, true}},
				{"WriteString", []any{" item-text-0 "}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" item-desc-0 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{27}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" item-text-1 "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" item-desc-1 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{27}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" item-text-2 "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" item-desc-2 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{27}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" item-text-3 "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" item-desc-3 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{27}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" item-text-4 "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" item-desc-4 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{27}},
				{"CursorUp", []any{5}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
			},
		},
		{
			name: "Scrollbar - Correct Thumb Position",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer()
				b := NewBuffer()
				c := NewCompletionManager(8)       // windowHeight = 8
				c.tmp = genSuggestions(40, "item") // contentHeight = 40
				c.verticalScroll = 20              // 20/40 = 50% scrolled
				c.selected = -1
				// fractionVisible = 8/40 = 0.2
				// scrollbarHeight = int(clamp(8, 1, 8*0.2)) = int(clamp(8,1,1.6)) = 1
				// fractionAbove = 20/40 = 0.5
				// scrollbarTop = int(8 * 0.5) = 4
				// Thumb should be at row 4 and have height 1. So only row 4.
				return r, b, c
			},
			expectedCalls: []mockCall{
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"CursorUp", []any{8}},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-20 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-20 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorBackward", []any{29}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-21 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-21 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorBackward", []any{29}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-22 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-22 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorBackward", []any{29}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-23 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-23 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorBackward", []any{29}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-24 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-24 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorBackward", []any{29}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-25 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-25 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorBackward", []any{29}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-26 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-26 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorBackward", []any{29}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-27 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-27 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorBackward", []any{29}},
				{"CursorUp", []any{8}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
			},
		},
		{
			name: "Edge Case - Very Small Terminal Width",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer()
				r.col = 10 // Very small width
				b := NewBuffer()
				c := NewCompletionManager(5)
				c.tmp = genSuggestions(1, "item")
				// available width = 10(col) - 2(prefix) - 1(scroll) = 7
				// left requires at least 1(prefix) + 1(suffix) + 3(ellipsis) = 5
				// right requires at least 1(prefix) + 1(suffix) + 3(ellipsis) = 5
				// This should result in a very small rendered box.
				return r, b, c
			},
			expectedCalls: []mockCall{
				{"WriteRawString", []any{"\n"}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" it... "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{""}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{1}},
				{"CursorForward", []any{1}},
				{"CursorBackward", []any{7}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
			},
		},
		{
			name: "Edge Case - Multi-line Input Buffer",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer()
				b := NewBuffer()
				b.InsertTextMoveCursor("first line\nsecond", 80, 20, false)
				c := NewCompletionManager(5)
				c.tmp = []Suggest{{Text: "suggestion", Description: "desc"}}
				// Cursor is at Y=1, X=8. Prefix is "> ", width 2.
				// Total cursor pos: Y=1, X=10.
				return r, b, c
			},
			expectedCalls: []mockCall{
				{"WriteRawString", []any{"\n"}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{8}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" suggestion "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" desc "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{19}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
			},
		},
		{
			name: "Edge Case - Long Prefix",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer()
				r.prefixCallback = func() string { return "my-very-long-prefix------> " } // len 28
				b := NewBuffer()
				c := NewCompletionManager(5)
				c.tmp = []Suggest{{Text: "s", Description: "d"}}
				// Available width is now 80 - 28 - 1 = 51
				return r, b, c
			},
			expectedCalls: []mockCall{
				{"WriteRawString", []any{"\n"}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{27}}, // Aligned after long prefix
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" s "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" d "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{7}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
			},
		},
		{
			name: "Edge Case - Exact Line Fill Before Cursor",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer() // col 80, prefix "> " (2)
				b := NewBuffer()
				// available width = 80 - 2 = 78.
				// Insert exactly 78 chars to trigger wrap in positionAtEndOfString
				b.InsertTextMoveCursor(strings.Repeat("a", 78), 80, 20, false)
				c := NewCompletionManager(5)
				c.tmp = []Suggest{{Text: "suggestion", Description: "desc"}}
				// positionAtEndOfString will return Y=0, X=78.
				// The code then adds prefixWidth, so cursor start is Y=0, X=80.
				// 80 + 19 (completion width) > 80, so it shifts back 19.
				return r, b, c
			},
			expectedCalls: []mockCall{
				{"CursorBackward", []any{1}},
				{"CursorForward", []any{1}},
				{"WriteRawString", []any{"\n"}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{61}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" suggestion "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" desc "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{1}},
				{"CursorForward", []any{1}},
				{"CursorBackward", []any{18}},
				{"CursorForward", []any{18}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
			},
		},
		{
			name: "Dynamic Completion - Height Clamp",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer()
				r.dynamicCompletion = true
				r.row = 2
				b := NewBuffer()
				c := NewCompletionManager(5)
				c.tmp = genSuggestions(3, "item")
				c.selected = 2
				return r, b, c
			},
			expectedCalls: []mockCall{
				{"WriteRawString", []any{"\n"}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{2}},
				{"SetColor", []any{Black, Turquoise, true}},
				{"WriteString", []any{" item-text-2 "}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" item-desc-2 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{27}},
				{"CursorUp", []any{1}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
			},
		},
		{
			name: "Dynamic Completion - Zero Available Rows",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer()
				r.dynamicCompletion = true
				r.row = 1
				b := NewBuffer()
				c := NewCompletionManager(5)
				c.tmp = genSuggestions(5, "item")
				c.selected = 0
				return r, b, c
			},
			expectedCalls: []mockCall{}, // Should return early due to availableRows <= 0
		},
		{
			name: "Dynamic Completion - Multi-line Input",
			setup: func() (*Renderer, *Buffer, *CompletionManager) {
				r := defaultRenderer()
				r.dynamicCompletion = true
				r.row = 10
				b := NewBuffer()
				// Create multi-line input so cursor is on Y=2
				b.InsertTextMoveCursor("line1\nline2\nline3", 80, 10, false)
				c := NewCompletionManager(10) // Set max higher than available rows
				c.tmp = genSuggestions(8, "item")
				c.selected = 0
				// cursorLine = 2, r.row = 10, availableRows = 10 - 2 - 1 = 7
				// windowHeight should be min(7, 8) = 7 (clamped by availableRows)
				return r, b, c
			},
			expectedCalls: []mockCall{
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"WriteRawString", []any{"\n"}},
				{"CursorUp", []any{7}},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{7}},
				{"SetColor", []any{Black, Turquoise, true}},
				{"WriteString", []any{" item-text-0 "}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" item-desc-0 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{27}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{7}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" item-text-1 "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" item-desc-1 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{27}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{7}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" item-text-2 "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" item-desc-2 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{27}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{7}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" item-text-3 "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" item-desc-3 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{27}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{7}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" item-text-4 "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" item-desc-4 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{27}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{7}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" item-text-5 "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" item-desc-5 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{27}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{7}},
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" item-text-6 "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" item-desc-6 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorBackward", []any{27}},
				{"CursorUp", []any{7}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			renderer, buffer, completionMgr := tc.setup()
			mockOut, ok := renderer.out.(*mockWriterLogger)
			if !ok {
				t.Fatal("Renderer's writer is not a mockWriterLogger")
			}
			mockOut.reset()

			// Compute clamped cursor (mirrors Render's clamping) and call.
			cursor, cursorIsFullWidth := clampedCursorForTest(renderer, buffer)
			renderer.renderCompletion(completionMgr, cursor, cursorIsFullWidth)

			// Compare actual calls with expected calls
			if !reflect.DeepEqual(mockOut.calls, tc.expectedCalls) {
				t.Logf("ACTUAL: %#v", mockOut.calls)
				t.Logf("EXPECTED: %#v", tc.expectedCalls)
				// Create a more readable diff
				var sb strings.Builder
				sb.WriteString("mock writer calls do not match expected calls.\n")
				sb.WriteString("----------- GOT -----------\n")
				for i, got := range mockOut.calls {
					sb.WriteString(fmt.Sprintf("%d: %s(%v)\n", i, got.method, got.args))
				}
				sb.WriteString("---------- EXPECTED ----------\n")
				for i, want := range tc.expectedCalls {
					sb.WriteString(fmt.Sprintf("%d: %s(%v)\n", i, want.method, want.args))
				}
				t.Error(sb.String())
			}
		})
	}
}

// TestRenderCompletion_DynamicWithCursorAboveViewport verifies that when the
// logical cursor is ABOVE the visible viewport (stale startLine), the
// dynamic completion sizing does not inflate availableRows beyond the
// actual screen height.
//
// Before the fix: cursorLine = cursorPos.Y - startLine could be negative,
// causing availableRows = r.row - negative - 1 > r.row.
//
// After the fix: cursorLine is clamped to [0, r.row-1], so availableRows
// is correctly bounded.
func TestRenderCompletion_DynamicWithCursorAboveViewport(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:               mockOut,
		prefixCallback:    func() string { return "> " }, // width 2
		col:               80,
		row:               10,
		inputTextColor:    DefaultColor,
		inputBGColor:      DefaultColor,
		dynamicCompletion: true,
	}

	b := NewBuffer()
	// Single-line content. Cursor at end (Y=0).
	b.InsertText("hello", false)
	// Force startLine beyond cursor — simulating a stale scroll position.
	// With the bug: cursorPos.Y=0, startLine=5 → cursorLine = -5
	// availableRows = 10 - (-5) - 1 = 14 (WRONG — exceeds r.row=10)
	b.startLine = 5

	c := NewCompletionManager(20)
	c.tmp = genSuggestions(20, "suggestion")

	// Cursor is above viewport (startLine=5, logical Y=0). The clamped cursor
	// should be {X: prefixWidth, Y: 0} — matching what Render() would clamp to.
	// cursorIsFullWidth is false (5 < 78).
	r.renderCompletion(c, Position{X: 2, Y: 0}, false)

	// The completion window must not write more lines than the screen has.
	// r.row = 10, availableRows (after fix) = 10 - 0 - 1 = 9.
	// With prefix "> " (width 2), content columns = 77.
	// Each formatted suggestion line: 1 suggestion text + 1 space = 2 + scrollbar = 3 cols wide.
	// windowHeight = min(20 suggestions, 9 available) = 9.
	// We verify by counting the number of newlines written.
	newlineCount := 0
	for _, call := range mockOut.Calls() {
		if call.method == "WriteRawString" && call.args[0] == "\n" {
			newlineCount++
		}
	}
	// Must be at most r.row - 1 = 9 newlines for the window.
	if newlineCount > 9 {
		t.Errorf("completion window wrote %d newlines, expected at most 9 (r.row-1). Bug: cursorLine was negative, inflating availableRows.", newlineCount)
	}
}

// TestRenderCompletion_DynamicBidirectionalAvailableRows verifies that
// renderCompletion with dynamicCompletion=true uses the MAX of rows-above and
// rows-below when computing availableRows, not just rows-below.
//
// Before the fix: cursor on last row (Y=r.row-1) → rowsBelow=0 → dropdown hidden.
// After the fix: max(rowsAbove, rowsBelow) used → dropdown opens above.
func TestRenderCompletion_DynamicBidirectionalAvailableRows(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:                          mockOut,
		prefixCallback:               func() string { return "> " }, // width 2
		col:                          80,
		row:                          5,
		dynamicCompletion:            true,
		suggestionTextColor:          White,
		suggestionBGColor:            Cyan,
		selectedSuggestionTextColor:  Black,
		selectedSuggestionBGColor:    Turquoise,
		descriptionTextColor:         Black,
		descriptionBGColor:           Turquoise,
		selectedDescriptionTextColor: White,
		selectedDescriptionBGColor:   Cyan,
		scrollbarThumbColor:          DarkGray,
		scrollbarBGColor:             Cyan,
	}

	tests := []struct {
		name          string
		cursorLine    int // viewport-relative cursor Y
		contentHeight int // number of suggestions
		wantHeight    int // expected windowHeight (number of newlines)
	}{
		{
			name:          "cursor on last row uses rows-above",
			cursorLine:    4, // last row (r.row-1)
			contentHeight: 4,
			// rowsBelow = 5-4-1 = 0, rowsAbove = 4
			// max = 4, windowHeight = min(4, 4) = 4
			wantHeight: 4,
		},
		{
			name:          "cursor on first row uses rows-below",
			cursorLine:    0,
			contentHeight: 4,
			// rowsBelow = 5-0-1 = 4, rowsAbove = 0
			// max = 4, windowHeight = min(4, 4) = 4
			wantHeight: 4,
		},
		{
			name:          "cursor in middle uses smaller direction",
			cursorLine:    2,
			contentHeight: 10,
			// rowsBelow = 5-2-1 = 2, rowsAbove = 2
			// max = 2, windowHeight = min(2, 10) = 2
			wantHeight: 2,
		},
		{
			name:          "more content than space uses max",
			cursorLine:    4,
			contentHeight: 10,
			// rowsBelow=0, rowsAbove=4, max=4
			// windowHeight = min(4, 10) = 4
			wantHeight: 4,
		},
		{
			name:          "zero rows below uses rows above",
			cursorLine:    r.row - 1, // last row
			contentHeight: 3,
			// rowsBelow=0, rowsAbove=4, max=4
			// windowHeight = min(3, 4) = 3
			wantHeight: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.InsertTextMoveCursor("hello", 80, 20, false)
			c := NewCompletionManager(20)
			c.tmp = genSuggestions(tt.contentHeight, "suggestion")
			c.selected = 0

			// Clamp cursor to viewport range
			cursorLine := tt.cursorLine
			if cursorLine < 0 {
				cursorLine = 0
			} else if cursorLine >= r.row {
				cursorLine = r.row - 1
			}
			clampedCursor := Position{X: 2, Y: cursorLine}

			mockOut.reset()
			n := r.renderCompletion(c, clampedCursor, false)

			// The return value is the windowHeight (number of rendered lines).
			// For below-cursor rendering this equals newline count; for
			// above-cursor rendering lines are separated by CursorDown
			// instead of newlines. Check the return value directly.
			if n != tt.wantHeight {
				// Also count newlines and CursorDown calls for diagnostics.
				newlineCount := 0
				cursorDownCount := 0
				for _, call := range mockOut.Calls() {
					if call.method == "WriteRawString" && call.args[0] == "\n" {
						newlineCount++
					}
					if call.method == "CursorDown" {
						cursorDownCount++
					}
				}
				t.Errorf("cursorLine=%d, wantHeight=%d, got windowHeight=%d (newlines=%d, CursorDown=%d)",
					tt.cursorLine, tt.wantHeight, n, newlineCount, cursorDownCount)
			}
		})
	}
}

func TestRenderCompletion_AboveRightMarginSyncBeforeCursorDown(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:                          mockOut,
		prefixCallback:               func() string { return "> " },
		col:                          18,
		row:                          4,
		dynamicCompletion:            true,
		suggestionTextColor:          White,
		suggestionBGColor:            Cyan,
		selectedSuggestionTextColor:  Black,
		selectedSuggestionBGColor:    Turquoise,
		descriptionTextColor:         Black,
		descriptionBGColor:           Turquoise,
		selectedDescriptionTextColor: White,
		selectedDescriptionBGColor:   Cyan,
		scrollbarThumbColor:          DarkGray,
		scrollbarBGColor:             Cyan,
	}

	c := NewCompletionManager(5)
	c.tmp = []Suggest{
		{Text: "alpha", Description: "012345"},
		{Text: "bravo", Description: "012345"},
	}
	c.selected = 0

	lines := r.renderCompletion(c, Position{X: 15, Y: 3}, false)
	if lines != 2 {
		t.Fatalf("renderCompletion returned %d lines, want 2", lines)
	}

	calls := mockOut.Calls()
	cursorDowns := 0
	for i, call := range calls {
		if call.method != "CursorDown" || call.args[0].(int) != 1 {
			continue
		}
		cursorDowns++
		foundSync := false
		for j := i - 1; j >= 1 && j >= i-6; j-- {
			if calls[j-1].method == "CursorBackward" && calls[j-1].args[0].(int) == 1 &&
				calls[j].method == "CursorForward" && calls[j].args[0].(int) == 1 {
				foundSync = true
				break
			}
		}
		if !foundSync {
			t.Fatalf("CursorDown at call %d was not preceded by wrap-state sync: %+v", i, calls)
		}
	}
	if cursorDowns != 2 {
		t.Fatalf("CursorDown count = %d, want 2 (between rows plus return to cursor row)", cursorDowns)
	}
}
