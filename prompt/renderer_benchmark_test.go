package prompt

import (
	"strings"
	"testing"

	istrings "github.com/joeycumines/go-prompt/strings"
)

// BenchmarkRenderText_HotPath measures renderText performance using the hot path
// (pre-populated reflowScratch from computeReflow).
func BenchmarkRenderText_HotPath(b *testing.B) {
	b.ReportAllocs()
	mockOut := newMockWriter()
	r := &Renderer{
		out:            mockOut,
		col:            istrings.Width(80),
		row:            24,
		prefixCallback: func() string { return "> " },
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
		reflowScratch:  make([]ReflowState, 0, 100),
		tokenScratch:   nil,
	}
	text := strings.Repeat("hello world line \n", 100)
	lexer := NewEagerLexer(func(_ string) []Token { return nil })

	// Pre-populate reflowScratch via computeReflow (mimics what Render does).
	r.computeReflow(text, 0, 1000, istrings.Width(80))

	b.ResetTimer()
	for b.Loop() {
		r.renderText(lexer, text, 0)
	}
}

// BenchmarkRenderText_FallbackPath measures renderText when reflowScratch has
// zero capacity, forcing the fallback reflower-per-call path.
func BenchmarkRenderText_FallbackPath(b *testing.B) {
	b.ReportAllocs()
	mockOut := newMockWriter()
	r := &Renderer{
		out:            mockOut,
		col:            istrings.Width(80),
		row:            24,
		prefixCallback: func() string { return "> " },
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
		reflowScratch:  nil, // nil = zero capacity = fallback path
		tokenScratch:   nil,
	}
	text := strings.Repeat("hello world line \n", 100)
	lexer := NewEagerLexer(func(_ string) []Token { return nil })

	b.ResetTimer()
	for b.Loop() {
		r.renderText(lexer, text, 0)
	}
}

// BenchmarkLex_HotPath measures lex performance using the hot path
// (pre-populated reflowScratch + pooled tokenScratch).
func BenchmarkLex_HotPath(b *testing.B) {
	b.ReportAllocs()
	mockOut := newMockWriter()
	r := &Renderer{
		out:            mockOut,
		col:            istrings.Width(80),
		row:            24,
		prefixCallback: func() string { return "> " },
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
		reflowScratch:  make([]ReflowState, 0, 200),
		tokenScratch:   make([]Token, 0, 50),
	}
	text := strings.Repeat("some_token  ", 500)
	lexer := NewEagerLexer(func(input string) []Token {
		var tokens []Token
		for i := 0; i < len(input); i += 10 {
			end := min(i+10, len(input))
			tokens = append(tokens, NewSimpleToken(
				istrings.ByteNumber(i),
				istrings.ByteNumber(end-1),
				SimpleTokenWithColor(DefaultColor),
			))
		}
		return tokens
	})

	// Pre-populate reflowScratch.
	r.computeReflow(text, 0, 1000, istrings.Width(80))

	b.ResetTimer()
	for b.Loop() {
		r.lex(lexer, text, 0)
	}
}

// BenchmarkLex_FallbackPath measures lex when reflowScratch has zero capacity.
func BenchmarkLex_FallbackPath(b *testing.B) {
	b.ReportAllocs()
	mockOut := newMockWriter()
	r := &Renderer{
		out:            mockOut,
		col:            istrings.Width(80),
		row:            24,
		prefixCallback: func() string { return "> " },
		inputTextColor: DefaultColor,
		inputBGColor:   DefaultColor,
		reflowScratch:  nil,
		tokenScratch:   nil,
	}
	text := strings.Repeat("some_token  ", 500)
	lexer := NewEagerLexer(func(input string) []Token {
		var tokens []Token
		for i := 0; i < len(input); i += 10 {
			end := min(i+10, len(input))
			tokens = append(tokens, NewSimpleToken(
				istrings.ByteNumber(i),
				istrings.ByteNumber(end-1),
				SimpleTokenWithColor(DefaultColor),
			))
		}
		return tokens
	})

	b.ResetTimer()
	for b.Loop() {
		r.lex(lexer, text, 0)
	}
}

// BenchmarkTerminalReflower_CJK measures TerminalReflower with wide CJK characters.
func BenchmarkTerminalReflower_CJK(b *testing.B) {
	b.ReportAllocs()
	text := strings.Repeat("日本語テスト日本語", 100) // ~30 bytes per rep, width=18
	b.ResetTimer()
	for b.Loop() {
		tr := NewTerminalReflower(text, 0, 1000, istrings.Width(40), false)
		for {
			_, ok := tr.Next()
			if !ok {
				break
			}
		}
	}
}

// BenchmarkTerminalReflower_CombiningMarks measures reflower with combining marks
// (zero-width characters that should not trigger wrapping).
func BenchmarkTerminalReflower_CombiningMarks(b *testing.B) {
	b.ReportAllocs()
	// "e\u0301" = é (e + combining acute accent). Each char width=1.
	// Repeated with zero-width joiners: "e\u0301\u200d" (e + combining + ZWJ)
	text := strings.Repeat("e\u0301\u0300\u200d", 200)
	b.ResetTimer()
	for b.Loop() {
		tr := NewTerminalReflower(text, 0, 1000, istrings.Width(80), false)
		for {
			_, ok := tr.Next()
			if !ok {
				break
			}
		}
	}
}
