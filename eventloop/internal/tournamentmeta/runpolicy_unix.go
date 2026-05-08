//go:build unix

package main

import (
	"fmt"
	"os"
)

func validatePrivateRunRoot(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect private run artifact root: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("run artifact root %q is not private: mode %04o", path, info.Mode().Perm())
	}
	return nil
}

func validRunExecutable(info os.FileInfo) bool {
	return info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func sameRunPath(left, right string) bool {
	return left == right
}

func sameRunEnvironmentKey(left, right string) (bool, error) {
	return left == right, nil
}

func validateRunEnvironmentPlatform([]string) error {
	return nil
}
