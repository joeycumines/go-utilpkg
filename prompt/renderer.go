package prompt

import (
	"strings"
	"unicode/utf8"

	"github.com/joeycumines/go-prompt/debug"
	istrings "github.com/joeycumines/go-prompt/strings"
	"github.com/rivo/uniseg"
)

const multilinePrefixCharacter = '.'

// Takes care of the rendering process
type Renderer struct {
	out               Writer
	prefixCallback    PrefixCallback
	breakLineCallback func(*Document)
	title             string
	row               int
	col               istrings.Width
	indentSize        int // How many spaces constitute a single indentation level

	previousCursor Position
	// previousCursorIsFullWidth preserves the logical exact-fill state of the
	// last prompt cursor so later movement/clear math can map it to the correct
	// physical column (rightmost cell, not the logical X==columns sentinel).
	previousCursorIsFullWidth bool

	// Track previous frame dimensions for precise clearing
	previousInputLines      int
	previousCompletionLines int

	// colors,
	prefixTextColor              Color
	prefixBGColor                Color
	inputTextColor               Color
	inputBGColor                 Color
	suggestionTextColor          Color
	suggestionBGColor            Color
	selectedSuggestionTextColor  Color
	selectedSuggestionBGColor    Color
	descriptionTextColor         Color
	descriptionBGColor           Color
	selectedDescriptionTextColor Color
	selectedDescriptionBGColor   Color
	scrollbarThumbColor          Color
	scrollbarBGColor             Color

	// dynamicCompletion controls whether the completion dropdown adapts to
	// the available space below the cursor.
	dynamicCompletion bool

	// Performance optimization buffers
	reflowScratch         []ReflowState
	tokenScratch          []Token
	reflowScratchPrepared bool // true IFF reflowScratch was populated by computeReflow for the CURRENT buffer text

	// wasZeroHeight tracks whether the previous frame had r.row <= 0.
	// When the terminal recovers to a positive height, this flag forces
	// the Buffer to recalculate startLine before rendering, preventing
	// stale viewport anchoring artifacts.
	wasZeroHeight bool

	// previousCompletionAbove tracks whether the completion dropdown was
	// rendered above the cursor in the previous frame, so that clear()
	// can erase lines in the correct direction.
	previousCompletionAbove bool
}

