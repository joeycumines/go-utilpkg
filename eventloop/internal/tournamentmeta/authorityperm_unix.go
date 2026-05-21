//go:build unix

package main

import "os"

// authorityPerm reduces a filesystem mode to the permission bits that the
// tamper-evidence authority treats as load-bearing for a stable regular file.
// On POSIX hosts the full permission set is physically represented by the
// filesystem, so every bit is authoritative and the identity is exact.
func authorityPerm(mode os.FileMode) os.FileMode {
	return mode.Perm()
}
