//go:build !unix && !windows

package main

import "os"

func validatePrivateRunRoot(string) error {
	return nil
}

func validRunExecutable(info os.FileInfo) bool {
	return info.Mode().IsRegular()
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
