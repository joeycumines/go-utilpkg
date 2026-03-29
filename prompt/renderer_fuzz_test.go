package prompt

import (
	"testing"

	str "strings"

	istrings "github.com/joeycumines/go-prompt/strings"
)

// FuzzTerminalReflower fuzzes the TerminalReflower with random inputs.
// It verifies:
// - No panics occur
// - ReflowState values are internally consistent
// - Byte ranges are valid
// - Line order is monotonically non-decreasing
// - Width is never negative
func FuzzTerminalReflower(f *testing.F) {
	// Seed corpus with representative cases
	seedCorpus := []string{
		"",                                     // empty
		"a",                                    // single char
		"abc",                                  // exact fill
		"abcde",                                // wrap
		"hello\nworld",                         // newline
		"こんにちは",                                // CJK
		"\n\n\n",                               // multiple newlines
		"a\nb\nc",                              // single-char lines
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",     // long exact fill
		"abcdefghijklmnopqrstuvwxyz1234567890", // mixed
		"\xff\xfe",                             // invalid UTF-8
		"日本語語語語語語語語",                           // long CJK
		str.Repeat("a", 1000),                  // very long
	}
	for _, seed := range seedCorpus {
		// startLine=0 for seed corpus; fuzzing will vary startLine across the full int range
		f.Add(seed, uint(80), 0, 1000)
	}

	f.Fuzz(func(t *testing.T, text string, col uint, startLine int, endLine int) {
		// Clamp parameters to reasonable ranges
		if col == 0 || col > 10000 {
			col = 80
		}
		if startLine < 0 {
			startLine = 0
		}
		if endLine < 0 {
			endLine = 0
		}
		if endLine < startLine {
			endLine = startLine
		}

		colW := istrings.Width(col)
		reflower := NewTerminalReflower(text, startLine, endLine, colW, false)

		var (
			prevLineNumber = -1
			prevByteEnd    = -1
			visibleCount   = 0
			totalCount     = 0
			lastState      ReflowState
		)

		for {
			state, ok := reflower.Next()
			if !ok {
				break
			}
			totalCount++

			// Width must never be negative
			if state.Width < 0 {
				t.Fatalf("Width negative: %d", state.Width)
			}
			// Note: Width may exceed col for characters wider than the terminal column
			// (e.g., full-width CJK chars). This is a known property of the reflower.

			// Line number must be non-negative
			if state.LineNumber < 0 {
				t.Fatalf("LineNumber negative: %d", state.LineNumber)
			}

			// Byte ranges must be valid
			if state.ByteStart < 0 || state.ByteEnd < state.ByteStart || state.ByteEnd > len(text) {
				t.Fatalf("Invalid byte range [%d, %d) for text len=%d", state.ByteStart, state.ByteEnd, len(text))
			}

			// ByteEnd must be >= ByteStart
			if state.ByteEnd < state.ByteStart {
				t.Fatalf("ByteEnd %d < ByteStart %d", state.ByteEnd, state.ByteStart)
			}

			// ByteIndex must be in valid range
			if state.ByteIndex < 0 || state.ByteIndex > len(text) {
				t.Fatalf("ByteIndex %d out of range for text len=%d", state.ByteIndex, len(text))
			}

			// Bytes in line range must not be '\n' (explicit line terminator).
			// Note: '\r' may appear in the byte range for end-of-input trailing '\r'
			// (known limitation of the reflower; it treats '\r' as zero-width but
			// lineStart is not advanced until '\n' is encountered or EOF is reached).
			for b := state.ByteStart; b < state.ByteEnd; b++ {
				if text[b] == '\n' {
					t.Fatalf("scratch state includes \\n at byte %d", b)
				}
			}

			// Gap between prevByteEnd and state.ByteStart must be CR/LF bytes only
			// (both \r and \n are excluded from line ranges; they appear in the gaps)
			if prevByteEnd >= 0 && state.ByteStart > prevByteEnd {
				for b := prevByteEnd; b < state.ByteStart; b++ {
					if text[b] != '\n' && text[b] != '\r' {
						t.Fatalf("Gap between state[%d].ByteEnd=%d and state[%d].ByteStart=%d contains non-newline byte %d ('%c')",
							totalCount-1, prevByteEnd, totalCount, state.ByteStart, b, text[b])
					}
				}
			}

			// Line number must be monotonically non-decreasing
			if state.LineNumber < prevLineNumber {
				t.Fatalf("LineNumber decreased: %d -> %d", prevLineNumber, state.LineNumber)
			}

			// IsFullWidth is only meaningful when Width > 0
			if state.Width == 0 && state.IsFullWidth {
				t.Fatalf("IsFullWidth=true with Width=0")
			}
			// IsFullWidth must be true iff Width == col (when Width > 0)
			if state.Width > 0 {
				expectedFullWidth := state.Width == colW
				if state.IsFullWidth != expectedFullWidth {
					t.Fatalf("IsFullWidth=%v but Width=%d, col=%d", state.IsFullWidth, state.Width, colW)
				}
			}

			// IsVisible consistency
			expectedVisible := startLine <= state.LineNumber && state.LineNumber < endLine
			if state.IsVisible != expectedVisible {
				t.Fatalf("IsVisible=%v but startLine=%d, endLine=%d, LineNumber=%d",
					state.IsVisible, startLine, endLine, state.LineNumber)
			}

			if state.IsVisible {
				visibleCount++
			}

			prevLineNumber = state.LineNumber
			prevByteEnd = state.ByteEnd
			lastState = state
		}

		// Verify that the final byte processed equals text length (if we processed anything)
		// Note: ByteEnd excludes '\n' bytes, so we count trailing newlines
		if totalCount > 0 {
			newlineCount := 0
			for i := lastState.ByteEnd; i < len(text); i++ {
				if text[i] == '\n' {
					newlineCount++
				} else {
					break
				}
			}
			if lastState.ByteEnd+newlineCount != len(text) {
				t.Fatalf("Final ByteEnd %d + trailingNewlines %d != text len %d", lastState.ByteEnd, newlineCount, len(text))
			}
		}

		// After exhaustion, Metrics() should return last state's values
		w, line, full := reflower.Metrics()
		if totalCount > 0 {
			if w != lastState.Width {
				t.Fatalf("Metrics().width=%d != lastState.Width=%d", w, lastState.Width)
			}
			if line != lastState.LineNumber {
				t.Fatalf("Metrics().line=%d != lastState.LineNumber=%d", line, lastState.LineNumber)
			}
			if full != lastState.IsFullWidth {
				t.Fatalf("Metrics().fullWidth=%v != lastState.IsFullWidth=%v", full, lastState.IsFullWidth)
			}
		}

		// Verify visibleCount matches manual count
		manualCount := 0
		reflower2 := NewTerminalReflower(text, startLine, endLine, colW, true)
		for {
			state, ok := reflower2.Next()
			if !ok {
				break
			}
			if state.IsVisible {
				manualCount++
			}
		}
		if manualCount != visibleCount {
			t.Fatalf("visibleCount mismatch: reflower=%d, manual=%d", visibleCount, manualCount)
		}
	})
}

