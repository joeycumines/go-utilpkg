//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

var compareStringOrdinal = windows.NewLazySystemDLL("kernel32.dll").NewProc("CompareStringOrdinal")

func validatePrivateRunRoot(string) error {
	return nil
}

func validRunExecutable(info os.FileInfo) bool {
	return info.Mode().IsRegular()
}

func sameRunPath(left, right string) bool {
	equal, err := compareWindowsOrdinal(left, right)
	return err == nil && equal == 0
}

func sameRunEnvironmentKey(left, right string) (bool, error) {
	comparison, err := compareWindowsOrdinal(left, right)
	return comparison == 0, err
}

func validateRunEnvironmentPlatform(environment []string) error {
	foundSystemRoot := false
	previous := ""
	for index, record := range environment {
		key, value, _ := strings.Cut(record, "=")
		if !utf8.ValidString(record) {
			return fmt.Errorf("run environment record %q is not valid UTF-8", record)
		}
		if index != 0 {
			comparison, err := compareWindowsOrdinal(previous, key)
			if err != nil {
				return fmt.Errorf("compare Windows environment order: %w", err)
			}
			if comparison >= 0 {
				return fmt.Errorf("windows run environment is not strictly ordinal-case-insensitive sorted: %q before %q", previous, key)
			}
		}
		previous = key
		if strings.EqualFold(key, "SYSTEMROOT") {
			if value == "" {
				return errors.New("run environment SYSTEMROOT must not be empty")
			}
			foundSystemRoot = true
		}
	}
	if !foundSystemRoot {
		return errors.New("run environment requires explicit SYSTEMROOT")
	}
	return nil
}

func compareWindowsOrdinal(left, right string) (int, error) {
	if !utf8.ValidString(left) || !utf8.ValidString(right) {
		return 0, errors.New("ordinal comparison input is not valid UTF-8")
	}
	leftUTF16, err := windows.UTF16PtrFromString(left)
	if err != nil {
		return 0, err
	}
	rightUTF16, err := windows.UTF16PtrFromString(right)
	if err != nil {
		return 0, err
	}
	result, _, callErr := compareStringOrdinal.Call(
		uintptr(unsafe.Pointer(leftUTF16)),
		uintptr(^uint32(0)),
		uintptr(unsafe.Pointer(rightUTF16)),
		uintptr(^uint32(0)),
		1,
	)
	if result == 0 {
		if callErr == nil || errors.Is(callErr, syscall.Errno(0)) {
			callErr = errors.New("CompareStringOrdinal failed without an error code")
		}
		return 0, callErr
	}
	return int(result) - 2, nil
}
