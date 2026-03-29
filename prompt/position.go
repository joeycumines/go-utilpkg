package prompt

import (
	istrings "github.com/joeycumines/go-prompt/strings"
)

// Position stores the coordinates
// of a p.
//
// (0, 0) represents the top-left corner of the prompt,
// while (n, n) the bottom-right corner.
type Position struct {
	X istrings.Width
	Y int
}

// Join two positions and return a new position.
func (p Position) Join(other Position) Position {
	if other.Y == 0 {
		p.X += other.X
	} else {
		p.X = other.X
		p.Y += other.Y
	}
	return p
}

// Add two positions and return a new position.
func (p Position) Add(other Position) Position {
	return Position{
		X: p.X + other.X,
		Y: p.Y + other.Y,
	}
}

// Subtract two positions and return a new position.
func (p Position) Subtract(other Position) Position {
	return Position{
		X: p.X - other.X,
		Y: p.Y - other.Y,
	}
}

// legacyNormalize converts the internal exact-fill model back to the
// legacy coordinate space expected by old exported APIs.
//
// When fullWidth is true, the internal representation is:
//
//	X = columns, Y = same line, fullWidth = true
//
// This function converts it to the legacy model:
//
//	X = 0, Y += 1
//
// This preserves backward compatibility for downstream consumers that
// assume X is always within [0, columns-1] and observe wrapping via Y.
func legacyNormalize(p Position, fullWidth bool) Position {
	if fullWidth {
		return Position{
			X: 0,
			Y: p.Y + 1,
		}
	}
	return p
}

// positionAtEndOfStringLine calculates the position of the
// p at the end of the given string or the end of the given line.
func positionAtEndOfStringLine(str string, columns istrings.Width, line int) (Position, bool) {
	reflower := NewTerminalReflower(str, 0, 1<<30, columns, true)
	var lastState ReflowState
	var hit bool
	for {
		state, ok := reflower.Next()
		if !ok {
			break
		}
		lastState = state
		if state.LineNumber >= line {
			hit = true
			break
		}
	}
	if hit {
		return Position{
			X: lastState.Width,
			Y: lastState.LineNumber,
		}, lastState.IsFullWidth
	}
	w, lineNum, fullWidth := reflower.Metrics()
	return Position{
		X: w,
		Y: lineNum,
	}, fullWidth
}

// positionAtEndOfString calculates the position
// at the end of the given string.
func positionAtEndOfString(str string, columns istrings.Width) (Position, bool) {
	reflower := NewTerminalReflower(str, 0, 1<<30, columns, true)
	for {
		_, ok := reflower.Next()
		if !ok {
			break
		}
	}
	w, lineNum, fullWidth := reflower.Metrics()
	return Position{
		X: w,
		Y: lineNum,
	}, fullWidth
}
