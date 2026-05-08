// Command tournamentmeta produces and verifies eventloop tournament metadata.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		exitUsage("missing command")
	}

	var code int
	switch os.Args[1] {
	case "snapshot":
		code = snapshotCommand(os.Args[2:])
	case "source-fingerprint":
		code = sourceFingerprintCommand(os.Args[2:])
	case "source-identity":
		code = sourceIdentityCommand(os.Args[2:])
	case "profile":
		code = profileCommand(os.Args[2:])
	case "host":
		code = hostCommand(os.Args[2:])
	case "history":
		code = historyCommand(os.Args[2:])
	case "lineage":
		code = lineageCommand(os.Args[2:])
	case "run":
		code = runCommand(os.Args[2:])
	case "sha256":
		code = sha256Command(os.Args[2:])
	default:
		exitUsage("unknown command %q", os.Args[1])
	}
	os.Exit(code)
}

func exitUsage(format string, values ...any) {
	fmt.Fprintf(os.Stderr, "tournamentmeta: "+format+"\n", values...)
	fmt.Fprintln(os.Stderr, "usage: tournamentmeta <snapshot|source-fingerprint|source-identity|profile|host|history|lineage|run|sha256> [arguments]")
	os.Exit(2)
}

func commandError(err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintf(os.Stderr, "tournamentmeta: %v\n", err)
	return 1
}
