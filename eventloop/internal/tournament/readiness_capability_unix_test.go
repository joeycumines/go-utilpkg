//go:build darwin || linux

package tournament

import (
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestReadinessCapability(t *testing.T) {
	for _, impl := range Implementations() {
		if !impl.HasCapability(CapabilityReadiness) {
			continue
		}
		t.Run(impl.Name, func(t *testing.T) {
			readFD, writeFD := tournamentPipe(t)
			loop, cleanup := startTournamentTestLoop(t, impl)
			readinessLoop, ok := loop.(ReadinessEventLoop)
			if !ok {
				t.Fatalf("%s declares readiness capability without ReadinessEventLoop", impl.VariantID)
			}
			callbackErr := make(chan error, 1)
			done := make(chan struct{}, 1)
			if err := readinessLoop.RegisterReadable(readFD, func() {
				var buffer [1]byte
				_, err := unix.Read(readFD, buffer[:])
				callbackErr <- err
				done <- struct{}{}
			}); err != nil {
				t.Fatalf("RegisterReadable: %v", err)
			}
			registered := true
			t.Cleanup(func() {
				if registered {
					if err := readinessLoop.UnregisterReadiness(readFD); err != nil {
						t.Errorf("cleanup UnregisterReadiness: %v", err)
					}
				}
			})
			if _, err := unix.Write(writeFD, []byte{1}); err != nil {
				t.Fatalf("Write readiness byte: %v", err)
			}
			waitTournamentSignal(t, done, "readiness callback")
			if err := <-callbackErr; err != nil {
				t.Fatalf("read readiness byte: %v", err)
			}
			if err := readinessLoop.UnregisterReadiness(readFD); err != nil {
				t.Fatalf("UnregisterReadiness: %v", err)
			}
			registered = false
			cleanup()
		})
	}
}

func BenchmarkReadinessRoundTrip(b *testing.B) {
	for _, impl := range Implementations() {
		if !impl.HasCapability(CapabilityReadiness) {
			continue
		}
		b.Run(impl.Name, func(b *testing.B) {
			readFD, writeFD := tournamentPipe(b)
			loop, cleanup := startBenchmarkEventLoop(b, impl)
			defer cleanup()
			readinessLoop, ok := loop.(ReadinessEventLoop)
			if !ok {
				b.Fatalf("%s declares readiness capability without ReadinessEventLoop", impl.VariantID)
			}
			done := make(chan struct{}, 1)
			callbackErr := make(chan error, 1)
			if err := readinessLoop.RegisterReadable(readFD, func() {
				var buffer [1]byte
				_, err := unix.Read(readFD, buffer[:])
				callbackErr <- err
				done <- struct{}{}
			}); err != nil {
				b.Fatalf("RegisterReadable: %v", err)
			}
			var unregisterOnce sync.Once
			unregister := func() {
				unregisterOnce.Do(func() {
					if err := readinessLoop.UnregisterReadiness(readFD); err != nil {
						b.Errorf("UnregisterReadiness: %v", err)
					}
				})
			}
			defer unregister()
			deadline := time.NewTimer(30 * time.Minute)
			defer deadline.Stop()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := unix.Write(writeFD, []byte{1}); err != nil {
					b.Fatalf("Write readiness byte: %v", err)
				}
				waitBenchmarkDeadline(b, done, deadline.C, "readiness round trip")
				if err := <-callbackErr; err != nil {
					b.Fatalf("read readiness byte: %v", err)
				}
			}
			b.StopTimer()
			unregister()
		})
	}
}

func tournamentPipe(tb testing.TB) (int, int) {
	tb.Helper()
	var pipe [2]int
	if err := unix.Pipe(pipe[:]); err != nil {
		tb.Fatalf("Pipe: %v", err)
	}
	tb.Cleanup(func() {
		if err := unix.Close(pipe[0]); err != nil {
			tb.Errorf("close read descriptor: %v", err)
		}
		if err := unix.Close(pipe[1]); err != nil {
			tb.Errorf("close write descriptor: %v", err)
		}
	})
	return pipe[0], pipe[1]
}
