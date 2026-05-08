package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var protocolBenchmarkPattern = regexp.MustCompile(`^Benchmark[A-Za-z0-9_]+$`)

const (
	comparisonIdentityDomain = "go-utilpkg/eventloop/tournament/comparison/v1"
	unitIdentityDomain       = "go-utilpkg/eventloop/tournament/unit/v1"
	selectionPlanDomain      = "go-utilpkg/eventloop/tournament/selection-plan/v1"
	executionIdentityDomain  = "go-utilpkg/eventloop/tournament/execution/v1"
)

type comparisonIdentityInput struct { // betteralign:ignore canonical JSON field order
	ImplementationID          string           `json:"implementation_id"`
	WorkloadID                string           `json:"workload_id"`
	Configuration             []lineageSetting `json:"configuration"`
	SemanticHarnessID         string           `json:"semantic_harness_id"`
	MeasurementContractSHA256 string           `json:"measurement_contract_sha256"`
	StableLeaf                string           `json:"stable_leaf"`
}

type unitIdentityInput struct { // betteralign:ignore canonical JSON field order
	LaneID                    string                `json:"lane_id"`
	BuildCellID               string                `json:"build_cell_id"`
	ModuleID                  string                `json:"module_id"`
	Package                   string                `json:"package"`
	RawRootID                 string                `json:"raw_root_id"`
	HarnessID                 string                `json:"harness_id"`
	Applicability             string                `json:"applicability"`
	MeasurementContractSHA256 string                `json:"measurement_contract_sha256"`
	Bindings                  []unitIdentityBinding `json:"bindings"`
}

type unitIdentityBinding struct { // betteralign:ignore canonical JSON field order
	BindingID         string               `json:"binding_id"`
	Benchmark         string               `json:"benchmark"`
	ImplementationID  string               `json:"implementation_id"`
	SnapshotIDs       []string             `json:"snapshot_ids"`
	WorkloadID        string               `json:"workload_id"`
	Configuration     []lineageSetting     `json:"configuration"`
	SemanticHarnessID string               `json:"semantic_harness_id"`
	Results           []unitIdentityResult `json:"results"`
}

type unitIdentityResult struct { // betteralign:ignore canonical JSON field order
	EmittedLeaf  string `json:"emitted_leaf"`
	StableLeaf   string `json:"stable_leaf"`
	ComparisonID string `json:"comparison_id"`
}

type selectionPlanIdentityInput struct { // betteralign:ignore canonical JSON field order
	ManifestSHA256            string                     `json:"manifest_sha256"`
	LineageSHA256             string                     `json:"lineage_sha256"`
	LineageFloorSHA256        string                     `json:"lineage_floor_sha256"`
	SharedSourceID            string                     `json:"shared_source_id"`
	SourceCaptureID           string                     `json:"source_capture_id"`
	HostAuthorityID           string                     `json:"host_authority_id"`
	MeasurementContractSHA256 string                     `json:"measurement_contract_sha256"`
	BuildCellIDs              []string                   `json:"build_cell_ids"`
	UnitIDs                   []string                   `json:"unit_ids"`
	Dispositions              []selectionPlanDisposition `json:"dispositions"`
}

type selectionPlanDisposition struct { // betteralign:ignore canonical JSON field order
	Kind        string `json:"kind"`
	SubjectID   string `json:"subject_id"`
	AuthorityID string `json:"authority_id"`
}

type executionIdentityInput struct { // betteralign:ignore canonical JSON field order
	PlanID                   string   `json:"plan_id"`
	UnitID                   string   `json:"unit_id"`
	SharedSourceID           string   `json:"shared_source_id"`
	SourceCaptureID          string   `json:"source_capture_id"`
	BinarySHA256             string   `json:"binary_sha256"`
	ToolchainAuthoritySHA256 string   `json:"toolchain_authority_sha256"`
	ModuleGraphSHA256        string   `json:"module_graph_sha256"`
	NativeAuthoritySHA256    string   `json:"native_authority_sha256"`
	HostAuthorityID          string   `json:"host_authority_id"`
	MeasurementProfileSHA256 string   `json:"measurement_profile_sha256"`
	ExecutionProfileSHA256   string   `json:"execution_profile_sha256"`
	Argv                     []string `json:"argv"`
	Environment              []string `json:"environment"`
}

