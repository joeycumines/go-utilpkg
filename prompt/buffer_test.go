package prompt

import (
	"reflect"
	"testing"

	istrings "github.com/joeycumines/go-prompt/strings"
)

func TestNewBuffer(t *testing.T) {
	b := NewBuffer()
	if b.workingIndex != 0 {
		t.Errorf("workingIndex should be %#v, got %#v", 0, b.workingIndex)
	}
	if !reflect.DeepEqual(b.workingLines, []string{""}) {
		t.Errorf("workingLines should be %#v, got %#v", []string{""}, b.workingLines)
	}
}

func TestBuffer_InsertText(t *testing.T) {
	b := NewBuffer()
	b.InsertTextMoveCursor("some_text", DefColCount, DefRowCount, false)

	if b.Text() != "some_text" {
		t.Errorf("Text should be %#v, got %#v", "some_text", b.Text())
	}

	if b.cursorPosition != istrings.RuneCountInString("some_text") {
		t.Errorf("cursorPosition should be %#v, got %#v", istrings.RuneCountInString("some_text"), b.cursorPosition)
	}
}

func TestBuffer_InsertText_Overwrite(t *testing.T) {
	b := NewBuffer()
	b.InsertTextMoveCursor("ABC", DefColCount, DefRowCount, false)

	if b.Text() != "ABC" {
		t.Errorf("Text should be %#v, got %#v", "ABC", b.Text())
	}

	if b.cursorPosition != istrings.RuneCountInString("ABC") {
		t.Errorf("cursorPosition should be %#v, got %#v", istrings.RuneCountInString("ABC"), b.cursorPosition)
	}

	b.CursorLeft(1, DefColCount, DefRowCount)
	// Replace C with DEF in ABC
	b.InsertTextMoveCursor("DEF", DefColCount, DefRowCount, true)

	if b.Text() != "ABDEF" {
		t.Errorf("Text should be %#v, got %#v", "ABDEF", b.Text())
	}

	b.CursorLeft(100, DefColCount, DefRowCount)
	// Replace ABD with GHI in ABDEF
	b.InsertTextMoveCursor("GHI", DefColCount, DefRowCount, true)

	if b.Text() != "GHIEF" {
		t.Errorf("Text should be %#v, got %#v", "GHIEF", b.Text())
	}

	b.CursorLeft(100, DefColCount, DefRowCount)
	// Replace GHI with J\nK in GHIEF
	b.InsertTextMoveCursor("J\nK", DefColCount, DefRowCount, true)

	if b.Text() != "J\nKEF" {
		t.Errorf("Text should be %#v, got %#v", "J\nKEF", b.Text())
	}

	b.CursorUp(100, DefColCount, DefRowCount)
	b.CursorLeft(100, DefColCount, DefRowCount)
	// Replace J with LMN in J\nKEF test end of line
	b.InsertTextMoveCursor("LMN", DefColCount, DefRowCount, true)

	if b.Text() != "LMN\nKEF" {
		t.Errorf("Text should be %#v, got %#v", "LMN\nKEF", b.Text())
	}
}

func TestBuffer_CursorMovement(t *testing.T) {
	b := NewBuffer()
	b.InsertTextMoveCursor("some_text", DefColCount, DefRowCount, false)

	b.CursorLeft(1, DefColCount, DefRowCount)
	b.CursorLeft(2, DefColCount, DefRowCount)
	b.CursorRight(1, DefColCount, DefRowCount)
	b.InsertTextMoveCursor("A", DefColCount, DefRowCount, false)
	if b.Text() != "some_teAxt" {
		t.Errorf("Text should be %#v, got %#v", "some_teAxt", b.Text())
	}
	if b.cursorPosition != istrings.RuneCountInString("some_teA") {
		t.Errorf("Text should be %#v, got %#v", istrings.RuneCountInString("some_teA"), b.cursorPosition)
	}

	// Moving over left character counts.
	b.CursorLeft(100, DefColCount, DefRowCount)
	b.InsertTextMoveCursor("A", DefColCount, DefRowCount, false)
	if b.Text() != "Asome_teAxt" {
		t.Errorf("Text should be %#v, got %#v", "some_teAxt", b.Text())
	}
	if b.cursorPosition != istrings.RuneCountInString("A") {
		t.Errorf("Text should be %#v, got %#v", istrings.RuneCountInString("some_teA"), b.cursorPosition)
	}

	// TODO: Going right already at right end.
}

