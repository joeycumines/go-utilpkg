//go:build aix && ppc64

package eventloop

type pollEventMask = uint16

func pollEventBits(mask pollEventMask) uint32 {
	return uint32(mask)
}