func comparisonIdentity(input comparisonIdentityInput) (string, error) {
	if !lineageIDPattern.MatchString(input.ImplementationID) || !lineageIDPattern.MatchString(input.WorkloadID) ||
		!lineageIDPattern.MatchString(input.SemanticHarnessID) ||
		!historySHA256Pattern.MatchString(input.MeasurementContractSHA256) || !validLineageLeaf(input.StableLeaf) ||
		input.Configuration == nil {
		return "", errors.New("comparison identity input is incomplete")
	}
	if err := validateLineageSettings(input.Configuration); err != nil {
		return "", fmt.Errorf("comparison configuration: %w", err)
	}
	fields := []string{
		"implementation-id", input.ImplementationID,
		"workload-id", input.WorkloadID,
		"semantic-harness-id", input.SemanticHarnessID,
		"measurement-contract-sha256", input.MeasurementContractSHA256,
		"stable-leaf", input.StableLeaf,
	}
	fields = appendProtocolSettings(fields, input.Configuration)
	return protocolIdentityDigest(comparisonIdentityDomain, fields), nil
}

func unitIdentity(input unitIdentityInput) (string, error) {
	if !lineageIDPattern.MatchString(input.LaneID) || !lineageIDPattern.MatchString(input.BuildCellID) ||
		!lineageTokenPattern.MatchString(input.ModuleID) || !oneLine(input.Package) ||
		!lineageIDPattern.MatchString(input.RawRootID) || !lineageIDPattern.MatchString(input.HarnessID) ||
		!lineageEnum(input.Applicability, "diagnostic", "executable") ||
		!historySHA256Pattern.MatchString(input.MeasurementContractSHA256) || len(input.Bindings) == 0 {
		return "", errors.New("unit identity input is incomplete")
	}
	fields := []string{
		"lane-id", input.LaneID,
		"build-cell-id", input.BuildCellID,
		"module-id", input.ModuleID,
		"package", input.Package,
		"raw-root-id", input.RawRootID,
		"harness-id", input.HarnessID,
		"applicability", input.Applicability,
		"measurement-contract-sha256", input.MeasurementContractSHA256,
		"binding-count", strconv.Itoa(len(input.Bindings)),
	}
	for index, binding := range input.Bindings {
		if index != 0 && compareUnitIdentityBinding(input.Bindings[index-1], binding) >= 0 {
			return "", errors.New("unit bindings are not a strictly sorted set")
		}
		bindingFields, err := unitBindingIdentityFields(binding, input.Applicability, input.MeasurementContractSHA256)
		if err != nil {
			return "", fmt.Errorf("unit binding %d: %w", index, err)
		}
		fields = append(fields, "binding")
		fields = append(fields, bindingFields...)
	}
	return protocolIdentityDigest(unitIdentityDomain, fields), nil
}

func unitBindingIdentityFields(binding unitIdentityBinding, applicability, measurementContractSHA256 string) ([]string, error) {
	if !lineageIDPattern.MatchString(binding.BindingID) || !protocolBenchmarkPattern.MatchString(binding.Benchmark) ||
		!lineageIDPattern.MatchString(binding.ImplementationID) || !lineageIDPattern.MatchString(binding.WorkloadID) ||
		!lineageIDPattern.MatchString(binding.SemanticHarnessID) || binding.Configuration == nil || binding.Results == nil {
		return nil, errors.New("binding authority is incomplete")
	}
	if err := validateSortedStrings(binding.SnapshotIDs, true, "snapshot IDs"); err != nil {
		return nil, err
	}
	for _, snapshotID := range binding.SnapshotIDs {
		if !lineageIDPattern.MatchString(snapshotID) {
			return nil, fmt.Errorf("invalid snapshot ID %q", snapshotID)
		}
	}
	if err := validateLineageSettings(binding.Configuration); err != nil {
		return nil, fmt.Errorf("configuration: %w", err)
	}
	if applicability == "executable" && len(binding.Results) == 0 || applicability == "diagnostic" && len(binding.Results) != 0 {
		return nil, fmt.Errorf("result set contradicts applicability %q", applicability)
	}
	fields := []string{
		"binding-id", binding.BindingID,
		"benchmark", binding.Benchmark,
		"implementation-id", binding.ImplementationID,
		"snapshot-count", strconv.Itoa(len(binding.SnapshotIDs)),
	}
	for _, snapshotID := range binding.SnapshotIDs {
		fields = append(fields, "snapshot-id", snapshotID)
	}
	fields = append(fields,
		"workload-id", binding.WorkloadID,
		"semantic-harness-id", binding.SemanticHarnessID,
	)
	fields = appendProtocolSettings(fields, binding.Configuration)
	fields = append(fields, "result-count", strconv.Itoa(len(binding.Results)))
	for index, result := range binding.Results {
		if !validLineageLeaf(result.EmittedLeaf) || !validLineageLeaf(result.StableLeaf) ||
			!historySHA256Pattern.MatchString(result.ComparisonID) ||
			index != 0 && compareUnitIdentityResult(binding.Results[index-1], result) >= 0 {
			return nil, errors.New("results are not a strictly sorted valid set")
		}
		want, err := comparisonIdentity(comparisonIdentityInput{
			ImplementationID: binding.ImplementationID, WorkloadID: binding.WorkloadID,
			Configuration: binding.Configuration, SemanticHarnessID: binding.SemanticHarnessID,
			MeasurementContractSHA256: measurementContractSHA256, StableLeaf: result.StableLeaf,
		})
		if err != nil {
			return nil, err
		}
		if result.ComparisonID != want {
			return nil, fmt.Errorf("comparison ID = %s, want %s", result.ComparisonID, want)
		}
		fields = append(fields,
			"result", "emitted-leaf", result.EmittedLeaf,
			"stable-leaf", result.StableLeaf,
			"comparison-id", result.ComparisonID,
		)
	}
	return fields, nil
}

