package prompt

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// mockWriterLogger captures all calls to the Writer interface for verification.
type mockWriterLogger struct {
	Writer
	calls []mockCall
}

// mockCall represents a single method call, for log-style mocking calls.
type mockCall struct {
	method string
	args   []any
}

func (m *mockWriterLogger) reset() {
	m.calls = []mockCall{}
}

func (m *mockWriterLogger) addCall(method string, args ...any) {
	m.calls = append(m.calls, mockCall{method: method, args: args})
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
func (m *mockWriterLogger) ShowCursor()             { m.addCall("ShowCursor") }
func (m *mockWriterLogger) HideCursor()             { m.addCall("HideCursor") }
func (m *mockWriterLogger) Flush() error            { m.addCall("Flush"); return nil }
func (m *mockWriterLogger) SetTitle(title string)   { m.addCall("SetTitle", title) }
func (m *mockWriterLogger) ClearTitle()             { m.addCall("ClearTitle") }
func (m *mockWriterLogger) ScrollUp()               { m.addCall("ScrollUp") }
func (m *mockWriterLogger) ScrollDown()             { m.addCall("ScrollDown") }

func TestRenderer_renderCompletion(t *testing.T) {
	// Helper function to create a slice of suggestions
	genSuggestions := func(n int, prefix string) []Suggest {
		s := make([]Suggest, n)
		for i := 0; i < n; i++ {
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
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
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
				{"CursorUp", []any{0}},
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
				{"CursorUp", []any{0}},
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
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
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
				{"CursorUp", []any{0}},
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
				{"CursorUp", []any{0}},
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
				{"ScrollDown", nil},
				{"ScrollUp", nil},
				{"CursorUp", []any{0}},
				{"CursorBackward", []any{0}},
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
				{"CursorUp", []any{0}},
				{"CursorBackward", []any{28}},
				{"CursorForward", []any{0}},
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
				{"ScrollDown", nil},
				{"ScrollUp", nil},
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
				{"CursorUp", []any{0}},
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
				{"ScrollDown", nil},
				{"ScrollUp", nil},
				{"CursorUp", []any{0}},
				{"CursorBackward", []any{18}},
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
				{"CursorUp", []any{0}},
				{"CursorBackward", []any{26}},
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
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-5 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-5 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{27}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{Black, Turquoise, true}}, {"WriteString", []any{" item-text-6 "}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-desc-6 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{27}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-7 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-7 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{27}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-8 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-8 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{27}},
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
			// selected = 0 - 2 = -2. No item should be highlighted.
			expectedCalls: []mockCall{
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-2 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-2 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{27}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-3 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-3 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{27}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-4 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-4 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{27}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-5 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-5 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{27}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-6 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-6 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{27}},
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
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollDown", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
				{"ScrollUp", nil},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-20 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-20 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{29}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-21 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-21 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{29}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-22 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-22 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{29}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-23 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-23 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{29}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-24 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-24 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{29}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-25 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-25 "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{29}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-26 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-26 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{29}},
				{"CursorDown", []any{1}}, {"WriteString", []any{"\r"}}, {"CursorForward", []any{2}},
				{"SetColor", []any{White, Cyan, false}}, {"WriteString", []any{" item-text-27 "}},
				{"SetColor", []any{Black, Turquoise, false}}, {"WriteString", []any{" item-desc-27 "}},
				{"SetColor", []any{DefaultColor, Cyan, false}}, {"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}}, {"CursorUp", []any{0}}, {"CursorBackward", []any{29}},
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
				{"ScrollDown", nil},
				{"ScrollUp", nil},
				{"CursorUp", []any{0}},
				{"CursorBackward", []any{0}},
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
				{"CursorUp", []any{0}},
				{"CursorBackward", []any{8}},
				{"CursorForward", []any{0}},
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
				{"ScrollDown", nil},
				{"ScrollUp", nil},
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
				{"CursorUp", []any{0}},
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
				{"ScrollDown", nil},
				{"ScrollUp", nil},
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
				{"CursorUp", []any{0}},
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
				// positionAtEndOfString will return Y=1, X=0.
				// The code then adds prefixWidth, so cursor start is Y=1, X=2.
				return r, b, c
			},
			expectedCalls: []mockCall{
				{"ScrollDown", nil},
				{"ScrollUp", nil},
				{"SetColor", []any{White, Cyan, false}},
				{"CursorDown", []any{1}},
				{"WriteString", []any{"\r"}},
				{"CursorForward", []any{2}}, // Starts on the next line
				{"SetColor", []any{White, Cyan, false}},
				{"WriteString", []any{" suggestion "}},
				{"SetColor", []any{Black, Turquoise, false}},
				{"WriteString", []any{" desc "}},
				{"SetColor", []any{DefaultColor, DarkGray, false}},
				{"WriteString", []any{" "}},
				{"SetColor", []any{DefaultColor, DefaultColor, false}},
				{"CursorUp", []any{0}},
				{"CursorBackward", []any{19}},
				{"CursorUp", []any{1}},
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

			// Call the function under test
			renderer.renderCompletion(buffer, completionMgr)

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
