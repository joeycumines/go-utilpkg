package prompt

import (
	"reflect"
	"testing"

	istrings "github.com/joeycumines/go-prompt/strings"
)

func TestFormatShortSuggestion(t *testing.T) {
	var scenarioTable = []struct {
		in       []Suggest
		expected []Suggest
		max      istrings.Width
		exWidth  istrings.Width
	}{
		{
			in: []Suggest{
				{Text: "foo"},
				{Text: "bar"},
				{Text: "fuga"},
			},
			expected: []Suggest{
				{Text: " foo  "},
				{Text: " bar  "},
				{Text: " fuga "},
			},
			max:     100,
			exWidth: 6,
		},
		{
			in: []Suggest{
				{Text: "apple", Description: "This is apple."},
				{Text: "banana", Description: "This is banana."},
				{Text: "coconut", Description: "This is coconut."},
			},
			expected: []Suggest{
				{Text: " apple   ", Description: " This is apple.   "},
				{Text: " banana  ", Description: " This is banana.  "},
				{Text: " coconut ", Description: " This is coconut. "},
			},
			max:     100,
			exWidth: istrings.Width(len(" apple   " + " This is apple.   ")),
		},
		{
			in: []Suggest{
				{Text: "This is apple."},
				{Text: "This is banana."},
				{Text: "This is coconut."},
			},
			expected: []Suggest{
				{Text: " Thi... "},
				{Text: " Thi... "},
				{Text: " Thi... "},
			},
			max:     8,
			exWidth: 8,
		},
		{
			in: []Suggest{
				{Text: "This is apple."},
				{Text: "This is banana."},
				{Text: "This is coconut."},
			},
			expected: []Suggest{},
			max:      3,
			exWidth:  0,
		},
		{
			in: []Suggest{
				{Text: "--all-namespaces", Description: "-------------------------------------------------------------------------------------------------------------------------------------------"},
				{Text: "--allow-missing-template-keys", Description: "-----------------------------------------------------------------------------------------------------------------------------------------------"},
				{Text: "--export", Description: "----------------------------------------------------------------------------------------------------------"},
				{Text: "-f", Description: "-----------------------------------------------------------------------------------"},
				{Text: "--filename", Description: "-----------------------------------------------------------------------------------"},
				{Text: "--include-extended-apis", Description: "------------------------------------------------------------------------------------"},
			},
			expected: []Suggest{
				{Text: " --all-namespaces              ", Description: " --------------... "},
				{Text: " --allow-missing-template-keys ", Description: " --------------... "},
				{Text: " --export                      ", Description: " --------------... "},
				{Text: " -f                            ", Description: " --------------... "},
				{Text: " --filename                    ", Description: " --------------... "},
				{Text: " --include-extended-apis       ", Description: " --------------... "},
			},
			max:     50,
			exWidth: istrings.Width(len(" --include-extended-apis       " + " ---------------...")),
		},
		{
			in: []Suggest{
				{Text: "--all-namespaces", Description: "If present, list the requested object(s) across all namespaces. Namespace in current context is ignored even if specified with --namespace."},
				{Text: "--allow-missing-template-keys", Description: "If true, ignore any errors in templates when a field or map key is missing in the template. Only applies to golang and jsonpath output formats."},
				{Text: "--export", Description: "If true, use 'export' for the resources.  Exported resources are stripped of cluster-specific information."},
				{Text: "-f", Description: "Filename, directory, or URL to files identifying the resource to get from a server."},
				{Text: "--filename", Description: "Filename, directory, or URL to files identifying the resource to get from a server."},
				{Text: "--include-extended-apis", Description: "If true, include definitions of new APIs via calls to the API server. [default true]"},
			},
			expected: []Suggest{
				{Text: " --all-namespaces              ", Description: " If present, list the requested object(s) across all namespaces. Namespace in current context is ignored even if specified with --namespace.     "},
				{Text: " --allow-missing-template-keys ", Description: " If true, ignore any errors in templates when a field or map key is missing in the template. Only applies to golang and jsonpath output formats. "},
				{Text: " --export                      ", Description: " If true, use 'export' for the resources.  Exported resources are stripped of cluster-specific information.                                      "},
				{Text: " -f                            ", Description: " Filename, directory, or URL to files identifying the resource to get from a server.                                                             "},
				{Text: " --filename                    ", Description: " Filename, directory, or URL to files identifying the resource to get from a server.                                                             "},
				{Text: " --include-extended-apis       ", Description: " If true, include definitions of new APIs via calls to the API server. [default true]                                                            "},
			},
			max:     500,
			exWidth: istrings.Width(len(" --include-extended-apis       " + " If true, include definitions of new APIs via calls to the API server. [default true]                                                            ")),
		},
	}

	for i, s := range scenarioTable {
		actual, width := formatSuggestions(s.in, s.max)
		if width != s.exWidth {
			t.Errorf("[scenario %d] Want %d but got %d\n", i, s.exWidth, width)
		}
		if !reflect.DeepEqual(actual, s.expected) {
			t.Errorf("[scenario %d] Want %#v, but got %#v\n", i, s.expected, actual)
		}
	}
}

