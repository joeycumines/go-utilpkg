//go:build !unix && !windows

package main

import "os"

// authorityPerm reduces a filesystem mode to the permission bits that the
// tamper-evidence authority treats as load-bearing for a stable regular file.
// On platforms that do not provide a POSIX permission model (and are not
// Windows), no permission bit is a reliable authority signal, so the authority
// collapses every mode to a single zero value that always compares equal. This
// keeps the file-identity checks (regular type, size, and same-file identity)
// authoritative while avoiding false rejections on hosts that cannot honor
// permission bits at all.
func authorityPerm(mode os.FileMode) os.FileMode {
	_ = mode
	return 0
}
