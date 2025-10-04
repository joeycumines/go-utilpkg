# Completion Hiding Demo

This example demonstrates the new completion window hiding features in go-prompt.

## Features

### 1. Manual Control via Key Binding

You can bind any key to hide or show the completion window:

```go
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
)
```

### 2. Auto-Hide on New Input

Enable automatic hiding of completions when starting a new prompt (after submitting input with Enter or canceling with Ctrl+C):

```go
prompt.WithExecuteHidesCompletions(true)
```

This is particularly useful for:
- Keeping the screen clean between commands
- Preventing stale completions from appearing when starting fresh
- Improving the user experience in command-line tools

## Running the Example

```bash
cd _example/completion-hiding
go run main.go
```

## Usage

1. **Start typing** - completions will appear as usual
2. **Press Escape** - hides the completion window
3. **Press Escape again** - shows the completion window
4. **Press Enter** - submits input and hides completions (due to auto-hide setting)
5. **Press Ctrl+C** - clears input and hides completions (due to auto-hide setting)
6. **Continue typing** - completions reappear automatically

## API Reference

### CompletionManager Methods

- `Hide()` - Explicitly hides the completion window
- `Show()` - Explicitly shows the completion window
- `IsHidden() bool` - Returns true if the window is hidden
- `HideAfterExecute(bool)` - Sets whether to auto-hide on new input
- `ShouldHideAfterExecute() bool` - Returns the auto-hide setting

### Constructor Options

- `CompletionManagerWithHideAfterExecute(bool)` - Configure auto-hide behavior on a custom completion manager
- `WithExecuteHidesCompletions(bool)` - Configure auto-hide behavior on the prompt

### Prompt Methods

- `Completion() *CompletionManager` - Access the completion manager

### Key Binding Functions

- `HideCompletions(p *Prompt) bool` - Function to hide completions
- `ShowCompletions(p *Prompt) bool` - Function to show completions

## Notes

- The hidden state is independent of whether suggestions exist
- Navigation (Tab, Shift+Tab) still works while hidden
- Updates to suggestions still occur while hidden
- The hidden flag persists across suggestion updates
- Multiline mode (when Enter doesn't execute) doesn't trigger auto-hide
