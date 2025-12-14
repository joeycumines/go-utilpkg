package prompt

import (
	"fmt"
	istrings "github.com/joeycumines/go-prompt/strings"
	"testing"
)

// Compare pressing PageDown vs Right arrow after Tab-applied selection
func TestCompletion_TabThen_PageDown_vs_Right(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		idx := d.CurrentRuneIndex()
		start := idx - 1
		if start < 0 {
			start = 0
		}
		return []Suggest{{Text: "alpha"}, {Text: "bravo"}}, start, idx
	}

	for _, key := range []Key{PageDown, Right} {
		p := &Prompt{
			buffer:     NewBuffer(),
			completion: NewCompletionManager(1, CompletionManagerWithCompleter(completer)),
			renderer: &Renderer{
				out:            &mockWriterLogger{},
				col:            80,
				row:            20,
				prefixCallback: func() string { return "> " },
				indentSize:     2,
			},
			history: NewHistory(),
			executeOnEnterCallback: func(prompt *Prompt, indentSize int) (int, bool) {
				return 0, true
			},
		}

		p.buffer.InsertTextMoveCursor("add a", 80, 20, false)
		p.completion.Update(*p.buffer.Document())

		// PageDown -> Tab -> (either PageDown or Right)
		if shouldExit, _, _ := p.feed(findASCIICode(PageDown)); shouldExit {
			t.Fatal("unexpected exit on PageDown")
		}
		if shouldExit, _, _ := p.feed(findASCIICode(Tab)); shouldExit {
			t.Fatal("unexpected exit on Tab")
		}

		if shouldExit, _, _ := p.feed(findASCIICode(key)); shouldExit {
			t.Fatalf("unexpected exit on key %v", key)
		}

		t.Logf("key %v: buffer=%q selected=%d applied=%t", key, p.buffer.Text(), p.completion.selected, p.completionSelectionApplied)

	}
}

// Test to verify that PageDown then Tab replaces only the token range, preserving prefix
func TestCompletion_PageDownThenTab_ReplacesTokenPreservingPrefix(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		// Replace the last character only
		idx := d.CurrentRuneIndex()
		// Replace the previous rune only (a single partial word 'a')
		start := idx - 1
		if start < 0 {
			start = 0
		}
		return []Suggest{{Text: "alpha"}, {Text: "bravo"}}, start, idx
	}

	p := &Prompt{
		buffer:     NewBuffer(),
		completion: NewCompletionManager(1, CompletionManagerWithCompleter(completer)),
		renderer: &Renderer{
			out:            &mockWriterLogger{},
			col:            80,
			row:            20,
			prefixCallback: func() string { return "> " },
			indentSize:     2,
		},
		history: NewHistory(),
		executeOnEnterCallback: func(prompt *Prompt, indentSize int) (int, bool) {
			return 0, true
		},
	}

	// Seed input with prefix and a partial token to be replaced.
	p.buffer.InsertTextMoveCursor("add a", 80, 20, false)
	p.completion.Update(*p.buffer.Document())

	pageDown := findASCIICode(PageDown)
	if pageDown == nil {
		t.Fatal("PageDown ASCII sequence not found")
	}
	if shouldExit, _, _ := p.feed(pageDown); shouldExit {
		t.Fatal("unexpected exit on PageDown")
	}

	// Confirming via Tab should replace only the 'a' and keep the prefix 'add '
	tab := findASCIICode(Tab)
	if tab == nil {
		t.Fatal("Tab ASCII sequence not found")
	}
	if shouldExit, _, _ := p.feed(tab); shouldExit {
		t.Fatal("unexpected exit on Tab")
	}

	if got, want := p.buffer.Text(), "add bravo"; got != want {
		t.Fatalf("completion applied incorrectly: got %q, want %q", got, want)
	}
}

// Regression: pressing Tab to apply a completion then navigating PageDown should preserve prefix
func TestCompletion_TabThenPageDown_PreservesPrefix(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		idx := d.CurrentRuneIndex()
		start := idx - 1
		if start < 0 {
			start = 0
		}
		return []Suggest{{Text: "alpha"}, {Text: "bravo"}}, start, idx
	}

	p := &Prompt{
		buffer:     NewBuffer(),
		completion: NewCompletionManager(1, CompletionManagerWithCompleter(completer)),
		renderer: &Renderer{
			out:            &mockWriterLogger{},
			col:            80,
			row:            20,
			prefixCallback: func() string { return "> " },
			indentSize:     2,
		},
		history: NewHistory(),
		executeOnEnterCallback: func(prompt *Prompt, indentSize int) (int, bool) {
			return 0, true
		},
	}

	p.buffer.InsertTextMoveCursor("add a", 80, 20, false)
	p.completion.Update(*p.buffer.Document())

	// Apply first suggestion via Tab
	tab := findASCIICode(Tab)
	if tab == nil {
		t.Fatal("Tab ASCII sequence not found")
	}
	if shouldExit, _, _ := p.feed(tab); shouldExit {
		t.Fatal("unexpected exit on Tab")
	}

	if got, want := p.buffer.Text(), "add alpha"; got != want {
		t.Fatalf("after first Tab: got %q, want %q", got, want)
	}

	// PageDown should move selection to next suggestion and update buffer
	pageDown := findASCIICode(PageDown)
	if pageDown == nil {
		t.Fatal("PageDown ASCII sequence not found")
	}
	if shouldExit, _, _ := p.feed(pageDown); shouldExit {
		t.Fatal("unexpected exit on PageDown")
	}

	if got, want := p.buffer.Text(), "add bravo"; got != want {
		t.Fatalf("after PageDown: got %q, want %q", got, want)
	}
}

