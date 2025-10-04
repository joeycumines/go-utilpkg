package prompt

import (
	"testing"

	istrings "github.com/joeycumines/go-prompt/strings"
)

func TestCompletionManager_Hide(t *testing.T) {
	c := NewCompletionManager(5)
	c.tmp = []Suggest{
		{Text: "test1", Description: "desc1"},
		{Text: "test2", Description: "desc2"},
	}

	if c.IsHidden() {
		t.Error("CompletionManager should not be hidden initially")
	}

	c.Hide()

	if !c.IsHidden() {
		t.Error("CompletionManager should be hidden after Hide()")
	}

	// Verify that suggestions are still available
	if len(c.GetSuggestions()) != 2 {
		t.Error("Hide should not affect suggestions")
	}
}

func TestCompletionManager_Show(t *testing.T) {
	c := NewCompletionManager(5)
	c.tmp = []Suggest{
		{Text: "test1", Description: "desc1"},
		{Text: "test2", Description: "desc2"},
	}

	c.Hide()
	if !c.IsHidden() {
		t.Error("CompletionManager should be hidden")
	}

	c.Show()
	if c.IsHidden() {
		t.Error("CompletionManager should be visible after Show()")
	}
}

func TestCompletionManager_IsHidden(t *testing.T) {
	c := NewCompletionManager(5)

	// Test initial state
	if c.IsHidden() {
		t.Error("CompletionManager should not be hidden initially")
	}

	// Test after hiding
	c.Hide()
	if !c.IsHidden() {
		t.Error("IsHidden should return true after Hide()")
	}

	// Test after showing
	c.Show()
	if c.IsHidden() {
		t.Error("IsHidden should return false after Show()")
	}
}

func TestCompletionManager_HideAfterExecute(t *testing.T) {
	// Test default value
	c := NewCompletionManager(5)
	if c.ShouldHideAfterExecute() {
		t.Error("hideAfterExecute should be false by default")
	}

	// Test setting to true
	c.HideAfterExecute(true)
	if !c.ShouldHideAfterExecute() {
		t.Error("ShouldHideAfterExecute should return true after HideAfterExecute(true)")
	}

	// Test setting to false
	c.HideAfterExecute(false)
	if c.ShouldHideAfterExecute() {
		t.Error("ShouldHideAfterExecute should return false after HideAfterExecute(false)")
	}
}

func TestCompletionManagerHideAfterExecute_ManualConfiguration(t *testing.T) {
	// Test enabling via method after construction
	c := NewCompletionManager(5)
	c.HideAfterExecute(true)
	if !c.ShouldHideAfterExecute() {
		t.Error("CompletionManager configured with HideAfterExecute(true) should have hideAfterExecute=true")
	}

	// Test disabling via method after construction
	c.HideAfterExecute(false)
	if c.ShouldHideAfterExecute() {
		t.Error("CompletionManager configured with HideAfterExecute(false) should have hideAfterExecute=false")
	}
}

func TestCompletionManager_HideWithSelection(t *testing.T) {
	c := NewCompletionManager(5)
	c.tmp = []Suggest{
		{Text: "test1", Description: "desc1"},
		{Text: "test2", Description: "desc2"},
		{Text: "test3", Description: "desc3"},
	}

	// Select an item
	c.selected = 1

	// Hide completions
	c.Hide()

	// Verify selection is preserved
	if !c.Completing() {
		t.Error("Completing() should still return true when hidden with selection")
	}

	suggestion, ok := c.GetSelectedSuggestion()
	if !ok {
		t.Error("GetSelectedSuggestion should still work when hidden")
	}
	if suggestion.Text != "test2" {
		t.Errorf("Expected selected suggestion to be 'test2', got '%s'", suggestion.Text)
	}

	// Show again
	c.Show()

	// Verify selection is still there
	if !c.Completing() {
		t.Error("Completing() should return true after showing")
	}

	suggestion, ok = c.GetSelectedSuggestion()
	if !ok {
		t.Error("GetSelectedSuggestion should work after showing")
	}
	if suggestion.Text != "test2" {
		t.Errorf("Expected selected suggestion to be 'test2', got '%s'", suggestion.Text)
	}
}

func TestCompletionManager_ResetDoesNotClearHidden(t *testing.T) {
	c := NewCompletionManager(5, CompletionManagerWithCompleter(NoopCompleter))
	c.tmp = []Suggest{
		{Text: "test1", Description: "desc1"},
	}

	// Hide completions
	c.Hide()
	if !c.IsHidden() {
		t.Error("CompletionManager should be hidden")
	}

	// Reset should not clear the hidden state
	// (User explicitly hid it, so it should stay hidden)
	c.Reset()

	// However, after reset, suggestions should be empty
	if len(c.GetSuggestions()) != 0 {
		t.Error("Reset should clear suggestions")
	}

	// Hidden state should be preserved
	if !c.IsHidden() {
		t.Error("Reset should not change hidden state")
	}
}

