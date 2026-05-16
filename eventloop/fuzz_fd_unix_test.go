//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func FuzzUnixFDReadinessAndLifecycle(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7})
	f.Add([]byte("pipe-read-register-modify-unregister"))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := newFuzzReader(data)
		loop, err := New(WithAutoExit(true))
		if err != nil {
			panic(err)
		}

		if got := captureErrorContractPanic(func() { _ = loop.RegisterFD(-1, EventRead, func(IOEvents) {}) }); got == nil {
			t.Fatal("RegisterFD(-1) did not panic")
		}
		if got := captureErrorContractPanic(func() { _ = loop.ModifyFD(-1, EventRead) }); got == nil {
			t.Fatal("ModifyFD(-1) did not panic")
		}
		if got := captureErrorContractPanic(func() { _ = loop.UnregisterFD(-1) }); got == nil {
			t.Fatal("UnregisterFD(-1) did not panic")
		}
		if err := loop.ModifyFD(0, EventRead); !errors.Is(err, ErrFDNotRegistered) {
			t.Fatalf("ModifyFD before registration = %v, want ErrFDNotRegistered", err)
		}
		if err := loop.UnregisterFD(0); !errors.Is(err, ErrFDNotRegistered) {
			t.Fatalf("UnregisterFD before registration = %v, want ErrFDNotRegistered", err)
		}

		var fds [2]int
		if err := unix.Pipe(fds[:]); err != nil {
			t.Fatalf("pipe: %v", err)
		}
		registerTestFDCleanupT(t, &fds[0], &fds[1])
		readFD, writeFD := fds[0], fds[1]
		if err := unix.SetNonblock(readFD, true); err != nil {
			t.Fatalf("set read nonblocking: %v", err)
		}
		if err := unix.SetNonblock(writeFD, true); err != nil {
			t.Fatalf("set write nonblocking: %v", err)
		}

		var callbackErrs fuzzErrs
		var callbacks int
		var observed IOEvents
		callback := func(events IOEvents) {
			callbacks++
			observed |= events
			if events&EventRead == 0 {
				callbackErrs.add("read callback observed events %v without EventRead", events)
			}
			var buf [256]byte
			for {
				n, err := unix.Read(readFD, buf[:])
				if n > 0 {
					continue
				}
				if err == nil || err == unix.EAGAIN || err == unix.EWOULDBLOCK {
					break
				}
				callbackErrs.add("read callback failed: %v", err)
				break
			}
			if err := loop.UnregisterFD(readFD); err != nil && !errors.Is(err, ErrFDNotRegistered) {
				callbackErrs.add("UnregisterFD from callback: %v", err)
			}
		}

		if err := loop.RegisterFD(readFD, EventRead, callback); err != nil {
			t.Fatalf("RegisterFD(pipe read): %v", err)
		}
		if err := loop.RegisterFD(readFD, EventRead, callback); !errors.Is(err, ErrFDAlreadyRegistered) {
			t.Fatalf("duplicate RegisterFD = %v, want ErrFDAlreadyRegistered", err)
		}
		if err := loop.ModifyFD(readFD, EventRead); err != nil {
			t.Fatalf("ModifyFD registered read fd: %v", err)
		}

		unregisterBeforeRun := r.bool()
		if unregisterBeforeRun {
			if err := loop.UnregisterFD(readFD); err != nil {
				t.Fatalf("pre-run UnregisterFD: %v", err)
			}
		}

		payloadLen := 1 + r.intn(128)
		payload := make([]byte, payloadLen)
		for i := range payload {
			payload[i] = byte(1 + r.byte())
		}
		if n, err := unix.Write(writeFD, payload); err != nil || n <= 0 {
			t.Fatalf("write pipe = (%d, %v), want positive write", n, err)
		}

		if err := runAutoExitLoop(t, loop); err != nil {
			t.Fatalf("Run: %v", err)
		}
		callbackErrs.failNow(t)

		if unregisterBeforeRun {
			if callbacks != 0 {
				t.Fatalf("callback fired after pre-run unregister: count=%d events=%v", callbacks, observed)
			}
			return
		}
		if callbacks != 1 {
			t.Fatalf("callback count = %d, want exactly 1", callbacks)
		}
		if observed&EventRead == 0 {
			t.Fatalf("observed events %v, want EventRead", observed)
		}
	})
}
