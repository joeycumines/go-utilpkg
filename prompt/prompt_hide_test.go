package prompt

import (
	"testing"

	istrings "github.com/joeycumines/go-prompt/strings"
)

// TestPrompt_HideAfterExecute_Enter tests that completions are hidden on Enter when configured
func TestPrompt_HideAfterExecute_Enter(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		return []Suggest{
			{Text: "test1", Description: "desc1"},
			{Text: "test2", Description: "desc2"},
		}, 0, d.CurrentRuneIndex()
	}

	// Create prompt with hideAfterExecute enabled
	completion := NewCompletionManager(5, CompletionManagerWithCompleter(completer), CompletionManagerWithHideAfterExecute(true))

	p := &Prompt{
		buffer:     NewBuffer(),
		completion: completion,
		renderer: &Renderer{
			out:            &mockWriterLogger{},
			col:            80,
			row:            20,
			prefixCallback: func() string { return "> " },
		},
		history: NewHistory(),
		executeOnEnterCallback: func(prompt *Prompt, indentSize int) (indent int, execute bool) {
			return 0, true // Always execute
		},
	}

	// Update suggestions
	p.completion.Update(*p.buffer.Document())

	// Verify suggestions are available
	if len(p.completion.GetSuggestions()) != 2 {
		t.Error("Expected 2 suggestions")
	}

	// Verify not hidden
	if p.completion.IsHidden() {
		t.Error("Completions should not be hidden initially")
	}

	// Simulate Enter key
	shouldExit, _, userInput := p.feed([]byte{0xa}) // Enter

	// Should not exit
	if shouldExit {
		t.Error("Should not exit on Enter")
	}

	// Should return user input
	if userInput == nil {
		t.Error("Expected user input")
	}

	// Completions should now be hidden
	if !p.completion.IsHidden() {
		t.Error("Completions should be hidden after Enter with hideAfterExecute enabled")
	}
}

// TestPrompt_HideAfterExecute_ControlC tests that completions are hidden on Ctrl+C when configured
func TestPrompt_HideAfterExecute_ControlC(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		return []Suggest{
			{Text: "test1", Description: "desc1"},
			{Text: "test2", Description: "desc2"},
		}, 0, d.CurrentRuneIndex()
	}

	// Create prompt with hideAfterExecute enabled
	completion := NewCompletionManager(5, CompletionManagerWithCompleter(completer), CompletionManagerWithHideAfterExecute(true))

	p := &Prompt{
		buffer:     NewBuffer(),
		completion: completion,
		renderer: &Renderer{
			out:            &mockWriterLogger{},
			col:            80,
			row:            20,
			prefixCallback: func() string { return "> " },
		},
		history: NewHistory(),
		executeOnEnterCallback: func(prompt *Prompt, indentSize int) (indent int, execute bool) {
			return 0, true
		},
	}

	// Add some text
	p.buffer.InsertTextMoveCursor("test", 80, 20, false)

	// Update suggestions
	p.completion.Update(*p.buffer.Document())

	// Verify suggestions are available
	if len(p.completion.GetSuggestions()) != 2 {
		t.Error("Expected 2 suggestions")
	}

	// Verify not hidden
	if p.completion.IsHidden() {
		t.Error("Completions should not be hidden initially")
	}

	// Simulate Ctrl+C
	shouldExit, _, _ := p.feed([]byte{0x3}) // ControlC

	// Should not exit
	if shouldExit {
		t.Error("Should not exit on ControlC")
	}

	// Buffer should be cleared
	if p.buffer.Text() != "" {
		t.Error("Buffer should be cleared after ControlC")
	}

	// Completions should now be hidden
	if !p.completion.IsHidden() {
		t.Error("Completions should be hidden after ControlC with hideAfterExecute enabled")
	}
}

