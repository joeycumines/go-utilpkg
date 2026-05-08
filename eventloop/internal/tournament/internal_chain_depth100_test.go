package tournament

import "testing"

// TestInternalChainDepth100 verifies a deeper recursive internal-priority chain.
func TestInternalChainDepth100(t *testing.T) {
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			testInternalChain(t, impl, 100)
		})
	}
}
