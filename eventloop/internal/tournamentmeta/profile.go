package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
)

type profileManifest struct { // betteralign:ignore canonical JSON field order
	SchemaVersion       int                      `json:"schema_version"`
	SourceHistory       json.RawMessage          `json:"source_history"`
	Lineage             manifestLineageReference `json:"lineage"`
	SourceAuthority     manifestSourceAuthority  `json:"source_authority"`
	Measurement         profileMeasurement       `json:"measurement"`
	Variants            json.RawMessage          `json:"variants"`
	VariantGroups       json.RawMessage          `json:"variant_groups"`
	Lanes               []profileLane            `json:"lanes"`
	Concepts            json.RawMessage          `json:"concepts"`
	RevisionVariants    json.RawMessage          `json:"revision_variants"`
	RevisionCheckpoints json.RawMessage          `json:"revision_checkpoints"`
	RootDispositions    json.RawMessage          `json:"root_dispositions,omitempty"`
}

type profileMeasurement struct { // betteralign:ignore canonical JSON field order
	SampleCount        int               `json:"sample_count"`
	BenchmarkTime      profileTime       `json:"benchmark_time"`
	Benchmem           bool              `json:"benchmem"`
	GoFlags            []string          `json:"go_flags"`
	CPUCardinality     int               `json:"cpu_cardinality"`
	PackageParallelism int               `json:"package_parallelism"`
	Environment        map[string]string `json:"environment"`
}

type profileTime struct { // betteralign:ignore canonical JSON field order
	Mode  string `json:"mode"`
	Value int64  `json:"value"`
}

type profileLane struct { // betteralign:ignore canonical JSON field order
	ID                          string                        `json:"id"`
	Package                     string                        `json:"package"`
	Required                    bool                          `json:"required"`
	Benchmarks                  json.RawMessage               `json:"benchmarks"`
	BenchmarkVariantGroups      json.RawMessage               `json:"benchmark_variant_groups"`
	BenchmarkGOOS               json.RawMessage               `json:"benchmark_goos"`
	BenchmarkLeaves             json.RawMessage               `json:"benchmark_leaves"`
	BenchmarkVariantExtraLeaves json.RawMessage               `json:"benchmark_variant_extra_leaves"`
	VariantIDs                  json.RawMessage               `json:"variant_ids"`
	DefaultVariantID            string                        `json:"default_variant_id"`
	GoDiagnosticTimeoutNS       int64                         `json:"go_diagnostic_timeout_ns"`
	RunnerWatchdogTimeoutNS     int64                         `json:"runner_watchdog_timeout_ns"`
	OrchestrationWatchdogNS     int64                         `json:"orchestration_watchdog_timeout_ns"`
	WorkloadDefinitions         json.RawMessage               `json:"workload_definitions"`
	BuildCellIDs                []string                      `json:"build_cell_ids,omitempty"`
	BenchmarkBindings           []manifestV5BindingProjection `json:"benchmark_bindings,omitempty"`
}

type observedProfile struct { // betteralign:ignore canonical JSON field order
	SampleCount             int               `json:"sample_count"`
	BenchmarkTime           profileTime       `json:"benchmark_time"`
	Benchmem                bool              `json:"benchmem"`
	GoFlags                 []string          `json:"go_flags"`
	CPUCardinality          int               `json:"cpu_cardinality"`
	BenchmarkProcs          int               `json:"benchmark_procs"`
	PackageParallelism      int               `json:"package_parallelism"`
	GoDiagnosticTimeoutNS   int64             `json:"go_diagnostic_timeout_ns"`
	RunnerWatchdogTimeoutNS int64             `json:"runner_watchdog_timeout_ns"`
	OrchestrationWatchdogNS int64             `json:"orchestration_watchdog_timeout_ns"`
	Environment             map[string]string `json:"environment"`
}

const minimumOrchestrationGapNS int64 = 10 * 60 * 1_000_000_000