// Regression: pressing Tab to apply a completion then navigating PageUp should preserve prefix
func TestCompletion_TabThenPageUp_PreservesPrefix(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		idx := d.CurrentRuneIndex()
		start := idx - 1
		if start < 0 {
			start = 0
		}
		return []Suggest{{Text: "alpha"}, {Text: "bravo"}}, start, idx
	}

	p := &Prompt{
		buffer:     NewBuffer(),
		completion: NewCompletionManager(1, CompletionManagerWithCompleter(completer)),
		renderer: &Renderer{
			out:            &mockWriterLogger{},
			col:            80,
			row:            20,
			prefixCallback: func() string { return "> " },
			indentSize:     2,
		},
		history: NewHistory(),
		executeOnEnterCallback: func(prompt *Prompt, indentSize int) (int, bool) {
			return 0, true
		},
	}

	p.buffer.InsertTextMoveCursor("add a", 80, 20, false)
	p.completion.Update(*p.buffer.Document())

	// Apply first suggestion via Tab
	tab := findASCIICode(Tab)
	if tab == nil {
		t.Fatal("Tab ASCII sequence not found")
	}
	if shouldExit, _, _ := p.feed(tab); shouldExit {
		t.Fatal("unexpected exit on Tab")
	}

	if got, want := p.buffer.Text(), "add alpha"; got != want {
		t.Fatalf("after first Tab: got %q, want %q", got, want)
	}

	// PageUp should move selection to previous suggestion and update buffer
	pageUp := findASCIICode(PageUp)
	if pageUp == nil {
		t.Fatal("PageUp ASCII sequence not found")
	}
	if shouldExit, _, _ := p.feed(pageUp); shouldExit {
		t.Fatal("unexpected exit on PageUp")
	}

	if got, want := p.buffer.Text(), "add alpha"; got != want {
		t.Fatalf("after PageUp: got %q, want %q", got, want)
	}
}

// Reproducer for PageDown -> Tab -> PageDown behavior
func TestCompletion_PageDown_Tab_Then_PageDown_Behavior(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		idx := d.CurrentRuneIndex()
		start := idx - 1
		if start < 0 {
			start = 0
		}
		return []Suggest{{Text: "alpha"}, {Text: "bravo"}}, start, idx
	}

	p := &Prompt{
		buffer:     NewBuffer(),
		completion: NewCompletionManager(1, CompletionManagerWithCompleter(completer)),
		renderer: &Renderer{
			out:            &mockWriterLogger{},
			col:            80,
			row:            20,
			prefixCallback: func() string { return "> " },
			indentSize:     2,
		},
		history: NewHistory(),
		executeOnEnterCallback: func(prompt *Prompt, indentSize int) (int, bool) {
			return 0, true
		},
	}

	// Seed input with prefix and a partial token to be replaced.
	p.buffer.InsertTextMoveCursor("add a", 80, 20, false)
	p.completion.Update(*p.buffer.Document())

	pageDown := findASCIICode(PageDown)
	tab := findASCIICode(Tab)
	if pageDown == nil || tab == nil {
		t.Fatal("PageDown/Tab ascii not found")
	}

	// PageDown
	if shouldExit, rerender, _ := p.feed(pageDown); shouldExit {
		t.Fatal("unexpected exit on PageDown")
	} else {
		t.Logf("after PageDown: buffer=%q, selected=%d, applied=%t, start=%d end=%d, rerender=%t", p.buffer.Text(), p.completion.selected, p.completionSelectionApplied, p.completion.startCharIndex, p.completion.endCharIndex, rerender)
	}

	// Tab
	if shouldExit, rerender, _ := p.feed(tab); shouldExit {
		t.Fatal("unexpected exit on Tab")
	} else {
		t.Logf("after Tab: buffer=%q, selected=%d, applied=%t, start=%d end=%d, rerender=%t", p.buffer.Text(), p.completion.selected, p.completionSelectionApplied, p.completion.startCharIndex, p.completion.endCharIndex, rerender)
	}

	// PageDown again
	if shouldExit, rerender, _ := p.feed(pageDown); shouldExit {
		t.Fatal("unexpected exit on PageDown")
	} else {
		t.Logf("after PageDown 2: buffer=%q, selected=%d, applied=%t, start=%d end=%d, rerender=%t", p.buffer.Text(), p.completion.selected, p.completionSelectionApplied, p.completion.startCharIndex, p.completion.endCharIndex, rerender)
	}
}

