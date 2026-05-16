//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import "testing"

func TestRegisterFDUnregisterFDLinearization(t *testing.T) {
	boundaries := []struct {
		name    string
		install func(*loopTestHooks, func())
	}{
		{name: "rollback-check", install: func(hooks *loopTestHooks, block func()) { hooks.BeforeRegisterFDRollbackCheck = block }},
		{name: "commit", install: func(hooks *loopTestHooks, block func()) { hooks.BeforeRegisterFDCommit = block }},
	}

	for _, boundary := range boundaries {
		t.Run(boundary.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)

			fd, cleanup := testCreateIOFD(t)
			defer cleanup()

			atBoundary := make(chan struct{})
			releaseRegister := make(chan struct{})
			releaseRegisterFn := releaseSignalT(t, releaseRegister)
			unregisterAtLock := make(chan struct{})
			hooks := &loopTestHooks{
				BeforeFDUnregisterLock: func() { close(unregisterAtLock) },
			}
			boundary.install(hooks, func() {
				close(atBoundary)
				<-releaseRegister
			})
			loop.testHooks = hooks

			registerDone := make(chan error, 1)
			go func() { registerDone <- loop.RegisterFD(fd, EventRead, func(IOEvents) {}) }()
			waitContractSignal(t, atBoundary, "RegisterFD "+boundary.name+" boundary")

			unregisterDone := make(chan error, 1)
			go func() { unregisterDone <- loop.UnregisterFD(fd) }()
			waitContractSignal(t, unregisterAtLock, "UnregisterFD ownership-lock boundary")
			if loop.fdMu.TryLock() {
				loop.fdMu.Unlock()
				t.Fatalf("RegisterFD %s boundary did not retain FD ownership", boundary.name)
			}

			releaseRegisterFn()
			if err := waitContractValue(t, registerDone, "RegisterFD completion"); err != nil {
				t.Fatalf("RegisterFD after %s release: %v", boundary.name, err)
			}
			if err := waitContractValue(t, unregisterDone, "UnregisterFD completion"); err != nil {
				t.Fatalf("UnregisterFD after RegisterFD completed: %v", err)
			}
			if got := loop.userIOFDCount.Load(); got != 0 {
				t.Fatalf("final userIOFDCount = %d, want 0 after register-then-unregister", got)
			}
		})
	}
}
