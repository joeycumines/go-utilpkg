//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"syscall"
	"time"
)

// groupLivenessSettleWindow bounds how long an ambiguous process-group liveness
// probe is retried while the group finishes unregistering. groupLivenessSettleInterval
// spaces those probes.
const (
	groupLivenessSettleWindow   = 50 * time.Millisecond
	groupLivenessSettleInterval = time.Millisecond
)

type ownedProcess struct {
	command *exec.Cmd
	pid     int
}

func validateProcessSpec(spec processSpec) error {
	command := exec.Command(spec.Executable, spec.Arguments[1:]...)
	command.Dir = spec.Directory
	command.Env = spec.Environment
	if applied := command.Environ(); !slices.Equal(applied, spec.Environment) {
		return errors.New("applied environment differs from governed environment")
	}
	return nil
}

func startOwnedProcess(spec processSpec) (*ownedProcess, error) {
	command := exec.Command(spec.Executable, spec.Arguments[1:]...)
	command.Dir = spec.Directory
	command.Env = spec.Environment
	command.Stdin = spec.Stdin
	command.Stdout = spec.Stdout
	command.Stderr = spec.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &ownedProcess{command: command, pid: command.Process.Pid}, nil
}

func (process *ownedProcess) wait() error {
	return process.command.Wait()
}

func (process *ownedProcess) forward(signal os.Signal) error {
	native, ok := signal.(syscall.Signal)
	if !ok {
		return syscall.EINVAL
	}
	return process.signal(native)
}

func (process *ownedProcess) terminate() error {
	return process.signal(syscall.SIGTERM)
}

func (process *ownedProcess) kill() error {
	return process.signal(syscall.SIGKILL)
}

func (process *ownedProcess) signal(value syscall.Signal) error {
	err := syscall.Kill(-process.pid, value)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	// A group signal can report EPERM for a scope that is already tearing down.
	// Resolve it through the membership probe ownedProcess.alive uses so a
	// signal and a liveness observation of one scope never disagree.
	if errors.Is(err, syscall.EPERM) {
		if alive, aliveErr := process.alive(); aliveErr == nil && !alive {
			return nil
		}
	}
	return err
}

func (process *ownedProcess) alive() (bool, error) {
	err := syscall.Kill(-process.pid, 0)
	if err == nil {
		return true, nil
	}
	// Teardown makes repeated probes of one process group disagree: instrumented
	// runs show the same pgid reporting EPERM on successive probes and ESRCH
	// once the group unregisters. A single probe is therefore not decisive.
	if errors.Is(err, syscall.EPERM) {
		return process.settleGroupLiveness()
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	return false, err
}

// settleGroupLiveness re-probes an EPERM group until it answers ESRCH, which
// is the only observation that proves the scope is gone. A scope that never
// settles is reported live so containment stays conservative.
func (process *ownedProcess) settleGroupLiveness() (bool, error) {
	deadline := time.Now().Add(groupLivenessSettleWindow)
	for {
		err := syscall.Kill(-process.pid, 0)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, syscall.ESRCH):
			return false, nil
		case !errors.Is(err, syscall.EPERM):
			return false, err
		}
		if time.Now().After(deadline) {
			return true, nil
		}
		time.Sleep(groupLivenessSettleInterval)
	}
}

func (process *ownedProcess) close() error {
	return nil
}

func wrapperSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func wrapperSignalCode(signal os.Signal) int {
	if native, ok := signal.(syscall.Signal); ok {
		return 128 + int(native)
	}
	return 1
}

func scopeInterruptAction() string {
	return "forward-signal"
}

func scopeTimeoutAction() string {
	return "signal-term"
}

func scopeKind() string {
	return "unix-process-group"
}

func childProcessResult(err error) (*int, string, int) {
	if err == nil {
		code := 0
		return &code, "", code
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() {
				return nil, status.Signal().String(), 128 + int(status.Signal())
			}
			if status.Exited() {
				code := status.ExitStatus()
				return &code, "", code
			}
		}
		if code := exitError.ExitCode(); code >= 0 {
			return &code, "", code
		}
	}
	code := 1
	return &code, "", code
}
