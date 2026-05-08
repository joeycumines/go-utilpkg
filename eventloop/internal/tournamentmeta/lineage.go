package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
)

const (
	lineageMinimumSchemaVersion = 2
	lineageLatestSchemaVersion  = 3
)

var lineageIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)+$`)
var lineageTokenPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

type lineageCatalog struct { // betteralign:ignore canonical JSON field order
	SchemaVersion       int                     `json:"schema_version"`
	SourceHistorySHA256 string                  `json:"source_history_sha256"`
	Introductions       []lineageIntroduction   `json:"introductions"`
	Snapshots           []lineageSnapshot       `json:"snapshots"`
	Implementations     []lineageImplementation `json:"implementations"`
	Concepts            []lineageConcept        `json:"concepts"`
	RawRoots            []lineageRawRoot        `json:"raw_roots"`
	Harnesses           []lineageHarness        `json:"harnesses"`
	Workloads           []lineageWorkload       `json:"workloads"`
	Bindings            []lineageBinding        `json:"bindings"`
	Aliases             []lineageAlias          `json:"aliases"`
	Reconstructions     []lineageReconstruction `json:"reconstructions"`
	Dispositions        []lineageDisposition    `json:"dispositions"`
}

type lineageIntroduction struct { // betteralign:ignore canonical JSON field order
	ID                 string `json:"id"`
	SourceKind         string `json:"source_kind"`
	SourceID           string `json:"source_id"`
	SourcePath         string `json:"source_path"`
	SourceIdentityKind string `json:"source_identity_kind"`
	SourceIdentity     string `json:"source_identity"`
}

type lineageSnapshot struct { // betteralign:ignore canonical JSON field order
	ID           string   `json:"id"`
	SourceKind   string   `json:"source_kind"`
	SourceID     string   `json:"source_id"`
	SourcePath   string   `json:"source_path"`
	IdentityKind string   `json:"identity_kind"`
	Identity     string   `json:"identity"`
	Adaptations  []string `json:"adaptations"`
}

type lineageImplementation struct { // betteralign:ignore canonical JSON field order
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	IntroductionID string `json:"introduction_id"`
}

type lineageConcept struct { // betteralign:ignore canonical JSON field order
	ID           string `json:"id"`
	Name         string `json:"name"`
	SourcePath   string `json:"source_path"`
	SourceSHA256 string `json:"source_sha256"`
	Status       string `json:"status"`
	Disposition  string `json:"disposition"`
}

type lineageRawRoot struct { // betteralign:ignore canonical JSON field order
	ID           string   `json:"id"`
	ModuleID     string   `json:"module_id"`
	Package      string   `json:"package"`
	Benchmarks   []string `json:"benchmarks"`
	SourcePath   string   `json:"source_path"`
	IdentityKind string   `json:"identity_kind"`
	Identity     string   `json:"identity"`
	SnapshotID   string   `json:"snapshot_id"`
}

type lineageHarness struct { // betteralign:ignore canonical JSON field order
	ID             string                `json:"id"`
	PhysicalRoots  []lineagePhysicalRoot `json:"physical_roots"`
	ClosurePolicy  string                `json:"closure_policy"`
	BuildSelection lineageBuildSelection `json:"build_selection"`
}

type lineagePhysicalRoot struct { // betteralign:ignore canonical JSON field order
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Path     string `json:"path"`
	Identity string `json:"identity"`
}

type lineageBuildSelection struct { // betteralign:ignore canonical JSON field order
	ModuleID            string                     `json:"module_id"`
	Package             string                     `json:"package"`
	BuildCellID         string                     `json:"build_cell_id"`
	GOOS                string                     `json:"goos"`
	GOARCH              string                     `json:"goarch"`
	CGOEnabled          bool                       `json:"cgo_enabled"`
	GoVersion           string                     `json:"go_version,omitempty"`
	GoDirective         string                     `json:"go_directive,omitempty"`
	ArchitectureFeature lineageArchitectureFeature `json:"architecture_feature"`
	BuildTags           []string                   `json:"build_tags"`
	SelectionFlags      []string                   `json:"selection_flags"`
}

type lineageArchitectureFeature struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type lineageWorkload struct { // betteralign:ignore canonical JSON field order
	ID              string                  `json:"id"`
	Operation       string                  `json:"operation"`
	Parameters      []lineageSetting        `json:"parameters"`
	SemanticHarness *lineageSemanticHarness `json:"semantic_harness,omitempty"`
}

type lineageSemanticHarness struct { // betteralign:ignore canonical JSON field order
	ID       string `json:"id"`
	Setup    string `json:"setup"`
	Timed    string `json:"timed"`
	Teardown string `json:"teardown"`
}

type lineageBinding struct { // betteralign:ignore canonical JSON field order
	ID               string           `json:"id"`
	RawRootID        string           `json:"raw_root_id"`
	Benchmark        string           `json:"benchmark"`
	ImplementationID string           `json:"implementation_id"`
	SnapshotIDs      []string         `json:"snapshot_ids"`
	WorkloadID       string           `json:"workload_id"`
	HarnessID        string           `json:"harness_id"`
	Applicability    string           `json:"applicability"`
	Configuration    []lineageSetting `json:"configuration"`
	Results          []lineageResult  `json:"results"`
}

type lineageResult struct {
	EmittedLeaf string `json:"emitted_leaf"`
	StableLeaf  string `json:"stable_leaf"`
}

type lineageSetting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type lineageAlias struct { // betteralign:ignore canonical JSON field order
	ID                 string `json:"id"`
	Kind               string `json:"kind"`
	AliasSubjectID     string `json:"alias_subject_id"`
	CanonicalSubjectID string `json:"canonical_subject_id"`
	Reason             string `json:"reason"`
	Rerun              bool   `json:"rerun"`
}

type lineageReconstruction struct { // betteralign:ignore canonical JSON field order
	ID                    string   `json:"id"`
	ConceptID             string   `json:"concept_id"`
	BasisSnapshotIDs      []string `json:"basis_snapshot_ids"`
	Method                string   `json:"method"`
	OutputSnapshotID      string   `json:"output_snapshot_id"`
	OriginalClaimEligible bool     `json:"original_claim_eligible"`
}

type lineageDisposition struct { // betteralign:ignore canonical JSON field order
	ID                  string `json:"id"`
	SubjectKind         string `json:"subject_kind"`
	SubjectID           string `json:"subject_id"`
	SnapshotID          string `json:"snapshot_id"`
	Platform            string `json:"platform"`
	BuildStatus         string `json:"build_status"`
	CorrectnessStatus   string `json:"correctness_status"`
	ComparabilityStatus string `json:"comparability_status"`
	EvidenceStatus      string `json:"evidence_status"`
	Reason              string `json:"reason"`
}

func loadLineage(path string) (lineageCatalog, error) {
	data, err := readRegularStable(path, 0o644)
	if err != nil {
		return lineageCatalog{}, fmt.Errorf("read lineage inventory: %w", err)
	}
	return decodeLineage(data)
}

func decodeLineage(data []byte) (lineageCatalog, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return lineageCatalog{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var inventory lineageCatalog
	if err := decoder.Decode(&inventory); err != nil {
		return lineageCatalog{}, fmt.Errorf("decode lineage inventory: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return lineageCatalog{}, errors.New("lineage inventory has trailing JSON")
	}
	if err := validateLineage(inventory); err != nil {
		return lineageCatalog{}, err
	}
	canonical, err := encodeLineage(inventory)
	if err != nil {
		return lineageCatalog{}, err
	}
	if !bytes.Equal(data, canonical) {
		return lineageCatalog{}, errors.New("lineage inventory is not canonical JSON")
	}
	return inventory, nil
}

func encodeLineage(inventory lineageCatalog) ([]byte, error) {
	data, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode lineage inventory: %w", err)
	}
	return append(data, '\n'), nil
}

func validateLineage(inventory lineageCatalog) error {
	if inventory.SchemaVersion < lineageMinimumSchemaVersion || inventory.SchemaVersion > lineageLatestSchemaVersion {
		return fmt.Errorf("lineage schema = %d, supported range %d..%d", inventory.SchemaVersion, lineageMinimumSchemaVersion, lineageLatestSchemaVersion)
	}
	if !historySHA256Pattern.MatchString(inventory.SourceHistorySHA256) {
		return errors.New("lineage source-history SHA-256 is invalid")
	}
	if inventory.Introductions == nil || inventory.Snapshots == nil || inventory.Implementations == nil ||
		inventory.Concepts == nil || inventory.RawRoots == nil || inventory.Harnesses == nil || inventory.Workloads == nil ||
		inventory.Bindings == nil || inventory.Aliases == nil || inventory.Reconstructions == nil ||
		inventory.Dispositions == nil {
		return errors.New("lineage record arrays must be non-null")
	}
	ids := make(map[string]string)
	introductions := make(map[string]struct{}, len(inventory.Introductions))
	snapshots := make(map[string]struct{}, len(inventory.Snapshots))
	implementations := make(map[string]struct{}, len(inventory.Implementations))
	concepts := make(map[string]struct{}, len(inventory.Concepts))
	rawRoots := make(map[string]lineageRawRoot, len(inventory.RawRoots))
	harnesses := make(map[string]struct{}, len(inventory.Harnesses))
	workloads := make(map[string]struct{}, len(inventory.Workloads))
	semanticHarnesses := make(map[string]lineageSemanticHarness, len(inventory.Workloads))
	check := func(kind, id string, index int, previous *string) error {
		if !lineageIDPattern.MatchString(id) {
			return fmt.Errorf("lineage %s record %d has invalid ID %q", kind, index, id)
		}
		if id <= *previous {
			return fmt.Errorf("lineage %s records are not a strictly sorted ID set at %q", kind, id)
		}
		*previous = id
		if prior, ok := ids[id]; ok {
			return fmt.Errorf("lineage ID %q belongs to both %s and %s", id, prior, kind)
		}
		ids[id] = kind
		return nil
	}

	previous := ""
	for index, record := range inventory.Introductions {
		if err := check("introduction", record.ID, index, &previous); err != nil {
			return err
		}
		if err := validateLineageSource(inventory.SchemaVersion, record.SourceKind, record.SourceID, record.SourcePath, record.SourceIdentityKind, record.SourceIdentity); err != nil {
			return fmt.Errorf("lineage introduction %q: %w", record.ID, err)
		}
		introductions[record.ID] = struct{}{}
	}
	previous = ""
	for index, record := range inventory.Snapshots {
		if err := check("snapshot", record.ID, index, &previous); err != nil {
			return err
		}
		if err := validateLineageSource(inventory.SchemaVersion, record.SourceKind, record.SourceID, record.SourcePath, record.IdentityKind, record.Identity); err != nil {
			return fmt.Errorf("lineage snapshot %q: %w", record.ID, err)
		}
		if err := validateSortedStrings(record.Adaptations, false, "adaptations"); err != nil {
			return fmt.Errorf("lineage snapshot %q: %w", record.ID, err)
		}
		snapshots[record.ID] = struct{}{}
	}
	previous = ""
	for index, record := range inventory.Implementations {
		if err := check("implementation", record.ID, index, &previous); err != nil {
			return err
		}
		if !oneLine(record.Name) || !validLineageImplementationKind(inventory.SchemaVersion, record.Kind) {
			return fmt.Errorf("lineage implementation %q has invalid kind or name", record.ID)
		}
		if _, ok := introductions[record.IntroductionID]; !ok {
			return fmt.Errorf("lineage implementation %q has unknown introduction %q", record.ID, record.IntroductionID)
		}
		implementations[record.ID] = struct{}{}
	}
	previous = ""
	for index, record := range inventory.Concepts {
		if err := check("concept", record.ID, index, &previous); err != nil {
			return err
		}
		if !oneLine(record.Name) || !oneLine(record.Disposition) || validateRelativePath(record.SourcePath) != nil ||
			!historySHA256Pattern.MatchString(record.SourceSHA256) ||
			!lineageEnum(record.Status, "concept-only", "rejected-reconstructible") {
			return fmt.Errorf("lineage concept %q is invalid", record.ID)
		}
		concepts[record.ID] = struct{}{}
	}
	previous = ""
	benchmarkName := regexp.MustCompile(`^Benchmark[A-Za-z0-9_]+$`)
	for index, record := range inventory.RawRoots {
		if err := check("raw-root", record.ID, index, &previous); err != nil {
			return err
		}
		if !lineageTokenPattern.MatchString(record.ModuleID) || !oneLine(record.Package) ||
			len(record.Benchmarks) == 0 || !slices.IsSorted(record.Benchmarks) || validateRelativePath(record.SourcePath) != nil ||
			record.IdentityKind != "sha256" || !historySHA256Pattern.MatchString(record.Identity) {
			return fmt.Errorf("lineage raw root %q is invalid", record.ID)
		}
		for benchmarkIndex, benchmark := range record.Benchmarks {
			if !benchmarkName.MatchString(benchmark) || benchmarkIndex != 0 && record.Benchmarks[benchmarkIndex-1] == benchmark {
				return fmt.Errorf("lineage raw root %q has invalid benchmark set", record.ID)
			}
		}
		if _, ok := snapshots[record.SnapshotID]; !ok {
			return fmt.Errorf("lineage raw root %q has unknown snapshot %q", record.ID, record.SnapshotID)
		}
		rawRoots[record.ID] = record
	}
	previous = ""
	for index, record := range inventory.Harnesses {
		if err := check("harness", record.ID, index, &previous); err != nil {
			return err
		}
		if len(record.PhysicalRoots) == 0 || record.ClosurePolicy != "go-test-package-capture-v1" ||
			record.BuildSelection.BuildTags == nil || record.BuildSelection.SelectionFlags == nil ||
			!lineageTokenPattern.MatchString(record.BuildSelection.ModuleID) || !oneLine(record.BuildSelection.Package) ||
			!lineageIDPattern.MatchString(record.BuildSelection.BuildCellID) ||
			!oneLine(record.BuildSelection.GOOS) || !oneLine(record.BuildSelection.GOARCH) ||
			!regexp.MustCompile(`^GO[A-Z0-9]+$`).MatchString(record.BuildSelection.ArchitectureFeature.Name) ||
			!oneLine(record.BuildSelection.ArchitectureFeature.Value) {
			return fmt.Errorf("lineage harness %q has incomplete selection authority", record.ID)
		}
		if err := validateLineageGoSelection(inventory.SchemaVersion, record.BuildSelection); err != nil {
			return fmt.Errorf("lineage harness %q: %w", record.ID, err)
		}
		if err := validateSortedStrings(record.BuildSelection.BuildTags, false, "build tags"); err != nil {
			return fmt.Errorf("lineage harness %q: %w", record.ID, err)
		}
		if err := validateSortedStrings(record.BuildSelection.SelectionFlags, false, "selection flags"); err != nil {
			return fmt.Errorf("lineage harness %q: %w", record.ID, err)
		}
		rootPrevious := ""
		benchmarkRootCount := 0
		for rootIndex, root := range record.PhysicalRoots {
			if root.ID <= rootPrevious || !lineageEnum(root.Kind, "benchmark-root", "source-file") || !lineageIDPattern.MatchString(root.ID) ||
				validateRelativePath(root.Path) != nil || !historySHA256Pattern.MatchString(root.Identity) {
				return fmt.Errorf("lineage harness %q physical root %d is invalid or unsorted", record.ID, rootIndex)
			}
			if root.Kind == "benchmark-root" {
				rawRoot, ok := rawRoots[root.ID]
				if !ok {
					return fmt.Errorf("lineage harness %q has unknown raw root %q", record.ID, root.ID)
				}
				if rawRoot.ModuleID != record.BuildSelection.ModuleID || rawRoot.Package != record.BuildSelection.Package ||
					rawRoot.SourcePath != root.Path || rawRoot.Identity != root.Identity {
					return fmt.Errorf("lineage harness %q raw root %q authority differs", record.ID, root.ID)
				}
				benchmarkRootCount++
			}
			rootPrevious = root.ID
		}
		if benchmarkRootCount == 0 {
			return fmt.Errorf("lineage harness %q has no benchmark root", record.ID)
		}
		harnesses[record.ID] = struct{}{}
	}
	previous = ""
	for index, record := range inventory.Workloads {
		if err := check("workload", record.ID, index, &previous); err != nil {
			return err
		}
		if !oneLine(record.Operation) || record.Parameters == nil {
			return fmt.Errorf("lineage workload %q is invalid", record.ID)
		}
		switch inventory.SchemaVersion {
		case 2:
			if record.SemanticHarness != nil {
				return fmt.Errorf("lineage workload %q has schema-3 semantic harness under schema 2", record.ID)
			}
		case 3:
			if record.SemanticHarness == nil || !lineageIDPattern.MatchString(record.SemanticHarness.ID) ||
				!oneLine(record.SemanticHarness.Setup) || !oneLine(record.SemanticHarness.Timed) ||
				!oneLine(record.SemanticHarness.Teardown) {
				return fmt.Errorf("lineage workload %q has invalid semantic harness authority", record.ID)
			}
			if previousHarness, exists := semanticHarnesses[record.SemanticHarness.ID]; exists &&
				previousHarness != *record.SemanticHarness {
				return fmt.Errorf("lineage semantic harness %q has conflicting contracts", record.SemanticHarness.ID)
			}
			semanticHarnesses[record.SemanticHarness.ID] = *record.SemanticHarness
		}
		if err := validateLineageSettings(record.Parameters); err != nil {
			return fmt.Errorf("lineage workload %q: %w", record.ID, err)
		}
		workloads[record.ID] = struct{}{}
	}
	previous = ""
	emittedResults := make(map[string]struct {
		binding string
		stable  string
	})
	for index, record := range inventory.Bindings {
		if err := check("binding", record.ID, index, &previous); err != nil {
			return err
		}
		if _, ok := implementations[record.ImplementationID]; !ok {
			return fmt.Errorf("lineage binding %q has unknown implementation %q", record.ID, record.ImplementationID)
		}
		rawRoot, ok := rawRoots[record.RawRootID]
		if !ok {
			return fmt.Errorf("lineage binding %q has unknown raw root %q", record.ID, record.RawRootID)
		}
		if _, found := slices.BinarySearch(rawRoot.Benchmarks, record.Benchmark); !found {
			return fmt.Errorf("lineage binding %q benchmark %q is absent from raw root %q", record.ID, record.Benchmark, record.RawRootID)
		}
		if err := validateSortedStrings(record.SnapshotIDs, true, "snapshot IDs"); err != nil {
			return fmt.Errorf("lineage binding %q: %w", record.ID, err)
		}
		for _, snapshotID := range record.SnapshotIDs {
			if _, ok := snapshots[snapshotID]; !ok {
				return fmt.Errorf("lineage binding %q has unknown snapshot %q", record.ID, snapshotID)
			}
		}
		if _, found := slices.BinarySearch(record.SnapshotIDs, rawRoot.SnapshotID); !found {
			return fmt.Errorf("lineage binding %q omits raw-root snapshot %q", record.ID, rawRoot.SnapshotID)
		}
		if _, ok := workloads[record.WorkloadID]; !ok {
			return fmt.Errorf("lineage binding %q has unknown workload %q", record.ID, record.WorkloadID)
		}
		if _, ok := harnesses[record.HarnessID]; !ok {
			return fmt.Errorf("lineage binding %q has unknown harness %q", record.ID, record.HarnessID)
		}
		harness := findLineageHarness(inventory.Harnesses, record.HarnessID)
		hasRawRoot := false
		for _, root := range harness.PhysicalRoots {
			if root.Kind == "benchmark-root" && root.ID == record.RawRootID {
				hasRawRoot = true
				break
			}
		}
		if !hasRawRoot {
			return fmt.Errorf("lineage binding %q raw root %q is absent from harness %q", record.ID, record.RawRootID, record.HarnessID)
		}
		if record.Configuration == nil {
			return fmt.Errorf("lineage binding %q has null configuration", record.ID)
		}
		if err := validateLineageSettings(record.Configuration); err != nil {
			return fmt.Errorf("lineage binding %q: %w", record.ID, err)
		}
		validApplicability := lineageEnum(record.Applicability, "alias-only", "executable", "not-applicable") ||
			inventory.SchemaVersion >= 3 && record.Applicability == "diagnostic"
		if !validApplicability {
			return fmt.Errorf("lineage binding %q has invalid applicability %q", record.ID, record.Applicability)
		}
		if record.Results == nil {
			return fmt.Errorf("lineage binding %q has null results", record.ID)
		}
		if (record.Applicability == "alias-only" || record.Applicability == "executable") && len(record.Results) == 0 ||
			(record.Applicability == "diagnostic" || record.Applicability == "not-applicable") && len(record.Results) != 0 {
			return fmt.Errorf("lineage binding %q result set contradicts applicability %q", record.ID, record.Applicability)
		}
		previousResult := lineageResult{}
		for resultIndex, result := range record.Results {
			if !validLineageLeaf(result.EmittedLeaf) || !validLineageLeaf(result.StableLeaf) ||
				resultIndex != 0 && compareLineageResult(previousResult, result) >= 0 {
				return fmt.Errorf("lineage binding %q results are not a sorted valid set", record.ID)
			}
			emittedKey := harness.BuildSelection.BuildCellID + "\x00" + harness.BuildSelection.Package + "\x00" +
				record.Benchmark + "\x00" + result.EmittedLeaf
			if owner, exists := emittedResults[emittedKey]; exists {
				return fmt.Errorf(
					"lineage bindings %q and %q map the same emitted result to %q and %q",
					owner.binding,
					record.ID,
					owner.stable,
					result.StableLeaf,
				)
			}
			emittedResults[emittedKey] = struct {
				binding string
				stable  string
			}{binding: record.ID, stable: result.StableLeaf}
			previousResult = result
		}
		if record.Applicability == "diagnostic" || record.Applicability == "not-applicable" {
			var bindingDispositions []lineageDisposition
			for _, disposition := range inventory.Dispositions {
				if disposition.SubjectKind == "binding" && disposition.SubjectID == record.ID {
					bindingDispositions = append(bindingDispositions, disposition)
				}
			}
			if len(bindingDispositions) != 1 {
				return fmt.Errorf("lineage binding %q has %d applicability dispositions, want 1", record.ID, len(bindingDispositions))
			}
			disposition := bindingDispositions[0]
			wantBuild, wantCorrectness := "not-applicable", "not-applicable"
			if record.Applicability == "diagnostic" {
				wantBuild, wantCorrectness = "build-valid", "correctness-invalid"
			}
			wantPlatform := harness.BuildSelection.GOOS + "/" + harness.BuildSelection.GOARCH
			_, hasSnapshot := slices.BinarySearch(record.SnapshotIDs, disposition.SnapshotID)
			if disposition.BuildStatus != wantBuild || disposition.CorrectnessStatus != wantCorrectness ||
				disposition.ComparabilityStatus != "noncomparable" || disposition.EvidenceStatus != "evidence-complete" ||
				disposition.Platform != wantPlatform || !hasSnapshot {
				return fmt.Errorf("lineage binding %q has invalid %s disposition %+v", record.ID, record.Applicability, disposition)
			}
		}
		if record.Applicability == "alias-only" {
			var helperAlias *lineageAlias
			for _, alias := range inventory.Aliases {
				if alias.Kind == "helper-identity" && alias.AliasSubjectID == record.ID && !alias.Rerun {
					if helperAlias != nil {
						return fmt.Errorf("lineage binding %q has multiple helper aliases", record.ID)
					}
					copy := alias
					helperAlias = &copy
				}
			}
			if helperAlias == nil {
				return fmt.Errorf("lineage binding %q has no helper alias", record.ID)
			}
			var canonical *lineageBinding
			for bindingIndex := range inventory.Bindings {
				if inventory.Bindings[bindingIndex].ID == helperAlias.CanonicalSubjectID {
					canonical = &inventory.Bindings[bindingIndex]
					break
				}
			}
			if canonical == nil {
				return fmt.Errorf("lineage binding %q helper alias targets non-binding %q", record.ID, helperAlias.CanonicalSubjectID)
			}
			if canonical.Applicability != "executable" || canonical.ImplementationID != record.ImplementationID ||
				canonical.WorkloadID != record.WorkloadID || canonical.HarnessID != record.HarnessID ||
				!slices.Equal(canonical.SnapshotIDs, record.SnapshotIDs) ||
				!slices.Equal(canonical.Configuration, record.Configuration) ||
				!slices.Equal(stableLineageLeaves(canonical.Results), stableLineageLeaves(record.Results)) {
				return fmt.Errorf("lineage binding %q helper alias differs from canonical binding %q", record.ID, canonical.ID)
			}
		}
	}
	previous = ""
	for index, record := range inventory.Aliases {
		if err := check("alias", record.ID, index, &previous); err != nil {
			return err
		}
		if !lineageEnum(record.Kind, "compatibility-spelling", "exact-source", "helper-identity") ||
			!lineageIDPattern.MatchString(record.AliasSubjectID) || !oneLine(record.Reason) || record.Rerun {
			return fmt.Errorf("lineage alias %q is invalid", record.ID)
		}
		if _, ok := ids[record.CanonicalSubjectID]; !ok {
			return fmt.Errorf("lineage alias %q has unknown canonical subject %q", record.ID, record.CanonicalSubjectID)
		}
		if ids[record.CanonicalSubjectID] == "alias" {
			return fmt.Errorf("lineage alias %q targets another alias", record.ID)
		}
	}
	previous = ""
	for index, record := range inventory.Reconstructions {
		if err := check("reconstruction", record.ID, index, &previous); err != nil {
			return err
		}
		if _, ok := concepts[record.ConceptID]; !ok || !oneLine(record.Method) || record.OriginalClaimEligible {
			return fmt.Errorf("lineage reconstruction %q is invalid", record.ID)
		}
		if len(record.BasisSnapshotIDs) == 0 {
			return fmt.Errorf("lineage reconstruction %q has no basis snapshots", record.ID)
		}
		if err := validateSortedStrings(record.BasisSnapshotIDs, true, "basis snapshots"); err != nil {
			return fmt.Errorf("lineage reconstruction %q: %w", record.ID, err)
		}
		for _, id := range record.BasisSnapshotIDs {
			if _, ok := snapshots[id]; !ok {
				return fmt.Errorf("lineage reconstruction %q has unknown basis snapshot %q", record.ID, id)
			}
		}
		if _, ok := snapshots[record.OutputSnapshotID]; !ok {
			return fmt.Errorf("lineage reconstruction %q has unknown output snapshot %q", record.ID, record.OutputSnapshotID)
		}
	}
	previous = ""
	for index, record := range inventory.Dispositions {
		if err := check("disposition", record.ID, index, &previous); err != nil {
			return err
		}
		if !lineageEnum(record.SubjectKind, "binding", "concept", "implementation", "raw-root", "snapshot") ||
			!lineageIDPattern.MatchString(record.SubjectID) || !oneLine(record.Platform) || !oneLine(record.Reason) ||
			!lineageEnum(record.BuildStatus, "build-invalid", "build-unassessed", "build-valid", "not-applicable") ||
			!lineageEnum(record.CorrectnessStatus, "correctness-invalid", "correctness-unassessed", "correctness-valid", "not-applicable") ||
			!lineageEnum(record.ComparabilityStatus, "comparable", "noncomparable", "reconstructible-only", "unassessed") ||
			!lineageEnum(record.EvidenceStatus, "evidence-complete", "evidence-incomplete", "evidence-none", "provenance-incomplete") {
			return fmt.Errorf("lineage disposition %q is invalid", record.ID)
		}
		switch record.SubjectKind {
		case "concept":
			if _, ok := concepts[record.SubjectID]; !ok {
				return fmt.Errorf("lineage disposition %q has unknown concept %q", record.ID, record.SubjectID)
			}
		case "snapshot":
			if _, ok := snapshots[record.SubjectID]; !ok {
				return fmt.Errorf("lineage disposition %q has unknown snapshot %q", record.ID, record.SubjectID)
			}
		case "implementation":
			if _, ok := implementations[record.SubjectID]; !ok {
				return fmt.Errorf("lineage disposition %q has unknown implementation %q", record.ID, record.SubjectID)
			}
		case "raw-root":
			if _, ok := rawRoots[record.SubjectID]; !ok {
				return fmt.Errorf("lineage disposition %q has unknown raw root %q", record.ID, record.SubjectID)
			}
		case "binding":
			found := false
			for _, binding := range inventory.Bindings {
				if binding.ID == record.SubjectID {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("lineage disposition %q has unknown binding %q", record.ID, record.SubjectID)
			}
		}
		if record.SnapshotID != "none" {
			if _, ok := snapshots[record.SnapshotID]; !ok {
				return fmt.Errorf("lineage disposition %q has unknown snapshot scope %q", record.ID, record.SnapshotID)
			}
		}
	}
	return nil
}

func validateLineageSource(schemaVersion int, sourceKind, sourceID, sourcePath, identityKind, identity string) error {
	validSourceKind := lineageEnum(sourceKind, "external", "file-history", "source-occurrence") ||
		schemaVersion >= 3 && sourceKind == "dynamic-frozen-filesystem"
	if !validSourceKind ||
		!lineageIDPattern.MatchString(sourceID) || sourcePath != "." && validateRelativePath(sourcePath) != nil {
		return errors.New("invalid source authority")
	}
	if sourceKind == "dynamic-frozen-filesystem" && sourceID != "source.eventloop.current" {
		return errors.New("dynamic source authority has an invalid source ID")
	}
	switch identityKind {
	case "git-blob-sha1", "git-tree-sha1":
		if !historyOIDPattern.MatchString(identity) {
			return fmt.Errorf("invalid %s identity", identityKind)
		}
	case "sha256":
		if !historySHA256Pattern.MatchString(identity) {
			return errors.New("invalid SHA-256 identity")
		}
	case "component-tree-sha256":
		if sourceKind != "dynamic-frozen-filesystem" || schemaVersion < 3 || !historySHA256Pattern.MatchString(identity) {
			return errors.New("invalid component-tree SHA-256 identity")
		}
	case "external-id":
		if sourceKind != "external" || !lineageIDPattern.MatchString(identity) {
			return errors.New("invalid external identity")
		}
	default:
		return fmt.Errorf("unknown identity kind %q", identityKind)
	}
	return nil
}

func validLineageImplementationKind(schemaVersion int, kind string) bool {
	if lineageEnum(kind, "external", "future", "goja-adapter", "goja-promise-job", "promise", "scheduler", "wake") {
		return true
	}
	return schemaVersion >= 3 && lineageEnum(kind, "eventtarget", "metrics", "poller", "timer")
}

func validateLineageGoSelection(schemaVersion int, selection lineageBuildSelection) error {
	switch schemaVersion {
	case 2:
		if !oneLine(selection.GoVersion) || selection.GoDirective != "" {
			return errors.New("schema 2 requires go_version and forbids go_directive")
		}
	case 3:
		if selection.GoVersion != "" || !regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?$`).MatchString(selection.GoDirective) {
			return errors.New("schema 3 requires go_directive and forbids go_version")
		}
	default:
		return fmt.Errorf("unsupported lineage schema %d", schemaVersion)
	}
	return nil
}