func TestBuffer_CursorMovement_WithMultiByte(t *testing.T) {
	b := NewBuffer()
	b.InsertTextMoveCursor("あいうえお", DefColCount, DefRowCount, false)
	b.CursorLeft(1, DefColCount, DefRowCount)
	if l := b.Document().TextAfterCursor(); l != "お" {
		t.Errorf("Should be 'お', but got %s", l)
	}
	b.InsertTextMoveCursor("żółć", DefColCount, DefRowCount, true)
	if b.Text() != "あいうえżółć" {
		t.Errorf("Text should be %#v, got %#v", "あいうえżółć", b.Text())
	}
}

func TestBuffer_CursorUp(t *testing.T) {
	b := NewBuffer()
	b.InsertTextMoveCursor("long line1\nline2", DefColCount, DefRowCount, false)
	b.CursorUp(1, DefColCount, DefRowCount)
	if b.Document().cursorPosition != 5 {
		t.Errorf("Should be %#v, got %#v", 5, b.Document().cursorPosition)
	}

	// Going up when already at the top.
	b.CursorUp(1, DefColCount, DefRowCount)
	if b.Document().cursorPosition != 5 {
		t.Errorf("Should be %#v, got %#v", 5, b.Document().cursorPosition)
	}

	// Going up to a line that's shorter.
	b.setDocument(&Document{}, DefColCount, DefRowCount)
	b.InsertTextMoveCursor("line1\nlong line2", DefColCount, DefRowCount, false)
	b.CursorUp(1, DefColCount, DefRowCount)
	if b.Document().cursorPosition != 5 {
		t.Errorf("Should be %#v, got %#v", 5, b.Document().cursorPosition)
	}
}

func TestBuffer_CursorDown(t *testing.T) {
	b := NewBuffer()
	b.InsertTextMoveCursor("line1\nline2", DefColCount, DefRowCount, false)
	b.cursorPosition = 3
	b.preferredColumn = -1

	// Normally going down
	b.CursorDown(1, DefColCount, DefRowCount)
	if b.Document().cursorPosition != istrings.RuneCountInString("line1\nlin") {
		t.Errorf("Should be %#v, got %#v", istrings.RuneCountInString("line1\nlin"), b.Document().cursorPosition)
	}

	// Going down to a line that's storter.
	b = NewBuffer()
	b.InsertTextMoveCursor("long line1\na\nb", DefColCount, DefRowCount, false)
	b.cursorPosition = 3
	b.CursorDown(1, DefColCount, DefRowCount)
	if b.Document().cursorPosition != istrings.RuneCountInString("long line1\na") {
		t.Errorf("Should be %#v, got %#v", istrings.RuneCountInString("long line1\na"), b.Document().cursorPosition)
	}
}

func TestBuffer_DeleteBeforeCursor(t *testing.T) {
	b := NewBuffer()
	b.InsertTextMoveCursor("some_text", DefColCount, DefRowCount, false)
	b.CursorLeft(2, DefColCount, DefRowCount)
	deleted := b.DeleteBeforeCursor(1, DefColCount, DefRowCount)

	if b.Text() != "some_txt" {
		t.Errorf("Should be %#v, got %#v", "some_txt", b.Text())
	}
	if deleted != "e" {
		t.Errorf("Should be %#v, got %#v", deleted, "e")
	}
	if b.cursorPosition != istrings.RuneCountInString("some_t") {
		t.Errorf("Should be %#v, got %#v", istrings.RuneCountInString("some_t"), b.cursorPosition)
	}

	// Delete over the characters length before cursor.
	deleted = b.DeleteBeforeCursor(100, DefColCount, DefRowCount)
	if deleted != "some_t" {
		t.Errorf("Should be %#v, got %#v", "some_t", deleted)
	}
	if b.Text() != "xt" {
		t.Errorf("Should be %#v, got %#v", "xt", b.Text())
	}

	// If cursor position is a beginning of line, it has no effect.
	deleted = b.DeleteBeforeCursor(1, DefColCount, DefRowCount)
	if deleted != "" {
		t.Errorf("Should be empty, got %#v", deleted)
	}
}

func TestBuffer_NewLine(t *testing.T) {
	b := NewBuffer()
	b.InsertTextMoveCursor("  hello", DefColCount, DefRowCount, false)
	b.NewLine(DefColCount, DefRowCount, false)
	ac := b.Text()
	ex := "  hello\n"
	if ac != ex {
		t.Errorf("Should be %#v, got %#v", ex, ac)
	}

	b = NewBuffer()
	b.InsertTextMoveCursor("  hello", DefColCount, DefRowCount, false)
	b.NewLine(DefColCount, DefRowCount, true)
	ac = b.Text()
	ex = "  hello\n  "
	if ac != ex {
		t.Errorf("Should be %#v, got %#v", ex, ac)
	}
}