func TestFormatText(t *testing.T) {
	var scenarioTable = []struct {
		in       []string
		expected []string
		max      istrings.Width
		exWidth  istrings.Width
	}{
		{
			in: []string{
				"",
				"",
			},
			expected: []string{
				"",
				"",
			},
			max:     10,
			exWidth: 0,
		},
		{
			in: []string{
				"apple",
				"banana",
				"coconut",
			},
			expected: []string{
				"",
				"",
				"",
			},
			max:     2,
			exWidth: 0,
		},
		{
			in: []string{
				"apple",
				"banana",
				"coconut",
			},
			expected: []string{
				"",
				"",
				"",
			},
			max:     istrings.GetWidth(" " + " " + shortenSuffix),
			exWidth: 0,
		},
		{
			in: []string{
				"apple",
				"banana",
				"coconut",
			},
			expected: []string{
				" apple   ",
				" banana  ",
				" coconut ",
			},
			max:     100,
			exWidth: istrings.GetWidth(" coconut "),
		},
		{
			in: []string{
				"apple",
				"banana",
				"coconut",
			},
			expected: []string{
				" a... ",
				" b... ",
				" c... ",
			},
			max:     6,
			exWidth: 6,
		},
	}

	for i, s := range scenarioTable {
		actual, width := formatTexts(s.in, s.max, " ", " ")
		if width != s.exWidth {
			t.Errorf("[scenario %d] Want %d but got %d\n", i, s.exWidth, width)
		}
		if !reflect.DeepEqual(actual, s.expected) {
			t.Errorf("[scenario %d] Want %#v, but got %#v\n", i, s.expected, actual)
		}
	}
}

func TestNoopCompleter(t *testing.T) {
	sug, start, end := NoopCompleter(Document{})
	if sug != nil || start != 0 || end != 0 {
		t.Errorf("NoopCompleter should return nil")
	}
}

func TestCompletionManager_NextPage(t *testing.T) {
	c := NewCompletionManager(5)
	c.tmp = []Suggest{
		{Text: "item0"},
		{Text: "item1"},
		{Text: "item2"},
		{Text: "item3"},
		{Text: "item4"},
		{Text: "item5"},
		{Text: "item6"},
		{Text: "item7"},
		{Text: "item8"},
		{Text: "item9"},
		{Text: "item10"},
		{Text: "item11"},
	}

	// Start at -1 (no selection), NextPage should select first item (index 0)
	c.selected = -1
	c.NextPage()
	if c.selected != 0 {
		t.Errorf("NextPage from -1: expected selected=0, got %d", c.selected)
	}

	// From item 0, NextPage should advance by pageHeight (5) to item 5
	c.NextPage()
	if c.selected != 5 {
		t.Errorf("NextPage from 0: expected selected=5, got %d", c.selected)
	}

	// From item 5, NextPage should advance to last page start (item 7, maxScroll=12-5=7)
	c.NextPage()
	if c.selected != 7 {
		t.Errorf("NextPage from 5: expected selected=7, got %d", c.selected)
	}

	// From last page, NextPage should select last item (clamp, not wrap)
	c.NextPage()
	if c.selected != 11 {
		t.Errorf("NextPage from 7 (last page): expected selected=11 (last item), got %d", c.selected)
	}

	// Another NextPage should be idempotent
	c.NextPage()
	if c.selected != 11 {
		t.Errorf("NextPage idempotent: expected selected=11, got %d", c.selected)
	}
}