func TestCompletion_PageDown_Tab_Then_PageUp_Behavior(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		idx := d.CurrentRuneIndex()
		start := idx - 1
		if start < 0 {
			start = 0
		}
		return []Suggest{{Text: "alpha"}, {Text: "bravo"}}, start, idx
	}

	p := &Prompt{
		buffer:     NewBuffer(),
		completion: NewCompletionManager(1, CompletionManagerWithCompleter(completer)),
		renderer: &Renderer{
			out:            &mockWriterLogger{},
			col:            80,
			row:            20,
			prefixCallback: func() string { return "> " },
			indentSize:     2,
		},
		history: NewHistory(),
		executeOnEnterCallback: func(prompt *Prompt, indentSize int) (int, bool) {
			return 0, true
		},
	}

	p.buffer.InsertTextMoveCursor("add a", 80, 20, false)
	p.completion.Update(*p.buffer.Document())

	// PageDown -> Tab -> PageUp
	if shouldExit, _, _ := p.feed(findASCIICode(PageDown)); shouldExit {
		t.Fatal("unexpected exit on PageDown")
	}
	if shouldExit, _, _ := p.feed(findASCIICode(Tab)); shouldExit {
		t.Fatal("unexpected exit on Tab")
	}
	if shouldExit, _, _ := p.feed(findASCIICode(PageUp)); shouldExit {
		t.Fatal("unexpected exit on PageUp")
	}

	t.Logf("after PageUp: buffer=%q selected=%d applied=%t", p.buffer.Text(), p.completion.selected, p.completionSelectionApplied)
}

// Reproducer for the PageUp visibility/edge behavior described in issue.
func TestCompletion_PageUp_PreservesPreviousSelectionVisibility(t *testing.T) {
	// Build a long list of suggestions (indexable)
	suggestions := []string{}
	for i := 0; i < 50; i++ {
		suggestions = append(suggestions, fmt.Sprintf("item%02d", i))
	}

	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		idx := d.CurrentRuneIndex()
		start := idx
		if start < 0 {
			start = 0
		}
		s := make([]Suggest, len(suggestions))
		for i := range suggestions {
			s[i] = Suggest{Text: suggestions[i]}
		}
		return s, 0, 0
	}

	p := &Prompt{
		buffer:                 NewBuffer(),
		completion:             NewCompletionManager(10, CompletionManagerWithCompleter(completer)),
		renderer:               &Renderer{out: &mockWriterLogger{}, col: 80, row: 12, prefixCallback: func() string { return "> " }, indentSize: 2},
		history:                NewHistory(),
		executeOnEnterCallback: func(prompt *Prompt, indentSize int) (int, bool) { return 0, true },
	}

	p.completion.Update(*p.buffer.Document())

	pageUp := findASCIICode(PageUp)
	backTab := findASCIICode(BackTab)
	if pageUp == nil || backTab == nil {
		t.Fatal("required control sequences not found")
	}

	// Step 1: PageUp from deselected -> should select last page, last item
	if shouldExit, _, _ := p.feed(pageUp); shouldExit {
		t.Fatal("unexpected exit on PageUp")
	}

	// Step 2: PageUp again -> snap to top of last page
	if shouldExit, _, _ := p.feed(pageUp); shouldExit {
		t.Fatal("unexpected exit on PageUp")
	}

	// Step 3: BackTab -> move up one item (and scroll if needed)
	if shouldExit, _, _ := p.feed(backTab); shouldExit {
		t.Fatal("unexpected exit on BackTab")
	}

	prevSelected := p.completion.selected

	// Step 4: PageUp once more -> after this, the previous selection should be
	// visible in the viewport (and should be positioned as the last item on
	// the page relative to the direction (up) when possible).
	if shouldExit, _, _ := p.feed(pageUp); shouldExit {
		t.Fatal("unexpected exit on PageUp")
	}

	pageHeight := p.completion.effectivePageHeight()
	if pageHeight <= 0 {
		t.Fatal("invalid page height")
	}

	// Check if prevSelected is within the current visible window [verticalScroll, verticalScroll + pageHeight)
	if !(prevSelected >= p.completion.verticalScroll && prevSelected < p.completion.verticalScroll+pageHeight) {
		t.Fatalf("previously-selected item %d not visible after PageUp: scroll=%d height=%d", prevSelected, p.completion.verticalScroll, pageHeight)
	}
}
