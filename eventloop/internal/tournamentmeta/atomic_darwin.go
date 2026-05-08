//go:build darwin

package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func promoteAtomicNew(temporary, target string) error {
	if err := unix.RenamexNp(temporary, target, unix.RENAME_EXCL); err != nil {
		return err
	}
	return syncAtomicDirectory(filepath.Dir(target))
}

func syncAtomicDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}