func TestCompletionManager_Previous_DynamicHeight(t *testing.T) {
	// Test that Previous() correctly uses the dynamic lastWindowHeight (pageHeight)
	// for auto-scrolling logic when moving up.
	c := NewCompletionManager(10)
	c.tmp = []Suggest{
		{Text: "item0"}, {Text: "item1"}, {Text: "item2"}, {Text: "item3"},
		{Text: "item4"}, {Text: "item5"}, {Text: "item6"}, {Text: "item7"},
	}

	// Simulate a dynamic render where the actual window height is 4
	c.lastWindowHeight = 4

	// Position selection at the top of the visible page, but not page 0
	// With lastWindowHeight=4, if scroll is 2, visible indices are 2-5
	c.selected = 2
	c.verticalScroll = 2

	// Call Previous() - should move to item 1 and scroll up to keep it visible
	// The check `c.verticalScroll == c.selected` will be true, so it scrolls.
	c.Previous()

	if c.selected != 1 {
		t.Errorf("Previous() with lastWindowHeight=4: expected selected=1, got %d", c.selected)
	}
	if c.verticalScroll != 1 {
		t.Errorf("Previous() with lastWindowHeight=4: expected verticalScroll=1 (scrolled up), got %d", c.verticalScroll)
	}
}

func TestCompletionManager_PreviousPage(t *testing.T) {
	c := NewCompletionManager(5)
	c.tmp = []Suggest{
		{Text: "item0"},
		{Text: "item1"},
		{Text: "item2"},
		{Text: "item3"},
		{Text: "item4"},
		{Text: "item5"},
		{Text: "item6"},
		{Text: "item7"},
		{Text: "item8"},
		{Text: "item9"},
		{Text: "item10"},
		{Text: "item11"},
	}

	// Start at last page (item 7, scroll=7), PreviousPage should go to item 5
	c.selected = 7
	c.verticalScroll = 7
	c.PreviousPage()
	if c.selected != 5 {
		t.Errorf("PreviousPage from 7: expected selected=5, got %d", c.selected)
	}

	// From item 5, PreviousPage should go to item 0 (first page)
	c.PreviousPage()
	if c.selected != 0 {
		t.Errorf("PreviousPage from 5: expected selected=0, got %d", c.selected)
	}

	// From first page, PreviousPage should be idempotent
	c.PreviousPage()
	if c.selected != 0 {
		t.Errorf("PreviousPage idempotent: expected selected=0, got %d", c.selected)
	}

	// From -1, PreviousPage should go to last item on last page
	c.selected = -1
	c.verticalScroll = 0
	c.PreviousPage()
	if c.selected != 11 {
		t.Errorf("PreviousPage from -1: expected selected=11 (last item), got %d", c.selected)
	}
	if c.verticalScroll != 7 {
		t.Errorf("PreviousPage from -1: expected verticalScroll=7, got %d", c.verticalScroll)
	}
}

func TestCompletionManager_PageNavigation_SmallList(t *testing.T) {
	// Test with a list smaller than max window height
	c := NewCompletionManager(10)
	c.tmp = []Suggest{
		{Text: "item0"},
		{Text: "item1"},
		{Text: "item2"},
	}

	// NextPage from -1 should select first item (list smaller than pageHeight)
	c.selected = -1
	c.NextPage()
	if c.selected != 0 {
		t.Errorf("NextPage from -1 with small list: expected selected=0, got %d", c.selected)
	}

	// NextPage from 0 should select last item (maxScroll=0 since list < pageHeight)
	c.NextPage()
	if c.selected != 2 {
		t.Errorf("NextPage from 0 with small list: expected selected=2 (last item), got %d", c.selected)
	}

	// Another NextPage should be idempotent
	c.NextPage()
	if c.selected != 2 {
		t.Errorf("NextPage idempotent with small list: expected selected=2, got %d", c.selected)
	}
}

