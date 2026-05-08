package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

var manifestV4LaneRequiredKeys = []string{
	"benchmarks",
	"go_diagnostic_timeout_ns",
	"id",
	"orchestration_watchdog_timeout_ns",
	"package",
	"required",
	"runner_watchdog_timeout_ns",
	"variant_ids",
	"workload_definitions",
}

var manifestV4LaneOptionalKeys = []string{
	"benchmark_goos",
	"benchmark_leaves",
	"benchmark_variant_extra_leaves",
	"benchmark_variant_groups",
	"default_variant_id",
}

func validateSourceManifestV4ProjectionShape(root map[string]json.RawMessage) error {
	var lanes []json.RawMessage
	if err := json.Unmarshal(root["lanes"], &lanes); err != nil || len(lanes) == 0 {
		return errors.New("manifest v4 lanes must be a non-null nonempty array")
	}
	seen := make(map[string]struct{}, len(lanes))
	for index, raw := range lanes {
		description := fmt.Sprintf("manifest v4 lane %d", index)
		lane, err := decodeSourceRawObject(raw, description)
		if err != nil {
			return err
		}
		if err := validateManifestV4LaneKeys(lane, description); err != nil {
			return err
		}
		for key, value := range lane {
			if string(bytes.TrimSpace(value)) == "null" {
				return fmt.Errorf("%s field %q is null", description, key)
			}
		}
		var typed profileLane
		if err := decodeManifestReference(raw, &typed); err != nil {
			return fmt.Errorf("decode %s: %w", description, err)
		}
		if !lineageTokenPattern.MatchString(typed.ID) || !oneLine(typed.Package) {
			return fmt.Errorf("%s has invalid ID or package", description)
		}
		if _, duplicate := seen[typed.ID]; duplicate {
			return fmt.Errorf("manifest v4 repeats lane %q", typed.ID)
		}
		seen[typed.ID] = struct{}{}
		if err := validateProfileTimeouts(
			typed.GoDiagnosticTimeoutNS,
			typed.RunnerWatchdogTimeoutNS,
			typed.OrchestrationWatchdogNS,
		); err != nil {
			return fmt.Errorf("%s timeouts: %w", description, err)
		}
		if err := validateManifestV4RequiredCollections(lane, description); err != nil {
			return err
		}
	}
	return nil
}

func validateManifestV4LaneKeys(lane map[string]json.RawMessage, description string) error {
	allowed := make(map[string]struct{}, len(manifestV4LaneRequiredKeys)+len(manifestV4LaneOptionalKeys))
	for _, key := range manifestV4LaneRequiredKeys {
		allowed[key] = struct{}{}
		if _, ok := lane[key]; !ok {
			return fmt.Errorf("%s omits required field %q", description, key)
		}
	}
	for _, key := range manifestV4LaneOptionalKeys {
		allowed[key] = struct{}{}
	}
	for key := range lane {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("%s has unknown field %q", description, key)
		}
	}
	return nil
}

func validateManifestV4RequiredCollections(lane map[string]json.RawMessage, description string) error {
	for _, key := range []string{"benchmarks", "variant_ids"} {
		var values []string
		if err := json.Unmarshal(lane[key], &values); err != nil || values == nil {
			return fmt.Errorf("%s field %q must be a non-null string array", description, key)
		}
	}
	var workloads map[string]json.RawMessage
	if err := json.Unmarshal(lane["workload_definitions"], &workloads); err != nil || workloads == nil {
		return fmt.Errorf("%s workload_definitions must be a non-null object", description)
	}
	return nil
}
