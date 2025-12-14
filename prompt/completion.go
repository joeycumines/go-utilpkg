package prompt

import (
	"strings"

	"github.com/joeycumines/go-prompt/debug"
	istrings "github.com/joeycumines/go-prompt/strings"
	runewidth "github.com/mattn/go-runewidth"
)

const (
	shortenSuffix = "..."
	leftPrefix    = " "
	leftSuffix    = " "
	rightPrefix   = " "
	rightSuffix   = " "
)

// Suggest represents a single suggestion
// in the auto-complete box.
type Suggest struct {
	Text        string
	Description string
}

// CompletionManager manages which suggestion is now selected.
type CompletionManager struct {
	selected       int // -1 means nothing is selected.
	tmp            []Suggest
	max            uint16
	completer      Completer
	startCharIndex istrings.RuneNumber // index of the first char of the text that should be replaced by the selected suggestion
	endCharIndex   istrings.RuneNumber // index of the last char of the text that should be replaced by the selected suggestion
	shouldUpdate   bool

	verticalScroll int
	wordSeparator  string
	showAtStart    bool

	// lastWindowHeight tracks the actual rendered window height from the last render,
	// used to calculate effective page height for paging operations.
	lastWindowHeight int

	// hidden controls whether the completion window is explicitly hidden,
	// independent of whether there are suggestions available.
	hidden bool
	// hideAfterExecute controls whether the completion window should be hidden
	// when a new input prompt starts (e.g., after submitting input).
	hideAfterExecute bool
}

// GetSelectedSuggestion returns the selected item.
func (c *CompletionManager) GetSelectedSuggestion() (s Suggest, ok bool) {
	if c.selected == -1 || c.selected >= len(c.tmp) {
		return Suggest{}, false
	} else if c.selected < -1 {
		debug.Assert(false, "must not reach here")
		c.selected = -1
		return Suggest{}, false
	}

	return c.tmp[c.selected], true
}

// GetSuggestions returns the list of suggestion.
func (c *CompletionManager) GetSuggestions() []Suggest {
	return c.tmp
}

// Unselect the currently selected suggestion.
func (c *CompletionManager) Reset() {
	c.selected = -1
	c.verticalScroll = 0
	c.Update(*NewDocument())
}

// ClearWindowCache resets the cached window height, forcing recalculation on next render.
// This must be called when external events (resize) invalidate the cached geometry.
// Selection state is preserved and will be adjusted by adjustWindowHeight if needed.
func (c *CompletionManager) ClearWindowCache() {
	c.lastWindowHeight = 0
}

// Update the suggestions.
func (c *CompletionManager) Update(in Document) {
	c.tmp, c.startCharIndex, c.endCharIndex = c.completer(in)
}

// Select the previous suggestion item.
func (c *CompletionManager) Previous() {
	pageHeight := c.effectivePageHeight()
	if pageHeight <= 0 {
		return
	}
	if c.verticalScroll == c.selected && c.selected > 0 {
		c.verticalScroll--
	}
	c.selected--
	c.update()
}

// effectivePageHeight returns the page height to use for paging operations.
// It uses the last rendered window height if available, otherwise falls back to max.
func (c *CompletionManager) effectivePageHeight() int {
	if c.lastWindowHeight > 0 {
		return c.lastWindowHeight
	}
	return int(c.max)
}

// Next to select the next suggestion item.
func (c *CompletionManager) Next() int {
	pageHeight := c.effectivePageHeight()
	if pageHeight <= 0 {
		return c.selected
	}
	if c.verticalScroll+pageHeight-1 == c.selected {
		c.verticalScroll++
	}
	c.selected++
	c.update()
	return c.selected
}

