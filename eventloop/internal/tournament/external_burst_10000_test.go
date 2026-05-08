package tournament

import "testing"

// TestExternalBurst10000 verifies exact callback conservation for a 10,000-task burst.
func TestExternalBurst10000(t *testing.T) {
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			testExternalBurst(t, impl, 10000)
		})
	}
}
