package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
)

type manifestSourceHistoryReference struct { // betteralign:ignore canonical JSON field order
	Path          string                        `json:"path"`
	SchemaVersion int                           `json:"schema_version"`
	SHA256        string                        `json:"sha256"`
	Floor         manifestHistoryFloorReference `json:"floor"`
}

type manifestHistoryFloorReference struct { // betteralign:ignore canonical JSON field order
	Path                      string `json:"path"`
	SchemaVersion             int    `json:"schema_version"`
	Sequence                  int    `json:"sequence"`
	SHA256                    string `json:"sha256"`
	CumulativeRecordSetSHA256 string `json:"cumulative_record_set_sha256"`
}

type manifestV5BindingProjection struct { // betteralign:ignore canonical JSON field order
	BindingID        string `json:"binding_id"`
	ImplementationID string `json:"implementation_id"`
	ModuleID         string `json:"module_id"`
}

type manifestV5Lane struct { // betteralign:ignore canonical JSON field order
	ID                      string                        `json:"id"`
	Required                bool                          `json:"required"`
	BuildCellIDs            []string                      `json:"build_cell_ids"`
	BenchmarkBindings       []manifestV5BindingProjection `json:"benchmark_bindings"`
	GoDiagnosticTimeoutNS   int64                         `json:"go_diagnostic_timeout_ns"`
	RunnerWatchdogTimeoutNS int64                         `json:"runner_watchdog_timeout_ns"`
	OrchestrationWatchdogNS int64                         `json:"orchestration_watchdog_timeout_ns"`
}

type manifestV5RootDisposition struct { // betteralign:ignore canonical JSON field order
	RawRootID     string `json:"raw_root_id"`
	DispositionID string `json:"disposition_id"`
}

