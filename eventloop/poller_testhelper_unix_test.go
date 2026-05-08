//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func (p *fastPoller) appendReadyEvent(fd int, events IOEvents) bool {
	p.fdMu.RLock()
	info, ok := p.fdInfoLocked(fd)
	p.fdMu.RUnlock()
	if !ok || info.callback == nil {
		return false
	}
	p.readyMu.Lock()
	defer p.readyMu.Unlock()
	if p.closed.Load() {
		return false
	}
	p.readyEvents = append(p.readyEvents, pollEvent{fd: fd, events: events, generation: info.generation, internal: info.internal})
	return true
}
func requireDescriptorClosed(t *testing.T, fd int) {
	t.Helper()
	if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
		t.Fatalf("descriptor %d remains open: %v", fd, err)
	}
}

func registerPollerCleanupT(t *testing.T, poller *fastPoller) {
	t.Helper()
	t.Cleanup(func() {
		if err := poller.Close(); err != nil {
			t.Errorf("fastPoller cleanup Close: %v", err)
		}
	})
}