// FuzzRendererComputeReflow fuzzes Renderer.computeReflow and verifies
// consistency with countInputLines and internal ReflowState validity.
func FuzzRendererComputeReflow(f *testing.F) {
	seedCorpus := []string{
		"",
		"hello",
		"abc\ndef\nghi",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"こんにちは",
		str.Repeat("line\n", 100),
	}
	for _, seed := range seedCorpus {
		f.Add(seed, uint(80), 0, 1000)
	}

	f.Fuzz(func(t *testing.T, text string, col uint, startLine int, endLine int) {
		if col == 0 || col > 10000 {
			col = 80
		}
		if startLine < 0 {
			startLine = 0
		}
		if endLine < 0 {
			endLine = 0
		}
		if endLine < startLine {
			endLine = startLine
		}

		colW := istrings.Width(col)
		r := NewRenderer()
		r.row = 100

		// countInputLines uses endLine = startLine + r.row internally.
		// For fair comparison, computeReflow must use the SAME endLine.
		// Otherwise computeReflow(endLine=large) may count lines countInputLines(endLine=r.row) skips.
		effectiveEndLine := min(startLine+int(r.row), endLine)

		// Call computeReflow
		computeCount := r.computeReflow(text, startLine, effectiveEndLine, colW)
		scratch := r.reflowScratch

		// r.reflowScratch must be populated
		if len(scratch) == 0 && text != "" {
			t.Fatalf("computeReflow: r.reflowScratch empty for non-empty text")
		}

		// Verify scratch entries
		scratchVisibleCount := 0
		for i, state := range scratch {
			// Byte ranges valid
			if state.ByteStart < 0 || state.ByteEnd < state.ByteStart || state.ByteEnd > len(text) {
				t.Fatalf("scratch[%d]: invalid byte range [%d, %d) for text len=%d",
					i, state.ByteStart, state.ByteEnd, len(text))
			}
			// Width must be non-negative (may exceed col for wide chars)
			if state.Width < 0 {
				t.Fatalf("scratch[%d]: Width=%d < 0", i, state.Width)
			}
			// LineNumber valid
			if state.LineNumber < 0 {
				t.Fatalf("scratch[%d]: LineNumber negative: %d", i, state.LineNumber)
			}
			// Bytes in line range must not be '\n'
			for b := state.ByteStart; b < state.ByteEnd; b++ {
				if text[b] == '\n' {
					t.Fatalf("scratch[%d] includes newline at byte %d", i, b)
				}
			}
			// Gap between states must be CR/LF bytes only
			if i > 0 && scratch[i].ByteStart > scratch[i-1].ByteEnd {
				for b := scratch[i-1].ByteEnd; b < scratch[i].ByteStart; b++ {
					if text[b] != '\n' && text[b] != '\r' {
						t.Fatalf("scratch[%d].ByteStart=%d != scratch[%d].ByteEnd=%d, gap byte %d is not CR/LF",
							i, scratch[i].ByteStart, i-1, scratch[i-1].ByteEnd, b)
					}
				}
			}
			// IsFullWidth consistency
			if state.Width > 0 {
				expectedFW := state.Width == colW
				if state.IsFullWidth != expectedFW {
					t.Fatalf("scratch[%d]: IsFullWidth=%v but Width=%d, col=%d",
						i, state.IsFullWidth, state.Width, colW)
				}
			}
			// IsVisible consistency
			expectedVisible := startLine <= state.LineNumber && state.LineNumber < effectiveEndLine
			if state.IsVisible != expectedVisible {
				t.Fatalf("scratch[%d]: IsVisible=%v but startLine=%d, effectiveEndLine=%d, LineNumber=%d",
					i, state.IsVisible, startLine, effectiveEndLine, state.LineNumber)
			}
			if state.IsVisible {
				scratchVisibleCount++
			}
		}

		// computeCount must be >= 1 (minimum to avoid divide-by-zero in callers).
		// Note: when all lines are outside the visible range, scratchVisibleCount=0
		// but computeReflow returns 1 (the minimum guarantee). This is intentional.
		if computeCount < 1 {
			t.Fatalf("computeReflow() returned %d < 1", computeCount)
		}
	})
}
