package eventloop

import (
	"sync"
	"sync/atomic"
)

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
