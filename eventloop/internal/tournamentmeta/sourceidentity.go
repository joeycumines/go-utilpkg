package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
)

const (
	sharedSourceDomain           = "go-utilpkg-eventloop-tournament-shared-source-v1"
	sourceCaptureDomain          = "go-utilpkg-eventloop-tournament-source-capture-v1"
	sourceCaptureAuthorityDomain = "go-utilpkg-eventloop-tournament-source-capture-authority-v1"
	sourceCapturePolicy          = "go-tool-enumerator-v1"
)

type sourceLogicalAuthority struct { // betteralign:ignore canonical JSON field order
	EnumerationPolicy             string                `json:"enumeration_policy"`
	ManifestPath                  string                `json:"manifest_path"`
	ManifestSHA256                string                `json:"manifest_sha256"`
	ManifestSourceAuthoritySHA256 string                `json:"manifest_source_authority_sha256"`
	BuildCells                    []sourceCellAuthority `json:"build_cells"`
	PhysicalPaths                 sourcePathSet         `json:"physical_paths"`
	BuildUnion                    sourcePathSet         `json:"build_union"`
	GovernedUnion                 sourcePathSet         `json:"governed_union"`
	EnvironmentPolicy             string                `json:"environment_policy"`
	ModuleMode                    string                `json:"module_mode"`
	WorkspaceMode                 string                `json:"workspace_mode"`
	ProxyMode                     string                `json:"proxy_mode"`
	ToolchainMode                 string                `json:"toolchain_mode"`
	BuildVCS                      bool                  `json:"build_vcs"`
}

type sourceCaptureAuthority struct {
	Policy string       `json:"policy"`
	GoTool sourceGoTool `json:"go_tool"`
}

type sourceIdentity struct { // betteralign:ignore canonical JSON field order
	SchemaVersion          int    `json:"schema_version"`
	SharedSourceID         string `json:"shared_source_id"`
	CaptureID              string `json:"capture_id"`
	CaptureAuthoritySHA256 string `json:"capture_authority_sha256"`
	LegacyV4Fingerprint    string `json:"legacy_v4_fingerprint"`
}

