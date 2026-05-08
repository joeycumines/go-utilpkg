package main

import "testing"

func TestRejectDuplicateJSONKeys(t *testing.T) {
	for _, value := range []string{
		`{"value":1}`,
		`[{"value":1},{"value":2}]`,
		`{"nested":{"left":1,"right":2}}`,
	} {
		if err := rejectDuplicateJSONKeys([]byte(value)); err != nil {
			t.Errorf("rejectDuplicateJSONKeys(%s): %v", value, err)
		}
	}
	for _, value := range []string{
		`{"value":1,"value":2}`,
		`{"nested":{"value":1,"value":2}}`,
		`[{"value":1,"value":2}]`,
		`{} {}`,
	} {
		if err := rejectDuplicateJSONKeys([]byte(value)); err == nil {
			t.Errorf("rejectDuplicateJSONKeys(%s) unexpectedly passed", value)
		}
	}
}
