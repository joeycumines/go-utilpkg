//go:build solaris && amd64

package eventloop

type pollEventMask = int16

func pollEventBits(mask pollEventMask) uint32 {
	return uint32(uint16(mask))
}