func TestCompletionManager_PageNavigation_WithDynamicHeight(t *testing.T) {
	// Test that page navigation works correctly when adjustWindowHeight is called
	// (simulating dynamic completion scenarios)
	c := NewCompletionManager(10)
	c.tmp = []Suggest{
		{Text: "item0"},
		{Text: "item1"},
		{Text: "item2"},
		{Text: "item3"},
		{Text: "item4"},
		{Text: "item5"},
		{Text: "item6"},
		{Text: "item7"},
		{Text: "item8"},
		{Text: "item9"},
		{Text: "item10"},
		{Text: "item11"},
		{Text: "item12"},
		{Text: "item13"},
		{Text: "item14"},
	}

	// Start at item 0
	c.selected = 0

	// Simulate renderer setting lastWindowHeight to 5
	c.lastWindowHeight = 5

	// NextPage should advance by lastWindowHeight (5), not c.max (10)
	c.NextPage()
	if c.selected != 5 {
		t.Errorf("NextPage with lastWindowHeight=5: expected selected=5, got %d", c.selected)
	}
	if c.verticalScroll != 5 {
		t.Errorf("NextPage with lastWindowHeight=5: expected verticalScroll=5, got %d", c.verticalScroll)
	}

	// NextPage again should advance by 5 to item 10 (maxScroll=15-5=10)
	c.NextPage()
	if c.selected != 10 {
		t.Errorf("NextPage from 5: expected selected=10, got %d", c.selected)
	}
	if c.verticalScroll != 10 {
		t.Errorf("NextPage from 5: expected verticalScroll=10, got %d", c.verticalScroll)
	}

	// NextPage should select last item (last page)
	c.NextPage()
	if c.selected != 14 {
		t.Errorf("NextPage to last page: expected selected=14 (last item), got %d", c.selected)
	}

	// PreviousPage should go back by 5
	c.PreviousPage()
	if c.selected != 10 {
		t.Errorf("PreviousPage from last page: expected selected=10, got %d", c.selected)
	}
	if c.verticalScroll != 10 {
		t.Errorf("PreviousPage from last page: expected verticalScroll=10, got %d", c.verticalScroll)
	}
}

func TestCompletionManager_adjustWindowHeight(t *testing.T) {
	c := NewCompletionManager(10)
	c.tmp = []Suggest{
		{Text: "item0"},
		{Text: "item1"},
		{Text: "item2"},
		{Text: "item3"},
		{Text: "item4"},
		{Text: "item5"},
		{Text: "item6"},
		{Text: "item7"},
		{Text: "item8"},
		{Text: "item9"},
	}

	// Test that selected item remains visible after window height adjustment
	c.selected = 7
	c.verticalScroll = 0

	// Adjust to window height of 3
	c.adjustWindowHeight(3, len(c.tmp))

	// Should adjust scroll so item 7 is visible (verticalScroll should be 5)
	// because selected (7) >= verticalScroll (0) + windowHeight (3)
	if c.verticalScroll != 5 {
		t.Errorf("adjustWindowHeight: expected verticalScroll=5, got %d", c.verticalScroll)
	}

	// Verify selected is still 7
	if c.selected != 7 {
		t.Errorf("adjustWindowHeight: expected selected=7, got %d", c.selected)
	}
}

