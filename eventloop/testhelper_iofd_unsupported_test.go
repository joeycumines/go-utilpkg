//go:build plan9 || windows || ((js || wasip1) && wasm)

package eventloop

import "testing"

// testCreateIOFD skips public descriptor-readiness tests on task-only targets.
func testCreateIOFD(t *testing.T) (fd int, cleanup func()) {
	t.Helper()
	t.Skip("this target cannot preserve the public descriptor-readiness ownership contract")
	return -1, func() {}
}
