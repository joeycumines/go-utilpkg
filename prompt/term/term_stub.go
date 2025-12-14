//go:build !unix

package term

// This file exists so the package can be built on non-Unix targets
// where POSIX-specific files are excluded by build constraints.
