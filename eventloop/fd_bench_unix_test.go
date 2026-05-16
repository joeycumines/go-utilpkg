//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func startFDBenchmarkLoop(b *testing.B) (*Loop, func()) {
	b.Helper()
	return startBenchmarkLoop(b, WithFastPathMode(FastPathDisabled))
}

func TestFastPollerPollIOAllocations(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := poller.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	if _, err := poller.PollIO(0); err != nil {
		t.Fatalf("warm PollIO: %v", err)
	}

	var pollErr error
	allocs := testing.AllocsPerRun(1000, func() {
		_, pollErr = poller.PollIO(0)
	})
	if pollErr != nil {
		t.Fatalf("PollIO: %v", pollErr)
	}
	if allocs != 0 {
		t.Fatalf("PollIO allocations = %f, want 0", allocs)
	}
}

func BenchmarkFDReadinessDispatchSingle(b *testing.B) {
	loop, cleanup := startFDBenchmarkLoop(b)
	defer cleanup()

	r, w, err := os.Pipe()
	if err != nil {
		b.Fatalf("os.Pipe: %v", err)
	}
	registered := false
	var cleanupOnce sync.Once
	cleanupFD := func() {
		cleanupOnce.Do(func() {
			if registered {
				if err := loop.UnregisterFD(int(r.Fd())); err != nil {
					b.Errorf("UnregisterFD: %v", err)
				}
			}
			if err := r.Close(); err != nil {
				b.Errorf("reader Close: %v", err)
			}
			if err := w.Close(); err != nil {
				b.Errorf("writer Close: %v", err)
			}
		})
	}
	b.Cleanup(cleanupFD)

	events := make(chan error, 1)
	fd := int(r.Fd())
	if err := loop.RegisterFD(fd, EventRead, func(got IOEvents) {
		var buf [1]byte
		if got&EventRead == 0 {
			events <- fmt.Errorf("fd %d callback missing EventRead: %v", fd, got)
			return
		}
		n, err := r.Read(buf[:])
		if err != nil {
			events <- fmt.Errorf("fd %d read: %w", fd, err)
			return
		}
		if n != 1 || buf[0] != 1 {
			events <- fmt.Errorf("fd %d read = (%d, %d), want (1, 1)", fd, n, buf[0])
			return
		}
		events <- nil
	}); err != nil {
		b.Fatalf("RegisterFD: %v", err)
	}
	registered = true
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := w.Write([]byte{1}); err != nil {
			b.Fatalf("write pipe: %v", err)
		}
		select {
		case err := <-events:
			if err != nil {
				b.Fatalf("FD readiness callback: %v", err)
			}
		case <-deadline.C:
			b.Fatal("timed out waiting for single-FD readiness callback")
		}
	}
	b.StopTimer()
	cleanupFD()
}

