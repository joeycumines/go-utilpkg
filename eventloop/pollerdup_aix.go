//go:build aix && ppc64

package eventloop

func duplicatePollDescriptor(fd int) (int, error) {
	return duplicatePollDescriptorLegacy(fd)
}
