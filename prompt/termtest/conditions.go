package termtest

import (
	"regexp"
	"strings"
)

// Condition defines criteria for validating output relative to a Snapshot.
type Condition func(outputSinceSnapshot string) bool

// All creates a Condition that requires all given Conditions to be true.
func All(conds ...Condition) Condition {
	return func(outputSinceSnapshot string) bool {
		for _, cond := range conds {
			if !cond(outputSinceSnapshot) {
				return false
			}
		}
		return true
	}
}

// Any creates a Condition that requires at least one of the given Conditions to be true.
func Any(conds ...Condition) Condition {
	return func(outputSinceSnapshot string) bool {
		for _, cond := range conds {
			if cond(outputSinceSnapshot) {
				return true
			}
		}
		return false
	}
}

// Not creates a Condition that negates the given Condition.
func Not(cond Condition) Condition {
	return func(outputSinceSnapshot string) bool {
		return !cond(outputSinceSnapshot)
	}
}

// Contains creates a Condition that checks if the normalized output contains the given substring.
// It normalizes the output (stripping ANSI codes, handling CR/LF) before checking.
func Contains(substr string) Condition {
	return func(outputSinceSnapshot string) bool {
		// 1. Raw check
		if strings.Contains(outputSinceSnapshot, substr) {
			return true
		}
		// 2. Normalized check
		norm := normalizeTTYOutput(outputSinceSnapshot)
		if strings.Contains(norm, substr) {
			return true
		}
		// 3. Collapsed whitespace check
		return strings.Contains(collapseWhitespace(norm), collapseWhitespace(substr))
	}
}

// ContainsRaw creates a Condition that checks if the raw output contains the given substring.
// This is useful for checking for specific ANSI escape sequences.
func ContainsRaw(substr string) Condition {
	return func(outputSinceSnapshot string) bool {
		return strings.Contains(outputSinceSnapshot, substr)
	}
}

// Matches creates a Condition that checks if the normalized output matches the given regex.
func Matches(re *regexp.Regexp) Condition {
	return func(outputSinceSnapshot string) bool {
		norm := normalizeTTYOutput(outputSinceSnapshot)
		return re.MatchString(norm)
	}
}

// normalizeTTYOutput removes ANSI escape/control sequences and carriage returns from a TTY capture.
// It uses a state-machine approach to ensure robust handling of various escape sequences and
// avoids data corruption when stripping incomplete sequences.
func normalizeTTYOutput(s string) string {
	// Fast path: if no ESC and no CR, return as-is
	if !strings.ContainsAny(s, "\x1b\r") {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\r' {
			continue // Drop carriage return
		}
		if c != 0x1b { // ESC
			b.WriteByte(c)
			continue
		}

		// Handle ESC sequences
		if i+1 >= len(s) {
			break // Incomplete sequence at buffer end; drop the dangling ESC
		}

		switch s[i+1] {
		case '[': // CSI: ESC [ ... [0x40-0x7E]
			i += 2
			for i < len(s) {
				ch := s[i]
				if ch >= 0x40 && ch <= 0x7E {
					break // Found terminator
				}
				i++
			}
		case ']': // OSC: ESC ] ... \a OR ESC \
			i += 2
			for i < len(s) {
				if s[i] == 0x07 { // BEL terminator
					break
				}
				// Check for ST (ESC \) terminator
				if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
					i++ // Skip the \
					break
				}
				i++
			}
		case '(', ')', '*', '+': // G0-G3 Character Set: ESC ( C
			// These are 3-byte sequences (ESC + designator + charset)
			i += 2
		default:
			// Assume standard 2-byte sequence (ESC M, ESC 7, ESC c, etc.)
			// We skip the ESC and the following character to prevent "ESC M" becoming "M".
			i++
		}
	}

	return b.String()
}

// collapseWhitespace reduces all contiguous whitespace to a single space.
func collapseWhitespace(s string) string {
	if !strings.ContainsAny(s, "\t\n\r\u00A0") && !strings.Contains(s, "  ") {
		return s
	}
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}
