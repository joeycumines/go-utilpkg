package prompt

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	istrings "github.com/joeycumines/go-prompt/strings"
)

// TestPositionJoinZeroOperand_PreservesLeft verifies that joining with an empty
// Position returns the left operand unchanged.
//
// This is the minimal proof that the review.md Blocking Issue 1 describes:
//
//	p := Position{X: 3, Y: 0}
//	got := p.Join(Position{})
//	// got must == p
//
// NOTE: Position does NOT have IsFullWidth in this codebase. The review.md
// appears to have analyzed a DIFFERENT version of the code. This test
// proves the claim is not applicable here.
func TestPositionJoinZeroOperand_PreservesLeft(t *testing.T) {
	cases := []struct {
		name  string
		left  Position
		right Position
	}{
		{
			name:  "X_only_zero_right",
			left:  Position{X: 3},
			right: Position{},
		},
		{
			name:  "Y_only_zero_right",
			left:  Position{Y: 5},
			right: Position{},
		},
		{
			name:  "both_zero_right",
			left:  Position{X: 3, Y: 5},
			right: Position{},
		},
		{
			name:  "zero_left",
			left:  Position{},
			right: Position{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.left.Join(tc.right)
			if diff := cmp.Diff(tc.left, got); diff != "" {
				t.Fatalf("Join with zero operand changed left:\n%s", diff)
			}
		})
	}
}

// TestPositionAddZeroOperand_PreservesLeft verifies that adding an empty
// Position returns the left operand unchanged.
//
// Same note as above: Position has no IsFullWidth field.
func TestPositionAddZeroOperand_PreservesLeft(t *testing.T) {
	cases := []struct {
		name  string
		left  Position
		right Position
	}{
		{
			name:  "X_only_zero_right",
			left:  Position{X: 3},
			right: Position{},
		},
		{
			name:  "Y_only_zero_right",
			left:  Position{Y: 5},
			right: Position{},
		},
		{
			name:  "both_zero_right",
			left:  Position{X: 3, Y: 5},
			right: Position{},
		},
		{
			name:  "zero_left",
			left:  Position{},
			right: Position{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.left.Add(tc.right)
			if diff := cmp.Diff(tc.left, got); diff != "" {
				t.Fatalf("Add with zero operand changed left:\n%s", diff)
			}
		})
	}
}

// TestPositionJoin_CompositionRules verifies the join semantics are correct
// when the right operand has Y > 0 (different line).
func TestPositionJoin_CompositionRules(t *testing.T) {
	// From the original tests, Join means:
	// - if right.Y == 0: p.X += right.X (same line advance)
	// - if right.Y > 0: p = right (new absolute position)
	cases := []struct {
		name  string
		left  Position
		right Position
		want  Position
	}{
		{
			name:  "same_line_X_adds",
			left:  Position{X: 3, Y: 1},
			right: Position{X: 5},
			want:  Position{X: 8, Y: 1},
		},
		{
			name:  "different_line_absolute",
			left:  Position{X: 3, Y: 1},
			right: Position{X: 5, Y: 2},
			want:  Position{X: 5, Y: 3}, // p.X = right.X, p.Y += right.Y
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.left.Join(tc.right)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("Join wrong:\n%s", diff)
			}
		})
	}
}

// TestRender_StartLinePastContent_MultilineCursorAboveVisible tests the scenario
// described in review.md Blocking Issue 2, but generalized to multi-line content.
//
// The review claims that when startLine > content, the cursor X ends up past
// the rendered content because only Y is clamped. We verify the current code
// clamps BOTH X and Y (already implemented).
//
// This test checks: multi-line content, startLine > 0, cursor at the first
// visible line (edge case where clamp fires just barely).
func TestRender_StartLinePastContent_MultilineCursorAboveVisible(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return "> " },
		col:            80,
		row:            24,
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
	}
	prefixWidth := istrings.GetWidth("> ")

	b := NewBuffer()
	// Multi-line: 3 lines
	// Line 0: "line0"
	// Line 1: "line1"
	// Line 2: "line2"
	b.InsertText("line0\nline1\nline2", false)
	// Set cursor at end of line 1 (before the '\n')
	// TextBeforeCursor = "line0\nline1"
	// cursorPosition = 12 (len of "line0\nline1")
	b.cursorPosition = istrings.RuneCountInString("line0\nline1")
	// buffer.startLine = 2 means viewport shows lines 2+
	// But cursor is at line 1 (above visible). This would be corrected by
	// recalculateStartLine if called, but Render doesn't call it — it uses
	// the values as given.
	b.startLine = 2

	mockOut.reset()
	r.Render(b, NewCompletionManager(0), nil)

	// The clamp should fire: cursor's absolute Y (1) < buffer.startLine (2)
	// Expected: cursor clamped to {X: prefixWidth, Y: 0}
	// Because: only the first visible line (line 2) is rendered, and it's at viewport Y=0.
	want := Position{X: prefixWidth, Y: 0}
	if r.previousCursor != want {
		t.Errorf("previousCursor: want %+v, got %+v", want, r.previousCursor)
	}
}

// TestRender_StartLinePastContent_AllLinesAboveVisible tests the simplest case:
// single-line content, startLine way past it. The review's concrete example.
func TestRender_StartLinePastContent_AllLinesAboveVisible(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return "> " },
		col:            80,
		row:            24,
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
	}
	prefixWidth := istrings.GetWidth("> ")

	b := NewBuffer()
	b.InsertText("hello", false)
	b.startLine = 5 // scrolled far past content

	mockOut.reset()
	r.Render(b, NewCompletionManager(0), nil)

	// cursor.Y (0) < buffer.startLine (5) → clamp to {X: prefixWidth, Y: 0}
	want := Position{X: prefixWidth, Y: 0}
	if r.previousCursor != want {
		t.Errorf("previousCursor: want %+v, got %+v", want, r.previousCursor)
	}
}

// TestRender_StartLineZero_CursorInMiddle verifies the NORMAL case works:
// buffer.startLine = 0, cursor at end of multi-line content.
func TestRender_StartLineZero_CursorInMiddle(t *testing.T) {
	mockOut := &mockWriterLogger{}
	r := &Renderer{
		out:            mockOut,
		prefixCallback: func() string { return "> " },
		col:            80,
		row:            24,
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
	}
	prefixWidth := istrings.GetWidth("> ")

	b := NewBuffer()
	b.InsertText("line0\nline1\nline2", false)
	// cursor at end of line 1 (not the last line)
	b.cursorPosition = istrings.RuneCountInString("line0\nline1")
	b.startLine = 0

	mockOut.reset()
	r.Render(b, NewCompletionManager(0), nil)

	// cursor absolute = {X: 5, Y: 1}
	// viewport relative: X += 2, Y -= 0 → {X: 7, Y: 1}
	// No clamp (cursor.Y >= startLine)
	want := Position{X: prefixWidth + 5, Y: 1}
	if r.previousCursor != want {
		t.Errorf("previousCursor: want %+v, got %+v", want, r.previousCursor)
	}
}
