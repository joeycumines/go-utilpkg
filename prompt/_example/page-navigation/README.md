# Page Navigation Example

This example demonstrates the page navigation feature for the completion list.

## Features

- **Large Suggestion List**: Generates 50 suggestions to showcase scrolling
- **Page Down/Up**: Navigate through suggestions using `PageDown` and `PageUp` keys
- **Dynamic Completion**: Tests with dynamic completion enabled to verify correct behavior with variable window heights
- **Single Item Navigation**: Still supports traditional `Up`/`Down` arrow key navigation

## Usage

```bash
# Build and run
make build
./bin/page-navigation

# Or run directly
go run _example/page-navigation/main.go
```

## Testing Page Navigation

1. Start typing `command` to see the suggestion list appear
2. Use `PageDown` to jump forward by one page (10 items)
3. Use `PageUp` to jump backward by one page (10 items)
4. Use `Up`/`Down` arrows for single-item navigation
5. Press `Tab` or `Enter` to select a suggestion

## Key Bindings

| Key | Action |
|-----|--------|
| `PageDown` | Navigate down by one page (window height) |
| `PageUp` | Navigate up by one page (window height) |
| `Down` | Navigate down by one item |
| `Up` | Navigate up by one item |
| `Tab`/`Enter` | Select current suggestion |
| `Ctrl+C` | Exit |

## Implementation Details

The page navigation feature:
- Advances selection by the completion window height (`max` parameter)
- Works correctly with dynamic completion (adapts to available terminal space)
- Automatically wraps at list boundaries
- Maintains proper vertical scroll positioning
