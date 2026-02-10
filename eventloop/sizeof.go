package eventloop

// These constants are verified via unit tests.
const (
	// sizeOfCacheLine is the size of a CPU cache line.
	// 64 bytes is standard for x86-64.
	// 128 bytes is standard for Apple Silicon (M1/M2/M3) and other ARM64.
	// We use 128 to satisfy the largest common alignment requirement.
	sizeOfCacheLine = 128

	// sizeOfAtomicUint64 is the size of an atomic.Uint64 variable.
	sizeOfAtomicUint64 = 8
)