// TestPrompt_HideAfterExecute_RevealOnInput ensures completions reappear on first general input
func TestPrompt_HideAfterExecute_RevealOnInput(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		return []Suggest{
			{Text: "alpha", Description: "desc1"},
			{Text: "beta", Description: "desc2"},
		}, 0, d.CurrentRuneIndex()
	}

	completion := NewCompletionManager(5, CompletionManagerWithCompleter(completer), CompletionManagerWithHideAfterExecute(true))

	p := &Prompt{
		buffer:     NewBuffer(),
		completion: completion,
		renderer: &Renderer{
			out:            &mockWriterLogger{},
			col:            80,
			row:            20,
			prefixCallback: func() string { return "> " },
		},
		history: NewHistory(),
		executeOnEnterCallback: func(prompt *Prompt, indentSize int) (indent int, execute bool) {
			return 0, true
		},
	}

	// Prime suggestions
	p.completion.Update(*p.buffer.Document())

	// Ensure completions visible initially
	if p.completion.IsHidden() {
		t.Fatal("Completions should start visible")
	}

	// Simulate Enter to execute and hide completions
	_, _, userInput := p.feed([]byte{0xa})
	if userInput == nil {
		t.Fatal("Expected user input on execute")
	}
	if !p.completion.IsHidden() {
		t.Fatal("Completions should be hidden after execute")
	}
	if !p.completionHiddenByExecute {
		t.Fatal("completionHiddenByExecute should be true after execute hide")
	}

	// Simulate general input (typing a rune)
	shouldExit, rerender, input := p.feed([]byte("a"))
	if shouldExit {
		t.Fatal("Should not exit when typing")
	}
	if input != nil {
		t.Fatal("Typing should not produce user input")
	}
	if !rerender {
		t.Error("Typing should trigger rerender")
	}

	if p.completion.IsHidden() {
		t.Error("Completions should be visible after typing")
	}
	if p.completionHiddenByExecute {
		t.Error("completionHiddenByExecute should be cleared after typing")
	}
}

// TestPrompt_HideAfterExecute_Disabled tests that completions are not hidden when feature is disabled
func TestPrompt_HideAfterExecute_Disabled(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		return []Suggest{
			{Text: "test1", Description: "desc1"},
			{Text: "test2", Description: "desc2"},
		}, 0, d.CurrentRuneIndex()
	}

	// Create prompt with hideAfterExecute disabled (default)
	completion := NewCompletionManager(5, CompletionManagerWithCompleter(completer))

	p := &Prompt{
		buffer:     NewBuffer(),
		completion: completion,
		renderer: &Renderer{
			out:            &mockWriterLogger{},
			col:            80,
			row:            20,
			prefixCallback: func() string { return "> " },
		},
		history: NewHistory(),
		executeOnEnterCallback: func(prompt *Prompt, indentSize int) (indent int, execute bool) {
			return 0, true
		},
	}

	// Update suggestions
	p.completion.Update(*p.buffer.Document())

	// Verify not hidden
	if p.completion.IsHidden() {
		t.Error("Completions should not be hidden initially")
	}

	// Simulate Enter key
	shouldExit, _, userInput := p.feed([]byte{0xa}) // Enter

	// Should not exit
	if shouldExit {
		t.Error("Should not exit on Enter")
	}

	// Should return user input
	if userInput == nil {
		t.Error("Expected user input")
	}

	// Completions should NOT be hidden (feature disabled)
	if p.completion.IsHidden() {
		t.Error("Completions should not be hidden with hideAfterExecute disabled")
	}
}

// TestPrompt_HideAfterExecute_MultilineNoExecute tests multiline behavior
func TestPrompt_HideAfterExecute_MultilineNoExecute(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		return []Suggest{
			{Text: "test1", Description: "desc1"},
			{Text: "test2", Description: "desc2"},
		}, 0, d.CurrentRuneIndex()
	}

	// Create prompt with hideAfterExecute enabled
	completion := NewCompletionManager(5, CompletionManagerWithCompleter(completer), CompletionManagerWithHideAfterExecute(true))

	p := &Prompt{
		buffer:     NewBuffer(),
		completion: completion,
		renderer: &Renderer{
			out:            &mockWriterLogger{},
			col:            80,
			row:            20,
			prefixCallback: func() string { return "> " },
			indentSize:     4,
		},
		history: NewHistory(),
		executeOnEnterCallback: func(prompt *Prompt, indentSize int) (indent int, execute bool) {
			// Don't execute on Enter (multiline mode)
			return 0, false
		},
	}

	// Update suggestions
	p.completion.Update(*p.buffer.Document())

	// Verify not hidden
	if p.completion.IsHidden() {
		t.Error("Completions should not be hidden initially")
	}

	// Simulate Enter key (which will add newline, not execute)
	shouldExit, rerender, userInput := p.feed([]byte{0xa}) // Enter

	// Should not exit
	if shouldExit {
		t.Error("Should not exit on Enter in multiline mode")
	}

	// Should not return user input (multiline mode)
	if userInput != nil {
		t.Error("Should not return user input in multiline mode")
	}

	// Should rerender
	if !rerender {
		t.Error("Should rerender after newline")
	}

	// Completions should NOT be hidden (no execution in multiline mode)
	if p.completion.IsHidden() {
		t.Error("Completions should not be hidden when Enter doesn't execute (multiline mode)")
	}

	// Buffer should have newline
	if p.buffer.Text() != "\n" {
		t.Errorf("Expected newline in buffer, got %q", p.buffer.Text())
	}
}

