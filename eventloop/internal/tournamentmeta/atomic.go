package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func writeAtomicNew(path string, data []byte, mode os.FileMode) (err error) {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".tournament-atomic-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %q: %w", path, err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		if temporary != nil {
			err = errors.Join(err, temporary.Close())
		}
		if temporaryPath != "" {
			removeErr := os.Remove(temporaryPath)
			if !os.IsNotExist(removeErr) {
				err = errors.Join(err, removeErr)
			}
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		return fmt.Errorf("set temporary permissions for %q: %w", path, err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary file for %q: %w", path, err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary file for %q: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		temporary = nil
		return fmt.Errorf("close temporary file for %q: %w", path, err)
	}
	temporary = nil
	if err := promoteAtomicNew(temporaryPath, path); err != nil {
		return fmt.Errorf("promote atomic file %q: %w", path, err)
	}
	temporaryPath = ""
	return nil
}
