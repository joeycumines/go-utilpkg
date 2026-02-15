package alternatethree

// NewRegistryForTesting creates a new registry for testing purposes.
// This function is exported in test builds only.
func NewRegistryForTesting(t any) *registry {
	return newRegistry()
}

// PromiseInternal exposes the internal promise type for testing.
type PromiseInternal = promise
