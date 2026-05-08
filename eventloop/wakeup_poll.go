//go:build (aix && ppc64) || (solaris && amd64)

package eventloop

// createWakeFD creates the nonblocking, close-on-exec self-pipe used to wake
// the poll backend through the loop's ordinary internal readiness callback.
func createWakeFD() (int, int, error) {
	return createPollPipe()
}
