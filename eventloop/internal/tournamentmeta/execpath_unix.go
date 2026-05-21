//go:build unix

package main

// normalizeExecutablePath is a no-op on POSIX platforms, where command -v and
// the filesystem already agree on executable paths.
func normalizeExecutablePath(path string) string {
	return path
}