func selectionPlanIdentity(input selectionPlanIdentityInput) (string, error) {
	for name, value := range map[string]string{
		"manifest": input.ManifestSHA256, "lineage": input.LineageSHA256,
		"lineage floor": input.LineageFloorSHA256, "shared source": input.SharedSourceID,
		"source capture": input.SourceCaptureID, "host authority": input.HostAuthorityID,
		"measurement contract": input.MeasurementContractSHA256,
	} {
		if !historySHA256Pattern.MatchString(value) {
			return "", fmt.Errorf("selection plan %s identity is invalid", name)
		}
	}
	if err := validateProtocolIDs(input.BuildCellIDs, true, false, "build-cell IDs"); err != nil {
		return "", err
	}
	if err := validateProtocolIDs(input.UnitIDs, true, true, "unit IDs"); err != nil {
		return "", err
	}
	if input.Dispositions == nil {
		return "", errors.New("selection plan dispositions must be non-null")
	}
	fields := []string{
		"manifest-sha256", input.ManifestSHA256,
		"lineage-sha256", input.LineageSHA256,
		"lineage-floor-sha256", input.LineageFloorSHA256,
		"shared-source-id", input.SharedSourceID,
		"source-capture-id", input.SourceCaptureID,
		"host-authority-id", input.HostAuthorityID,
		"measurement-contract-sha256", input.MeasurementContractSHA256,
		"build-cell-count", strconv.Itoa(len(input.BuildCellIDs)),
	}
	for _, buildCellID := range input.BuildCellIDs {
		fields = append(fields, "build-cell-id", buildCellID)
	}
	fields = append(fields, "unit-count", strconv.Itoa(len(input.UnitIDs)))
	for _, unitID := range input.UnitIDs {
		fields = append(fields, "unit-id", unitID)
	}
	fields = append(fields, "disposition-count", strconv.Itoa(len(input.Dispositions)))
	for index, disposition := range input.Dispositions {
		if !validSelectionDisposition(disposition) ||
			index != 0 && compareSelectionDisposition(input.Dispositions[index-1], disposition) >= 0 {
			return "", errors.New("selection plan dispositions are not a strictly sorted valid set")
		}
		fields = append(fields,
			"disposition", "kind", disposition.Kind,
			"subject-id", disposition.SubjectID,
			"authority-id", disposition.AuthorityID,
		)
	}
	return protocolIdentityDigest(selectionPlanDomain, fields), nil
}

