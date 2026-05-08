package oracle

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
)

// Command implements the oracle command-line contract.
func Command(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("goja-eventloop-oracle", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "path to the authenticated oracle manifest")
	nodeArchive := flags.String("node-archive", "", "official Node "+NodeTag+" .tar.gz archive for this host")
	validateOnly := flags.Bool("validate", false, "validate the manifest and authenticated assets without executing cases")
	if err := flags.Parse(args); err != nil {
		return ExitInvalidRun
	}
	if flags.NArg() != 0 || *manifestPath == "" {
		fmt.Fprintln(stderr, "goja-eventloop-oracle: -manifest is required and positional arguments are forbidden")
		return ExitInvalidRun
	}
	manifest, err := LoadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "goja-eventloop-oracle: %v\n", err)
		return ExitInvalidRun
	}
	if *validateOnly {
		record := struct {
			Type           string `json:"type"`
			Status         string `json:"status"`
			Schema         string `json:"schema"`
			ManifestSHA256 string `json:"manifestSHA256"`
			Cases          int    `json:"cases"`
			Surfaces       int    `json:"surfaces"`
		}{"validation", "pass", manifest.Manifest.Schema, manifest.SHA256, len(manifest.Manifest.Fixtures), len(manifest.Manifest.Surfaces)}
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(record); err != nil {
			fmt.Fprintf(stderr, "goja-eventloop-oracle: write validation record: %v\n", err)
			return ExitInvalidRun
		}
		return ExitPass
	}
	if *nodeArchive == "" {
		fmt.Fprintln(stderr, "goja-eventloop-oracle: -node-archive is required unless -validate is used")
		return ExitInvalidRun
	}
	return Run(ctx, manifest, *nodeArchive, stdout, stderr)
}
