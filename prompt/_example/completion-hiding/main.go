package main

import (
	"fmt"
	"os"
	"strings"

	prompt "github.com/joeycumines/go-prompt"
	pstrings "github.com/joeycumines/go-prompt/strings"
)

func completer(in prompt.Document) ([]prompt.Suggest, pstrings.RuneNumber, pstrings.RuneNumber) {
	s := []prompt.Suggest{
		{Text: "users", Description: "Store the username and age"},
		{Text: "articles", Description: "Store the article text posted by user"},
		{Text: "comments", Description: "Store the text commented to articles"},
		{Text: "groups", Description: "Combine users with specific rules"},
		{Text: "select", Description: "Query data from database"},
		{Text: "insert", Description: "Insert data into database"},
		{Text: "update", Description: "Update existing data"},
		{Text: "delete", Description: "Delete data from database"},
	}
	endIndex := in.CurrentRuneIndex()
	w := in.GetWordBeforeCursor()
	startIndex := endIndex - pstrings.RuneCountInString(w)

	return prompt.FilterHasPrefix(s, w, true), startIndex, endIndex
}

func executor(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	fmt.Println("You executed:", s)
}

func main() {
	fmt.Println("Completion Hiding Demo")
	fmt.Println("======================")
	fmt.Println("")
	fmt.Println("Features demonstrated:")
	fmt.Println("  - Press Escape to hide/show the completion window")
	fmt.Println("  - Completions auto-hide when you submit input (press Enter)")
	fmt.Println("  - Type to show completions again")
	fmt.Println("  - Press Ctrl+C to clear input (completions also hide)")
	fmt.Println("  - Press Ctrl+D to exit")
	fmt.Println("")

	p := prompt.New(
		executor,
		prompt.WithPrefix(">>> "),
		prompt.WithTitle("completion-hiding-demo"),
		prompt.WithHistory([]string{"SELECT * FROM users;", "INSERT INTO groups VALUES (1, 'admin');"}),
		prompt.WithPrefixTextColor(prompt.Yellow),
		prompt.WithSelectedSuggestionBGColor(prompt.LightGray),
		prompt.WithSuggestionBGColor(prompt.DarkGray),
		prompt.WithCompleter(completer),
		// Enable auto-hiding completions when submitting input
		prompt.WithExecuteHidesCompletions(true),
		// Bind Escape key to toggle completion visibility
		prompt.WithKeyBindings(
			prompt.KeyBind{
				Key: prompt.Escape,
				Fn: func(p *prompt.Prompt) bool {
					// Toggle: if hidden, show; if visible, hide
					if p.Completion().IsHidden() {
						p.Completion().Show()
					} else {
						p.Completion().Hide()
					}
					return true
				},
			},
		),
	)

	p.Run()
	os.Exit(0)
}