func sourceIdentityCommand(arguments []string) int {
	flags := flag.NewFlagSet("source-identity", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("root", "", "live repository root")
	format := flags.String("format", "json", "json, shared-id, capture-id, or legacy-v4")
	buildFlags := registerSourceBuildFlags(flags)
	if err := flags.Parse(arguments); err != nil {
		return commandError(err)
	}
	if flags.NArg() != 0 || *root == "" {
		return commandError(errors.New("source-identity requires -root and accepts no positional arguments"))
	}
	if !sourceIdentityFormatSupported(*format) {
		return commandError(fmt.Errorf("source-identity format %q is unsupported", *format))
	}
	config := buildFlags.config()
	identity, err := stableSourceIdentity(
		*root,
		func() (sourceCapture, error) { return governedSourceCapture(*root, config) },
		inspectSourceRecords,
	)
	if err != nil {
		return commandError(err)
	}
	output, err := formatSourceIdentity(*format, identity)
	if err != nil {
		return commandError(err)
	}
	if _, err := os.Stdout.Write(output); err != nil {
		return commandError(fmt.Errorf("write source identity: %w", err))
	}
	return 0
}

func sourceIdentityFormatSupported(format string) bool {
	switch format {
	case "json", "shared-id", "capture-id", "legacy-v4", "raw-v2":
		return true
	default:
		return false
	}
}

func formatSourceIdentity(format string, identity sourceIdentity) ([]byte, error) {
	var output string
	switch format {
	case "json":
		data, err := json.Marshal(identity)
		if err != nil {
			return nil, fmt.Errorf("encode source identity: %w", err)
		}
		return append(data, '\n'), nil
	case "shared-id":
		output = identity.SharedSourceID
	case "capture-id":
		output = identity.CaptureID
	case "legacy-v4":
		output = identity.LegacyV4Fingerprint
	case "raw-v2":
		output = fmt.Sprintf(
			"tournament: meta=shared-source-id=%s\n"+
				"tournament: meta=capture-id=%s\n"+
				"tournament: meta=capture-authority-sha256=%s\n"+
				"tournament: meta=legacy-v4-fingerprint=%s",
			identity.SharedSourceID,
			identity.CaptureID,
			identity.CaptureAuthoritySHA256,
			identity.LegacyV4Fingerprint,
		)
	default:
		return nil, fmt.Errorf("source-identity format %q is unsupported", format)
	}
	return []byte(output + "\n"), nil
}

func stableSourceIdentity(
	root string,
	capture func() (sourceCapture, error),
	inspect func(string, []string) ([]sourceRecord, error),
) (sourceIdentity, error) {
	startCapture, err := capture()
	if err != nil {
		return sourceIdentity{}, err
	}
	startRecords, err := inspect(root, startCapture.Files)
	if err != nil {
		return sourceIdentity{}, err
	}
	startIdentity, err := identifySource(startCapture.Authority, startRecords)
	if err != nil {
		return sourceIdentity{}, err
	}
	endCapture, err := capture()
	if err != nil {
		return sourceIdentity{}, err
	}
	if !reflect.DeepEqual(endCapture, startCapture) {
		return sourceIdentity{}, errors.New("governed source authority or file set changed during identity capture")
	}
	endRecords, err := inspect(root, endCapture.Files)
	if err != nil {
		return sourceIdentity{}, err
	}
	endIdentity, err := identifySource(endCapture.Authority, endRecords)
	if err != nil {
		return sourceIdentity{}, err
	}
	if !reflect.DeepEqual(endIdentity, startIdentity) {
		return sourceIdentity{}, errors.New("governed source records changed during identity capture")
	}
	return startIdentity, nil
}

func identifySource(authority sourceAuthority, records []sourceRecord) (sourceIdentity, error) {
	legacy, err := fingerprintSource(authority, records)
	if err != nil {
		return sourceIdentity{}, err
	}
	logical := logicalSourceAuthority(authority)
	logicalData, err := json.Marshal(logical)
	if err != nil {
		return sourceIdentity{}, fmt.Errorf("encode logical source authority: %w", err)
	}
	recordFingerprint, err := fingerprintRecords(records)
	if err != nil {
		return sourceIdentity{}, err
	}
	shared := sha256.New()
	writeFingerprintFrame(shared, []byte(sharedSourceDomain))
	writeFingerprintFrame(shared, logicalData)
	writeFingerprintFrame(shared, []byte(recordFingerprint))
	sharedID := hex.EncodeToString(shared.Sum(nil))

	capture := sourceCaptureAuthority{Policy: sourceCapturePolicy, GoTool: authority.GoTool}
	if err := validateSourceCaptureAuthority(capture); err != nil {
		return sourceIdentity{}, err
	}
	captureData, err := json.Marshal(capture)
	if err != nil {
		return sourceIdentity{}, fmt.Errorf("encode source capture authority: %w", err)
	}
	captureAuthority := sha256.New()
	writeFingerprintFrame(captureAuthority, []byte(sourceCaptureAuthorityDomain))
	writeFingerprintFrame(captureAuthority, captureData)
	captureAuthoritySHA256 := hex.EncodeToString(captureAuthority.Sum(nil))
	captureDigest := sha256.New()
	writeFingerprintFrame(captureDigest, []byte(sourceCaptureDomain))
	writeFingerprintFrame(captureDigest, []byte(sharedID))
	writeFingerprintFrame(captureDigest, []byte(captureAuthoritySHA256))
	return sourceIdentity{
		SchemaVersion:          1,
		SharedSourceID:         sharedID,
		CaptureID:              hex.EncodeToString(captureDigest.Sum(nil)),
		CaptureAuthoritySHA256: captureAuthoritySHA256,
		LegacyV4Fingerprint:    legacy,
	}, nil
}

func identifySnapshotSource(authority sourceAuthority, records []sourceRecord) (sourceIdentity, error) {
	if authority.EnumerationPolicy == testSourcePolicy {
		legacy, err := fingerprintSource(authority, records)
		if err != nil {
			return sourceIdentity{}, err
		}
		return sourceIdentity{LegacyV4Fingerprint: legacy}, nil
	}
	return identifySource(authority, records)
}

func logicalSourceAuthority(authority sourceAuthority) sourceLogicalAuthority {
	return sourceLogicalAuthority{
		EnumerationPolicy:             authority.EnumerationPolicy,
		ManifestPath:                  authority.ManifestPath,
		ManifestSHA256:                authority.ManifestSHA256,
		ManifestSourceAuthoritySHA256: authority.ManifestSourceAuthoritySHA256,
		BuildCells:                    authority.BuildCells,
		PhysicalPaths:                 authority.PhysicalPaths,
		BuildUnion:                    authority.BuildUnion,
		GovernedUnion:                 authority.GovernedUnion,
		EnvironmentPolicy:             authority.EnvironmentPolicy,
		ModuleMode:                    authority.ModuleMode,
		WorkspaceMode:                 authority.WorkspaceMode,
		ProxyMode:                     authority.ProxyMode,
		ToolchainMode:                 authority.ToolchainMode,
		BuildVCS:                      authority.BuildVCS,
	}
}

func combineSourceAuthority(logical sourceLogicalAuthority, capture sourceCaptureAuthority) sourceAuthority {
	return sourceAuthority{
		EnumerationPolicy:             logical.EnumerationPolicy,
		ManifestPath:                  logical.ManifestPath,
		ManifestSHA256:                logical.ManifestSHA256,
		ManifestSourceAuthoritySHA256: logical.ManifestSourceAuthoritySHA256,
		GoTool:                        capture.GoTool,
		BuildCells:                    logical.BuildCells,
		PhysicalPaths:                 logical.PhysicalPaths,
		BuildUnion:                    logical.BuildUnion,
		GovernedUnion:                 logical.GovernedUnion,
		EnvironmentPolicy:             logical.EnvironmentPolicy,
		ModuleMode:                    logical.ModuleMode,
		WorkspaceMode:                 logical.WorkspaceMode,
		ProxyMode:                     logical.ProxyMode,
		ToolchainMode:                 logical.ToolchainMode,
		BuildVCS:                      logical.BuildVCS,
	}
}

func validateSourceCaptureAuthority(authority sourceCaptureAuthority) error {
	if authority.Policy != sourceCapturePolicy {
		return fmt.Errorf("source capture policy = %q, want %q", authority.Policy, sourceCapturePolicy)
	}
	return validateSourceGoTool(authority.GoTool)
}
