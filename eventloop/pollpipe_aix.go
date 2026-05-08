//go:build aix && ppc64

package eventloop

func createPollPipe() (int, int, error) {
	return createPollPipeLegacy()
}