// Build a new Renderer.
func NewRenderer() *Renderer {
	defaultWriter := NewStdoutWriter()
	registerWriter(defaultWriter)

	return &Renderer{
		out:                          defaultWriter,
		indentSize:                   DefaultIndentSize,
		prefixCallback:               DefaultPrefixCallback,
		prefixTextColor:              Blue,
		prefixBGColor:                DefaultColor,
		inputTextColor:               DefaultColor,
		inputBGColor:                 DefaultColor,
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

// Setup to initialize console output.
func (r *Renderer) Setup() {
	if r.title != "" {
		r.out.SetTitle(r.title)
		r.flush()
	}
}

func (r *Renderer) renderPrefix(prefix string) {
	r.out.SetColor(r.prefixTextColor, r.prefixBGColor, false)
	if _, err := r.out.WriteString("\r"); err != nil {
		panic(err)
	}
	if _, err := r.out.WriteString(prefix); err != nil {
		panic(err)
	}
	r.out.SetColor(DefaultColor, DefaultColor, false)
}

// Close to clear title and erase.
func (r *Renderer) Close() {
	r.out.ClearTitle()
	r.out.EraseDown()
	r.flush()
	r.previousInputLines = 0
	r.previousCompletionLines = 0
	r.previousCompletionAbove = false
	r.previousCursor = Position{}
	r.previousCursorIsFullWidth = false
	r.reflowScratchPrepared = false
}

func (r *Renderer) prepareArea(lines int) {
	// Create space below the cursor for the completion window by
	// writing newlines (which scroll at the bottom of the screen)
	// then moving back up. This replaces ESC D / ESC M (Index /
	// Reverse Index) which behave unreliably on macOS Terminal.app.
	for range lines {
		r.out.WriteRawString("\n")
	}
	r.out.CursorUp(lines)
}

// UpdateWinSize called when window size is changed.
func (r *Renderer) UpdateWinSize(ws *WinSize) {
	r.row = int(ws.Row)
	r.col = istrings.Width(ws.Col)
}

// renderCompletion renders the completion dropdown near the given cursor position.
// cursor and cursorIsFullWidth must be the VIEWPORT-CLAMPED cursor already computed
// by the caller (Render). The dropdown renders below the cursor when space permits,
// or above the cursor when more space is available above.
func (r *Renderer) renderCompletion(completions *CompletionManager, cursor Position, cursorIsFullWidth bool) int {
	suggestions := completions.GetSuggestions()
	if len(suggestions) == 0 || completions.IsHidden() {
		r.previousCompletionAbove = false
		return 0
	}
	prefix := r.prefixCallback()
	prefixWidth := istrings.GetWidth(prefix)
	formatted, width := formatSuggestions(
		suggestions,
		r.col-prefixWidth-1, // -1 means a width of scrollbar
	)
	// +1 means a width of scrollbar.
	width++

	contentHeight := len(formatted)
	windowHeight := min(contentHeight, int(completions.max))

	renderAbove := false
	if r.dynamicCompletion {
		cursorLine := cursor.Y
		if cursorLine < 0 {
			cursorLine = 0
		} else if cursorLine >= r.row {
			cursorLine = r.row - 1
		}
		availableRowsBelow := r.row - cursorLine - 1
		availableRowsAbove := cursorLine

		if availableRowsBelow <= 0 && availableRowsAbove <= 0 {
			r.previousCompletionAbove = false
			return 0
		}

		// Decide direction: prefer below, switch to above only when below
		// cannot fit the window but above can fit more.
		if availableRowsBelow >= windowHeight {
			// Enough room below — render below.
		} else if availableRowsAbove > availableRowsBelow {
			// More room above — render above.
			renderAbove = true
			if windowHeight > availableRowsAbove {
				windowHeight = availableRowsAbove
			}
		} else {
			// Below is the best we have.
			if windowHeight > availableRowsBelow {
				windowHeight = availableRowsBelow
			}
		}
	}
	if windowHeight <= 0 {
		r.previousCompletionAbove = false
		return 0
	}

	// Store the actual window height for paging calculations
	completions.lastWindowHeight = windowHeight

	// Adjust CompletionManager state to match actual window height
	completions.adjustWindowHeight(windowHeight, contentHeight)

	formatted = formatted[completions.verticalScroll : completions.verticalScroll+windowHeight]

	x := cursor.X
	adjustedCursor := cursor
	needsBackward := x+width >= r.col
	backwardShift := istrings.Width(0)
	if needsBackward {
		backwardShift = x + width - r.col
		// Compute adjusted cursor position without emitting terminal commands yet.
		adjustedCursor = Position{X: cursor.X - backwardShift, Y: cursor.Y}
	}

	fractionVisible := float64(windowHeight) / float64(contentHeight)
	fractionAbove := float64(completions.verticalScroll) / float64(contentHeight)

	scrollbarHeight := int(clamp(float64(windowHeight), 1, float64(windowHeight)*fractionVisible))
	scrollbarTop := int(float64(windowHeight) * fractionAbove)

	isScrollThumb := func(row int) bool {
		return scrollbarTop <= row && row < scrollbarTop+scrollbarHeight
	}

	selected := completions.selected - completions.verticalScroll
	cursorColumnSpacing := adjustedCursor

	r.previousCompletionAbove = renderAbove

	if renderAbove {
		// Render ABOVE the cursor. Move up windowHeight rows, render
		// top-to-bottom, then return to the cursor row.
		r.syncCursor(cursorIsFullWidth)
		r.out.CursorUp(windowHeight)

		r.out.SetColor(White, Cyan, false)
		for i := 0; i < windowHeight; i++ {
			// Position: move to column, render on current line, then down.
			if _, err := r.out.WriteString("\r"); err != nil {
				panic(err)
			}
			if cursorColumnSpacing.X > 0 {
				r.out.CursorForward(int(cursorColumnSpacing.X))
			}

			if i == selected {
				r.out.SetColor(r.selectedSuggestionTextColor, r.selectedSuggestionBGColor, true)
			} else {
				r.out.SetColor(r.suggestionTextColor, r.suggestionBGColor, false)
			}
			if _, err := r.out.WriteString(formatted[i].Text); err != nil {
				panic(err)
			}

			if i == selected {
				r.out.SetColor(r.selectedDescriptionTextColor, r.selectedDescriptionBGColor, false)
			} else {
				r.out.SetColor(r.descriptionTextColor, r.descriptionBGColor, false)
			}
			if _, err := r.out.WriteString(formatted[i].Description); err != nil {
				panic(err)
			}

			if isScrollThumb(i) {
				r.out.SetColor(DefaultColor, r.scrollbarThumbColor, false)
			} else {
				r.out.SetColor(DefaultColor, r.scrollbarBGColor, false)
			}
			if _, err := r.out.WriteString(" "); err != nil {
				panic(err)
			}
			r.out.SetColor(DefaultColor, DefaultColor, false)

			if i < windowHeight-1 {
				if needsBackward {
					r.syncCursor(true)
					r.restoreColumn(cursorColumnSpacing.X)
				}
				r.out.CursorDown(1)
			}
		}

		// Return to cursor row: we rendered windowHeight lines starting
		// from windowHeight rows above. After the last line we're 1 row
		// above the cursor, so move down 1.
		if needsBackward {
			r.syncCursor(true)
			r.restoreColumn(cursorColumnSpacing.X)
		}
		r.out.CursorDown(1)

		// Restore horizontal position to original cursor X.
		r.restoreColumn(r.physicalColumn(cursor, cursorIsFullWidth))
	} else {
		// Render BELOW the cursor (original path).
		r.syncCursor(cursorIsFullWidth)
		r.prepareArea(windowHeight)

		r.out.SetColor(White, Cyan, false)
		for i := 0; i < windowHeight; i++ {
			alignNextLine(r, cursorColumnSpacing.X)

			if i == selected {
				r.out.SetColor(r.selectedSuggestionTextColor, r.selectedSuggestionBGColor, true)
			} else {
				r.out.SetColor(r.suggestionTextColor, r.suggestionBGColor, false)
			}
			if _, err := r.out.WriteString(formatted[i].Text); err != nil {
				panic(err)
			}

			if i == selected {
				r.out.SetColor(r.selectedDescriptionTextColor, r.selectedDescriptionBGColor, false)
			} else {
				r.out.SetColor(r.descriptionTextColor, r.descriptionBGColor, false)
			}
			if _, err := r.out.WriteString(formatted[i].Description); err != nil {
				panic(err)
			}

			if isScrollThumb(i) {
				r.out.SetColor(DefaultColor, r.scrollbarThumbColor, false)
			} else {
				r.out.SetColor(DefaultColor, r.scrollbarBGColor, false)
			}
			if _, err := r.out.WriteString(" "); err != nil {
				panic(err)
			}
			r.out.SetColor(DefaultColor, DefaultColor, false)

			rowEnd := adjustedCursor.Add(Position{X: width})
			if needsBackward {
				// Normalize wrap-pending state before horizontal motion, matching
				// the pattern used in the main Render path.
				r.syncCursor(true)
			}
			r.move(rowEnd, needsBackward, adjustedCursor, false)
		}

		if adjustedCursor != cursor || cursorIsFullWidth {
			r.move(adjustedCursor, false, cursor, cursorIsFullWidth)
		}

		r.out.CursorUp(windowHeight)
	}

	r.out.SetColor(DefaultColor, DefaultColor, false)
	return windowHeight
}

// countInputLines calculates how many visual lines the input text will occupy
// when rendered.
//
// Wrap semantics: lookahead `>` is used so that a character that would cause
// the line to EXCEED the column width is placed on the new line.  This matches
// how real terminals handle auto-wrapping: the character that overflows is
// rendered on the next line, not discarded.  Using `>=` (non-lookahead) means
// a character at exactly the boundary is placed on the current line, which can
// allow multi-width characters to overshoot the terminal width.
func (r *Renderer) countInputLines(text string, startLine int, col istrings.Width) int {
	if text == "" {
		return 1
	}

	endLine := startLine + int(r.row)
	reflower := NewTerminalReflower(text, startLine, endLine, col, true)
	lineCount := 0

	for {
		state, ok := reflower.Next()
		if !ok {
			break
		}
		if state.IsVisible {
			lineCount++
			// Optimization: once we've counted enough lines to fill the visible
			// viewport (r.row lines), we can stop — the viewport can't show more.
			if lineCount >= r.row {
				break
			}
		}
	}

	if lineCount == 0 {
		lineCount = 1
	}

	return lineCount
}

// computeReflow computes the visual reflow for the given input within the
// viewport [startLine, endLine) and populates r.reflowScratch with the results.
// Returns the count of visible (in-viewport) lines. r.reflowScratch is reused
// across calls; caller must not retain references to its elements.
//
// This is the single source of truth for all physical line calculations.
// The hot-path renderText/lex read directly from r.reflowScratch to avoid
// re-allocating a TerminalReflower per render.
func (r *Renderer) computeReflow(input string, startLine, endLine int, col istrings.Width) int {
	r.reflowScratch = r.reflowScratch[:0]
	r.reflowScratchPrepared = false
	reflower := NewTerminalReflower(input, startLine, endLine, col, true)

	visibleCount := 0
	for {
		state, ok := reflower.Next()
		if !ok {
			break
		}
		r.reflowScratch = append(r.reflowScratch, state)
		if state.IsVisible {
			visibleCount++
			// Viewport height break: once we've collected enough visible
			// lines to fill the viewport, stop. Lines beyond the viewport
			// cannot be displayed and processing them is wasted O(N) work.
			if visibleCount >= r.row {
				break
			}
		}
	}

	r.reflowScratchPrepared = true

	if visibleCount == 0 {
		return 1 // Always at least 1 to avoid divide-by-zero in callers
	}
	return visibleCount
}

// Render renders to the console.
func (r *Renderer) Render(buffer *Buffer, completion *CompletionManager, lexer Lexer) {
	// In situations where a pseudo tty is allocated (e.g. within a docker container),
	// window size via TIOCGWINSZ is not immediately available and will result in 0,0 dimensions.
	if r.col <= 0 || r.row <= 0 {
		r.wasZeroHeight = true
		return
	}

	// Intercept recovery from zero-height: force the Buffer to recalculate
	// startLine using the newly recovered dimensions so the viewport anchors correctly.
	if r.wasZeroHeight {
		buffer.recalculateStartLine(r.UserInputColumns(), r.row)
		r.wasZeroHeight = false
	}

	defer func() { r.flush() }()

	r.clear(r.previousCursor, r.previousCursorIsFullWidth)

	text := buffer.Text()
	prefix := r.prefixCallback()
	prefixWidth := istrings.GetWidth(prefix)
	col := r.col - prefixWidth

	// Viewport bounds: [startLine, viewportEndExclusive) is the exclusive range
	// for reflow and line counting. visibleEndLineInclusive is the last line
	// that positionAtEndOfStringLine should consider (inclusive).
	viewportEndExclusive := buffer.startLine + int(r.row)
	visibleEndLineInclusive := viewportEndExclusive - 1

	visibleEndCursor, visibleEndIsFullWidth := positionAtEndOfStringLine(text, col, visibleEndLineInclusive)
	if visibleEndCursor.Y < buffer.startLine {
		visibleEndCursor = Position{X: prefixWidth, Y: 0}
		visibleEndIsFullWidth = false
	} else {
		visibleEndCursor.X += prefixWidth
		visibleEndCursor.Y -= buffer.startLine
	}

	// Rendering
	r.out.HideCursor()
	defer r.out.ShowCursor()

	// Track input lines for next frame's clear operation.
	// computeReflow populates r.reflowScratch so renderText/lex can read
	// it without re-allocating a TerminalReflower.
	inputLineCount := r.computeReflow(buffer.Text(), buffer.startLine, viewportEndExclusive, col)
	r.previousInputLines = inputLineCount

	r.renderText(lexer, buffer.Text(), buffer.startLine)

	r.out.SetColor(DefaultColor, DefaultColor, false)

	// Cursor used for the movement relative to what's rendered.
	// This is always the end of the LAST rendered line (the content we just wrote).
	cursor := visibleEndCursor

	// Compute where the logical cursor actually is.
	targetCursor, targetIsFullWidth := buffer.DisplayCursorPositionFullWidth(col)

	if targetCursor.Y < buffer.startLine {
		// Logical cursor is above the visible viewport — clamp to the first visible rendered position.
		targetCursor = Position{X: prefixWidth, Y: 0}
		targetIsFullWidth = false
	} else if targetCursor.Y > visibleEndLineInclusive {
		// Logical cursor is below the visible rendered content.
		targetCursor = visibleEndCursor
		targetIsFullWidth = visibleEndIsFullWidth
	} else {
		// Logical cursor is within the visible range — convert to viewport-relative coordinates.
		targetCursor.X += prefixWidth
		targetCursor.Y -= buffer.startLine
	}
	// renderText just wrote the visible end position. If that line exactly filled
	// the terminal width, the terminal may be in a deferred-wrap state; clear
	// that ambiguity before issuing cursor motion away from the rendered end.
	r.syncCursor(visibleEndIsFullWidth)
	cursor = r.move(cursor, visibleEndIsFullWidth, targetCursor, targetIsFullWidth)

	// Ensure physical cursor state is normalized if we're at the right margin
	// before we potentially render the completion window.
	r.syncCursor(targetIsFullWidth)

	if completion != nil {
		completionLines := r.renderCompletion(completion, targetCursor, targetIsFullWidth)
		r.previousCompletionLines = completionLines
	} else {
		r.previousCompletionLines = 0
		r.previousCompletionAbove = false
	}
	r.previousCursor = cursor
	r.previousCursorIsFullWidth = targetIsFullWidth
}

func (r *Renderer) renderText(lexer Lexer, input string, startLine int) {
	if lexer != nil {
		r.lex(lexer, input, startLine)
		return
	}

	prefix := r.prefixCallback()
	prefixWidth := istrings.GetWidth(prefix)
	col := r.col - prefixWidth
	multilinePrefix := r.getMultilinePrefix(prefix)
	if startLine != 0 {
		prefix = multilinePrefix
	}

	// If r.reflowScratchPrepared is true, computeReflow populated the scratch
	// for the current buffer text. Use it for the zero-allocation hot path.
	// A bool flag (not cap check) prevents stale scratch from being used
	// when BreakLine calls renderText without going through computeReflow.
	if r.reflowScratchPrepared {
		// Count visible lines in r.reflowScratch.
		visibleCount := 0
		for i := range r.reflowScratch {
			if r.reflowScratch[i].IsVisible {
				visibleCount++
			}
		}

		if visibleCount == 0 {
			// r.reflowScratch was populated but contained no visible lines
			// (e.g., startLine is beyond all content). Render one blank line.
			r.renderLine(prefix, "", r.inputTextColor)
			return
		}

		// Peek-based iteration over r.reflowScratch.
		visibleProcessed := 0
		for i := range r.reflowScratch {
			state := &r.reflowScratch[i]
			if !state.IsVisible {
				continue
			}

			visibleProcessed++
			isLast := visibleProcessed == visibleCount

			// Use ByteStart/ByteEnd to slice the original input string directly.
			// This avoids the heap allocation that []byte(string) would cause.
			// ToValidUTF8 only allocates when the slice contains invalid bytes.
			line := strings.ToValidUTF8(input[state.ByteStart:state.ByteEnd], "\ufffd")
			r.renderLine(prefix, line, r.inputTextColor)
			if !isLast {
				// Emit a newline between visible lines. Using \n (not CursorDown) is
				// required: at the terminal's bottom margin, CursorDown silently
				// fails to scroll the viewport, causing wrapped lines to overwrite
				// each other. \n always triggers a scroll, allocating a fresh row.
				r.out.WriteRawString("\n")
			}
			prefix = multilinePrefix
		}
		return
	}

	// Fallback: r.reflowScratch has zero capacity (computeReflow not called —
	// direct call to renderText without going through Render). Delegate to the
	// original reflower-per-call logic to maintain correctness.
	endLine := startLine + int(r.row)
	reflower := NewTerminalReflower(input, startLine, endLine, col, true)

	// Peek-based iteration: advance two states per loop to know if the
	// current visible line is the last one.
	var current ReflowState
	var currentOK bool
	var next ReflowState
	var nextOK bool

	current, currentOK = reflower.Next()
	if !currentOK {
		r.renderLine(prefix, "", r.inputTextColor)
		return
	}

	next, nextOK = reflower.Next()

	anyVisible := false
	for {
		if currentOK && current.IsVisible {
			anyVisible = true
			isLast := !nextOK || !next.IsVisible
			line := strings.ToValidUTF8(input[current.ByteStart:current.ByteEnd], "\ufffd")
			r.renderLine(prefix, line, r.inputTextColor)
			if !isLast {
				r.out.WriteRawString("\n")
			}
			prefix = multilinePrefix
		}

		current = next
		currentOK = nextOK
		if !currentOK {
			break
		}
		next, nextOK = reflower.Next()
	}

	if !anyVisible {
		r.renderLine(prefix, "", r.inputTextColor)
	}
}

func (r *Renderer) flush() {
	debug.AssertNoError(r.out.Flush())
}

func (r *Renderer) renderLine(prefix, line string, color Color) {
	r.renderPrefix(prefix)
	r.writeStringColor(line, color)
}

func (r *Renderer) writeStringColor(text string, color Color) {
	r.out.SetColor(color, r.inputBGColor, false)
	if _, err := r.out.WriteString(text); err != nil {
		panic(err)
	}
}

func (r *Renderer) getMultilinePrefix(prefix string) string {
	var spaceCount int
	var dotCount int
	var nonSpaceCharSeen bool
	for len(prefix) != 0 {
		char, size := utf8.DecodeLastRuneInString(prefix)
		prefix = prefix[:len(prefix)-size]
		charWidth := istrings.GetRuneWidth(char)
		if nonSpaceCharSeen {
			dotCount += int(charWidth)
			continue
		}
		if char != ' ' {
			nonSpaceCharSeen = true
			dotCount += int(charWidth)
			continue
		}
		spaceCount += int(charWidth)
	}

	var multilinePrefixBuilder strings.Builder

	for i := 0; i < dotCount; i++ {
		multilinePrefixBuilder.WriteByte(multilinePrefixCharacter)
	}
	for i := 0; i < spaceCount; i++ {
		multilinePrefixBuilder.WriteByte(IndentUnit)
	}

	return multilinePrefixBuilder.String()
}

// lex processes the given input with the given lexer
// and writes the result.
func (r *Renderer) lex(lexer Lexer, input string, startLine int) {
	prefix := r.prefixCallback()
	multilinePrefix := r.getMultilinePrefix(prefix)
	if startLine != 0 {
		prefix = multilinePrefix
	}

	// Gather tokens into r.tokenScratch (pooled buffer to reduce allocations).
	lexer.Init(input)
	r.tokenScratch = r.tokenScratch[:0]
	for {
		tok, ok := lexer.Next()
		if !ok {
			break
		}
		r.tokenScratch = append(r.tokenScratch, tok)
	}
	allTokens := r.tokenScratch

	// If r.reflowScratchPrepared is true, computeReflow populated the scratch
	// for the current buffer text. Use it for the zero-allocation hot path.
	if r.reflowScratchPrepared {
		// Count visible lines.
		visibleCount := 0
		for i := range r.reflowScratch {
			if r.reflowScratch[i].IsVisible {
				visibleCount++
			}
		}

		if visibleCount == 0 {
			r.renderPrefix(prefix)
			return
		}

		// Iterate r.reflowScratch.
		visibleProcessed := 0
		for i := range r.reflowScratch {
			state := &r.reflowScratch[i]
			if !state.IsVisible {
				continue
			}

			visibleProcessed++
			isLast := visibleProcessed == visibleCount

			r.renderPrefix(prefix)

			lineStart := state.ByteStart
			lineEnd := state.ByteEnd
			pos := lineStart

			for _, tok := range allTokens {
				tokFirst := int(tok.FirstByteIndex())
				tokLast := int(tok.LastByteIndex())

				if tokLast < lineStart {
					continue
				}
				if tokFirst >= lineEnd {
					break
				}

				if pos < tokFirst && pos < lineEnd {
					segEnd := tokFirst
					if pos < segEnd {
						r.out.SetDisplayAttributes(r.inputTextColor, r.inputBGColor, DisplayReset)
						if _, err := r.out.WriteString(strings.ToValidUTF8(input[pos:segEnd], "\ufffd")); err != nil {
							panic(err)
						}
						r.resetFormatting()
					}
					pos = segEnd
				}

				if tokFirst < lineEnd && tokLast >= lineStart {
					segStart := max(tokFirst, pos)
					segEnd := min(tokLast+1, lineEnd)
					if segStart < segEnd {
						r.out.SetDisplayAttributes(tok.Color(), tok.BackgroundColor(), tok.DisplayAttributes()...)
						if _, err := r.out.WriteString(strings.ToValidUTF8(input[segStart:segEnd], "\ufffd")); err != nil {
							panic(err)
						}
						r.resetFormatting()
					}
				}
				pos = tokLast + 1
			}

			if pos < lineEnd {
				r.out.SetDisplayAttributes(r.inputTextColor, r.inputBGColor, DisplayReset)
				if _, err := r.out.WriteString(strings.ToValidUTF8(input[pos:lineEnd], "\ufffd")); err != nil {
					panic(err)
				}
				r.resetFormatting()
			}

			if !isLast {
				r.out.WriteRawString("\n")
			}
			prefix = multilinePrefix
		}
		return
	}

	// Fallback: r.reflowScratch not populated (lex called without going through Render).
	// Use original reflower-per-call logic to maintain correctness.
	prefixWidth := istrings.GetWidth(prefix)
	col := r.col - prefixWidth
	endLine := startLine + int(r.row)
	reflower := NewTerminalReflower(input, startLine, endLine, col, true)

	var current ReflowState
	var currentOK bool
	var next ReflowState
	var nextOK bool

	current, currentOK = reflower.Next()
	if !currentOK {
		r.renderPrefix(prefix)
		return
	}
	next, nextOK = reflower.Next()

	anyVisible := false
	for {
		if currentOK && current.IsVisible {
			anyVisible = true
			isLast := !nextOK || !next.IsVisible

			r.renderPrefix(prefix)

			lineStart := current.ByteStart
			lineEnd := current.ByteEnd
			pos := lineStart

			for _, tok := range allTokens {
				tokFirst := int(tok.FirstByteIndex())
				tokLast := int(tok.LastByteIndex())

				if tokLast < lineStart {
					continue
				}
				if tokFirst >= lineEnd {
					break
				}

				if pos < tokFirst && pos < lineEnd {
					segEnd := tokFirst
					if pos < segEnd {
						r.out.SetDisplayAttributes(r.inputTextColor, r.inputBGColor, DisplayReset)
						if _, err := r.out.WriteString(strings.ToValidUTF8(input[pos:segEnd], "\ufffd")); err != nil {
							panic(err)
						}
						r.resetFormatting()
					}
					pos = segEnd
				}

				if tokFirst < lineEnd && tokLast >= lineStart {
					segStart := max(tokFirst, pos)
					segEnd := min(tokLast+1, lineEnd)
					if segStart < segEnd {
						r.out.SetDisplayAttributes(tok.Color(), tok.BackgroundColor(), tok.DisplayAttributes()...)
						if _, err := r.out.WriteString(strings.ToValidUTF8(input[segStart:segEnd], "\ufffd")); err != nil {
							panic(err)
						}
						r.resetFormatting()
					}
				}
				pos = tokLast + 1
			}

			if pos < lineEnd {
				r.out.SetDisplayAttributes(r.inputTextColor, r.inputBGColor, DisplayReset)
				if _, err := r.out.WriteString(strings.ToValidUTF8(input[pos:lineEnd], "\ufffd")); err != nil {
					panic(err)
				}
				r.resetFormatting()
			}

			if !isLast {
				r.out.WriteRawString("\n")
			}
			prefix = multilinePrefix
		}

		current = next
		currentOK = nextOK
		if !currentOK {
			break
		}
		next, nextOK = reflower.Next()
	}

	if !anyVisible {
		r.renderPrefix(prefix)
	}
}

func (r *Renderer) resetFormatting() {
	r.out.SetDisplayAttributes(r.inputTextColor, r.inputBGColor, DisplayReset)
}

// BreakLine to break line.
func (r *Renderer) BreakLine(buffer *Buffer, lexer Lexer) {
	// Invalidate stale reflowScratch — BreakLine calls renderText on a
	// potentially different buffer than the last Render() that populated it.
	r.reflowScratchPrepared = false

	if r.col <= 0 || r.row <= 0 {
		// Zero-sized viewport: cannot render, but MUST still commit the input
		// by emitting the newline and firing the callback. wasZeroHeight is set
		// so that if the next call has positive dimensions, we force a
		// recalculateStartLine.
		r.wasZeroHeight = true
		if _, err := r.out.WriteString("\n"); err != nil {
			panic(err)
		}
		r.out.SetColor(DefaultColor, DefaultColor, false)
		r.flush()
		if r.breakLineCallback != nil {
			r.breakLineCallback(buffer.Document())
		}
		r.previousInputLines = 0
		r.previousCompletionLines = 0
		r.previousCompletionAbove = false
		r.previousCursor = Position{}
		r.previousCursorIsFullWidth = false
		return
	}

	// Intercept recovery from zero-height: force the Buffer to recalculate
	// startLine so the viewport anchors correctly after resizing.
	if r.wasZeroHeight {
		buffer.recalculateStartLine(r.UserInputColumns(), r.row)
		r.wasZeroHeight = false
	}

	// Normal path (r.row > 0): erase previous frame and render.
	r.clear(r.previousCursor, r.previousCursorIsFullWidth)

	r.renderText(lexer, buffer.Text(), buffer.startLine)

	// Ensure physical cursor state is normalized if we're at the right margin
	// before we emit the final newline for this prompt.
	prefix := r.prefixCallback()
	prefixWidth := istrings.GetWidth(prefix)
	col := r.col - prefixWidth
	visibleEndLineInclusive := buffer.startLine + int(r.row) - 1
	text := buffer.Text()
	visibleEndCursor, cursorIsFullWidth := positionAtEndOfStringLine(text, col, visibleEndLineInclusive)

	if visibleEndCursor.Y < buffer.startLine {
		visibleEndCursor = Position{X: prefixWidth, Y: 0}
		cursorIsFullWidth = false
	} else {
		visibleEndCursor.X += prefixWidth
		visibleEndCursor.Y -= buffer.startLine
	}

	r.syncCursor(cursorIsFullWidth)

	if _, err := r.out.WriteString("\n"); err != nil {
		panic(err)
	}

	r.out.SetColor(DefaultColor, DefaultColor, false)

	r.flush()
	if r.breakLineCallback != nil {
		r.breakLineCallback(buffer.Document())
	}

	r.previousInputLines = 0
	r.previousCompletionLines = 0
	r.previousCompletionAbove = false
	r.previousCursor = Position{}
	r.previousCursorIsFullWidth = false
}

// Get the number of columns that are available
// for user input.
func (r *Renderer) UserInputColumns() istrings.Width {
	return r.col - istrings.GetWidth(r.prefixCallback())
}

// clear erases the screen from a beginning of input
// even if there is a line break which means input length exceeds a window's width.
// Uses carriage return for reliable horizontal positioning and explicit
// cursor movement to return to start position (avoids terminal-dependent
// SaveCursor/UnSaveCursor behavior).
//
// When the previous frame's completion dropdown was rendered above the cursor
// (previousCompletionAbove == true), those rows overlap earlier input rows
// within the prompt area, so clearing the input area clears them too.
func (r *Renderer) clear(cursor Position, cursorIsFullWidth bool) {
	// Compute the number of rows to erase below the cursor position.
	//
	// Overlapping model:
	//   - Below-cursor completions extend BELOW the input area.  They add
	//     rows beyond the input, so the total is inputLines + completionLines.
	//   - Above-cursor completions render WITHIN the input area (guaranteed
	//     by windowHeight <= availableRowsAbove in renderCompletion).  They
	//     overwrite input rows rather than adding new ones.  The union of the
	//     two regions is simply the input area: inputLines.
	var belowLines int
	if r.previousCompletionAbove {
		belowLines = r.previousInputLines
	} else {
		belowLines = r.previousInputLines + r.previousCompletionLines
	}
	if belowLines <= 0 {
		// Fallback to simple clear if we don't have previous frame info
		r.move(cursor, cursorIsFullWidth, Position{}, false)
		r.out.EraseDown()
		return
	}

	// Move to the start position (row 0, column 0) of the input area.
	r.move(cursor, cursorIsFullWidth, Position{}, false)

	// Erase all lines top-to-bottom: \r moves to column 0, EraseLine clears
	// the row, \n advances to the next row.  Because above-cursor completions
	// overlap the input rows, no separate upward erasure pass is needed.
	for i := range belowLines {
		if _, err := r.out.WriteString("\r"); err != nil {
			panic(err)
		}
		r.out.EraseLine()
		if i < belowLines-1 {
			if _, err := r.out.WriteString("\n"); err != nil {
				panic(err)
			}
		}
	}

	// Return to start position (row 0, column 0).
	if belowLines > 1 {
		r.out.CursorUp(belowLines - 1)
	}
	if _, err := r.out.WriteString("\r"); err != nil {
		panic(err)
	}
}

// backward moves cursor to backward from a current cursor position
// regardless there is a line break.
func (r *Renderer) backward(from Position, fromIsFullWidth bool, n istrings.Width) Position {
	if n == 0 {
		return from
	}
	// The hardened move function correctly handles deltaX=0, so this is safe.
	return r.move(from, fromIsFullWidth, Position{X: from.X - n, Y: from.Y}, false)
}

// move moves cursor to specified position from the beginning of input
// even if there is a line break.
func (r *Renderer) move(from Position, fromIsFullWidth bool, to Position, toIsFullWidth bool) Position {
	// Vertical movement: check sign to prevent malformed VT100 sequences
	deltaY := from.Y - to.Y
	switch {
	case deltaY > 0:
		r.out.CursorUp(int(deltaY))
	case deltaY < 0:
		r.out.CursorDown(int(-deltaY))
	}
	// deltaY == 0: no vertical movement needed

	// Horizontal movement: check sign to prevent malformed VT100 sequences
	deltaX := r.physicalColumn(from, fromIsFullWidth) - r.physicalColumn(to, toIsFullWidth)
	switch {
	case deltaX > 0:
		r.out.CursorBackward(int(deltaX))
	case deltaX < 0:
		r.out.CursorForward(int(-deltaX))
	}
	// deltaX == 0: no horizontal movement needed

	return to
}

func (r *Renderer) physicalColumn(pos Position, isFullWidth bool) istrings.Width {
	if isFullWidth && pos.X == r.col && r.col > 0 {
		return r.col - 1
	}
	return pos.X
}

func (r *Renderer) restoreColumn(col istrings.Width) {
	if _, err := r.out.WriteString("\r"); err != nil {
		panic(err)
	}
	if col > 0 {
		r.out.CursorForward(int(col))
	}
}

func clamp(high, low, x float64) float64 {
	switch {
	case high < x:
		return high
	case x < low:
		return low
	default:
		return x
	}
}

func alignNextLine(r *Renderer, col istrings.Width) {
	r.out.CursorDown(1)
	if _, err := r.out.WriteString("\r"); err != nil {
		panic(err)
	}
	if col > 0 {
		r.out.CursorForward(int(col))
	}
}

// ---------------------------------------------------------------------------
// Unified Terminal Reflow Engine
// ---------------------------------------------------------------------------

// ReflowState holds the state of one visual line produced by TerminalReflower.
// ReflowState describes one visual line produced by a TerminalReflower.  Each
// call to TerminalReflower.Next yields one line.  All byte indices are relative
// to the original input string.
//
// Byte layout: between two consecutive ReflowStates, the bytes from the first's
// ByteEnd up to (but not including) the second's ByteStart are always CR and/or
// LF bytes (or the empty string).  The bytes in LineBuffer are never '\n' or
// '\r'.
//
// Width is the accumulated visual width of LineBuffer in terminal columns.  It
// may equal col (indicating the line reached the boundary and triggered a wrap)
// or be less than col (partial line or exact fill with no following content).
// Width may legitimately exceed col when a single character is wider than the
// terminal column (e.g. a CJK character with width 2 in a col=1 viewport).
//
// IsFullWidth is true when Width == col, meaning the next character would have
// wrapped.  This signals to callers that the line is "full" (xenl / wrap-pending).
type ReflowState struct {
	LineBuffer  []byte         // UTF-8 encoded bytes of this visual line
	Width       istrings.Width // Accumulated visual width of this line
	LineNumber  int            // 0-based absolute visual line index
	ByteStart   int            // Explicit start index of this line
	ByteEnd     int            // Explicit end index of this line in the original string before wrap offsets
	ByteIndex   int            // Byte index in input AFTER this line's content; start = ByteIndex - len(LineBuffer)
	IsVisible   bool           // true when LineNumber is within [startLine, endLine)
	IsFullWidth bool           // true if the line reached the column limit (wrap-pending)
}

// TerminalReflower is a precise emulator for VT100/ANSI deferred line wrapping.
// It is the single source of truth for all visual-line calculations in the
// renderer: countInputLines, renderText, and lex all delegate to it.
//
// Wrap semantics (xenl / lookahead):
//   - Explicit '\n': wrap to a new empty line before placing the newline.
//   - Lookahead soft wrap: before adding a character, check
//     currentWidth + charWidth > col.  If true, YIELD the current line FIRST,
//     then start the new line with this character.  A character at exactly the
//     boundary stays on the current line (>`=`).
//   - Zero-width immunity: characters with GetRuneWidth == 0 (combining marks,
//     zero-width spaces) are appended without triggering a wrap, even when
//     currentWidth == col.
//
// These rules match real terminal emulator behavior and the semantics used by
// positionAtEndOfString / positionAtEndOfStringLine.
type TerminalReflower struct {
	text      string
	startLine int // first visible line (inclusive)
	endLine   int // first non-visible line (exclusive)
	col       istrings.Width

	byteIndex    int            // byte position in text during iteration
	lineNumber   int            // absolute 0-based visual line being built
	currentWidth istrings.Width // accumulated width of current line
	lineStart    int            // byte index where the current line started
	exhausted    bool           // true once the input has been fully consumed
	metricsOnly  bool           // bypass line buffer allocation if true
}

// NewTerminalReflower creates a reflower for the given text within a viewport
// spanning [startLine, startLine+rowCount).  col is the terminal column width.
func NewTerminalReflower(text string, startLine int, endLine int, col istrings.Width, metricsOnly bool) *TerminalReflower {
	if endLine < startLine {
		endLine = startLine
	}
	return &TerminalReflower{
		text:        text,
		startLine:   startLine,
		endLine:     endLine,
		col:         col,
		metricsOnly: metricsOnly,
	}
}

func (tr *TerminalReflower) getLineBuffer() []byte {
	if tr.metricsOnly {
		return nil
	}
	return []byte(strings.ToValidUTF8(tr.text[tr.lineStart:tr.byteIndex], "\ufffd"))
}

// Next yields the next visual line.  It returns (state, true) while lines remain,
// or (ReflowState{}, false) when the input is exhausted.
//
// Each call processes input until a visual line boundary (wrap or end-of-input)
// is reached and returns that line.  The caller must consume all yielded lines
// in order; there is no random access.
func (tr *TerminalReflower) Next() (ReflowState, bool) {
	if tr.exhausted {
		return ReflowState{}, false
	}

	// Consume input until a wrap is required or the text ends.
	for tr.byteIndex < len(tr.text) {
		char, runeSize := utf8.DecodeRuneInString(tr.text[tr.byteIndex:])

		// Handle line terminators FIRST: \r\n, \n, and bare \r.
		// Bare \r must be treated as a line terminator to prevent raw
		// carriage returns from leaking into terminal output and
		// overwriting rendered content.
		if char == '\n' || char == '\r' {
			size := runeSize
			if char == '\r' && tr.byteIndex+runeSize < len(tr.text) && tr.text[tr.byteIndex+runeSize] == '\n' {
				size = runeSize + 1 // consume \r\n together
			}
			state := ReflowState{
				LineBuffer:  tr.getLineBuffer(),
				Width:       tr.currentWidth,
				LineNumber:  tr.lineNumber,
				ByteStart:   tr.lineStart,
				ByteEnd:     tr.byteIndex,
				ByteIndex:   tr.byteIndex + size,
				IsVisible:   tr.startLine <= tr.lineNumber && tr.lineNumber < tr.endLine,
				IsFullWidth: tr.currentWidth == tr.col,
			}
			tr.byteIndex += size
			tr.lineNumber++
			tr.currentWidth = 0
			tr.lineStart = tr.byteIndex
			return state, true
		}

		var cluster string
		var charWidth istrings.Width
		var size int

		if char == utf8.RuneError && runeSize == 1 {
			// Invalid byte sequence: Standard Replacement Character strategy
			cluster = "\ufffd"
			charWidth = 1 // \ufffd occupies 1 terminal cell
			size = 1      // Consume the single invalid byte
		} else {
			// Valid unicode: Extract the full grapheme cluster
			var width int
			cluster, _, width, _ = uniseg.FirstGraphemeClusterInString(tr.text[tr.byteIndex:], -1)
			charWidth = istrings.Width(width)
			size = len(cluster)
		}

		_ = cluster // cluster used only for size calculation at this point

		// Lookahead soft wrap: check BEFORE adding the character.
		// Zero-width characters (combining marks) never trigger a wrap.
		// CJK phantom line fix: when the line is empty (currentWidth == 0)
		// and the character is wider than the column, place it on the
		// current line anyway — yielding an empty line before an
		// unfittable character creates infinite phantom lines.
		if charWidth > 0 && tr.currentWidth+charWidth > tr.col && tr.currentWidth > 0 {
			// Yield the current line before wrapping.
			state := ReflowState{
				LineBuffer:  tr.getLineBuffer(),
				Width:       tr.currentWidth,
				LineNumber:  tr.lineNumber,
				ByteStart:   tr.lineStart,
				ByteEnd:     tr.byteIndex,
				ByteIndex:   tr.byteIndex,
				IsVisible:   tr.startLine <= tr.lineNumber && tr.lineNumber < tr.endLine,
				IsFullWidth: tr.currentWidth == tr.col,
			}
			// Start the new line with the overflow character.
			tr.lineNumber++
			tr.currentWidth = charWidth
			tr.lineStart = tr.byteIndex
			tr.byteIndex += size
			return state, true
		}

		// Character fits (or is forced onto an empty line); add it.
		tr.currentWidth += charWidth
		tr.byteIndex += size
	}

	// End of input: yield the final line if it has content, or if we are
	// exactly at the start of a new line (e.g. after a newline, or handling an exact-boundary wrap).
	if tr.byteIndex > tr.lineStart || (tr.currentWidth == 0 && tr.byteIndex == tr.lineStart) {
		tr.exhausted = true
		return ReflowState{
			LineBuffer:  tr.getLineBuffer(),
			Width:       tr.currentWidth,
			LineNumber:  tr.lineNumber,
			ByteStart:   tr.lineStart,
			ByteEnd:     tr.byteIndex,
			ByteIndex:   tr.byteIndex,
			IsVisible:   tr.startLine <= tr.lineNumber && tr.lineNumber < tr.endLine,
			IsFullWidth: tr.currentWidth == tr.col,
		}, true
	}

	tr.exhausted = true
	return ReflowState{}, false
}

// Metrics returns the accumulated state of the reflower after the last Next
// call: the visual width of the last yielded line, its 0-based absolute line
// number, and whether that line exactly filled the column (Width == col).
//
// If Next has not been called, all three return values are zero.
// If Next has been exhausted, Metrics returns the state of the final line.
// This decouples callers (position.go) from unexported field names of
// TerminalReflower.
func (tr *TerminalReflower) Metrics() (width istrings.Width, line int, isFullWidth bool) {
	return tr.currentWidth, tr.lineNumber, tr.currentWidth == tr.col
}

// syncCursor ensures the physical terminal cursor is in a non-ambiguous state.
// If the position is at the right margin (IsFullWidth), it performs a 1-cell
// back-and-forth movement to clear any terminal-specific deferred wrap state.
func (r *Renderer) syncCursor(isFullWidth bool) {
	if isFullWidth {
		// ANSI trick to clear wrap-pending state without risks of clobbering:
		// move left 1 cell and then right 1 cell.
		r.out.CursorBackward(1)
		r.out.CursorForward(1)
	}
}
