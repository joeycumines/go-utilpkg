//go:build plan9 || windows || ((js || wasip1) && wasm)

package eventloop

import "sync/atomic"

// fdDispatchGate on unsupported targets carries only the publication flag
// used by the cross-platform registration path. Wait-group coordination is
// unnecessary because native dispatch never starts.
type fdDispatchGate struct {
	published atomic.Bool
}

func newFDDispatchGate(published bool) *fdDispatchGate {
	gate := &fdDispatchGate{}
	gate.published.Store(published)
	return gate
}

func (g *fdDispatchGate) publish() {
	if g != nil {
		g.published.Store(true)
	}
}