func validateLineageSettings(values []lineageSetting) error {
	previous := lineageSetting{}
	for index, value := range values {
		if !lineageTokenPattern.MatchString(value.Key) || !oneLine(value.Value) ||
			(index != 0 && (value.Key < previous.Key || value.Key == previous.Key && value.Value <= previous.Value)) {
			return errors.New("settings are not a strictly sorted valid set")
		}
		previous = value
	}
	return nil
}

func validLineageLeaf(value string) bool {
	return value == "" || oneLine(value) && !strings.HasPrefix(value, "/") && !strings.HasSuffix(value, "/")
}

func compareLineageResult(left, right lineageResult) int {
	if result := strings.Compare(left.EmittedLeaf, right.EmittedLeaf); result != 0 {
		return result
	}
	return strings.Compare(left.StableLeaf, right.StableLeaf)
}

func stableLineageLeaves(results []lineageResult) []string {
	values := make([]string, len(results))
	for index, result := range results {
		values[index] = result.StableLeaf
	}
	slices.Sort(values)
	return values
}

func findLineageHarness(records []lineageHarness, id string) lineageHarness {
	for _, record := range records {
		if record.ID == id {
			return record
		}
	}
	panic("validated lineage harness disappeared: " + id)
}

func validateSortedStrings(values []string, requireNonempty bool, description string) error {
	if values == nil || requireNonempty && len(values) == 0 {
		qualifier := ""
		if requireNonempty {
			qualifier = " nonempty"
		}
		return fmt.Errorf("%s must be a non-null%s array", description, qualifier)
	}
	if !slices.IsSorted(values) {
		return fmt.Errorf("%s are not sorted", description)
	}
	for index, value := range values {
		if !oneLine(value) || index != 0 && values[index-1] == value {
			return fmt.Errorf("%s are not a valid set", description)
		}
	}
	return nil
}

func lineageEnum(value string, allowed ...string) bool {
	return slices.Contains(allowed, value)
}

func oneLine(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}
