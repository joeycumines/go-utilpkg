package main

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestSourceV4FingerprintGolden(t *testing.T) {
	digest := sha256.Sum256([]byte("abc"))
	records := []sourceRecord{{
		Path:   "eventloop/a.go",
		Mode:   "100644",
		Size:   3,
		SHA256: fmt.Sprintf("%x", digest),
	}}
	got, err := fingerprintSource(fixtureSourceAuthority(), records)
	if err != nil {
		t.Fatalf("fingerprintSource: %v", err)
	}
	const want = "b384dc159fecfcb40b7ac64514512e91507960beeee49d521e8cf6dbbbcf17da"
	if got != want {
		t.Fatalf("v4 fingerprint = %s, want %s", got, want)
	}
}

func TestFormatSourceIdentityRawV2(t *testing.T) {
	identity := sourceIdentity{
		SchemaVersion:          1,
		SharedSourceID:         strings.Repeat("a", 64),
		CaptureID:              strings.Repeat("b", 64),
		CaptureAuthoritySHA256: strings.Repeat("c", 64),
		LegacyV4Fingerprint:    strings.Repeat("d", 64),
	}
	got, err := formatSourceIdentity("raw-v2", identity)
	if err != nil {
		t.Fatalf("formatSourceIdentity: %v", err)
	}
	want := "tournament: meta=shared-source-id=" + strings.Repeat("a", 64) + "\n" +
		"tournament: meta=capture-id=" + strings.Repeat("b", 64) + "\n" +
		"tournament: meta=capture-authority-sha256=" + strings.Repeat("c", 64) + "\n" +
		"tournament: meta=legacy-v4-fingerprint=" + strings.Repeat("d", 64) + "\n"
	if string(got) != want {
		t.Fatalf("raw-v2 source identity = %q, want %q", got, want)
	}
	if _, err := formatSourceIdentity("unknown", identity); err == nil {
		t.Fatal("unsupported source identity format unexpectedly passed")
	}
}

func TestSourceIdentitySeparatesLogicalAndCaptureAuthority(t *testing.T) {
	repository, config := testGovernedSourceRepository(t)
	capture, err := governedSourceCapture(repository, config)
	if err != nil {
		t.Fatalf("governedSourceCapture: %v", err)
	}
	records, err := inspectSourceRecords(repository, capture.Files)
	if err != nil {
		t.Fatalf("inspectSourceRecords: %v", err)
	}
	baseline, err := identifySource(capture.Authority, records)
	if err != nil {
		t.Fatalf("identifySource: %v", err)
	}

	toolChanged := capture.Authority
	toolChanged.GoTool.GOHostOS = "different-host"
	changedCapture, err := identifySource(toolChanged, records)
	if err != nil {
		t.Fatalf("identifySource tool change: %v", err)
	}
	if changedCapture.SharedSourceID != baseline.SharedSourceID {
		t.Fatal("capture-host change altered shared source ID")
	}
	if changedCapture.CaptureID == baseline.CaptureID ||
		changedCapture.CaptureAuthoritySHA256 == baseline.CaptureAuthoritySHA256 ||
		changedCapture.LegacyV4Fingerprint == baseline.LegacyV4Fingerprint {
		t.Fatal("capture-host change did not alter capture authority, capture, and legacy identities")
	}

	logicalChanged := capture.Authority
	logicalChanged.BuildCells = slices.Clone(capture.Authority.BuildCells)
	logicalChanged.BuildCells[0].Argv = slices.Clone(logicalChanged.BuildCells[0].Argv)
	logicalChanged.BuildCells[0].Argv = append(logicalChanged.BuildCells[0].Argv, "./changed")
	changedLogical, err := identifySource(logicalChanged, records)
	if err != nil {
		t.Fatalf("identifySource logical change: %v", err)
	}
	if changedLogical.SharedSourceID == baseline.SharedSourceID ||
		changedLogical.CaptureID == baseline.CaptureID ||
		changedLogical.LegacyV4Fingerprint == baseline.LegacyV4Fingerprint {
		t.Fatal("logical authority change did not alter every linked identity")
	}
	if changedLogical.CaptureAuthoritySHA256 != baseline.CaptureAuthoritySHA256 {
		t.Fatal("logical authority change altered capture-authority identity")
	}

	driftedCapture := capture
	driftedCapture.Authority = logicalChanged
	captureCalls := 0
	_, err = stableSourceIdentity(
		repository,
		func() (sourceCapture, error) {
			captureCalls++
			if captureCalls == 1 {
				return capture, nil
			}
			return driftedCapture, nil
		},
		func(string, []string) ([]sourceRecord, error) { return records, nil },
	)
	if err == nil || !strings.Contains(err.Error(), "authority or file set changed") {
		t.Fatalf("stableSourceIdentity authority drift error = %v", err)
	}

	changedRecords := slices.Clone(records)
	changedDigest := sha256.Sum256([]byte("changed"))
	changedRecords[0].SHA256 = fmt.Sprintf("%x", changedDigest)
	inspectCalls := 0
	_, err = stableSourceIdentity(
		repository,
		func() (sourceCapture, error) { return capture, nil },
		func(string, []string) ([]sourceRecord, error) {
			inspectCalls++
			if inspectCalls == 1 {
				return records, nil
			}
			return changedRecords, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "records changed") {
		t.Fatalf("stableSourceIdentity record drift error = %v", err)
	}
}