func TestBuffer_JoinNextLine(t *testing.T) {
	b := NewBuffer()
	b.InsertTextMoveCursor("line1\nline2\nline3", DefColCount, DefRowCount, false)
	b.CursorUp(1, DefColCount, DefRowCount)
	b.JoinNextLine(" ", DefColCount, DefRowCount)

	ac := b.Text()
	ex := "line1\nline2 line3"
	if ac != ex {
		t.Errorf("Should be %#v, got %#v", ex, ac)
	}

	// Test when there is no '\n' in the text
	b = NewBuffer()
	b.InsertTextMoveCursor("line1", DefColCount, DefRowCount, false)
	b.cursorPosition = 0
	b.JoinNextLine(" ", DefColCount, DefRowCount)
	ac = b.Text()
	ex = "line1"
	if ac != ex {
		t.Errorf("Should be %#v, got %#v", ex, ac)
	}
}

func TestBuffer_SwapCharactersBeforeCursor(t *testing.T) {
	b := NewBuffer()
	b.InsertTextMoveCursor("hello world", DefColCount, DefRowCount, false)
	b.CursorLeft(2, DefColCount, DefRowCount)
	b.SwapCharactersBeforeCursor(DefColCount, DefRowCount)
	ac := b.Text()
	ex := "hello wrold"
	if ac != ex {
		t.Errorf("Should be %#v, got %#v", ex, ac)
	}
}

// ---------------------------------------------------------------------------
// Exact-Fill Test Matrix
// ---------------------------------------------------------------------------

// TestBuffer_DisplayCursorPosition_LegacyNormalization verifies that the
// legacy DisplayCursorPosition API returns (X=0, Y+=1) for exact-fill
// boundaries, preserving backward compatibility.
func TestBuffer_DisplayCursorPosition_LegacyNormalization(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		columns istrings.Width
		wantPos Position
	}{
		{
			name:    "non-fill returns as-is",
			text:    "ab",
			columns: 3,
			wantPos: Position{X: 2, Y: 0},
		},
		{
			name:    "exact-fill wraps to next line (legacy)",
			text:    "abc",
			columns: 3,
			wantPos: Position{X: 0, Y: 1},
		},
		{
			name:    "exact-fill with more text after",
			text:    "abcd",
			columns: 3,
			wantPos: Position{X: 1, Y: 1},
		},
		{
			name:    "multi-line exact-fill on last line",
			text:    "abcdef",
			columns: 3,
			wantPos: Position{X: 0, Y: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.InsertTextMoveCursor(tt.text, tt.columns, 25, false)
			got := b.DisplayCursorPosition(tt.columns)
			if got != tt.wantPos {
				t.Errorf("DisplayCursorPosition(%d) = %+v, want %+v", tt.columns, got, tt.wantPos)
			}
		})
	}
}

// TestBuffer_DisplayCursorPositionFullWidth verifies that the FullWidth API
// returns the raw exact-fill state without legacy normalization.
func TestBuffer_DisplayCursorPositionFullWidth(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		columns       istrings.Width
		wantPos       Position
		wantFullWidth bool
	}{
		{
			name:          "non-fill returns as-is",
			text:          "ab",
			columns:       3,
			wantPos:       Position{X: 2, Y: 0},
			wantFullWidth: false,
		},
		{
			name:          "exact-fill keeps raw state",
			text:          "abc",
			columns:       3,
			wantPos:       Position{X: 3, Y: 0},
			wantFullWidth: true,
		},
		{
			name:          "exact-fill with more text",
			text:          "abcd",
			columns:       3,
			wantPos:       Position{X: 1, Y: 1},
			wantFullWidth: false,
		},
		{
			name:          "multi-line exact-fill on last line",
			text:          "abcdef",
			columns:       3,
			wantPos:       Position{X: 3, Y: 1},
			wantFullWidth: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.InsertTextMoveCursor(tt.text, tt.columns, 25, false)
			pos, fullWidth := b.DisplayCursorPositionFullWidth(tt.columns)
			if pos != tt.wantPos {
				t.Errorf("Position = %+v, want %+v", pos, tt.wantPos)
			}
			if fullWidth != tt.wantFullWidth {
				t.Errorf("fullWidth = %v, want %v", fullWidth, tt.wantFullWidth)
			}
		})
	}
}

