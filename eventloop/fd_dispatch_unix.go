//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"sync"
	"sync/atomic"
)

// fdDispatchGate coordinates native callback dispatch with registration
// publication. starts tracks pending callback starts that must complete
// before a registration rollback or unregistration can safely retire the
// poller entry.
type fdDispatchGate struct {
	starts    sync.WaitGroup
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

func (g *fdDispatchGate) addPendingStart() {
	if g != nil {
		g.starts.Add(1)
	}
}

func (g *fdDispatchGate) dispatchStarted() {
	if g != nil {
		g.starts.Done()
	}
}

func (g *fdDispatchGate) waitPendingStarts() {
	if g != nil {
		g.starts.Wait()
	}
}