func validateSourceManifestV5ProjectionShape(root map[string]json.RawMessage) error {
	var authority manifestSourceAuthority
	if err := decodeManifestReference(root["source_authority"], &authority); err != nil {
		return fmt.Errorf("decode manifest v5 source authority: %w", err)
	}
	buildCells := make(map[string]struct{}, len(authority.BuildCells))
	for _, cell := range authority.BuildCells {
		buildCells[cell.ID] = struct{}{}
	}
	var lanes []json.RawMessage
	if err := json.Unmarshal(root["lanes"], &lanes); err != nil || lanes == nil {
		return errors.New("manifest v5 lanes must be a non-null array")
	}
	previousLane := ""
	for index, raw := range lanes {
		description := fmt.Sprintf("manifest v5 lane %d", index)
		lane, err := decodeSourceRawObject(raw, description)
		if err != nil {
			return err
		}
		if err := requireSourceRawKeys(lane, []string{
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
		var typed manifestV5Lane
		if err := decodeManifestReference(raw, &typed); err != nil {
			return fmt.Errorf("decode %s: %w", description, err)
		}
		if typed.ID <= previousLane {
			return fmt.Errorf("manifest v5 lanes are not a strictly sorted ID set at %q", typed.ID)
		}
		previousLane = typed.ID
		if err := validateManifestV5LaneShape(typed); err != nil {
			return err
		}
		for _, buildCellID := range typed.BuildCellIDs {
			if _, ok := buildCells[buildCellID]; !ok {
				return fmt.Errorf("manifest v5 lane %q has unknown build cell %q", typed.ID, buildCellID)
			}
		}
		var bindings []json.RawMessage
		if err := json.Unmarshal(lane["benchmark_bindings"], &bindings); err != nil || bindings == nil {
			return fmt.Errorf("%s benchmark_bindings must be a non-null array", description)
		}
		for bindingIndex, bindingRaw := range bindings {
			bindingDescription := fmt.Sprintf("%s benchmark binding %d", description, bindingIndex)
			binding, err := decodeSourceRawObject(bindingRaw, bindingDescription)
			if err != nil {
				return err
			}
			if err := requireSourceRawKeys(binding, []string{
				"binding_id", "implementation_id", "module_id",
			}, bindingDescription); err != nil {
				return err
			}
			for key, value := range binding {
				if string(bytes.TrimSpace(value)) == "null" {
					return fmt.Errorf("%s field %q is null", bindingDescription, key)
				}
			}
		}
	}

	var dispositions []json.RawMessage
	if err := json.Unmarshal(root["root_dispositions"], &dispositions); err != nil || dispositions == nil {
		return errors.New("manifest v5 root_dispositions must be a non-null array")
	}
	previousRoot := ""
	for index, raw := range dispositions {
		description := fmt.Sprintf("manifest v5 root disposition %d", index)
		disposition, err := decodeSourceRawObject(raw, description)
		if err != nil {
			return err
		}
		if err := requireSourceRawKeys(disposition, []string{
			"disposition_id", "raw_root_id",
		}, description); err != nil {
			return err
		}
		var typed manifestV5RootDisposition
		if err := decodeManifestReference(raw, &typed); err != nil {
			return fmt.Errorf("decode %s: %w", description, err)
		}
		if !lineageIDPattern.MatchString(typed.RawRootID) || !lineageIDPattern.MatchString(typed.DispositionID) ||
			typed.RawRootID <= previousRoot {
			return fmt.Errorf("%s is invalid or unsorted", description)
		}
		previousRoot = typed.RawRootID
	}
	return nil
}

func verifyManifestV5Lineage(manifestPath string, manifest sourceManifestEnvelope) error {
	if manifest.Lineage.SchemaVersion != lineageLatestSchemaVersion {
		return fmt.Errorf("lineage schema = %d, want %d", manifest.Lineage.SchemaVersion, lineageLatestSchemaVersion)
	}
	var sourceReference manifestSourceHistoryReference
	if err := decodeManifestReference(manifest.SourceHistory, &sourceReference); err != nil {
		return fmt.Errorf("decode source-history reference: %w", err)
	}
	if sourceReference.Path != "source_history.json" || sourceReference.SchemaVersion != historySchemaVersion ||
		!historySHA256Pattern.MatchString(sourceReference.SHA256) {
		return errors.New("invalid source-history inventory reference")
	}
	wantHistoryFloorPath := fmt.Sprintf("historyfloors/%06d.json", sourceReference.Floor.Sequence)
	if sourceReference.Floor.Sequence <= 0 || sourceReference.Floor.Sequence > 999999 ||
		sourceReference.Floor.Path != wantHistoryFloorPath ||
		sourceReference.Floor.SchemaVersion != historyFloorSchemaVersion ||
		!historySHA256Pattern.MatchString(sourceReference.Floor.SHA256) ||
		!historySHA256Pattern.MatchString(sourceReference.Floor.CumulativeRecordSetSHA256) {
		return errors.New("invalid source-history floor reference")
	}

	directory := filepath.Dir(manifestPath)
	historyPath := filepath.Join(directory, filepath.FromSlash(sourceReference.Path))
	lineagePath := filepath.Join(directory, filepath.FromSlash(manifest.Lineage.Path))
	bundle, err := loadLineageAuthorityBundle(lineagePath, historyPath)
	if err != nil {
		return err
	}
	inventory, history := bundle.Inventory, bundle.History
	if inventory.SchemaVersion != manifest.Lineage.SchemaVersion {
		return fmt.Errorf("lineage inventory schema = %d, want %d", inventory.SchemaVersion, manifest.Lineage.SchemaVersion)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(bundle.HistoryData)); got != sourceReference.SHA256 {
		return fmt.Errorf("source-history SHA-256 = %s, want %s", got, sourceReference.SHA256)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(bundle.InventoryData)); got != manifest.Lineage.SHA256 {
		return fmt.Errorf("lineage SHA-256 = %s, want %s", got, manifest.Lineage.SHA256)
	}
	sourceFloors, err := loadStableHistoryFloors(history, historyPath)
	if err != nil {
		return err
	}
	if len(sourceFloors) == 0 {
		return errors.New("source history has no immutable floor state")
	}
	sourceHead := sourceFloors[len(sourceFloors)-1].Authority
	historyHead := historyFloorHead{
		Path: sourceHead.Path, SHA256: sourceHead.SHA256,
		CumulativeRecordSetSHA256: sourceHead.CumulativeRecordSetSHA256, Sequence: sourceHead.Sequence,
	}
	wantHistoryHead := historyFloorHead{
		Path:                      sourceReference.Floor.Path,
		SHA256:                    sourceReference.Floor.SHA256,
		CumulativeRecordSetSHA256: sourceReference.Floor.CumulativeRecordSetSHA256,
		Sequence:                  sourceReference.Floor.Sequence,
	}
	if historyHead != wantHistoryHead {
		return fmt.Errorf("source-history floor head = %+v, want %+v", historyHead, wantHistoryHead)
	}
	lineageHead, err := validateLineageFloorsWithSource(inventory, lineagePath, sourceFloors)
	if err != nil {
		return err
	}
	wantLineageHead := lineageFloorHead{
		Path:                      manifest.Lineage.Floor.Path,
		SHA256:                    manifest.Lineage.Floor.SHA256,
		CumulativeRecordSetSHA256: manifest.Lineage.Floor.CumulativeRecordSetSHA256,
		Sequence:                  manifest.Lineage.Floor.Sequence,
		LineageSchemaVersion:      manifest.Lineage.SchemaVersion,
	}
	if lineageHead != wantLineageHead {
		return fmt.Errorf("lineage floor head = %+v, want %+v", lineageHead, wantLineageHead)
	}
	if err := validateManifestV5ExecutionProjection(manifest, inventory); err != nil {
		return fmt.Errorf("manifest v5 execution projection: %w", err)
	}
	return nil
}

func validateManifestV5ExecutionProjection(manifest sourceManifestEnvelope, inventory lineageCatalog) error {
	var lanes []manifestV5Lane
	if err := decodeManifestReference(manifest.Lanes, &lanes); err != nil {
		return fmt.Errorf("decode lanes: %w", err)
	}
	var rootDispositions []manifestV5RootDisposition
	if err := decodeManifestReference(manifest.RootDispositions, &rootDispositions); err != nil {
		return fmt.Errorf("decode root dispositions: %w", err)
	}

	bindings := make(map[string]lineageBinding, len(inventory.Bindings))
	selectable := make(map[string]struct{})
	for _, binding := range inventory.Bindings {
		bindings[binding.ID] = binding
		if binding.Applicability == "executable" || binding.Applicability == "diagnostic" {
			selectable[binding.ID] = struct{}{}
		}
	}
	rawRoots := make(map[string]lineageRawRoot, len(inventory.RawRoots))
	for _, root := range inventory.RawRoots {
		rawRoots[root.ID] = root
	}
	harnesses := make(map[string]lineageHarness, len(inventory.Harnesses))
	for _, harness := range inventory.Harnesses {
		harnesses[harness.ID] = harness
	}
	cells := make(map[string]manifestSourceCell, len(manifest.SourceAuthority.BuildCells))
	for _, cell := range manifest.SourceAuthority.BuildCells {
		cells[cell.ID] = cell
	}

	projected := make(map[string]string, len(selectable))
	selectedRoots := make(map[string]struct{})
	for _, lane := range lanes {
		if err := validateManifestV5LaneShape(lane); err != nil {
			return err
		}
		usedCells := make(map[string]struct{}, len(lane.BuildCellIDs))
		var previousKey []string
		for _, projection := range lane.BenchmarkBindings {
			binding, ok := bindings[projection.BindingID]
			if !ok {
				return fmt.Errorf("lane %q has unknown binding %q", lane.ID, projection.BindingID)
			}
			if owner, duplicate := projected[binding.ID]; duplicate {
				return fmt.Errorf("lanes %q and %q repeat binding %q", owner, lane.ID, binding.ID)
			}
			if _, ok := selectable[binding.ID]; !ok {
				return fmt.Errorf("lane %q selects non-executable binding %q with applicability %q", lane.ID, binding.ID, binding.Applicability)
			}
			root := rawRoots[binding.RawRootID]
			harness := harnesses[binding.HarnessID]
			if projection.ImplementationID != binding.ImplementationID || projection.ModuleID != root.ModuleID ||
				harness.BuildSelection.ModuleID != root.ModuleID || harness.BuildSelection.Package != root.Package {
				return fmt.Errorf("lane %q binding %q projection disagrees with lineage", lane.ID, binding.ID)
			}
			cell, ok := cells[harness.BuildSelection.BuildCellID]
			if !ok || !manifestV5HarnessCellEqual(harness.BuildSelection, cell) {
				return fmt.Errorf("lane %q binding %q harness cell disagrees with source authority", lane.ID, binding.ID)
			}
			if _, ok := slices.BinarySearch(lane.BuildCellIDs, harness.BuildSelection.BuildCellID); !ok {
				return fmt.Errorf("lane %q binding %q harness cell %q is absent from lane", lane.ID, binding.ID, harness.BuildSelection.BuildCellID)
			}
			key, err := manifestV5BindingOrderKey(binding, root, harness)
			if err != nil {
				return err
			}
			if previousKey != nil && slices.Compare(previousKey, key) >= 0 {
				return fmt.Errorf("lane %q bindings are not in canonical lineage order at %q", lane.ID, binding.ID)
			}
			previousKey = key
			projected[binding.ID] = lane.ID
			selectedRoots[binding.RawRootID] = struct{}{}
			usedCells[harness.BuildSelection.BuildCellID] = struct{}{}
		}
		wantCells := make([]string, 0, len(usedCells))
		for cellID := range usedCells {
			wantCells = append(wantCells, cellID)
		}
		slices.Sort(wantCells)
		if !slices.Equal(lane.BuildCellIDs, wantCells) {
			return fmt.Errorf("lane %q build cells = %q, selected harness cells = %q", lane.ID, lane.BuildCellIDs, wantCells)
		}
	}
	if len(projected) != len(selectable) {
		missing := make([]string, 0, len(selectable)-len(projected))
		for bindingID := range selectable {
			if _, ok := projected[bindingID]; !ok {
				missing = append(missing, bindingID)
			}
		}
		slices.Sort(missing)
		return fmt.Errorf("manifest omits executable lineage bindings %q", missing)
	}

	lineageDispositions := make(map[string]lineageDisposition)
	for _, disposition := range inventory.Dispositions {
		if disposition.SubjectKind != "raw-root" {
			continue
		}
		lineageDispositions[disposition.ID] = disposition
	}
	seenDispositions := make(map[string]struct{}, len(rootDispositions))
	selectedDispositionRoots := make(map[string]string, len(rootDispositions))
	for _, projection := range rootDispositions {
		disposition, ok := lineageDispositions[projection.DispositionID]
		if !ok || disposition.SubjectID != projection.RawRootID {
			return fmt.Errorf("raw-root disposition %q/%q disagrees with lineage", projection.RawRootID, projection.DispositionID)
		}
		if _, duplicate := seenDispositions[projection.DispositionID]; duplicate {
			return fmt.Errorf("manifest repeats raw-root disposition %q", projection.DispositionID)
		}
		if previous, duplicate := selectedDispositionRoots[projection.RawRootID]; duplicate {
			return fmt.Errorf("manifest raw root %q has dispositions %q and %q", projection.RawRootID, previous, projection.DispositionID)
		}
		seenDispositions[projection.DispositionID] = struct{}{}
		selectedDispositionRoots[projection.RawRootID] = projection.DispositionID
	}

	aliasGovernedRoots := make(map[string]struct{})
	for _, binding := range inventory.Bindings {
		if binding.Applicability != "alias-only" {
			continue
		}
		for _, alias := range inventory.Aliases {
			if alias.Kind != "helper-identity" || alias.AliasSubjectID != binding.ID || alias.Rerun {
				continue
			}
			if canonical, selected := projected[alias.CanonicalSubjectID]; selected && canonical != "" {
				aliasGovernedRoots[binding.RawRootID] = struct{}{}
			}
		}
	}
	for _, root := range inventory.RawRoots {
		_, bound := selectedRoots[root.ID]
		if _, aliasGoverned := aliasGovernedRoots[root.ID]; aliasGoverned {
			bound = true
		}
		_, disposed := selectedDispositionRoots[root.ID]
		if bound == disposed {
			return fmt.Errorf("raw root %q must be governed by exactly one of selected binding, selected alias, or current disposition", root.ID)
		}
	}
	return nil
}

func manifestV5HarnessCellEqual(selection lineageBuildSelection, cell manifestSourceCell) bool {
	return cell.CGOEnabled != nil && selection.BuildCellID == cell.ID && selection.ModuleID == cell.ModuleID &&
		selection.GOOS == cell.GOOS && selection.GOARCH == cell.GOARCH && selection.CGOEnabled == *cell.CGOEnabled &&
		selection.ArchitectureFeature.Name == cell.ArchitectureFeature.Name &&
		selection.ArchitectureFeature.Value == cell.ArchitectureFeature.Value &&
		slices.Equal(selection.BuildTags, cell.BuildTags) && slices.Equal(selection.SelectionFlags, cell.SelectionFlags)
}

func manifestV5BindingOrderKey(
	binding lineageBinding,
	root lineageRawRoot,
	harness lineageHarness,
) ([]string, error) {
	configuration, err := json.Marshal(binding.Configuration)
	if err != nil {
		return nil, fmt.Errorf("encode binding %q configuration: %w", binding.ID, err)
	}
	return []string{
		root.ModuleID,
		root.Package,
		binding.Benchmark,
		binding.ImplementationID,
		binding.WorkloadID,
		string(configuration),
		harness.BuildSelection.BuildCellID,
		binding.ID,
	}, nil
}

func decodeManifestReference(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("manifest reference has trailing JSON")
	}
	return nil
}

func validateManifestV5LaneShape(lane manifestV5Lane) error {
	if !lineageTokenPattern.MatchString(lane.ID) || len(lane.BuildCellIDs) == 0 || len(lane.BenchmarkBindings) == 0 ||
		!slices.IsSorted(lane.BuildCellIDs) {
		return fmt.Errorf("manifest v5 lane %q is incomplete or unsorted", lane.ID)
	}
	for index, buildCellID := range lane.BuildCellIDs {
		if !lineageIDPattern.MatchString(buildCellID) || index != 0 && lane.BuildCellIDs[index-1] == buildCellID {
			return fmt.Errorf("manifest v5 lane %q has invalid build-cell set", lane.ID)
		}
	}
	seenBindings := make(map[string]struct{}, len(lane.BenchmarkBindings))
	for _, binding := range lane.BenchmarkBindings {
		if !lineageIDPattern.MatchString(binding.BindingID) || !lineageIDPattern.MatchString(binding.ImplementationID) ||
			!lineageTokenPattern.MatchString(binding.ModuleID) {
			return fmt.Errorf("manifest v5 lane %q has invalid binding projection", lane.ID)
		}
		if _, duplicate := seenBindings[binding.BindingID]; duplicate {
			return fmt.Errorf("manifest v5 lane %q repeats binding %q", lane.ID, binding.BindingID)
		}
		seenBindings[binding.BindingID] = struct{}{}
	}
	return validateProfileTimeouts(
		lane.GoDiagnosticTimeoutNS,
		lane.RunnerWatchdogTimeoutNS,
		lane.OrchestrationWatchdogNS,
	)
}
