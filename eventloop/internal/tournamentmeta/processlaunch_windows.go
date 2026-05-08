//go:build windows

package main

import (
	"errors"
	"fmt"
	"runtime"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

func validateProcessSpec(spec processSpec) error {
	if len(spec.Arguments) == 0 || spec.Arguments[0] != spec.Executable {
		return errors.New("Windows process argv[0] must equal the governed executable")
	}
	_, err := windowsEnvironmentBlock(spec.Environment)
	return err
}

func startOwnedProcess(spec processSpec) (*ownedProcess, error) {
	job, err := createRunJob()
	if err != nil {
		return nil, err
	}
	failJob := func(cause error) (*ownedProcess, error) {
		return nil, errors.Join(cause, annotateError("close failed-launch job", windows.CloseHandle(job)))
	}
	executable, err := windows.UTF16PtrFromString(spec.Executable)
	if err != nil {
		return failJob(fmt.Errorf("encode Windows executable: %w", err))
	}
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(spec.Arguments))
	if err != nil {
		return failJob(fmt.Errorf("encode Windows command line: %w", err))
	}
	directory, err := windows.UTF16PtrFromString(spec.Directory)
	if err != nil {
		return failJob(fmt.Errorf("encode Windows working directory: %w", err))
	}
	environment, err := windowsEnvironmentBlock(spec.Environment)
	if err != nil {
		return failJob(err)
	}
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return failJob(fmt.Errorf("allocate Windows process attribute list: %w", err))
	}
	defer attributes.Delete()
	handles, err := duplicateRunHandles(spec)
	if err != nil {
		return failJob(err)
	}
	closeDuplicates := func() error {
		var errs []error
		for index, handle := range handles {
			if handle == 0 {
				continue
			}
			if err := windows.CloseHandle(handle); err != nil {
				errs = append(errs, fmt.Errorf("close inherited handle %d: %w", index, err))
				continue
			}
			handles[index] = 0
		}
		return errors.Join(errs...)
	}
	if err := attributes.Update(
		windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST,
		unsafe.Pointer(&handles[0]),
		uintptr(len(handles))*unsafe.Sizeof(handles[0]),
	); err != nil {
		return failJob(errors.Join(
			fmt.Errorf("restrict Windows inherited handles: %w", err),
			closeDuplicates(),
		))
	}
	startup := windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb:        uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
			Flags:     windows.STARTF_USESTDHANDLES,
			StdInput:  handles[0],
			StdOutput: handles[1],
			StdErr:    handles[2],
		},
		ProcThreadAttributeList: attributes.List(),
	}
	var information windows.ProcessInformation
	err = windows.CreateProcess(
		executable,
		commandLine,
		nil,
		nil,
		true,
		windows.CREATE_SUSPENDED|
			windows.CREATE_UNICODE_ENVIRONMENT|
			windows.EXTENDED_STARTUPINFO_PRESENT|
			windows.CREATE_DEFAULT_ERROR_MODE|
			windows.CREATE_NEW_PROCESS_GROUP,
		&environment[0],
		directory,
		&startup.StartupInfo,
		&information,
	)
	runtime.KeepAlive(handles)
	if err != nil {
		return failJob(errors.Join(
			fmt.Errorf("create suspended Windows child: %w", err),
			closeDuplicates(),
		))
	}
	if err := closeDuplicates(); err != nil {
		return nil, abortWindowsLaunch(job, &information, false, errors.Join(err, closeDuplicates()))
	}
	if err := windows.AssignProcessToJobObject(job, information.Process); err != nil {
		return nil, abortWindowsLaunch(
			job,
			&information,
			false,
			fmt.Errorf("assign suspended child to job object: %w", err),
		)
	}
	previous, err := windows.ResumeThread(information.Thread)
	if err != nil || previous != 1 {
		return nil, abortWindowsLaunch(
			job,
			&information,
			true,
			errors.Join(
				annotateError("resume assigned child primary thread", err),
				resumeCountError(previous, err),
			),
		)
	}
	if err := windows.CloseHandle(information.Thread); err != nil {
		return nil, abortWindowsLaunch(
			job,
			&information,
			true,
			fmt.Errorf("close assigned child primary thread: %w", err),
		)
	}
	information.Thread = 0
	return &ownedProcess{job: job, process: information.Process}, nil
}

func createRunJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("create job object: %w", err)
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits)),
	); err != nil {
		return 0, errors.Join(
			fmt.Errorf("configure job object: %w", err),
			annotateError("close unconfigured job object", windows.CloseHandle(job)),
		)
	}
	return job, nil
}

func duplicateRunHandles(spec processSpec) ([]windows.Handle, error) {
	sources := []windows.Handle{
		windows.Handle(spec.Stdin.Fd()),
		windows.Handle(spec.Stdout.Fd()),
		windows.Handle(spec.Stderr.Fd()),
	}
	targets := make([]windows.Handle, len(sources))
	current := windows.CurrentProcess()
	for index, source := range sources {
		if err := windows.DuplicateHandle(
			current,
			source,
			current,
			&targets[index],
			0,
			true,
			windows.DUPLICATE_SAME_ACCESS,
		); err != nil {
			var closeErrs []error
			for previous := 0; previous < index; previous++ {
				closeErrs = append(closeErrs, windows.CloseHandle(targets[previous]))
			}
			return nil, errors.Join(
				fmt.Errorf("duplicate standard handle %d: %w", index, err),
				errors.Join(closeErrs...),
			)
		}
	}
	return targets, nil
}

func windowsEnvironmentBlock(environment []string) ([]uint16, error) {
	block := make([]uint16, 0)
	for _, record := range environment {
		if !utf8.ValidString(record) {
			return nil, fmt.Errorf("Windows environment record is not valid UTF-8: %q", record)
		}
		if len(record) == 0 {
			return nil, errors.New("Windows environment contains an empty record")
		}
		block = append(block, utf16.Encode([]rune(record))...)
		block = append(block, 0)
	}
	block = append(block, 0)
	return block, nil
}

func abortWindowsLaunch(
	job windows.Handle,
	information *windows.ProcessInformation,
	assigned bool,
	cause error,
) error {
	var errs []error
	errs = append(errs, cause)
	if assigned {
		errs = append(errs, annotateError("terminate failed-launch job", windows.TerminateJobObject(job, 125)))
	} else if information.Process != 0 {
		errs = append(errs, annotateError("terminate failed-launch process", windows.TerminateProcess(information.Process, 125)))
	}
	if information.Process != 0 {
		event, err := windows.WaitForSingleObject(
			information.Process,
			windowsDeadlineMilliseconds(processKillDeadline),
		)
		if err != nil {
			errs = append(errs, fmt.Errorf("wait for failed-launch process: %w", err))
		} else if event != windows.WAIT_OBJECT_0 {
			errs = append(errs, fmt.Errorf("wait for failed-launch process returned event %#x", event))
		}
	}
	if information.Thread != 0 {
		errs = append(errs, annotateError("close failed-launch thread", windows.CloseHandle(information.Thread)))
		information.Thread = 0
	}
	if information.Process != 0 {
		errs = append(errs, annotateError("close failed-launch process", windows.CloseHandle(information.Process)))
		information.Process = 0
	}
	errs = append(errs, annotateError("close failed-launch job", windows.CloseHandle(job)))
	return errors.Join(errs...)
}

func windowsDeadlineMilliseconds(value time.Duration) uint32 {
	milliseconds := (value + time.Millisecond - 1) / time.Millisecond
	if milliseconds < 1 {
		return 1
	}
	if milliseconds >= time.Duration(windows.INFINITE) {
		return windows.INFINITE - 1
	}
	return uint32(milliseconds)
}

func resumeCountError(previous uint32, resumeErr error) error {
	if resumeErr == nil && previous == 1 {
		return nil
	}
	return fmt.Errorf("resume assigned child previous suspend count = %d, want 1", previous)
}
