//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import "testing"

func BenchmarkIdleWakeRecoveryTurnCost(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	if err := loop.ensurePoller(); err != nil {
		b.Fatal(err)
	}
	registerFDResourceCleanupT(b, loop)

	b.Run("empty-native-poll-turn", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := loop.poller.PollIO(0); err != nil {
				b.Fatal(err)
			}
			loop.poller.clearReadyEvents()
		}
	})
	b.Run("empty-fast-channel-probe", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			select {
			case <-loop.fastWakeupCh:
			default:
			}
		}
	})
}
