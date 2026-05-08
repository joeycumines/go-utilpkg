package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestManifestProfile(t *testing.T) {
	manifest, err := loadProfileManifest("../tournament/manifest.json")
	if err != nil {
		t.Fatalf("loadProfileManifest: %v", err)
	}
	if len(manifest.Lanes) == 0 {
		t.Fatal("manifest has no lanes")
	}
	profile := observedProfile{
		SampleCount:             manifest.Measurement.SampleCount,
		BenchmarkTime:           manifest.Measurement.BenchmarkTime,
		Benchmem:                manifest.Measurement.Benchmem,
		GoFlags:                 manifest.Measurement.GoFlags,
		CPUCardinality:          manifest.Measurement.CPUCardinality,
		BenchmarkProcs:          14,
		PackageParallelism:      manifest.Measurement.PackageParallelism,
		GoDiagnosticTimeoutNS:   manifest.Lanes[0].GoDiagnosticTimeoutNS,
		RunnerWatchdogTimeoutNS: manifest.Lanes[0].RunnerWatchdogTimeoutNS,
		OrchestrationWatchdogNS: manifest.Lanes[0].OrchestrationWatchdogNS,
		Environment:             manifest.Measurement.Environment,
	}
	if err := validateObservedProfile(profile); err != nil {
		t.Fatalf("validateObservedProfile: %v", err)
	}
	wantArguments := []string{
		"-benchmem",
		"-count=5",
		"-run=^$",
		"-benchtime=1000000000ns",
		"-cpu=14",
		"-p=1",
		"-timeout=3300000000000ns",
	}
	if got := profileArguments(profile); !slices.Equal(got, wantArguments) {
		t.Fatalf("profileArguments = %q, want %q", got, wantArguments)
	}
	wantEnvironment := []string{
		"CGO_ENABLED=1",
		"GODEBUG=",
		"GOENV=off",
		"GOEXPERIMENT=",
		"GOFLAGS=-buildvcs=false",
		"GOGC=100",
		"GOMAXPROCS=14",
		"GOMEMLIMIT=off",
		"GOPROXY=off",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
		"LANG=C",
		"LC_ALL=C",
		"TZ=UTC",
	}
	if got := profileEnvironment(profile); !slices.Equal(got, wantEnvironment) {
		t.Fatalf("profileEnvironment = %q, want %q", got, wantEnvironment)
	}

	var first bytes.Buffer
	if err := emitProfileMetadata(&first, profile); err != nil {
		t.Fatalf("emitProfileMetadata: %v", err)
	}
	var second bytes.Buffer
	if err := emitProfileMetadata(&second, profile); err != nil {
		t.Fatalf("emitProfileMetadata repeat: %v", err)
	}
	if first.String() != second.String() ||
		!strings.Contains(first.String(), "tournament: meta=measurement-profile=") ||
		!strings.Contains(first.String(), "tournament: meta=execution-profile=") ||
		!strings.Contains(first.String(), "tournament: meta=go-diagnostic-timeout-ns=3300000000000\n") ||
		!strings.Contains(first.String(), "tournament: meta=runner-watchdog-timeout-ns=3600000000000\n") ||
		!strings.Contains(first.String(), "tournament: meta=orchestration-watchdog-timeout-ns=4200000000000\n") ||
		!strings.Contains(first.String(), "tournament: meta=env-godebug=b64:\n") {
		t.Fatalf("profile metadata is not deterministic:\n%s\n%s", first.String(), second.String())
	}

	measurementDigest, executionDigest := profileDigests(profile)
	changed := profile
	changed.GoDiagnosticTimeoutNS--
	changed.RunnerWatchdogTimeoutNS++
	changed.OrchestrationWatchdogNS++
	changedMeasurement, changedExecution := profileDigests(changed)
	if measurementDigest != changedMeasurement {
		t.Fatalf("measurement digest changed with watchdog policy: %s != %s", measurementDigest, changedMeasurement)
	}
	if executionDigest == changedExecution {
		t.Fatalf("execution digest did not change with watchdog policy: %s", executionDigest)
	}

	var arguments bytes.Buffer
	if err := emitNULRecords(&arguments, wantArguments); err != nil {
		t.Fatalf("emitNULRecords: %v", err)
	}
	if got, want := arguments.String(), strings.Join(wantArguments, "\x00")+"\x00"; got != want {
		t.Fatalf("NUL arguments = %q, want %q", got, want)
	}
}

func TestManifestLaneTimeout(t *testing.T) {
	manifest, err := loadProfileManifest("../tournament/manifest.json")
	if err != nil {
		t.Fatalf("loadProfileManifest: %v", err)
	}
	if len(manifest.Lanes) == 0 {
		t.Fatal("manifest has no lanes")
	}
	for _, lane := range manifest.Lanes {
		if err := validateProfileTimeouts(
			lane.GoDiagnosticTimeoutNS,
			lane.RunnerWatchdogTimeoutNS,
			lane.OrchestrationWatchdogNS,
		); err != nil {
			t.Errorf("lane %q timeouts: %v", lane.ID, err)
		}
	}
}

func TestValidateObservedProfileRequiresTimeoutPolicy(t *testing.T) {
	manifest, err := loadProfileManifest("../tournament/manifest.json")
	if err != nil {
		t.Fatalf("loadProfileManifest: %v", err)
	}
	profile := observedProfile{
		SampleCount:        manifest.Measurement.SampleCount,
		BenchmarkTime:      manifest.Measurement.BenchmarkTime,
		Benchmem:           manifest.Measurement.Benchmem,
		GoFlags:            manifest.Measurement.GoFlags,
		CPUCardinality:     manifest.Measurement.CPUCardinality,
		BenchmarkProcs:     1,
		PackageParallelism: manifest.Measurement.PackageParallelism,
		Environment:        manifest.Measurement.Environment,
	}
	if err := validateObservedProfile(profile); err == nil || !strings.Contains(err.Error(), "go diagnostic timeout") {
		t.Fatalf("validateObservedProfile error = %v, want timeout-policy rejection", err)
	}
}