// NextPage selects the suggestion item one page down.
// Behavior uses a "Snap-then-Scroll" sliding window strategy:
//  1. If not at the bottom of the visible window, snap to the bottom.
//  2. If at the bottom, scroll such that the current bottom item becomes the
//     first visible item (top) of the new page, and select the new bottom.
func (c *CompletionManager) NextPage() {
	pageHeight := c.effectivePageHeight()
	if pageHeight <= 0 || len(c.tmp) == 0 {
		return
	}

	// On first press from no selection, select the first item.
	if c.selected == -1 {
		c.selected = 0
		c.verticalScroll = 0
		return
	}

	// Snap to Bottom
	bottomIndex := c.verticalScroll + pageHeight - 1
	// Clamp bottomIndex to available items
	if bottomIndex >= len(c.tmp) {
		bottomIndex = len(c.tmp) - 1
	}

	if c.selected != bottomIndex {
		c.selected = bottomIndex
		return
	}

	// Scroll Page
	// The item that was at the bottom (c.selected) becomes the top.
	newScroll := c.selected
	// Special case: If pageHeight is 1, top and bottom are same. Force advance.
	if pageHeight == 1 {
		newScroll++
	}

	maxScroll := len(c.tmp) - pageHeight
	if maxScroll < 0 {
		maxScroll = 0
	}

	if newScroll > maxScroll {
		newScroll = maxScroll
		// If we hit the bottom boundary, ensure we select the very last item.
		// (This handles cases where the jump would overshoot the list end).
		c.selected = len(c.tmp) - 1
	} else {
		// Normal sliding window: selected item becomes top.
		c.verticalScroll = newScroll
		// Select the new bottom of the viewport
		newSelected := c.verticalScroll + pageHeight - 1
		if newSelected >= len(c.tmp) {
			newSelected = len(c.tmp) - 1
		}
		c.selected = newSelected
	}

	// Ensure scroll is applied (redundant if branch fell through, but safe)
	c.verticalScroll = newScroll
}

// PreviousPage selects the suggestion item one page up.
// Behavior uses a "Snap-then-Scroll" sliding window strategy:
//  1. If not at the top of the visible window, snap to the top.
//  2. If at the top, scroll such that the current top item becomes the
//     last visible item (bottom) of the new page, and select the new top.
func (c *CompletionManager) PreviousPage() {
	pageHeight := c.effectivePageHeight()
	if pageHeight <= 0 || len(c.tmp) == 0 {
		return
	}

	// On first press from no selection, go to the last item on the last page.
	if c.selected == -1 {
		c.selected = len(c.tmp) - 1
		maxScroll := len(c.tmp) - pageHeight
		if maxScroll < 0 {
			maxScroll = 0
		}
		c.verticalScroll = maxScroll
		return
	}

	// Snap to Top
	topIndex := c.verticalScroll
	if c.selected != topIndex {
		c.selected = topIndex
		return
	}

	// Scroll Page
	// The item that was at the top (c.selected) becomes the bottom.
	// New Bottom = c.selected
	// New Top = New Bottom - pageHeight + 1
	newScroll := c.selected - pageHeight + 1
	// Special case: If pageHeight is 1, top and bottom are same. Force retreat.
	if pageHeight == 1 {
		newScroll--
	}

	if newScroll < 0 {
		newScroll = 0
	}

	c.verticalScroll = newScroll
	// Select the new top of the viewport
	c.selected = newScroll
}

// Completing returns true when the CompletionManager selects something.
func (c *CompletionManager) Completing() bool {
	return c.selected != -1
}

// Hide explicitly hides the completion window.
func (c *CompletionManager) Hide() {
	c.hidden = true
}

// Show explicitly shows the completion window.
func (c *CompletionManager) Show() {
	c.hidden = false
}

// IsHidden returns true if the completion window is explicitly hidden.
func (c *CompletionManager) IsHidden() bool {
	return c.hidden
}

// HideAfterExecute sets whether completions should be hidden when a new input prompt starts.
func (c *CompletionManager) HideAfterExecute(hide bool) {
	c.hideAfterExecute = hide
}

// ShouldHideAfterExecute returns whether completions should be hidden on new input.
func (c *CompletionManager) ShouldHideAfterExecute() bool {
	return c.hideAfterExecute
}

// adjustWindowHeight adjusts the vertical scroll position to account for
// the actual window height, which may differ from max due to dynamic completion.
// This must be called before rendering to ensure state consistency.
func (c *CompletionManager) adjustWindowHeight(windowHeight, contentHeight int) {
	if windowHeight <= 0 || contentHeight <= 0 {
		return
	}

	// Ensure selected item is visible
	if c.Completing() && c.selected >= 0 {
		if c.selected >= contentHeight {
			c.selected = contentHeight - 1
		}
		if c.selected >= c.verticalScroll+windowHeight {
			c.verticalScroll = c.selected - windowHeight + 1
		}
		if c.selected < c.verticalScroll {
			c.verticalScroll = c.selected
		}
	}

	// Clamp scroll to valid range (necessary and sufficient)
	if c.verticalScroll+windowHeight > contentHeight {
		c.verticalScroll = contentHeight - windowHeight
	}
	if c.verticalScroll < 0 {
		c.verticalScroll = 0
	}
}

