//go:build !race

package alternatethree

// NewRegistryForTesting creates a new registry for testing purposes.
// This function is exported in test builds only.
func NewRegistryForTesting(t interface{}) *registry {
	return newRegistry()
}

// PromiseInternal exposes the internal promise type for testing.
type PromiseInternal = promise