// TestBuffer_RecalculateStartLine_ExactFill verifies that recalculateStartLine
// is strictly idempotent and respects xenl (deferred-wrap) semantics: the
// viewport only scrolls when the cursor's logical line genuinely exceeds the
// visible range, not when an exact-fill wrap is pending.
func TestBuffer_RecalculateStartLine_ExactFill(t *testing.T) {
	t.Run("exact-fill does not preemptively scroll with rows=1", func(t *testing.T) {
		b := NewBuffer()
		// Text "abc" with columns=3 is exact-fill (X=3, Y=0, fullWidth=true).
		// Xenl semantics: the wrap has NOT occurred yet. The cursor is on line 0,
		// which is within [0, 0]. The viewport must NOT scroll preemptively.
		b.InsertTextMoveCursor("abc", 3, 1, false)
		if b.startLine != 0 {
			t.Errorf("startLine = %d, want 0 (xenl: exact-fill cursor is still on line 0)", b.startLine)
		}
		// Verify idempotency: calling again must not change startLine.
		b.recalculateStartLine(3, 1)
		if b.startLine != 0 {
			t.Errorf("startLine = %d after second call, want 0 (idempotency violation)", b.startLine)
		}
	})

	t.Run("non-fill does not scroll with rows=1", func(t *testing.T) {
		b := NewBuffer()
		// Text "ab" with columns=3 is NOT exact-fill, cursor at (2,0).
		// With rows=1, cursor is at Y=0 which fits within [0, 0], no scroll.
		b.InsertTextMoveCursor("ab", 3, 1, false)
		if b.startLine != 0 {
			t.Errorf("startLine = %d, want 0 (non-fill should not scroll)", b.startLine)
		}
	})

	t.Run("overflowing text scrolls correctly", func(t *testing.T) {
		b := NewBuffer()
		// "abcd" with columns=3: "abc" on line 0, "d" on line 1. Cursor at Y=1.
		// With rows=1, Y=1 > 0+1-1=0, so scroll to startLine=1.
		b.InsertTextMoveCursor("abcd", 3, 1, false)
		if b.startLine != 1 {
			t.Errorf("startLine = %d, want 1 (overflowing text should scroll)", b.startLine)
		}
		// Verify idempotency.
		b.recalculateStartLine(3, 1)
		if b.startLine != 1 {
			t.Errorf("startLine = %d after second call, want 1 (idempotency violation)", b.startLine)
		}
	})

	t.Run("exact-fill second line within viewport", func(t *testing.T) {
		b := NewBuffer()
		// "abcdef" with columns=3: line 0 "abc" exact-fills, line 1 "def" exact-fills.
		// Cursor at Y=1. With rows=2, Y=1 is within [0, 1]. No scroll.
		b.InsertTextMoveCursor("abcdef", 3, 2, false)
		if b.startLine != 0 {
			t.Errorf("startLine = %d, want 0 (cursor Y=1 is within rows=2 viewport)", b.startLine)
		}
	})

	t.Run("exact-fill third line scrolls with rows=2", func(t *testing.T) {
		b := NewBuffer()
		// "abcdefghi" with columns=3: 3 lines. Cursor at Y=2.
		// With rows=2, Y=2 > 0+2-1=1, so scroll to startLine=1.
		b.InsertTextMoveCursor("abcdefghi", 3, 2, false)
		if b.startLine != 1 {
			t.Errorf("startLine = %d, want 1 (cursor Y=2 beyond rows=2 viewport)", b.startLine)
		}
	})
}

// TestDocument_DisplayCursorPosition_LegacyNormalization verifies the legacy
// normalization is applied at the Document level too.
func TestDocument_DisplayCursorPosition_LegacyNormalization(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		cursor  istrings.RuneNumber
		columns istrings.Width
		want    Position
	}{
		{
			name:    "non-exact-fill",
			text:    "hello",
			cursor:  3,
			columns: 10,
			want:    Position{X: 3, Y: 0},
		},
		{
			name:    "exact-fill at cursor",
			text:    "abcde",
			cursor:  5,
			columns: 5,
			want:    Position{X: 0, Y: 1},
		},
		{
			name:    "cursor before exact-fill boundary",
			text:    "abcde",
			cursor:  3,
			columns: 5,
			want:    Position{X: 3, Y: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Document{
				Text:           tt.text,
				cursorPosition: tt.cursor,
			}
			got := d.DisplayCursorPosition(tt.columns)
			if got != tt.want {
				t.Errorf("DisplayCursorPosition(%d) = %+v, want %+v", tt.columns, got, tt.want)
			}
		})
	}
}
