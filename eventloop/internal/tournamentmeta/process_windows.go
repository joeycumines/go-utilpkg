//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

type ownedProcess struct {
	job      windows.Handle
	process  windows.Handle
	waitDone atomic.Bool
}

type windowsProcessExitError struct {
	code uint32
}

func (err windowsProcessExitError) Error() string {
	return fmt.Sprintf("exit status %d", err.code)
}

type jobObjectBasicAccountingInformation struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

func (process *ownedProcess) wait() error {
	event, err := windows.WaitForSingleObject(process.process, windows.INFINITE)
	if err != nil {
		return fmt.Errorf("wait for direct child: %w", err)
	}
	if event != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("wait for direct child returned event %#x", event)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(process.process, &code); err != nil {
		return fmt.Errorf("read direct child exit code: %w", err)
	}
	process.waitDone.Store(true)
	if code != 0 {
		return windowsProcessExitError{code: code}
	}
	return nil
}

func (process *ownedProcess) forward(os.Signal) error {
	return process.terminateJob(130)
}

func (process *ownedProcess) terminate() error {
	return process.terminateJob(124)
}

func (process *ownedProcess) kill() error {
	return process.terminateJob(125)
}

func (process *ownedProcess) terminateJob(code uint32) error {
	err := windows.TerminateJobObject(process.job, code)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		alive, queryErr := process.alive()
		if queryErr == nil && !alive {
			return nil
		}
	}
	return err
}

func (process *ownedProcess) alive() (bool, error) {
	var information jobObjectBasicAccountingInformation
	err := windows.QueryInformationJobObject(
		process.job,
		windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&information)),
		uint32(unsafe.Sizeof(information)),
		nil,
	)
	return information.ActiveProcesses != 0, err
}

func (process *ownedProcess) close() error {
	if !process.waitDone.Load() {
		return errors.New("direct child wait did not complete before process-scope close")
	}
	processErr := windows.CloseHandle(process.process)
	process.process = 0
	jobErr := windows.CloseHandle(process.job)
	process.job = 0
	return errors.Join(
		annotateError("close direct child process handle", processErr),
		annotateError("close child job handle", jobErr),
	)
}

func wrapperSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func wrapperSignalCode(os.Signal) int {
	return 130
}

func scopeInterruptAction() string {
	return "job-object-terminate-130"
}

func scopeTimeoutAction() string {
	return "job-object-terminate-124"
}

func childProcessResult(err error) (*int, string, int) {
	if err == nil {
		code := 0
		return &code, "", code
	}
	var exitError windowsProcessExitError
	if errors.As(err, &exitError) {
		code := int(exitError.code)
		return &code, "", code
	}
	code := 1
	return &code, "", code
}

func scopeKind() string {
	return "windows-job-object"
}
