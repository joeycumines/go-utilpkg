//go:build !unix && !windows

package main

import (
	"errors"
	"os"
)

type ownedProcess struct{}

func validateProcessSpec(processSpec) error {
	return nil
}

func startOwnedProcess(processSpec) (*ownedProcess, error) {
	return nil, errors.New("process-tree containment is unsupported on this target")
}

func (*ownedProcess) wait() error {
	return errors.New("process-tree containment is unsupported on this target")
}

func (*ownedProcess) forward(os.Signal) error {
	return errors.New("process-tree containment is unsupported on this target")
}

func (*ownedProcess) terminate() error {
	return errors.New("process-tree containment is unsupported on this target")
}

func (*ownedProcess) kill() error {
	return errors.New("process-tree containment is unsupported on this target")
}

func (*ownedProcess) alive() (bool, error) {
	return false, errors.New("process-tree containment is unsupported on this target")
}

func (*ownedProcess) close() error {
	return nil
}

func wrapperSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func wrapperSignalCode(os.Signal) int {
	return 1
}

func scopeInterruptAction() string {
	return "unsupported"
}

func scopeTimeoutAction() string {
	return "unsupported"
}

func scopeKind() string {
	return "unsupported"
}

func childProcessResult(error) (*int, string, int) {
	code := 1
	return &code, "", code
}