func TestCompletionManager_PreviousPage_SnapToTop(t *testing.T) {
	// Test the "snap-to-top" behavior: when PreviousPage is called and the
	// selected item is not at the top of the current page, it should first
	// move to the top of the current page before paging up.
	c := NewCompletionManager(5)
	c.tmp = []Suggest{
		{Text: "item0"},
		{Text: "item1"},
		{Text: "item2"},
		{Text: "item3"},
		{Text: "item4"},
		{Text: "item5"},
		{Text: "item6"},
		{Text: "item7"},
		{Text: "item8"},
		{Text: "item9"},
		{Text: "item10"},
		{Text: "item11"},
	}

	// Set up state: we're on the second page (verticalScroll=5)
	// but selected is item 7 (not at the top of the page)
	c.selected = 7
	c.verticalScroll = 5

	// First PreviousPage should snap to top of current page (item 5)
	c.PreviousPage()
	if c.selected != 5 {
		t.Errorf("First PreviousPage (snap-to-top): expected selected=5, got %d", c.selected)
	}
	if c.verticalScroll != 5 {
		t.Errorf("First PreviousPage (snap-to-top): expected verticalScroll=5, got %d", c.verticalScroll)
	}

	// Second PreviousPage should now page up to item 0
	c.PreviousPage()
	if c.selected != 0 {
		t.Errorf("Second PreviousPage (actual page up): expected selected=0, got %d", c.selected)
	}
	if c.verticalScroll != 0 {
		t.Errorf("Second PreviousPage (actual page up): expected verticalScroll=0, got %d", c.verticalScroll)
	}

	// Test another scenario: selected at middle of last page
	c.selected = 9
	c.verticalScroll = 7 // Last page starts at item 7

	// First PreviousPage should snap to item 7 (top of last page)
	c.PreviousPage()
	if c.selected != 7 {
		t.Errorf("Snap-to-top on last page: expected selected=7, got %d", c.selected)
	}
	if c.verticalScroll != 7 {
		t.Errorf("Snap-to-top on last page: expected verticalScroll=7, got %d", c.verticalScroll)
	}

	// Second PreviousPage should page up by pageHeight (5) to item 5
	c.PreviousPage()
	if c.selected != 5 {
		t.Errorf("Page up from last page: expected selected=5, got %d", c.selected)
	}

	// Test edge case: selected is already at top of page
	c.selected = 5
	c.verticalScroll = 5

	// PreviousPage should immediately page up (not snap, as we're already at top)
	c.PreviousPage()
	if c.selected != 0 {
		t.Errorf("PreviousPage when already at top: expected selected=0, got %d", c.selected)
	}
	if c.verticalScroll != 0 {
		t.Errorf("PreviousPage when already at top: expected verticalScroll=0, got %d", c.verticalScroll)
	}
}

func TestCompletionManager_Next_DynamicHeight(t *testing.T) {
	// Test that Next() correctly uses the dynamic lastWindowHeight (pageHeight)
	// instead of c.max for auto-scrolling logic. This is the regression test
	// for the bugfix that changed `c.verticalScroll+int(c.max)-1` to use pageHeight.
	c := NewCompletionManager(10)
	c.tmp = []Suggest{
		{Text: "item0"},
		{Text: "item1"},
		{Text: "item2"},
		{Text: "item3"},
		{Text: "item4"},
		{Text: "item5"},
		{Text: "item6"},
		{Text: "item7"},
		{Text: "item8"},
		{Text: "item9"},
	}

	// Simulate a dynamic render where the actual window height is 5 (not the max of 10)
	c.lastWindowHeight = 5

	// Position selection at the bottom of the visible page
	// With lastWindowHeight=5, visible indices are 0-4
	c.selected = 4
	c.verticalScroll = 0

	// Call Next() - should move to item 5 and scroll down
	// OLD BUG: Would check `c.verticalScroll + int(c.max) - 1` = 0 + 10 - 1 = 9
	//          Since selected (4) != 9, it would NOT scroll, breaking the view
	// FIXED: Checks `c.verticalScroll + pageHeight - 1` = 0 + 5 - 1 = 4
	//        Since selected (4) == 4, it WILL scroll to keep item 5 visible
	c.Next()

	if c.selected != 5 {
		t.Errorf("Next() with lastWindowHeight=5: expected selected=5, got %d", c.selected)
	}
	if c.verticalScroll != 1 {
		t.Errorf("Next() with lastWindowHeight=5: expected verticalScroll=1 (scrolled to keep item 5 visible), got %d", c.verticalScroll)
	}

	// Continue calling Next() to verify scrolling continues correctly
	c.Next() // selected=6, scroll should be 2
	if c.selected != 6 || c.verticalScroll != 2 {
		t.Errorf("Next() second call: expected selected=6, verticalScroll=2, got selected=%d, verticalScroll=%d", c.selected, c.verticalScroll)
	}

	// Test the same scenario but with a different window height
	c.selected = 0
	c.verticalScroll = 0
	c.lastWindowHeight = 3

	// Navigate to bottom of 3-item window
	c.Next() // 1
	c.Next() // 2

	if c.selected != 2 || c.verticalScroll != 0 {
		t.Errorf("Setup for 3-item window: expected selected=2, verticalScroll=0, got selected=%d, verticalScroll=%d", c.selected, c.verticalScroll)
	}

	// Next should now trigger scroll
	c.Next()
	if c.selected != 3 || c.verticalScroll != 1 {
		t.Errorf("Next() with lastWindowHeight=3: expected selected=3, verticalScroll=1, got selected=%d, verticalScroll=%d", c.selected, c.verticalScroll)
	}
}