func profileCommand(arguments []string) int {
	flags := flag.NewFlagSet("profile", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	manifestPath := flags.String("manifest", "", "tournament manifest")
	format := flags.String("format", "json", "json, argv, environment, metadata, go-diagnostic-timeout-ns, runner-watchdog-timeout-ns, or orchestration-watchdog-timeout-ns")
	sampleCount := flags.Int("sample-count", 0, "effective sample count override")
	timeMode := flags.String("time-mode", "", "duration or iterations override")
	timeValue := flags.Int64("time-value", 0, "nanoseconds or iterations override")
	procs := flags.Int("procs", 0, "effective benchmark processor setting")
	lane := flags.String("lane", "", "lane ID for timeout format")
	if err := flags.Parse(arguments); err != nil {
		return commandError(err)
	}
	if flags.NArg() != 0 || *manifestPath == "" {
		return commandError(errors.New("profile requires -manifest"))
	}
	manifest, err := loadProfileManifest(*manifestPath)
	if err != nil {
		return commandError(err)
	}
	if *format == "go-diagnostic-timeout-ns" ||
		*format == "runner-watchdog-timeout-ns" ||
		*format == "orchestration-watchdog-timeout-ns" {
		if *lane == "" {
			return commandError(errors.New("profile -format=timeout requires -lane"))
		}
		for _, candidate := range manifest.Lanes {
			if candidate.ID == *lane {
				var value int64
				switch *format {
				case "go-diagnostic-timeout-ns":
					value = candidate.GoDiagnosticTimeoutNS
				case "runner-watchdog-timeout-ns":
					value = candidate.RunnerWatchdogTimeoutNS
				case "orchestration-watchdog-timeout-ns":
					value = candidate.OrchestrationWatchdogNS
				}
				fmt.Println(value)
				return 0
			}
		}
		return commandError(fmt.Errorf("unknown lane %q", *lane))
	}
	if *lane == "" {
		return commandError(errors.New("profile requires -lane"))
	}
	var selectedLane *profileLane
	for index := range manifest.Lanes {
		if manifest.Lanes[index].ID == *lane {
			selectedLane = &manifest.Lanes[index]
			break
		}
	}
	if selectedLane == nil {
		return commandError(fmt.Errorf("unknown lane %q", *lane))
	}
	if *procs <= 0 {
		return commandError(errors.New("profile requires a positive -procs value"))
	}
	profile := observedProfile{
		SampleCount:             manifest.Measurement.SampleCount,
		BenchmarkTime:           manifest.Measurement.BenchmarkTime,
		Benchmem:                manifest.Measurement.Benchmem,
		GoFlags:                 slices.Clone(manifest.Measurement.GoFlags),
		CPUCardinality:          manifest.Measurement.CPUCardinality,
		BenchmarkProcs:          *procs,
		PackageParallelism:      manifest.Measurement.PackageParallelism,
		GoDiagnosticTimeoutNS:   selectedLane.GoDiagnosticTimeoutNS,
		RunnerWatchdogTimeoutNS: selectedLane.RunnerWatchdogTimeoutNS,
		OrchestrationWatchdogNS: selectedLane.OrchestrationWatchdogNS,
		Environment:             cloneStringMap(manifest.Measurement.Environment),
	}
	if *sampleCount != 0 {
		profile.SampleCount = *sampleCount
	}
	if *timeMode != "" {
		profile.BenchmarkTime.Mode = *timeMode
	}
	if *timeValue != 0 {
		profile.BenchmarkTime.Value = *timeValue
	}
	if err := validateObservedProfile(profile); err != nil {
		return commandError(err)
	}

	switch *format {
	case "json":
		data, err := json.Marshal(profile)
		if err != nil {
			return commandError(fmt.Errorf("encode profile: %w", err))
		}
		fmt.Println(string(data))
	case "argv":
		if err := emitNULRecords(os.Stdout, profileArguments(profile)); err != nil {
			return commandError(err)
		}
	case "environment":
		if err := emitNULRecords(os.Stdout, profileEnvironment(profile)); err != nil {
			return commandError(err)
		}
	case "metadata":
		if err := emitProfileMetadata(os.Stdout, profile); err != nil {
			return commandError(err)
		}
	default:
		return commandError(fmt.Errorf("unknown profile format %q", *format))
	}
	return 0
}

func loadProfileManifest(path string) (profileManifest, error) {
	data, err := readRegularStable(path, 0o644)
	if err != nil {
		return profileManifest{}, fmt.Errorf("read manifest profile: %w", err)
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return profileManifest{}, fmt.Errorf("validate manifest JSON: %w", err)
	}
	if err := validateSourceManifestJSONShape(data); err != nil {
		return profileManifest{}, fmt.Errorf("validate manifest shape: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest profileManifest
	if err := decoder.Decode(&manifest); err != nil {
		return profileManifest{}, fmt.Errorf("decode manifest profile: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return profileManifest{}, errors.New("manifest profile has trailing JSON")
	}
	if manifest.SchemaVersion != manifestSchemaVersionV4 && manifest.SchemaVersion != manifestSchemaVersionV5 {
		return profileManifest{}, fmt.Errorf(
			"manifest schema = %d, want %d or %d",
			manifest.SchemaVersion,
			manifestSchemaVersionV4,
			manifestSchemaVersionV5,
		)
	}
	if err := validateManifestLineageReference(manifest.Lineage); err != nil {
		return profileManifest{}, fmt.Errorf("manifest lineage: %w", err)
	}
	if err := validateManifestSourceAuthority(manifest.SourceAuthority); err != nil {
		return profileManifest{}, fmt.Errorf("manifest source authority: %w", err)
	}
	if manifest.SchemaVersion == manifestSchemaVersionV5 {
		var envelope sourceManifestEnvelope
		if err := decodeManifestReference(data, &envelope); err != nil {
			return profileManifest{}, fmt.Errorf("decode manifest v5 authority envelope: %w", err)
		}
		if err := verifyManifestV5Lineage(path, envelope); err != nil {
			return profileManifest{}, fmt.Errorf("manifest lineage authority: %w", err)
		}
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
	if err := validateMeasurementProfile(profile); err != nil {
		return profileManifest{}, fmt.Errorf("manifest measurement: %w", err)
	}
	seenLanes := make(map[string]struct{}, len(manifest.Lanes))
	buildCells := make(map[string]struct{}, len(manifest.SourceAuthority.BuildCells))
	for _, cell := range manifest.SourceAuthority.BuildCells {
		buildCells[cell.ID] = struct{}{}
	}
	for _, lane := range manifest.Lanes {
		if lane.ID == "" {
			return profileManifest{}, errors.New("manifest lane has an empty ID")
		}
		if _, exists := seenLanes[lane.ID]; exists {
			return profileManifest{}, fmt.Errorf("manifest has duplicate lane ID %q", lane.ID)
		}
		seenLanes[lane.ID] = struct{}{}
		if manifest.SchemaVersion == manifestSchemaVersionV5 {
			projection := manifestV5Lane{
				ID:                      lane.ID,
				Required:                lane.Required,
				BuildCellIDs:            lane.BuildCellIDs,
				BenchmarkBindings:       lane.BenchmarkBindings,
				GoDiagnosticTimeoutNS:   lane.GoDiagnosticTimeoutNS,
				RunnerWatchdogTimeoutNS: lane.RunnerWatchdogTimeoutNS,
				OrchestrationWatchdogNS: lane.OrchestrationWatchdogNS,
			}
			if err := validateManifestV5LaneShape(projection); err != nil {
				return profileManifest{}, err
			}
			for _, buildCellID := range lane.BuildCellIDs {
				if _, ok := buildCells[buildCellID]; !ok {
					return profileManifest{}, fmt.Errorf("manifest v5 lane %q has unknown build cell %q", lane.ID, buildCellID)
				}
			}
		}
		if err := validateProfileTimeouts(
			lane.GoDiagnosticTimeoutNS,
			lane.RunnerWatchdogTimeoutNS,
			lane.OrchestrationWatchdogNS,
		); err != nil {
			return profileManifest{}, fmt.Errorf(
				"manifest lane %q timeouts: %w",
				lane.ID,
				err,
			)
		}
	}
	if len(seenLanes) == 0 {
		return profileManifest{}, errors.New("manifest has no lanes")
	}
	return manifest, nil
}

func validateObservedProfile(profile observedProfile) error {
	if err := validateMeasurementProfile(profile); err != nil {
		return err
	}
	if err := validateProfileTimeouts(
		profile.GoDiagnosticTimeoutNS,
		profile.RunnerWatchdogTimeoutNS,
		profile.OrchestrationWatchdogNS,
	); err != nil {
		return err
	}
	return nil
}

func validateProfileTimeouts(goDiagnostic, runnerWatchdog, orchestrationWatchdog int64) error {
	if goDiagnostic <= 0 {
		return fmt.Errorf("go diagnostic timeout must be positive, got %d", goDiagnostic)
	}
	if runnerWatchdog < goDiagnostic {
		return fmt.Errorf(
			"runner watchdog timeout %d must be at least go diagnostic timeout %d",
			runnerWatchdog,
			goDiagnostic,
		)
	}
	if orchestrationWatchdog <= runnerWatchdog {
		return fmt.Errorf(
			"orchestration watchdog timeout %d must exceed runner watchdog timeout %d",
			orchestrationWatchdog,
			runnerWatchdog,
		)
	}
	if orchestrationWatchdog-runnerWatchdog < minimumOrchestrationGapNS {
		return fmt.Errorf(
			"orchestration watchdog margin = %d, want at least %d",
			orchestrationWatchdog-runnerWatchdog,
			minimumOrchestrationGapNS,
		)
	}
	return nil
}

func validateMeasurementProfile(profile observedProfile) error {
	if profile.SampleCount <= 0 {
		return fmt.Errorf("sample count must be positive, got %d", profile.SampleCount)
	}
	if profile.BenchmarkTime.Mode != "duration" && profile.BenchmarkTime.Mode != "iterations" {
		return fmt.Errorf("benchmark time mode must be duration or iterations, got %q", profile.BenchmarkTime.Mode)
	}
	if profile.BenchmarkTime.Value <= 0 {
		return fmt.Errorf("benchmark time value must be positive, got %d", profile.BenchmarkTime.Value)
	}
	if !profile.Benchmem {
		return errors.New("benchmem must be enabled")
	}
	if len(profile.GoFlags) != 1 || profile.GoFlags[0] != "-buildvcs=false" {
		return fmt.Errorf("go flags must be [-buildvcs=false], got %v", profile.GoFlags)
	}
	if profile.CPUCardinality != 1 {
		return fmt.Errorf("CPU cardinality must be 1, got %d", profile.CPUCardinality)
	}
	if profile.BenchmarkProcs <= 0 {
		return fmt.Errorf("benchmark procs must be positive, got %d", profile.BenchmarkProcs)
	}
	if profile.PackageParallelism != 1 {
		return fmt.Errorf("package parallelism must be 1, got %d", profile.PackageParallelism)
	}
	wantEnvironment := map[string]string{
		"CGO_ENABLED":  "1",
		"GODEBUG":      "",
		"GOENV":        "off",
		"GOEXPERIMENT": "",
		"GOFLAGS":      "-buildvcs=false",
		"GOGC":         "100",
		"GOMEMLIMIT":   "off",
		"GOMAXPROCS":   "benchmark-cpu-flag",
		"GOPROXY":      "off",
		"GOTOOLCHAIN":  "local",
		"GOWORK":       "off",
		"LANG":         "C",
		"LC_ALL":       "C",
		"TZ":           "UTC",
	}
	if !stringMapsEqual(profile.Environment, wantEnvironment) {
		return fmt.Errorf("environment = %v, want %v", profile.Environment, wantEnvironment)
	}
	return nil
}

func profileArguments(profile observedProfile) []string {
	timeValue := strconv.FormatInt(profile.BenchmarkTime.Value, 10)
	if profile.BenchmarkTime.Mode == "duration" {
		timeValue += "ns"
	} else {
		timeValue += "x"
	}
	return []string{
		"-benchmem",
		"-count=" + strconv.Itoa(profile.SampleCount),
		"-run=^$",
		"-benchtime=" + timeValue,
		"-cpu=" + strconv.Itoa(profile.BenchmarkProcs),
		"-p=" + strconv.Itoa(profile.PackageParallelism),
		// Go 1.26 disables the testing alarm while benchmarks execute. This
		// shorter setting is diagnostic only; tournamentmeta run enforces the
		// real process-tree deadline independently.
		"-timeout=" + strconv.FormatInt(profile.GoDiagnosticTimeoutNS, 10) + "ns",
	}
}

func profileEnvironment(profile observedProfile) []string {
	environment := cloneStringMap(profile.Environment)
	environment["GOMAXPROCS"] = strconv.Itoa(profile.BenchmarkProcs)
	result := make([]string, 0, len(environment))
	for key, value := range environment {
		result = append(result, key+"="+value)
	}
	slices.Sort(result)
	return result
}

func emitProfileMetadata(writer io.Writer, profile observedProfile) error {
	measurementDigest, executionDigest := profileDigests(profile)
	values := [][2]string{
		{"measurement-profile", measurementDigest},
		{"execution-profile", executionDigest},
		{"sample-count", strconv.Itoa(profile.SampleCount)},
		{"benchmark-time-mode", profile.BenchmarkTime.Mode},
		{"benchmark-time-value", strconv.FormatInt(profile.BenchmarkTime.Value, 10)},
		{"benchmem", strconv.FormatBool(profile.Benchmem)},
		{"go-flags", strings.Join(profile.GoFlags, ",")},
		{"cpu-cardinality", strconv.Itoa(profile.CPUCardinality)},
		{"benchmark-procs", strconv.Itoa(profile.BenchmarkProcs)},
		{"package-parallelism", strconv.Itoa(profile.PackageParallelism)},
		{"go-diagnostic-timeout-ns", strconv.FormatInt(profile.GoDiagnosticTimeoutNS, 10)},
		{"runner-watchdog-timeout-ns", strconv.FormatInt(profile.RunnerWatchdogTimeoutNS, 10)},
		{"orchestration-watchdog-timeout-ns", strconv.FormatInt(profile.OrchestrationWatchdogNS, 10)},
	}
	for _, environment := range profileEnvironment(profile) {
		key, value, _ := strings.Cut(environment, "=")
		values = append(values, [2]string{"env-" + strings.ToLower(key), encodeMetadataValue(value)})
	}
	for _, value := range values {
		if _, err := fmt.Fprintf(writer, "tournament: meta=%s=%s\n", value[0], value[1]); err != nil {
			return fmt.Errorf("write profile metadata: %w", err)
		}
	}
	return nil
}

func profileDigests(profile observedProfile) (string, string) {
	arguments := profileArguments(profile)
	environment := profileEnvironment(profile)
	measurement := sha256.New()
	writeFingerprintFrame(measurement, []byte("go-utilpkg-eventloop-tournament-measurement-profile-v1"))
	for _, argument := range arguments {
		if strings.HasPrefix(argument, "-timeout=") {
			continue
		}
		writeFingerprintFrame(measurement, []byte("argv"))
		writeFingerprintFrame(measurement, []byte(argument))
	}
	for _, value := range environment {
		writeFingerprintFrame(measurement, []byte("env"))
		writeFingerprintFrame(measurement, []byte(value))
	}
	measurementSum := measurement.Sum(nil)

	execution := sha256.New()
	writeFingerprintFrame(execution, []byte("go-utilpkg-eventloop-tournament-execution-profile-v1"))
	writeFingerprintFrame(execution, measurementSum)
	for _, argument := range arguments {
		writeFingerprintFrame(execution, []byte("argv"))
		writeFingerprintFrame(execution, []byte(argument))
	}
	for _, value := range environment {
		writeFingerprintFrame(execution, []byte("env"))
		writeFingerprintFrame(execution, []byte(value))
	}
	writeFingerprintFrame(execution, []byte("runner-watchdog-timeout-ns"))
	writeFingerprintFrame(execution, []byte(strconv.FormatInt(profile.RunnerWatchdogTimeoutNS, 10)))
	writeFingerprintFrame(execution, []byte("orchestration-watchdog-timeout-ns"))
	writeFingerprintFrame(execution, []byte(strconv.FormatInt(profile.OrchestrationWatchdogNS, 10)))
	return hex.EncodeToString(measurementSum), hex.EncodeToString(execution.Sum(nil))
}

func encodeMetadataValue(value string) string {
	return "b64:" + base64.RawURLEncoding.EncodeToString([]byte(value))
}

func emitNULRecords(writer io.Writer, records []string) error {
	if len(records) == 0 {
		return errors.New("cannot emit empty NUL records")
	}
	for _, record := range records {
		if strings.ContainsRune(record, 0) {
			return errors.New("NUL record contains NUL")
		}
		if _, err := io.WriteString(writer, record); err != nil {
			return fmt.Errorf("write NUL record: %w", err)
		}
		if _, err := writer.Write([]byte{0}); err != nil {
			return fmt.Errorf("terminate NUL record: %w", err)
		}
	}
	return nil
}

func cloneStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	maps.Copy(result, source)
	return result
}

func stringMapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
