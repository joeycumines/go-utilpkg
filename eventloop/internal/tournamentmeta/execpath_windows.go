//go:build windows

package main

import (
	"os"
	"path/filepath"
)

// normalizeExecutablePath resolves an executable path supplied on the command
// line to a form the Windows filesystem APIs accept. When tournamentmeta is
// launched from a POSIX-compatible shell such as Git bash (the only way to run
// the POSIX-style make recipes on Windows), tools like command -v report paths
// in MSYS form (/c/Program Files/Go/bin/go) without the .exe suffix. Go's
// filesystem calls reject both: the /c/ drive prefix is not a Windows path, and
// the executable file is go.exe. This restores the Windows-native form so the
// authority's EvalSymlinks check succeeds without requiring every caller to
// know which shell launched the process. It only rewrites the leading MSYS
// drive prefix and adds the executable suffix; absolute Windows paths and paths
// that already carry a suffix pass through unchanged.
func normalizeExecutablePath(path string) string {
	resolved := path
	if len(resolved) >= 3 && resolved[0] == '/' && isASCIILetter(resolved[1]) && (resolved[2] == '/' || resolved[2] == '\\') {
		resolved = string(toUpperASCII(resolved[1])) + ":" + resolved[2:]
	}
	if filepath.Ext(resolved) == "" {
		if _, err := os.Stat(resolved); err != nil {
			withSuffix := resolved + ".exe"
			if _, err := os.Stat(withSuffix); err == nil {
				resolved = withSuffix
			}
		}
	}
	return resolved
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func toUpperASCII(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - ('a' - 'A')
	}
	return b
}