// TestPrompt_HideShow_ToggleBehavior tests toggling visibility
func TestPrompt_HideShow_ToggleBehavior(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		return []Suggest{
			{Text: "test1", Description: "desc1"},
			{Text: "test2", Description: "desc2"},
		}, 0, d.CurrentRuneIndex()
	}

	completion := NewCompletionManager(5, CompletionManagerWithCompleter(completer))

	p := &Prompt{
		buffer:     NewBuffer(),
		completion: completion,
		renderer: &Renderer{
			out:            &mockWriterLogger{},
			col:            80,
			row:            20,
			prefixCallback: func() string { return "> " },
		},
	}

	// Update suggestions
	p.completion.Update(*p.buffer.Document())

	// Initially visible
	if p.completion.IsHidden() {
		t.Error("Should be visible initially")
	}

	// Hide
	HideCompletions(p)
	if !p.completion.IsHidden() {
		t.Error("Should be hidden after HideCompletions")
	}

	// Show
	ShowCompletions(p)
	if p.completion.IsHidden() {
		t.Error("Should be visible after ShowCompletions")
	}

	// Hide again
	HideCompletions(p)
	if !p.completion.IsHidden() {
		t.Error("Should be hidden after second HideCompletions")
	}
}

// TestPrompt_HiddenState_PersistsAcrossUpdates tests that hidden state persists
func TestPrompt_HiddenState_PersistsAcrossUpdates(t *testing.T) {
	updateCount := 0
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		updateCount++
		return []Suggest{
			{Text: "test1", Description: "desc1"},
			{Text: "test2", Description: "desc2"},
		}, 0, d.CurrentRuneIndex()
	}

	completion := NewCompletionManager(5, CompletionManagerWithCompleter(completer))

	p := &Prompt{
		buffer:     NewBuffer(),
		completion: completion,
		renderer: &Renderer{
			out:            &mockWriterLogger{},
			col:            80,
			row:            20,
			prefixCallback: func() string { return "> " },
		},
	}

	// Hide completions
	p.completion.Hide()

	// Update multiple times
	p.completion.Update(*p.buffer.Document())
	p.buffer.InsertTextMoveCursor("a", 80, 20, false)
	p.completion.Update(*p.buffer.Document())
	p.buffer.InsertTextMoveCursor("b", 80, 20, false)
	p.completion.Update(*p.buffer.Document())

	// Verify updates happened
	if updateCount != 3 {
		t.Errorf("Expected 3 updates, got %d", updateCount)
	}

	// Verify still hidden
	if !p.completion.IsHidden() {
		t.Error("Should remain hidden after updates")
	}
}

// TestPrompt_EmptySuggestions_WithHidden tests behavior with no suggestions and hidden state
func TestPrompt_EmptySuggestions_WithHidden(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		// No suggestions
		return []Suggest{}, 0, 0
	}

	completion := NewCompletionManager(5, CompletionManagerWithCompleter(completer))

	p := &Prompt{
		buffer:     NewBuffer(),
		completion: completion,
		renderer: &Renderer{
			out:            &mockWriterLogger{},
			col:            80,
			row:            20,
			prefixCallback: func() string { return "> " },
		},
	}

	// Hide completions
	p.completion.Hide()

	// Update with empty suggestions
	p.completion.Update(*p.buffer.Document())

	// Should still be hidden
	if !p.completion.IsHidden() {
		t.Error("Should remain hidden with no suggestions")
	}

	// Show
	p.completion.Show()

	// Should be visible now (even with no suggestions)
	if p.completion.IsHidden() {
		t.Error("Should be visible after Show(), even with no suggestions")
	}
}
