package main

import (
	"fmt"
	"strings"

	prompt "github.com/joeycumines/go-prompt"
	pstrings "github.com/joeycumines/go-prompt/strings"
)

func completer(in prompt.Document) ([]prompt.Suggest, pstrings.RuneNumber, pstrings.RuneNumber) {
	// Generate a large list of suggestions to test page navigation
	var s []prompt.Suggest
	for i := 0; i < 50; i++ {
		s = append(s, prompt.Suggest{
			Text:        fmt.Sprintf("command-%02d", i),
			Description: fmt.Sprintf("This is command number %d for testing page navigation", i),
		})
	}

	endIndex := in.CurrentRuneIndex()
	w := in.GetWordBeforeCursor()
	startIndex := endIndex - pstrings.RuneCountInString(w)

	// Filter based on prefix
	filtered := prompt.FilterHasPrefix(s, w, true)

	return filtered, startIndex, endIndex
}

func main() {
	fmt.Println("Page Navigation Test")
	fmt.Println("====================")
	fmt.Println("Start typing 'command' to see suggestions.")
	fmt.Println("Use PageDown/PageUp to navigate by pages.")
	fmt.Println("Use Up/Down arrows to navigate by single items.")
	fmt.Println()

	in := prompt.Input(
		prompt.WithPrefix(">>> "),
		prompt.WithTitle("page-navigation-test"),
		prompt.WithCompleter(completer),
		prompt.WithMaxSuggestion(10),       // Show 10 items at a time
		prompt.WithDynamicCompletion(true), // Test with dynamic completion enabled
	)

	if strings.TrimSpace(in) != "" {
		fmt.Println("\nYour input: " + in)
	}
}