func TestRenderer_renderCompletion_Hidden(t *testing.T) {
	// Setup renderer with mock writer
	renderer := &Renderer{
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

	buffer := NewBuffer()
	completion := NewCompletionManager(5)
	completion.tmp = []Suggest{
		{Text: "test1", Description: "desc1"},
		{Text: "test2", Description: "desc2"},
	}

	mockOut := renderer.out.(*mockWriterLogger)

	// Test 1: Render when visible (should render)
	mockOut.reset()
	renderer.renderCompletion(buffer, completion)
	if len(mockOut.calls) == 0 {
		t.Error("renderCompletion should produce output when not hidden")
	}

	// Test 2: Render when hidden (should not render)
	completion.Hide()
	mockOut.reset()
	renderer.renderCompletion(buffer, completion)
	if len(mockOut.calls) != 0 {
		t.Error("renderCompletion should not produce output when hidden")
	}

	// Test 3: Render when shown again (should render)
	completion.Show()
	mockOut.reset()
	renderer.renderCompletion(buffer, completion)
	if len(mockOut.calls) == 0 {
		t.Error("renderCompletion should produce output after Show()")
	}
}

func TestHideCompletions_KeyBindFunc(t *testing.T) {
	// Create a minimal prompt for testing
	p := &Prompt{
		buffer:     NewBuffer(),
		completion: NewCompletionManager(5),
		renderer: &Renderer{
			col: 80,
			row: 20,
		},
	}

	// Add some suggestions
	p.completion.tmp = []Suggest{
		{Text: "test1", Description: "desc1"},
		{Text: "test2", Description: "desc2"},
	}

	// Initially visible
	if p.completion.IsHidden() {
		t.Error("Completions should not be hidden initially")
	}

	// Call HideCompletions
	result := HideCompletions(p)

	// Should return true for rerender
	if !result {
		t.Error("HideCompletions should return true")
	}

	// Should be hidden
	if !p.completion.IsHidden() {
		t.Error("Completions should be hidden after HideCompletions")
	}
}

func TestShowCompletions_KeyBindFunc(t *testing.T) {
	// Create a minimal prompt for testing
	p := &Prompt{
		buffer:     NewBuffer(),
		completion: NewCompletionManager(5),
		renderer: &Renderer{
			col: 80,
			row: 20,
		},
	}

	// Add some suggestions
	p.completion.tmp = []Suggest{
		{Text: "test1", Description: "desc1"},
		{Text: "test2", Description: "desc2"},
	}

	// Hide first
	p.completion.Hide()
	if !p.completion.IsHidden() {
		t.Error("Completions should be hidden")
	}

	// Call ShowCompletions
	result := ShowCompletions(p)

	// Should return true for rerender
	if !result {
		t.Error("ShowCompletions should return true")
	}

	// Should be visible
	if p.completion.IsHidden() {
		t.Error("Completions should be visible after ShowCompletions")
	}
}

func TestCompletionManager_HideDoesNotAffectUpdate(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		return []Suggest{
			{Text: "apple", Description: "fruit"},
			{Text: "application", Description: "software"},
		}, 0, d.CurrentRuneIndex()
	}

	c := NewCompletionManager(5, CompletionManagerWithCompleter(completer))

	doc := Document{
		Text:           "app",
		cursorPosition: 3,
	}

	// Update while visible
	c.Update(doc)
	suggestions := c.GetSuggestions()
	if len(suggestions) != 2 {
		t.Errorf("Expected 2 suggestions, got %d", len(suggestions))
	}

	// Hide and update again
	c.Hide()
	doc.Text = "appl"
	doc.cursorPosition = 4
	c.Update(doc)

	// Suggestions should still be updated
	suggestions = c.GetSuggestions()
	if len(suggestions) != 2 {
		t.Errorf("Expected 2 suggestions even when hidden, got %d", len(suggestions))
	}

	// But it should still be hidden
	if !c.IsHidden() {
		t.Error("CompletionManager should remain hidden after Update")
	}
}

func TestCompletionManager_NavigationWhileHidden(t *testing.T) {
	c := NewCompletionManager(5)
	c.tmp = []Suggest{
		{Text: "test1", Description: "desc1"},
		{Text: "test2", Description: "desc2"},
		{Text: "test3", Description: "desc3"},
	}

	// Hide completions
	c.Hide()

	// Try to navigate
	c.Next()

	// Navigation should work
	if c.selected != 0 {
		t.Errorf("Expected selected=0, got %d", c.selected)
	}

	c.Next()
	if c.selected != 1 {
		t.Errorf("Expected selected=1, got %d", c.selected)
	}

	// Hidden state should be preserved
	if !c.IsHidden() {
		t.Error("Hidden state should be preserved during navigation")
	}
}

func TestCompletionManager_MultipleOptions(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		return []Suggest{{Text: "test"}}, 0, 0
	}

	c := NewCompletionManager(
		10,
		CompletionManagerWithCompleter(completer),
		CompletionManagerWithHideAfterExecute(true),
	)

	if c.completer == nil {
		t.Error("Completer should be set")
	}

	if !c.ShouldHideAfterExecute() {
		t.Error("hideAfterExecute should be true")
	}

	if c.max != 10 {
		t.Errorf("Expected max=10, got %d", c.max)
	}
}
