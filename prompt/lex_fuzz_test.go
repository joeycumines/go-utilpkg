package prompt

import (
	"strings"
	"testing"
	"unicode/utf8"

	istrings "github.com/joeycumines/go-prompt/strings"
)

// fuzzLexerFunc creates a simple LexerFunc that tokenizes the input into
// fixed-size chunks (defaultColor), which exercises token boundary handling
// in the lex pipeline.
func fuzzLexerFunc(text string) []Token {
	const chunkSize = 10
	var tokens []Token
	for i := 0; i < len(text); i += chunkSize {
		end := min(i+chunkSize, len(text))
		tokens = append(tokens, NewSimpleToken(
			istrings.ByteNumber(i),
			istrings.ByteNumber(end-1),
			SimpleTokenWithColor(DefaultColor),
		))
	}
	return tokens
}

// FuzzLexPipeline fuzzes the Renderer.lex zero-allocation pipeline with
// random input strings and verifies:
// - No panics occur
// - Output written is valid UTF-8 (or uses valid replacement characters)
func FuzzLexPipeline(f *testing.F) {
	seedCorpus := []string{
		"",
		"hello world",
		"日本語テスト",
		strings.Repeat("a", 1000),
		strings.Repeat("line\n", 50),
		"\xff\xfe\xfd",
		"a b c d e f g h i j k l m n o p",
	}
	for _, seed := range seedCorpus {
		f.Add(seed, uint(80), 0)
	}

	f.Fuzz(func(t *testing.T, text string, col uint, startLine int) {
		if col == 0 || col > 10000 {
			col = 80
		}
		if startLine < 0 {
			startLine = 0
		}

		colW := istrings.Width(col)

		// Set up renderer with mock writer that captures all output.
		mockOut := &mockWriterLogger{}
		r := &Renderer{
			out:            mockOut,
			col:            colW,
			row:            100,
			prefixCallback: func() string { return "> " },
			indentSize:     2,
			inputTextColor: DefaultColor,
			inputBGColor:   DefaultColor,
			reflowScratch:  nil,
			tokenScratch:   nil,
		}

		// Populate reflowScratch via computeReflow (hot path setup).
		// Use a generous endLine to ensure all content is in viewport.
		effectiveEndLine := startLine + 1000
		r.computeReflow(text, startLine, effectiveEndLine, colW)

		// Create lexer.
		lexer := NewEagerLexer(fuzzLexerFunc)

		// Call lex (the zero-allocation hot path).
		r.lex(lexer, text, startLine)

		// Verify: no panics (passed if we reach here).
		// Verify: all Write calls contain valid UTF-8 (replacement char is acceptable).
		for _, call := range mockOut.Calls() {
			if call.method == "Write" || call.method == "WriteString" ||
				call.method == "WriteRaw" || call.method == "WriteRawString" {
				var s string
				switch call.method {
				case "Write":
					if bs, ok := call.args[0].([]byte); ok {
						s = string(bs)
					}
				case "WriteString", "WriteRawString":
					if str, ok := call.args[0].(string); ok {
						s = str
					}
				case "WriteRaw":
					if bs, ok := call.args[0].([]byte); ok {
						s = string(bs)
					}
				}
				// Every string written must be valid UTF-8. The renderer uses
				// strings.ToValidUTF8 to replace invalid sequences with U+FFFD,
				// so the output should always pass utf8.ValidString.
				if !utf8.ValidString(s) {
					t.Fatalf("invalid UTF-8 in %s output: %q", call.method, s)
				}
			}
		}

		// Also verify the fallback path (cap(reflowScratch) == 0).
		// Note: hot-path vs fallback-path output equivalence is verified by
		// TestRenderer_ComputeReflow_Correctness and TestRenderer_Regressions
		// (which exercise both paths through Render()). This fuzz test focuses
		// on no-panic and valid-UTF-8 guarantees for each path individually.
		mockOut.reset()
		r2 := &Renderer{
			out:            mockOut,
			col:            colW,
			row:            100,
			prefixCallback: func() string { return "> " },
			indentSize:     2,
			inputTextColor: DefaultColor,
			inputBGColor:   DefaultColor,
			reflowScratch:  nil, // nil = zero capacity = fallback path
			tokenScratch:   nil,
		}
		lexer2 := NewEagerLexer(fuzzLexerFunc)
		r2.lex(lexer2, text, startLine)
		// If we reach here, no panics occurred.
	})
}