func (c *CompletionManager) update() {
	max := int(c.max)
	if len(c.tmp) < max {
		max = len(c.tmp)
	}

	// Reset to -1 when going past the end to create "unfocused" cycling behavior.
	// This allows TAB/Down to cycle: 0 → 1 → ... → N-1 → -1 (unfocused) → 0 → ...
	if c.selected >= len(c.tmp) {
		c.selected = -1
		c.verticalScroll = 0
	} else if c.selected < -1 {
		c.selected = len(c.tmp) - 1
		c.verticalScroll = len(c.tmp) - max
		if c.verticalScroll < 0 {
			c.verticalScroll = 0
		}
	}
}

func deleteBreakLineCharacters(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

func formatTexts(o []string, max istrings.Width, prefix, suffix string) (new []string, width istrings.Width) {
	l := len(o)
	n := make([]string, l)

	lenPrefix := istrings.GetWidth(prefix)
	lenSuffix := istrings.GetWidth(suffix)
	lenShorten := istrings.GetWidth(shortenSuffix)
	min := lenPrefix + lenSuffix + lenShorten
	for i := 0; i < l; i++ {
		o[i] = deleteBreakLineCharacters(o[i])

		w := istrings.GetWidth(o[i])
		if width < w {
			width = w
		}
	}

	if width == 0 {
		return n, 0
	}
	if min >= max {
		return n, 0
	}
	if lenPrefix+width+lenSuffix > max {
		width = max - lenPrefix - lenSuffix
	}

	for i := 0; i < l; i++ {
		x := istrings.GetWidth(o[i])
		if x <= width {
			spaces := strings.Repeat(" ", int(width-x))
			n[i] = prefix + o[i] + spaces + suffix
		} else if x > width {
			x := runewidth.Truncate(o[i], int(width), shortenSuffix)
			// When calling runewidth.Truncate("您好xxx您好xxx", 11, "...") returns "您好xxx..."
			// But the length of this result is 10. So we need fill right using runewidth.FillRight.
			n[i] = prefix + runewidth.FillRight(x, int(width)) + suffix
		}
	}
	return n, lenPrefix + width + lenSuffix
}

func formatSuggestions(suggests []Suggest, max istrings.Width) (new []Suggest, width istrings.Width) {
	num := len(suggests)
	new = make([]Suggest, num)

	left := make([]string, num)
	for i := 0; i < num; i++ {
		left[i] = suggests[i].Text
	}
	right := make([]string, num)
	for i := 0; i < num; i++ {
		right[i] = suggests[i].Description
	}

	left, leftWidth := formatTexts(left, max, leftPrefix, leftSuffix)
	if leftWidth == 0 {
		return []Suggest{}, 0
	}
	right, rightWidth := formatTexts(right, max-leftWidth, rightPrefix, rightSuffix)

	for i := 0; i < num; i++ {
		new[i] = Suggest{Text: left[i], Description: right[i]}
	}
	return new, istrings.Width(leftWidth + rightWidth)
}

// Constructor option for CompletionManager.
type CompletionManagerOption func(*CompletionManager)

// Set a custom completer.
func CompletionManagerWithCompleter(completer Completer) CompletionManagerOption {
	return func(c *CompletionManager) {
		c.completer = completer
	}
}

// CompletionManagerWithHideAfterExecute configures whether the completion window should be hidden
// when a new input prompt starts (e.g., after submitting input).
func CompletionManagerWithHideAfterExecute(hide bool) CompletionManagerOption {
	return func(c *CompletionManager) {
		c.hideAfterExecute = hide
	}
}

// NewCompletionManager returns an initialized CompletionManager object.
func NewCompletionManager(max uint16, opts ...CompletionManagerOption) *CompletionManager {
	c := &CompletionManager{
		selected:       -1,
		max:            max,
		completer:      NoopCompleter,
		verticalScroll: 0,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

var _ Completer = NoopCompleter

// NoopCompleter implements a Completer function
// that always returns no suggestions.
func NoopCompleter(_ Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
	return nil, 0, 0
}
