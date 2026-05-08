package tournament

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var tournamentManifestV5IDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)+$`)
var tournamentManifestV5TokenPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func TestManifestJSONRejectsDuplicateAndCaseAliasKeys(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
	}{
		{
			name: "duplicate",
			data: []byte(strings.Replace(string(manifestJSON), `"policy": "manifest-build-cells-v1",`, `"policy": "manifest-build-cells-v1", "policy": "manifest-build-cells-v1",`, 1)),
		},
		{
			name: "case alias",
			data: []byte(strings.Replace(string(manifestJSON), `"policy": "manifest-build-cells-v1",`, `"Policy": "manifest-build-cells-v1",`, 1)),
		},
		{
			name: "lineage case alias",
			data: []byte(strings.Replace(string(manifestJSON), `"lineage": {`, `"Lineage": {`, 1)),
		},
		{
			name: "lineage unknown",
			data: []byte(strings.Replace(string(manifestJSON), `"path": "lineage.json",`, `"path": "lineage.json", "unknown": true,`, 1)),
		},
		{
			name: "lineage null",
			data: []byte(strings.Replace(string(manifestJSON), `"lineage": {`, `"lineage": null, "discarded": {`, 1)),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeManifest(test.data); err == nil {
				t.Fatal("noncanonical manifest JSON unexpectedly passed")
			}
		})
	}
}

func validateTournamentManifestJSONShape(data []byte) error {
	root, err := decodeTournamentRawObject(data, "manifest")
	if err != nil {
		return err
	}
	var schemaVersion int
	if err := json.Unmarshal(root["schema_version"], &schemaVersion); err != nil {
		return errors.New("manifest schema_version must be an integer")
	}
	var rootKeys []string
	switch schemaVersion {
	case 4:
		rootKeys = []string{
			"concepts", "lanes", "lineage", "measurement", "revision_checkpoints", "revision_variants",
			"schema_version", "source_authority", "source_history", "variant_groups", "variants",
		}
	case 5:
		rootKeys = []string{
			"concepts", "lanes", "lineage", "measurement", "revision_checkpoints", "revision_variants",
			"root_dispositions", "schema_version", "source_authority", "source_history",
		}
	default:
		return fmt.Errorf("unsupported manifest schema %d", schemaVersion)
	}
	if err := requireTournamentRawKeys(root, rootKeys, "manifest"); err != nil {
		return err
	}
	for key, value := range root {
		if string(bytes.TrimSpace(value)) == "null" {
			return fmt.Errorf("manifest field %q is null", key)
		}
	}
	if schemaVersion == 4 {
		if err := validateTournamentManifestV4Shape(root); err != nil {
			return err
		}
	}
	if schemaVersion == 5 {
		if err := validateTournamentManifestV5Shape(root); err != nil {
			return err
		}
	}
	lineage, err := decodeTournamentRawObject(root["lineage"], "lineage")
	if err != nil {
		return err
	}
	if err := requireTournamentRawKeys(lineage, []string{"floor", "path", "schema_version", "sha256"}, "lineage"); err != nil {
		return err
	}
	var lineageSchema int
	if err := json.Unmarshal(lineage["schema_version"], &lineageSchema); err != nil {
		return errors.New("lineage schema_version must be an integer")
	}
	if schemaVersion == 5 && lineageSchema != 3 {
		return fmt.Errorf("manifest v5 lineage schema = %d, want 3", lineageSchema)
	}
	lineageFloor, err := decodeTournamentRawObject(lineage["floor"], "lineage.floor")
	if err != nil {
		return err
	}
	if err := requireTournamentRawKeys(lineageFloor, []string{
		"cumulative_record_set_sha256", "path", "schema_version", "sequence", "sha256",
	}, "lineage.floor"); err != nil {
		return err
	}
	authority, err := decodeTournamentRawObject(root["source_authority"], "source_authority")
	if err != nil {
		return err
	}
	if err := requireTournamentRawKeys(authority, []string{"build_cells", "modules", "physical_policy", "policy", "schema_version"}, "source_authority"); err != nil {
		return err
	}
	physical, err := decodeTournamentRawObject(authority["physical_policy"], "source_authority.physical_policy")
	if err != nil {
		return err
	}
	if err := requireTournamentRawKeys(physical, []string{"id", "root_controls", "runtime_assets", "trees"}, "source_authority.physical_policy"); err != nil {
		return err
	}
	var modules []json.RawMessage
	if err := json.Unmarshal(authority["modules"], &modules); err != nil || modules == nil {
		return errors.New("source_authority.modules must be a non-null array")
	}
	for index, raw := range modules {
		description := fmt.Sprintf("source_authority.modules[%d]", index)
		module, err := decodeTournamentRawObject(raw, description)
		if err != nil {
			return err
		}
		if err := requireTournamentRawKeys(module, []string{"buildable", "id", "module_path", "root"}, description); err != nil {
			return err
		}
	}
	var cells []json.RawMessage
	if err := json.Unmarshal(authority["build_cells"], &cells); err != nil || cells == nil {
		return errors.New("source_authority.build_cells must be a non-null array")
	}
	for index, raw := range cells {
		description := fmt.Sprintf("source_authority.build_cells[%d]", index)
		cell, err := decodeTournamentRawObject(raw, description)
		if err != nil {
			return err
		}
		if err := requireTournamentRawKeys(cell, []string{
			"architecture_feature", "build_tags", "cgo_enabled", "goarch", "goos", "id",
			"module_id", "package_patterns", "selection_flags",
		}, description); err != nil {
			return err
		}
		feature, err := decodeTournamentRawObject(cell["architecture_feature"], description+".architecture_feature")
		if err != nil {
			return err
		}
		if err := requireTournamentRawKeys(feature, []string{"name", "value"}, description+".architecture_feature"); err != nil {
			return err
		}
	}
	return nil
}

func validateTournamentManifestV4Shape(root map[string]json.RawMessage) error {
	required := []string{
		"benchmarks", "go_diagnostic_timeout_ns", "id", "orchestration_watchdog_timeout_ns", "package",
		"required", "runner_watchdog_timeout_ns", "variant_ids", "workload_definitions",
	}
	optional := []string{
		"benchmark_goos", "benchmark_leaves", "benchmark_variant_extra_leaves", "benchmark_variant_groups",
		"default_variant_id",
	}
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, key := range append(required, optional...) {
		allowed[key] = struct{}{}
	}
	var lanes []json.RawMessage
	if err := json.Unmarshal(root["lanes"], &lanes); err != nil || len(lanes) == 0 {
		return errors.New("manifest v4 lanes must be a non-null nonempty array")
	}
	seen := make(map[string]struct{}, len(lanes))
	for index, raw := range lanes {
		description := fmt.Sprintf("manifest v4 lane %d", index)
		lane, err := decodeTournamentRawObject(raw, description)
		if err != nil {
			return err
		}
		for _, key := range required {
			if _, ok := lane[key]; !ok {
				return fmt.Errorf("%s omits required field %q", description, key)
			}
		}
		for key, value := range lane {
			if _, ok := allowed[key]; !ok {
				return fmt.Errorf("%s has unknown field %q", description, key)
			}
			if string(bytes.TrimSpace(value)) == "null" {
				return fmt.Errorf("%s field %q is null", description, key)
			}
		}
		var typed manifestLane
		if err := json.Unmarshal(raw, &typed); err != nil {
			return fmt.Errorf("decode %s: %w", description, err)
		}
		if !tournamentManifestV5TokenPattern.MatchString(typed.ID) || typed.Package == "" ||
			typed.Benchmarks == nil || typed.VariantIDs == nil || typed.WorkloadDefinitions == nil {
			return fmt.Errorf("%s is incomplete", description)
		}
		if _, duplicate := seen[typed.ID]; duplicate {
			return fmt.Errorf("manifest v4 repeats lane %q", typed.ID)
		}
		seen[typed.ID] = struct{}{}
		if err := validateTournamentManifestV5Timeouts(typed); err != nil {
			return fmt.Errorf("%s: %w", description, err)
		}
	}
	return nil
}

func validateTournamentManifestV5Shape(root map[string]json.RawMessage) error {
	var authority manifestSourceAuthority
	if err := json.Unmarshal(root["source_authority"], &authority); err != nil {
		return fmt.Errorf("decode manifest v5 source authority: %w", err)
	}
	knownCells := make(map[string]struct{}, len(authority.BuildCells))
	for _, cell := range authority.BuildCells {
		knownCells[cell.ID] = struct{}{}
	}
	var lanes []json.RawMessage
	if err := json.Unmarshal(root["lanes"], &lanes); err != nil || lanes == nil {
		return errors.New("manifest v5 lanes must be a non-null array")
	}
	previousLane := ""
	seenBindings := make(map[string]string)
	for index, raw := range lanes {
		description := fmt.Sprintf("manifest v5 lane %d", index)
		lane, err := decodeTournamentRawObject(raw, description)
		if err != nil {
			return err
		}
		if err := requireTournamentRawKeys(lane, []string{
			"benchmark_bindings", "build_cell_ids", "go_diagnostic_timeout_ns", "id",
			"orchestration_watchdog_timeout_ns", "required", "runner_watchdog_timeout_ns",
		}, description); err != nil {
			return err
		}
		for key, value := range lane {
			if string(bytes.TrimSpace(value)) == "null" {
				return fmt.Errorf("%s field %q is null", description, key)
			}
		}
		var typed manifestLane
		if err := json.Unmarshal(raw, &typed); err != nil {
			return fmt.Errorf("decode %s: %w", description, err)
		}
		if !tournamentManifestV5TokenPattern.MatchString(typed.ID) || typed.ID <= previousLane {
			return fmt.Errorf("%s has invalid or unsorted ID %q", description, typed.ID)
		}
		previousLane = typed.ID
		if len(typed.BuildCellIDs) == 0 || !slices.IsSorted(typed.BuildCellIDs) {
			return fmt.Errorf("%s build_cell_ids must be a nonempty sorted set", description)
		}
		for cellIndex, cellID := range typed.BuildCellIDs {
			if !tournamentManifestV5IDPattern.MatchString(cellID) ||
				cellIndex != 0 && typed.BuildCellIDs[cellIndex-1] == cellID {
				return fmt.Errorf("%s has invalid build cell %q", description, cellID)
			}
			if _, ok := knownCells[cellID]; !ok {
				return fmt.Errorf("%s has unknown build cell %q", description, cellID)
			}
		}
		if err := validateTournamentManifestV5Timeouts(typed); err != nil {
			return fmt.Errorf("%s: %w", description, err)
		}
		var bindings []json.RawMessage
		if err := json.Unmarshal(lane["benchmark_bindings"], &bindings); err != nil || len(bindings) == 0 {
			return fmt.Errorf("%s benchmark_bindings must be a non-null nonempty array", description)
		}
		for bindingIndex, bindingRaw := range bindings {
			bindingDescription := fmt.Sprintf("%s benchmark binding %d", description, bindingIndex)
			binding, err := decodeTournamentRawObject(bindingRaw, bindingDescription)
			if err != nil {
				return err
			}
			if err := requireTournamentRawKeys(binding, []string{
				"binding_id", "implementation_id", "module_id",
			}, bindingDescription); err != nil {
				return err
			}
			for key, value := range binding {
				if string(bytes.TrimSpace(value)) == "null" {
					return fmt.Errorf("%s field %q is null", bindingDescription, key)
				}
			}
			var typedBinding manifestBindingProjection
			if err := json.Unmarshal(bindingRaw, &typedBinding); err != nil {
				return fmt.Errorf("decode %s: %w", bindingDescription, err)
			}
			if !tournamentManifestV5IDPattern.MatchString(typedBinding.BindingID) ||
				!tournamentManifestV5IDPattern.MatchString(typedBinding.ImplementationID) ||
				!tournamentManifestV5TokenPattern.MatchString(typedBinding.ModuleID) {
				return fmt.Errorf("%s has invalid identifiers", bindingDescription)
			}
			if owner, duplicate := seenBindings[typedBinding.BindingID]; duplicate {
				return fmt.Errorf("manifest v5 lanes %q and %q repeat binding %q", owner, typed.ID, typedBinding.BindingID)
			}
			seenBindings[typedBinding.BindingID] = typed.ID
		}
	}

	var dispositions []json.RawMessage
	if err := json.Unmarshal(root["root_dispositions"], &dispositions); err != nil || dispositions == nil {
		return errors.New("manifest v5 root_dispositions must be a non-null array")
	}
	previousRoot := ""
	for index, raw := range dispositions {
		description := fmt.Sprintf("manifest v5 root disposition %d", index)
		disposition, err := decodeTournamentRawObject(raw, description)
		if err != nil {
			return err
		}
		if err := requireTournamentRawKeys(disposition, []string{
			"disposition_id", "raw_root_id",
		}, description); err != nil {
			return err
		}
		for key, value := range disposition {
			if string(bytes.TrimSpace(value)) == "null" {
				return fmt.Errorf("%s field %q is null", description, key)
			}
		}
		var typed manifestRootDisposition
		if err := json.Unmarshal(raw, &typed); err != nil {
			return fmt.Errorf("decode %s: %w", description, err)
		}
		if !tournamentManifestV5IDPattern.MatchString(typed.RawRootID) ||
			!tournamentManifestV5IDPattern.MatchString(typed.DispositionID) || typed.RawRootID <= previousRoot {
			return fmt.Errorf("%s is invalid or unsorted", description)
		}
		previousRoot = typed.RawRootID
	}
	return nil
}

func validateTournamentManifestV5Timeouts(lane manifestLane) error {
	if lane.GoDiagnosticTimeoutNS <= 0 || lane.RunnerWatchdogTimeoutNS < lane.GoDiagnosticTimeoutNS ||
		lane.OrchestrationWatchdogNS <= lane.RunnerWatchdogTimeoutNS ||
		lane.OrchestrationWatchdogNS-lane.RunnerWatchdogTimeoutNS < 600_000_000_000 {
		return errors.New("invalid diagnostic, runner, or orchestration timeout ordering")
	}
	return nil
}

func decodeTournamentRawObject(data []byte, description string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be an object", description)
	}
	return object, nil
}

func requireTournamentRawKeys(object map[string]json.RawMessage, expected []string, description string) error {
	actual := make([]string, 0, len(object))
	for key := range object {
		actual = append(actual, key)
	}
	slices.Sort(actual)
	if !slices.Equal(actual, expected) {
		return fmt.Errorf("%s keys = %q, want %q", description, actual, expected)
	}
	return nil
}

func rejectTournamentManifestDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := validateTournamentJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("manifest JSON has multiple top-level values")
		}
		return err
	}
	return nil
}

func validateTournamentJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delimiter == '[' {
		for decoder.More() {
			if err := validateTournamentJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	}
	if delimiter != '{' {
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	keys := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("manifest JSON object key is not a string")
		}
		if _, exists := keys[key]; exists {
			return fmt.Errorf("manifest JSON object has duplicate key %q", key)
		}
		keys[key] = struct{}{}
		if err := validateTournamentJSONValue(decoder); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}
