//go:build !darwin && !linux && !windows

package main

import (
	"errors"
	"os"
	"path/filepath"
)

func promoteAtomicNew(temporary, target string) error {
	if err := os.Link(temporary, target); err != nil {
		return err
	}
	if err := os.Remove(temporary); err != nil {
		return errors.Join(err, os.Remove(target))
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
