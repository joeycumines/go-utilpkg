//go:build windows

package main

import "os"

// authorityPerm reduces a filesystem mode to the permission bits that the
// tamper-evidence authority treats as load-bearing for a stable regular file.
//
// NTFS does not store the POSIX permission set. Go's runtime reports the
// writable bits of every writable file as 0o666 and maps the Windows read-only
// attribute to clearing the owner write bit (0o200). Every other POSIX bit
// (group, other, setuid, and the execute bits) is fabricated and therefore not
// a reliable authority signal. The only distinction NTFS can physically
// preserve is whether the owner may write, so the authority reduces both the
// observed mode and the expected permission to that single bit. This mirrors
// the existing runpolicy_windows.go decision to drop execute-bit checks.
func authorityPerm(mode os.FileMode) os.FileMode {
	return mode.Perm() & 0o200
}
