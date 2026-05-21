//go:build !unix && !windows

package main

// normalizeExecutablePath is a no-op on platforms without a POSIX shell that
// rewrites executable paths before the tool sees them.
func normalizeExecutablePath(path string) string {
	return path
}