func TestCompletionManager_ClearWindowCache(t *testing.T) {
	// Test that ClearWindowCache() correctly resets the cached window height,
	// forcing recalculation and affecting subsequent paging behavior.
	c := NewCompletionManager(10)
	c.tmp = []Suggest{
		{Text: "item0"},
		{Text: "item1"},
		{Text: "item2"},
		{Text: "item3"},
		{Text: "item4"},
		{Text: "item5"},
		{Text: "item6"},
		{Text: "item7"},
		{Text: "item8"},
		{Text: "item9"},
		{Text: "item10"},
		{Text: "item11"},
		{Text: "item12"},
		{Text: "item13"},
		{Text: "item14"},
	}

	// Simulate a render that sets lastWindowHeight to 5
	c.lastWindowHeight = 5
	c.selected = 0
	c.verticalScroll = 0

	// NextPage should advance by 5 (the cached window height)
	c.NextPage()
	if c.selected != 5 {
		t.Errorf("Before cache clear: expected selected=5 (paged by lastWindowHeight=5), got %d", c.selected)
	}
	if c.verticalScroll != 5 {
		t.Errorf("Before cache clear: expected verticalScroll=5, got %d", c.verticalScroll)
	}

	// Clear the cache (simulating a terminal resize event)
	c.ClearWindowCache()

	// Verify cache was cleared
	if c.lastWindowHeight != 0 {
		t.Errorf("After ClearWindowCache(): expected lastWindowHeight=0, got %d", c.lastWindowHeight)
	}

	// Verify selection state was preserved
	if c.selected != 5 {
		t.Errorf("After ClearWindowCache(): expected selected=5 (preserved), got %d", c.selected)
	}

	// NextPage should now use the fallback c.max (10) for page height
	// From position 5, with pageHeight=10, maxScroll = 15-10 = 5
	// Since we're already at maxScroll (5), we should jump to last item (14)
	c.NextPage()
	if c.selected != 14 {
		t.Errorf("After cache clear: expected selected=14 (last item, since already at maxScroll), got %d", c.selected)
	}
	if c.verticalScroll != 5 {
		t.Errorf("After cache clear: expected verticalScroll=5 (unchanged, already at maxScroll), got %d", c.verticalScroll)
	}

	// Test that cache clear doesn't break when selection is active
	c.lastWindowHeight = 7
	c.selected = 12
	c.verticalScroll = 8

	c.ClearWindowCache()

	if c.lastWindowHeight != 0 {
		t.Errorf("Second ClearWindowCache(): expected lastWindowHeight=0, got %d", c.lastWindowHeight)
	}
	if c.selected != 12 {
		t.Errorf("Second ClearWindowCache(): expected selected=12 (preserved), got %d", c.selected)
	}
	if c.verticalScroll != 8 {
		t.Errorf("Second ClearWindowCache(): expected verticalScroll=8 (preserved), got %d", c.verticalScroll)
	}
}
