package prompt

import (
	"bytes"
	"testing"
)

func TestPosixParserGetKey(t *testing.T) {
	scenarioTable := []struct {
		name     string
		input    []byte
		expected Key
	}{
		{
			name:     "escape",
			input:    []byte{0x1b},
			expected: Escape,
		},
		{
			name:     "undefined",
			input:    []byte{'a'},
			expected: NotDefined,
		},
	}

	for _, s := range scenarioTable {
		t.Run(s.name, func(t *testing.T) {
			key := GetKey(s.input)
			if key != s.expected {
				t.Errorf("Should be %s, but got %s", key, s.expected)
			}
		})
	}
}

// TestASCIISequencesSorted verifies that ASCIISequences is sorted from longest to shortest.
// This sorting is critical for correct key matching: if one sequence is a prefix of another
// (e.g., \x1b for ESC and \x1b[A for UpArrow), matching the shorter prefix first would
// lead to incorrect interpretation. By maintaining longest-to-shortest order, we ensure
// the most specific match is always attempted first.
func TestASCIISequencesSorted(t *testing.T) {
	// Verify the slice is not empty
	if len(ASCIISequences) == 0 {
		t.Fatal("ASCIISequences should not be empty")
	}

	// Verify sorting: each element should have length >= the next element
	for i := 0; i < len(ASCIISequences)-1; i++ {
		current := ASCIISequences[i]
		next := ASCIISequences[i+1]
		if len(current.ASCIICode) < len(next.ASCIICode) {
			t.Errorf("ASCIISequences not sorted: index %d (len=%d, key=%v) < index %d (len=%d, key=%v)",
				i, len(current.ASCIICode), current.Key,
				i+1, len(next.ASCIICode), next.Key)
		}
	}
}

// TestASCIISequencesNoPrefixAmbiguity verifies that no sequence is a prefix of a longer one
// that appears later in the sorted array. This is a safety check: with longest-first sorting,
// any prefix relationship should have the longer sequence before the shorter one.
func TestASCIISequencesNoPrefixAmbiguity(t *testing.T) {
	for i := 0; i < len(ASCIISequences); i++ {
		current := ASCIISequences[i]
		// Check all sequences that come after this one
		for j := i + 1; j < len(ASCIISequences); j++ {
			longer := ASCIISequences[j]
			// If current is shorter and is a prefix of longer, that's an error
			if len(current.ASCIICode) < len(longer.ASCIICode) &&
				bytes.HasPrefix(longer.ASCIICode, current.ASCIICode) {
				t.Errorf("Prefix ambiguity detected: sequence at index %d (len=%d, key=%v, bytes=%v) is a prefix of sequence at index %d (len=%d, key=%v, bytes=%v)",
					i, len(current.ASCIICode), current.Key, current.ASCIICode,
					j, len(longer.ASCIICode), longer.Key, longer.ASCIICode)
			}
		}
	}
}

// TestASCIISequencesPrefixMatching tests that longer sequences are matched before their prefixes.
// This is a functional test demonstrating why the sorting is necessary.
func TestASCIISequencesPrefixMatching(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected Key
		note     string
	}{
		{
			name:     "up_arrow_not_escape",
			input:    []byte{0x1b, 0x5b, 0x41}, // ESC [ A
			expected: Up,
			note:     "Should match Up, not Escape (even though 0x1b is a prefix)",
		},
		{
			name:     "down_arrow_not_escape",
			input:    []byte{0x1b, 0x5b, 0x42}, // ESC [ B
			expected: Down,
			note:     "Should match Down, not Escape",
		},
		{
			name:     "f1_long_sequence",
			input:    []byte{0x1b, 0x5b, 0x31, 0x7e},
			expected: Home, // This maps to Home in the current mapping
			note:     "Should match the full 4-byte sequence",
		},
		{
			name:     "control_up",
			input:    []byte{0x1b, 0x5b, 0x31, 0x3b, 0x35, 0x41},
			expected: ControlUp,
			note:     "Should match the full 6-byte sequence, not shorter prefixes",
		},
		{
			name:     "alt_backspace_not_escape",
			input:    []byte{0x1b, 0x7f},
			expected: AltBackspace,
			note:     "Should match AltBackspace, not Escape",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetKey(tt.input)
			if got != tt.expected {
				t.Errorf("%s: GetKey(%v) = %v, want %v",
					tt.note, tt.input, got, tt.expected)
			}
		})
	}
}