func BenchmarkFDReadinessDispatchHighCount(b *testing.B) {
	const fdCount = 1000
	skipIfBenchmarkFDLimitBelow(b, 2*fdCount+64)
	loop, cleanup := startFDBenchmarkLoop(b)
	defer cleanup()

	type pipePair struct {
		r          *os.File
		w          *os.File
		registered bool
	}
	type fdEvent struct {
		err   error
		index int
	}
	pipes := make([]pipePair, fdCount)
	events := make(chan fdEvent, fdCount)
	cleanupPipes := func() {
		failed := false
		for i, p := range pipes {
			if p.r == nil && p.w == nil {
				continue
			}
			if p.registered {
				readFD := p.r.Fd()
				if err := loop.UnregisterFD(int(readFD)); err != nil {
					b.Errorf("UnregisterFD(%d): %v", readFD, err)
					failed = true
				}
			}
			if p.r != nil {
				readFD := p.r.Fd()
				if err := p.r.Close(); err != nil {
					b.Errorf("reader Close(%d): %v", readFD, err)
					failed = true
				}
			}
			if p.w != nil {
				writeFD := p.w.Fd()
				if err := p.w.Close(); err != nil {
					b.Errorf("writer Close(%d): %v", writeFD, err)
					failed = true
				}
			}
			pipes[i] = pipePair{}
		}
		if failed {
			b.FailNow()
		}
	}
	defer cleanupPipes()
	for i := range pipes {
		r, w, err := os.Pipe()
		if err != nil {
			b.Fatalf("os.Pipe %d: %v", i, err)
		}
		pipes[i] = pipePair{r: r, w: w}
		fd := int(r.Fd())
		index := i
		reader := r
		if err := loop.RegisterFD(fd, EventRead, func(got IOEvents) {
			var buf [1]byte
			if got&EventRead == 0 {
				events <- fdEvent{index: index, err: fmt.Errorf("fd %d callback missing EventRead: %v", fd, got)}
				return
			}
			n, err := reader.Read(buf[:])
			if err != nil {
				events <- fdEvent{index: index, err: fmt.Errorf("fd %d read: %w", fd, err)}
				return
			}
			if n != 1 || buf[0] != 1 {
				events <- fdEvent{index: index, err: fmt.Errorf("fd %d read = (%d, %d), want (1, 1)", fd, n, buf[0])}
				return
			}
			events <- fdEvent{index: index}
		}); err != nil {
			b.Fatalf("RegisterFD %d: %v", i, err)
		}
		pipes[i].registered = true
	}
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	seen := make([]uint64, fdCount)
	epoch := uint64(1)

	b.ReportAllocs()
	b.ReportMetric(fdCount, "fds/op")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range pipes {
			if _, err := p.w.Write([]byte{1}); err != nil {
				b.Fatalf("write pipe: %v", err)
			}
		}
		for range fdCount {
			var event fdEvent
			select {
			case event = <-events:
			case <-deadline.C:
				b.Fatal("timed out waiting for high-count FD readiness callbacks")
			}
			if event.err != nil {
				b.Fatalf("FD readiness callback: %v", event.err)
			}
			if event.index < 0 || event.index >= fdCount {
				b.Fatalf("FD readiness callback index %d outside [0,%d)", event.index, fdCount)
			}
			if seen[event.index] == epoch {
				b.Fatalf("FD readiness callback index %d dispatched more than once in iteration", event.index)
			}
			seen[event.index] = epoch
		}
		epoch++
		if epoch == 0 {
			clear(seen)
			epoch = 1
		}
	}
	b.StopTimer()
	b.ReportMetric(fdCount, "fds/op")
	cleanupPipes()
}

func skipIfBenchmarkFDLimitBelow(b *testing.B, needed int) {
	b.Helper()
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
		b.Logf("Getrlimit(RLIMIT_NOFILE) failed; continuing without descriptor-limit precheck: %v", err)
		return
	}
	if uint64(limit.Cur) < uint64(needed) {
		b.Skipf("requires at least %d file descriptors for the 1000-FD readiness benchmark; soft RLIMIT_NOFILE is %d", needed, limit.Cur)
	}
}

func BenchmarkSparseFDRegistration(b *testing.B) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		b.Fatal(err)
	}
	loopClosed := false
	b.Cleanup(func() {
		if !loopClosed {
			if err := loop.Close(); err != nil {
				b.Errorf("Close: %v", err)
			}
		}
	})

	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		b.Fatalf("Pipe: %v", err)
	}
	registerTestFDCleanupT(b, &pipeFDs[0], &pipeFDs[1])

	highFD := duplicateSparseBenchmarkFD(b, pipeFDs[1])
	registerTestFDCleanupT(b, &highFD)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := loop.RegisterFD(highFD, EventWrite, func(IOEvents) {}); err != nil {
			b.Fatalf("RegisterFD sparse fd %d: %v", highFD, err)
		}
		if err := loop.UnregisterFD(highFD); err != nil {
			b.Fatalf("UnregisterFD sparse fd %d: %v", highFD, err)
		}
	}
	b.StopTimer()
	if err := unix.Close(highFD); err != nil {
		b.Fatalf("Close sparse fd %d: %v", highFD, err)
	}
	highFD = -1
	if err := unix.Close(pipeFDs[0]); err != nil {
		b.Fatalf("Close pipe read fd %d: %v", pipeFDs[0], err)
	}
	pipeFDs[0] = -1
	if err := unix.Close(pipeFDs[1]); err != nil {
		b.Fatalf("Close pipe write fd %d: %v", pipeFDs[1], err)
	}
	pipeFDs[1] = -1
	if err := loop.Close(); err != nil {
		b.Fatalf("Close: %v", err)
	}
	loopClosed = true
}

func duplicateSparseBenchmarkFD(b *testing.B, fd int) int {
	b.Helper()
	dup, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD, maxFDs+32)
	if err != nil {
		b.Skipf("cannot allocate sparse benchmark fd above dense table: %v", err)
	}
	if dup < maxFDs {
		invalidFD := dup
		closeTestFDT(b, &dup)
		b.Fatalf("F_DUPFD returned fd %d below dense threshold %d", invalidFD, maxFDs)
	}
	return dup
}