func executionIdentity(input executionIdentityInput) (string, error) {
	for name, value := range map[string]string{
		"plan": input.PlanID, "unit": input.UnitID, "shared source": input.SharedSourceID,
		"source capture": input.SourceCaptureID, "binary": input.BinarySHA256,
		"toolchain authority": input.ToolchainAuthoritySHA256, "module graph": input.ModuleGraphSHA256,
		"host authority": input.HostAuthorityID, "measurement profile": input.MeasurementProfileSHA256,
		"execution profile": input.ExecutionProfileSHA256,
	} {
		if !historySHA256Pattern.MatchString(value) {
			return "", fmt.Errorf("execution %s identity is invalid", name)
		}
	}
	if input.NativeAuthoritySHA256 != "none" && !historySHA256Pattern.MatchString(input.NativeAuthoritySHA256) {
		return "", errors.New("execution native authority identity is invalid")
	}
	if len(input.Argv) == 0 || input.Environment == nil {
		return "", errors.New("execution argv and environment must be nonempty and non-null")
	}
	for _, argument := range input.Argv {
		if argument == "" || strings.ContainsRune(argument, 0) {
			return "", errors.New("execution argv contains an invalid argument")
		}
	}
	if err := validateProtocolEnvironment(input.Environment); err != nil {
		return "", err
	}
	fields := []string{
		"plan-id", input.PlanID,
		"unit-id", input.UnitID,
		"shared-source-id", input.SharedSourceID,
		"source-capture-id", input.SourceCaptureID,
		"binary-sha256", input.BinarySHA256,
		"toolchain-authority-sha256", input.ToolchainAuthoritySHA256,
		"module-graph-sha256", input.ModuleGraphSHA256,
		"native-authority-sha256", input.NativeAuthoritySHA256,
		"host-authority-id", input.HostAuthorityID,
		"measurement-profile-sha256", input.MeasurementProfileSHA256,
		"execution-profile-sha256", input.ExecutionProfileSHA256,
		"argv-count", strconv.Itoa(len(input.Argv)),
	}
	for _, argument := range input.Argv {
		fields = append(fields, "argv", argument)
	}
	fields = append(fields, "environment-count", strconv.Itoa(len(input.Environment)))
	for _, environment := range input.Environment {
		fields = append(fields, "environment", environment)
	}
	return protocolIdentityDigest(executionIdentityDomain, fields), nil
}

func protocolIdentityDigest(domain string, fields []string) string {
	digest := sha256.New()
	writeFingerprintFrame(digest, []byte(domain))
	for _, field := range fields {
		writeFingerprintFrame(digest, []byte(field))
	}
	return fmt.Sprintf("%x", digest.Sum(nil))
}

func appendProtocolSettings(fields []string, settings []lineageSetting) []string {
	fields = append(fields, "configuration-count", strconv.Itoa(len(settings)))
	for _, setting := range settings {
		fields = append(fields, "configuration", setting.Key, setting.Value)
	}
	return fields
}

func compareUnitIdentityBinding(left, right unitIdentityBinding) int {
	leftFields := []string{left.Benchmark, left.ImplementationID, left.WorkloadID}
	rightFields := []string{right.Benchmark, right.ImplementationID, right.WorkloadID}
	for _, setting := range left.Configuration {
		leftFields = append(leftFields, setting.Key, setting.Value)
	}
	for _, setting := range right.Configuration {
		rightFields = append(rightFields, setting.Key, setting.Value)
	}
	leftFields = append(leftFields, left.BindingID)
	rightFields = append(rightFields, right.BindingID)
	return slices.Compare(leftFields, rightFields)
}

func compareUnitIdentityResult(left, right unitIdentityResult) int {
	if result := strings.Compare(left.EmittedLeaf, right.EmittedLeaf); result != 0 {
		return result
	}
	return strings.Compare(left.StableLeaf, right.StableLeaf)
}

func validateProtocolIDs(values []string, requireNonempty, digest bool, description string) error {
	if values == nil || requireNonempty && len(values) == 0 || !slices.IsSorted(values) {
		return fmt.Errorf("%s are not a non-null sorted set", description)
	}
	for index, value := range values {
		valid := lineageIDPattern.MatchString(value)
		if digest {
			valid = historySHA256Pattern.MatchString(value)
		}
		if !valid || index != 0 && values[index-1] == value {
			return fmt.Errorf("%s are not a strictly sorted valid set", description)
		}
	}
	return nil
}

func validSelectionDisposition(disposition selectionPlanDisposition) bool {
	return lineageEnum(disposition.Kind,
		"alias-only", "diagnostic-unselected", "not-applicable", "root-disposition",
		"unavailable-native", "unavailable-toolchain", "unselected-build-cell",
	) && lineageIDPattern.MatchString(disposition.SubjectID) && lineageIDPattern.MatchString(disposition.AuthorityID)
}

func compareSelectionDisposition(left, right selectionPlanDisposition) int {
	return slices.Compare(
		[]string{left.Kind, left.SubjectID, left.AuthorityID},
		[]string{right.Kind, right.SubjectID, right.AuthorityID},
	)
}

func validateProtocolEnvironment(environment []string) error {
	if len(environment) == 0 || !slices.IsSorted(environment) {
		return errors.New("execution environment must be a nonempty sorted set")
	}
	previousKey := ""
	for index, record := range environment {
		key, _, found := strings.Cut(record, "=")
		if !found || key == "" || strings.ContainsAny(key, "\x00\r\n=") ||
			strings.ContainsRune(record, 0) || index != 0 && key == previousKey {
			return fmt.Errorf("invalid or duplicate execution environment record %q", record)
		}
		previousKey = key
	}
	return nil
}
