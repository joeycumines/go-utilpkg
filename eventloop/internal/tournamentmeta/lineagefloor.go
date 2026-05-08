package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	lineageFloorSchemaVersion = 1
	lineageRecordDigestDomain = "go-utilpkg/eventloop/tournament/lineage/record/v1"
	lineageSetDigestDomain    = "go-utilpkg/eventloop/tournament/lineage/set/v1"
	lineageDigestAlgorithm    = "sha256-domain-length-framed-v1"
)

type lineageFloor struct { // betteralign:ignore canonical JSON field order
	SchemaVersion             int                    `json:"schema_version"`
	Sequence                  int                    `json:"sequence"`
	PreviousFloor             *lineageFloorPrevious  `json:"previous_floor"`
	LineageSchemaVersion      int                    `json:"lineage_schema_version"`
	RecordDigestAlgorithm     string                 `json:"record_digest_algorithm"`
	SourceHistoryFloor        lineageSourceFloor     `json:"source_history_floor"`
	Additions                 []lineageFloorAddition `json:"additions"`
	CumulativeCounts          []lineageFloorCount    `json:"cumulative_counts"`
	CumulativeRecordSetSHA256 string                 `json:"cumulative_record_set_sha256"`
}

type lineageFloorPrevious struct { // betteralign:ignore canonical JSON field order
	Sequence                  int    `json:"sequence"`
	Path                      string `json:"path"`
	SHA256                    string `json:"sha256"`
	CumulativeRecordSetSHA256 string `json:"cumulative_record_set_sha256"`
}

type lineageSourceFloor struct { // betteralign:ignore canonical JSON field order
	Path                      string `json:"path"`
	SchemaVersion             int    `json:"schema_version"`
	Sequence                  int    `json:"sequence"`
	SHA256                    string `json:"sha256"`
	CumulativeRecordSetSHA256 string `json:"cumulative_record_set_sha256"`
}

type lineageFloorAddition struct { // betteralign:ignore canonical JSON field order
	Kind         string `json:"kind"`
	ID           string `json:"id"`
	RecordSHA256 string `json:"record_sha256"`
}

type lineageFloorCount struct { // betteralign:ignore canonical JSON field order
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

type lineageFloorRecord struct {
	Kind   string
	ID     string
	Digest string
}

type lineageFloorHead struct {
	Path                      string
	SHA256                    string
	CumulativeRecordSetSHA256 string
	Sequence                  int
	LineageSchemaVersion      int
}

type lineageSourceFloorState struct {
	Authority       lineageSourceFloor
	RecordSequences map[string]int
}

func lineageCommand(arguments []string) int {
	if len(arguments) == 0 {
		return commandError(errors.New("lineage requires floor-generate or verify"))
	}
	switch arguments[0] {
	case "floor-generate":
		return lineageFloorGenerateCommand(arguments[1:])
	case "verify":
		return lineageVerifyCommand(arguments[1:])
	default:
		return commandError(fmt.Errorf("unknown lineage operation %q", arguments[0]))
	}
}

func lineageVerifyCommand(arguments []string) int {
	flags := flag.NewFlagSet("lineage verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	inventoryPath := flags.String("inventory", "", "lineage inventory")
	historyPath := flags.String("source-history", "", "source-history inventory")
	if err := flags.Parse(arguments); err != nil {
		return commandError(err)
	}
	if flags.NArg() != 0 || *inventoryPath == "" || *historyPath == "" ||
		!filepath.IsAbs(*inventoryPath) || !filepath.IsAbs(*historyPath) {
		return commandError(errors.New("lineage verify requires absolute -inventory and -source-history"))
	}
	inventory, history, err := loadLineageAuthority(*inventoryPath, *historyPath)
	if err == nil {
		_, err = validateLineageFloors(inventory, *inventoryPath, history, *historyPath)
	}
	return commandError(err)
}

func lineageFloorGenerateCommand(arguments []string) int {
	flags := flag.NewFlagSet("lineage floor-generate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	inventoryPath := flags.String("inventory", "", "lineage inventory")
	historyPath := flags.String("source-history", "", "source-history inventory")
	floorDirectory := flags.String("floor-directory", "", "lineage floor directory")
	output := flags.String("output", "", "new floor path")
	if err := flags.Parse(arguments); err != nil {
		return commandError(err)
	}
	if flags.NArg() != 0 || *inventoryPath == "" || *historyPath == "" || *floorDirectory == "" || *output == "" ||
		!filepath.IsAbs(*inventoryPath) || !filepath.IsAbs(*historyPath) ||
		!filepath.IsAbs(*floorDirectory) || !filepath.IsAbs(*output) {
		return commandError(errors.New("lineage floor-generate requires absolute -inventory, -source-history, -floor-directory, and -output"))
	}
	inventory, history, err := loadLineageAuthority(*inventoryPath, *historyPath)
	if err != nil {
		return commandError(err)
	}
	floor, err := createLineageFloor(inventory, *inventoryPath, history, *historyPath, *floorDirectory, *output)
	if err != nil {
		return commandError(err)
	}
	data, err := encodeLineageFloor(floor)
	if err != nil {
		return commandError(err)
	}
	if err := writeAtomicNew(*output, data, 0o644); err != nil {
		return commandError(fmt.Errorf("write lineage floor: %w", err))
	}
	return 0
}

func loadLineageAuthority(inventoryPath, historyPath string) (lineageCatalog, historyInventory, error) {
	bundle, err := loadLineageAuthorityBundle(inventoryPath, historyPath)
	return bundle.Inventory, bundle.History, err
}

type lineageAuthorityBundle struct {
	Inventory     lineageCatalog
	History       historyInventory
	InventoryData []byte
	HistoryData   []byte
}

func loadLineageAuthorityBundle(inventoryPath, historyPath string) (lineageAuthorityBundle, error) {
	history, err := loadHistory(historyPath)
	if err != nil {
		return lineageAuthorityBundle{}, err
	}
	historyData, err := readRegularStable(historyPath, 0o644)
	if err != nil {
		return lineageAuthorityBundle{}, fmt.Errorf("stabilize source-history inventory: %w", err)
	}
	canonicalHistory, err := encodeHistory(history)
	if err != nil || !bytes.Equal(historyData, canonicalHistory) {
		return lineageAuthorityBundle{}, errors.Join(errors.New("source-history inventory changed while loading"), err)
	}
	inventory, err := loadLineage(inventoryPath)
	if err != nil {
		return lineageAuthorityBundle{}, err
	}
	wantHistorySHA := fmt.Sprintf("%x", sha256.Sum256(historyData))
	if inventory.SourceHistorySHA256 != wantHistorySHA {
		return lineageAuthorityBundle{}, fmt.Errorf("lineage source-history SHA-256 = %s, want %s", inventory.SourceHistorySHA256, wantHistorySHA)
	}
	if err := validateLineageCurrentSourceAliases(inventory, history); err != nil {
		return lineageAuthorityBundle{}, err
	}
	inventoryData, err := encodeLineage(inventory)
	if err != nil {
		return lineageAuthorityBundle{}, err
	}
	return lineageAuthorityBundle{
		Inventory: inventory, History: history, InventoryData: inventoryData, HistoryData: historyData,
	}, nil
}

func createLineageFloor(
	inventory lineageCatalog,
	inventoryPath string,
	history historyInventory,
	historyPath string,
	directory string,
	output string,
) (lineageFloor, error) {
	records, err := lineageFloorRecords(inventory)
	if err != nil {
		return lineageFloor{}, err
	}
	sourceFloors, err := loadStableHistoryFloors(history, historyPath)
	if err != nil {
		return lineageFloor{}, err
	}
	if len(sourceFloors) == 0 {
		return lineageFloor{}, errors.New("lineage has no source-history floor authority")
	}
	frozen, head, err := loadLineageFloorChain(inventory, inventoryPath, sourceFloors, directory, true)
	if err != nil {
		return lineageFloor{}, err
	}
	if head.Sequence >= 999999 {
		return lineageFloor{}, errors.New("lineage floor sequence is exhausted")
	}
	if head.Sequence == 0 && inventory.SchemaVersion > lineageMinimumSchemaVersion {
		return lineageFloor{}, fmt.Errorf("lineage genesis schema = %d, want at most %d", inventory.SchemaVersion, lineageMinimumSchemaVersion)
	}
	if head.Sequence != 0 && inventory.SchemaVersion > head.LineageSchemaVersion+1 {
		return lineageFloor{}, fmt.Errorf("lineage schema jumps from %d to %d", head.LineageSchemaVersion, inventory.SchemaVersion)
	}
	sequence := head.Sequence + 1
	wantOutput := filepath.Join(directory, fmt.Sprintf("%06d.json", sequence))
	if !samePhysicalFloorOutput(output, directory, filepath.Base(wantOutput)) {
		return lineageFloor{}, fmt.Errorf("lineage floor output = %q, want %q", output, wantOutput)
	}
	additions := make([]lineageFloorAddition, 0)
	for key, record := range records {
		if _, ok := frozen[key]; ok {
			continue
		}
		additions = append(additions, lineageFloorAddition{Kind: record.Kind, ID: record.ID, RecordSHA256: record.Digest})
	}
	slices.SortFunc(additions, compareLineageFloorAdditions)
	if len(additions) == 0 {
		return lineageFloor{}, errors.New("lineage floor has no new records")
	}
	additionKeys := make(map[string]struct{}, len(additions))
	for _, addition := range additions {
		additionKeys[lineageFloorKey(addition.Kind, addition.ID)] = struct{}{}
	}
	for _, addition := range additions {
		if err := validateLineageRecordDependencies(inventory, inventory.SchemaVersion, addition.Kind, addition.ID, frozen, additionKeys); err != nil {
			return lineageFloor{}, err
		}
	}
	source := sourceFloors[len(sourceFloors)-1]
	for _, addition := range additions {
		if err := validateLineageSourcePrefix(inventory, addition.Kind, addition.ID, source); err != nil {
			return lineageFloor{}, err
		}
		record := records[lineageFloorKey(addition.Kind, addition.ID)]
		frozen[lineageFloorKey(record.Kind, record.ID)] = record
	}
	var previous *lineageFloorPrevious
	if head.Sequence != 0 {
		previous = &lineageFloorPrevious{
			Sequence: head.Sequence, Path: head.Path, SHA256: head.SHA256,
			CumulativeRecordSetSHA256: head.CumulativeRecordSetSHA256,
		}
	}
	return lineageFloor{
		SchemaVersion: lineageFloorSchemaVersion, Sequence: sequence, PreviousFloor: previous,
		LineageSchemaVersion: inventory.SchemaVersion, RecordDigestAlgorithm: lineageDigestAlgorithm,
		SourceHistoryFloor: source.Authority, Additions: additions,
		CumulativeCounts:          countLineageFloorRecords(frozen),
		CumulativeRecordSetSHA256: digestLineageFloorSet(frozen),
	}, nil
}

func validateLineageFloors(
	inventory lineageCatalog,
	inventoryPath string,
	history historyInventory,
	historyPath string,
) (lineageFloorHead, error) {
	sourceFloors, err := loadStableHistoryFloors(history, historyPath)
	if err != nil {
		return lineageFloorHead{}, err
	}
	return validateLineageFloorsWithSource(inventory, inventoryPath, sourceFloors)
}

func validateLineageFloorsWithSource(
	inventory lineageCatalog,
	inventoryPath string,
	sourceFloors []lineageSourceFloorState,
) (lineageFloorHead, error) {
	directory := filepath.Join(filepath.Dir(inventoryPath), "lineagefloors")
	_, head, err := loadLineageFloorChain(inventory, inventoryPath, sourceFloors, directory, false)
	return head, err
}

func loadLineageFloorChain(
	inventory lineageCatalog,
	inventoryPath string,
	sourceFloors []lineageSourceFloorState,
	directory string,
	allowUnfloored bool,
) (map[string]lineageFloorRecord, lineageFloorHead, error) {
	records, err := lineageFloorRecords(inventory)
	if err != nil {
		return nil, lineageFloorHead{}, err
	}
	files, err := readStableDirectoryFiles(directory, 0o644)
	if os.IsNotExist(err) && allowUnfloored {
		return make(map[string]lineageFloorRecord), lineageFloorHead{}, nil
	}
	if err != nil {
		return nil, lineageFloorHead{}, fmt.Errorf("read lineage floor directory: %w", err)
	}
	if len(files) == 0 && !allowUnfloored {
		return nil, lineageFloorHead{}, errors.New("lineage has no immutable record floor")
	}
	frozen := make(map[string]lineageFloorRecord, len(records))
	var head lineageFloorHead
	previousSourceSequence := 0
	previousLineageSchemaVersion := 0
	for index, file := range files {
		sequence := index + 1
		wantName := fmt.Sprintf("%06d.json", sequence)
		if file.Name != wantName {
			return nil, lineageFloorHead{}, fmt.Errorf("lineage floor %q, want %q", file.Name, wantName)
		}
		floor, err := decodeLineageFloor(file.Data)
		if err != nil {
			return nil, lineageFloorHead{}, fmt.Errorf("decode lineage floor %q: %w", file.Name, err)
		}
		if floor.Sequence != sequence || floor.SchemaVersion != lineageFloorSchemaVersion ||
			floor.LineageSchemaVersion < 1 || floor.LineageSchemaVersion > inventory.SchemaVersion ||
			floor.LineageSchemaVersion < previousLineageSchemaVersion ||
			previousLineageSchemaVersion != 0 && floor.LineageSchemaVersion > previousLineageSchemaVersion+1 ||
			floor.RecordDigestAlgorithm != lineageDigestAlgorithm ||
			len(floor.Additions) == 0 {
			return nil, lineageFloorHead{}, fmt.Errorf("lineage floor %q authority changed", file.Name)
		}
		previousLineageSchemaVersion = floor.LineageSchemaVersion
		if sequence == 1 {
			if floor.PreviousFloor != nil {
				return nil, lineageFloorHead{}, errors.New("genesis lineage floor has a predecessor")
			}
		} else {
			wantPrevious := lineageFloorPrevious{
				Sequence: head.Sequence, Path: head.Path, SHA256: head.SHA256,
				CumulativeRecordSetSHA256: head.CumulativeRecordSetSHA256,
			}
			if floor.PreviousFloor == nil || *floor.PreviousFloor != wantPrevious {
				return nil, lineageFloorHead{}, fmt.Errorf("lineage floor %q predecessor changed", file.Name)
			}
		}
		source, ok := findLineageSourceFloor(sourceFloors, floor.SourceHistoryFloor)
		if !ok || source.Authority.Sequence < previousSourceSequence {
			return nil, lineageFloorHead{}, fmt.Errorf("lineage floor %q has invalid or regressed source-history authority", file.Name)
		}
		previousSourceSequence = source.Authority.Sequence
		additionKeys := make(map[string]struct{}, len(floor.Additions))
		for additionIndex, addition := range floor.Additions {
			if additionIndex != 0 && compareLineageFloorAdditions(floor.Additions[additionIndex-1], addition) >= 0 {
				return nil, lineageFloorHead{}, fmt.Errorf("lineage floor %q additions are not a sorted set", file.Name)
			}
			if !lineageIDPattern.MatchString(addition.ID) || !lineageRecordKindValid(addition.Kind) ||
				!historySHA256Pattern.MatchString(addition.RecordSHA256) {
				return nil, lineageFloorHead{}, fmt.Errorf("lineage floor %q has invalid addition %q/%q", file.Name, addition.Kind, addition.ID)
			}
			key := lineageFloorKey(addition.Kind, addition.ID)
			if _, duplicate := frozen[key]; duplicate {
				return nil, lineageFloorHead{}, fmt.Errorf("lineage floor %q repeats record %q", file.Name, key)
			}
			current, ok := records[key]
			if !ok || current.Digest != addition.RecordSHA256 {
				return nil, lineageFloorHead{}, fmt.Errorf("lineage floor %q record %q changed or disappeared", file.Name, key)
			}
			additionKeys[key] = struct{}{}
		}
		for _, addition := range floor.Additions {
			if err := validateLineageRecordDependencies(inventory, floor.LineageSchemaVersion, addition.Kind, addition.ID, frozen, additionKeys); err != nil {
				return nil, lineageFloorHead{}, fmt.Errorf("lineage floor %q: %w", file.Name, err)
			}
			if err := validateLineageSourcePrefix(inventory, addition.Kind, addition.ID, source); err != nil {
				return nil, lineageFloorHead{}, fmt.Errorf("lineage floor %q: %w", file.Name, err)
			}
			record := records[lineageFloorKey(addition.Kind, addition.ID)]
			frozen[lineageFloorKey(record.Kind, record.ID)] = record
		}
		if !slices.Equal(floor.CumulativeCounts, countLineageFloorRecords(frozen)) ||
			floor.CumulativeRecordSetSHA256 != digestLineageFloorSet(frozen) {
			return nil, lineageFloorHead{}, fmt.Errorf("lineage floor %q cumulative authority changed", file.Name)
		}
		head = lineageFloorHead{
			Sequence: sequence, Path: "lineagefloors/" + file.Name,
			SHA256:                    fmt.Sprintf("%x", sha256.Sum256(file.Data)),
			CumulativeRecordSetSHA256: floor.CumulativeRecordSetSHA256,
			LineageSchemaVersion:      floor.LineageSchemaVersion,
		}
	}
	if !allowUnfloored && previousLineageSchemaVersion != inventory.SchemaVersion {
		return nil, lineageFloorHead{}, fmt.Errorf("lineage floor schema head = %d, want %d", previousLineageSchemaVersion, inventory.SchemaVersion)
	}
	if !allowUnfloored && len(frozen) != len(records) {
		return nil, lineageFloorHead{}, fmt.Errorf("lineage has %d floored records/%d current records", len(frozen), len(records))
	}
	_ = inventoryPath
	return frozen, head, nil
}

func loadStableHistoryFloors(history historyInventory, historyPath string) ([]lineageSourceFloorState, error) {
	validatedHead, err := validateHistoryFloors(history, historyPath)
	if err != nil {
		return nil, err
	}
	records, err := historyFloorRecords(history)
	if err != nil {
		return nil, err
	}
	files, err := readStableDirectoryFiles(filepath.Join(filepath.Dir(historyPath), "historyfloors"), 0o644)
	if err != nil {
		return nil, fmt.Errorf("stabilize source-history floors: %w", err)
	}
	frozen := make(map[string]historyFloorRecord, len(records))
	sequences := make(map[string]int, len(records))
	states := make([]lineageSourceFloorState, 0, len(files))
	var head historyFloorHead
	for index, file := range files {
		sequence := index + 1
		wantName := fmt.Sprintf("%06d.json", sequence)
		if file.Name != wantName {
			return nil, fmt.Errorf("source-history floor %q, want %q", file.Name, wantName)
		}
		floor, err := decodeHistoryFloor(file.Data)
		if err != nil {
			return nil, fmt.Errorf("decode stable source-history floor %q: %w", file.Name, err)
		}
		if floor.SchemaVersion != historyFloorSchemaVersion || floor.Sequence != sequence ||
			floor.SourceHistorySchemaVersion != historySchemaVersion || floor.RecordDigestAlgorithm != lineageDigestAlgorithm ||
			len(floor.Additions) == 0 {
			return nil, fmt.Errorf("stable source-history floor %q authority changed", file.Name)
		}
		if sequence == 1 {
			if floor.PreviousFloor != nil {
				return nil, errors.New("stable source-history genesis has a predecessor")
			}
		} else {
			wantPrevious := historyFloorPrevious{
				Sequence: head.Sequence, Path: head.Path, SHA256: head.SHA256,
				CumulativeRecordSetSHA256: head.CumulativeRecordSetSHA256,
			}
			if floor.PreviousFloor == nil || *floor.PreviousFloor != wantPrevious {
				return nil, fmt.Errorf("stable source-history floor %q predecessor changed", file.Name)
			}
		}
		for additionIndex, addition := range floor.Additions {
			if additionIndex != 0 && compareHistoryFloorAdditions(floor.Additions[additionIndex-1], addition) >= 0 {
				return nil, fmt.Errorf("stable source-history floor %q additions are not sorted", file.Name)
			}
			key := historyFloorKey(addition.Kind, addition.ID)
			current, ok := records[key]
			if !ok || current.Digest != addition.RecordSHA256 {
				return nil, fmt.Errorf("stable source-history floor %q record %q changed or disappeared", file.Name, key)
			}
			if _, duplicate := frozen[key]; duplicate {
				return nil, fmt.Errorf("stable source-history floor %q repeats record %q", file.Name, key)
			}
			frozen[key] = current
			sequences[key] = sequence
		}
		if floor.CumulativeCounts != countHistoryFloorRecords(frozen) ||
			floor.CumulativeRecordSetSHA256 != digestHistoryFloorSet(frozen) {
			return nil, fmt.Errorf("stable source-history floor %q cumulative authority changed", file.Name)
		}
		head = historyFloorHead{
			Sequence: sequence, Path: "historyfloors/" + file.Name,
			SHA256:                    fmt.Sprintf("%x", sha256.Sum256(file.Data)),
			CumulativeRecordSetSHA256: floor.CumulativeRecordSetSHA256,
		}
		states = append(states, lineageSourceFloorState{
			Authority: lineageSourceFloor{
				Path: head.Path, SchemaVersion: historyFloorSchemaVersion, Sequence: sequence,
				SHA256: head.SHA256, CumulativeRecordSetSHA256: head.CumulativeRecordSetSHA256,
			},
			RecordSequences: cloneIntMap(sequences),
		})
	}
	if head != validatedHead {
		return nil, errors.New("stable source-history floor head differs from validated head")
	}
	return states, nil
}

func lineageFloorRecords(inventory lineageCatalog) (map[string]lineageFloorRecord, error) {
	records := make(map[string]lineageFloorRecord)
	add := func(kind, id string, value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode lineage floor record %s/%s: %w", kind, id, err)
		}
		key := lineageFloorKey(kind, id)
		if _, duplicate := records[key]; duplicate {
			return fmt.Errorf("duplicate lineage floor record %q", key)
		}
		records[key] = lineageFloorRecord{Kind: kind, ID: id, Digest: digestLineageFloorRecord(kind, id, data)}
		return nil
	}
	for _, record := range inventory.Introductions {
		if err := add("introduction", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.Snapshots {
		if err := add("snapshot", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.Implementations {
		if err := add("implementation", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.Concepts {
		if err := add("concept", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.RawRoots {
		if err := add("raw-root", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.Harnesses {
		if err := add("harness", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.Workloads {
		if err := add("workload", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.Bindings {
		if err := add("binding", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.Aliases {
		if err := add("alias", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.Reconstructions {
		if err := add("reconstruction", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.Dispositions {
		if err := add("disposition", record.ID, record); err != nil {
			return nil, err
		}
	}
	return records, nil
}

func validateLineageRecordDependencies(
	inventory lineageCatalog,
	lineageVersion int,
	kind string,
	id string,
	frozen map[string]lineageFloorRecord,
	batch map[string]struct{},
) error {
	if err := validateLineageRecordSchema(inventory, lineageVersion, kind, id); err != nil {
		return err
	}
	contains := func(recordKind, recordID string) bool {
		key := lineageFloorKey(recordKind, recordID)
		_, old := frozen[key]
		_, current := batch[key]
		return old || current
	}
	switch kind {
	case "raw-root":
		record := findLineageRawRoot(inventory.RawRoots, id)
		if !contains("snapshot", record.SnapshotID) {
			return fmt.Errorf("lineage raw root %q depends on unfloored snapshot %q", id, record.SnapshotID)
		}
	case "implementation":
		record := findLineageImplementation(inventory.Implementations, id)
		if !contains("introduction", record.IntroductionID) {
			return fmt.Errorf("lineage implementation %q depends on unfloored introduction %q", id, record.IntroductionID)
		}
	case "binding":
		record := findLineageBinding(inventory.Bindings, id)
		for _, dependency := range []struct{ kind, id string }{
			{kind: "raw-root", id: record.RawRootID},
			{kind: "implementation", id: record.ImplementationID},
			{kind: "workload", id: record.WorkloadID},
			{kind: "harness", id: record.HarnessID},
		} {
			if !contains(dependency.kind, dependency.id) {
				return fmt.Errorf("lineage binding %q depends on unfloored %s %q", id, dependency.kind, dependency.id)
			}
		}
		for _, snapshotID := range record.SnapshotIDs {
			if !contains("snapshot", snapshotID) {
				return fmt.Errorf("lineage binding %q depends on unfloored snapshot %q", id, snapshotID)
			}
		}
	case "alias":
		record := findLineageAlias(inventory.Aliases, id)
		canonicalKind, ok := lineageRecordKind(inventory, record.CanonicalSubjectID)
		if !ok || !contains(canonicalKind, record.CanonicalSubjectID) {
			return fmt.Errorf("lineage alias %q depends on unfloored subject %q", id, record.CanonicalSubjectID)
		}
	case "reconstruction":
		record := findLineageReconstruction(inventory.Reconstructions, id)
		if !contains("concept", record.ConceptID) || !contains("snapshot", record.OutputSnapshotID) {
			return fmt.Errorf("lineage reconstruction %q has unfloored concept or output", id)
		}
		for _, snapshotID := range record.BasisSnapshotIDs {
			if !contains("snapshot", snapshotID) {
				return fmt.Errorf("lineage reconstruction %q depends on unfloored snapshot %q", id, snapshotID)
			}
		}
	case "disposition":
		record := findLineageDisposition(inventory.Dispositions, id)
		if record.SubjectKind != "raw-root" && !contains(record.SubjectKind, record.SubjectID) {
			return fmt.Errorf("lineage disposition %q depends on unfloored subject %q", id, record.SubjectID)
		}
		if record.SubjectKind == "raw-root" && lineageVersion >= 2 && !contains("raw-root", record.SubjectID) {
			return fmt.Errorf("lineage disposition %q depends on unfloored raw root %q", id, record.SubjectID)
		}
		if record.SnapshotID != "none" && !contains("snapshot", record.SnapshotID) {
			return fmt.Errorf("lineage disposition %q depends on unfloored snapshot %q", id, record.SnapshotID)
		}
	}
	return nil
}

func digestLineageFloorRecord(kind, id string, data []byte) string {
	hash := sha256.New()
	writeLineageDigestFrame(hash, []byte(lineageRecordDigestDomain))
	writeLineageDigestFrame(hash, []byte(kind))
	writeLineageDigestFrame(hash, []byte(id))
	writeLineageDigestFrame(hash, data)
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func digestLineageFloorSet(records map[string]lineageFloorRecord) string {
	values := make([]lineageFloorRecord, 0, len(records))
	for _, record := range records {
		values = append(values, record)
	}
	slices.SortFunc(values, func(left, right lineageFloorRecord) int {
		if result := strings.Compare(left.Kind, right.Kind); result != 0 {
			return result
		}
		return strings.Compare(left.ID, right.ID)
	})
	hash := sha256.New()
	writeLineageDigestFrame(hash, []byte(lineageSetDigestDomain))
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(values)))
	writeLineageDigestFrame(hash, count[:])
	for _, record := range values {
		writeLineageDigestFrame(hash, []byte(record.Kind))
		writeLineageDigestFrame(hash, []byte(record.ID))
		digest, err := hex.DecodeString(record.Digest)
		if err != nil || len(digest) != sha256.Size {
			panic("validated lineage record digest became invalid")
		}
		writeLineageDigestFrame(hash, digest)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func writeLineageDigestFrame(writer io.Writer, data []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(data)))
	_, _ = writer.Write(size[:])
	_, _ = writer.Write(data)
}

func countLineageFloorRecords(records map[string]lineageFloorRecord) []lineageFloorCount {
	counts := make(map[string]int)
	for _, record := range records {
		counts[record.Kind]++
	}
	result := make([]lineageFloorCount, 0, len(counts))
	for kind, count := range counts {
		result = append(result, lineageFloorCount{Kind: kind, Count: count})
	}
	slices.SortFunc(result, func(left, right lineageFloorCount) int { return strings.Compare(left.Kind, right.Kind) })
	return result
}

func encodeLineageFloor(floor lineageFloor) ([]byte, error) {
	data, err := json.MarshalIndent(floor, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode lineage floor: %w", err)
	}
	return append(data, '\n'), nil
}

func decodeLineageFloor(data []byte) (lineageFloor, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return lineageFloor{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var floor lineageFloor
	if err := decoder.Decode(&floor); err != nil {
		return lineageFloor{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return lineageFloor{}, errors.New("lineage floor has trailing JSON")
	}
	if floor.Additions == nil || floor.CumulativeCounts == nil || len(floor.Additions) == 0 ||
		!historySHA256Pattern.MatchString(floor.CumulativeRecordSetSHA256) ||
		!historySHA256Pattern.MatchString(floor.SourceHistoryFloor.SHA256) ||
		!historySHA256Pattern.MatchString(floor.SourceHistoryFloor.CumulativeRecordSetSHA256) {
		return lineageFloor{}, errors.New("lineage floor has invalid required authority")
	}
	previousKind := ""
	for _, count := range floor.CumulativeCounts {
		if !lineageRecordKindValid(count.Kind) || count.Kind <= previousKind || count.Count <= 0 {
			return lineageFloor{}, errors.New("lineage floor cumulative counts are not a sorted positive set")
		}
		previousKind = count.Kind
	}
	canonical, err := encodeLineageFloor(floor)
	if err != nil {
		return lineageFloor{}, err
	}
	if !bytes.Equal(data, canonical) {
		return lineageFloor{}, errors.New("lineage floor is not canonical JSON")
	}
	return floor, nil
}

func compareLineageFloorAdditions(left, right lineageFloorAddition) int {
	if result := strings.Compare(left.Kind, right.Kind); result != 0 {
		return result
	}
	return strings.Compare(left.ID, right.ID)
}

func lineageFloorKey(kind, id string) string {
	return kind + "\x00" + id
}

func lineageRecordKindValid(kind string) bool {
	return lineageEnum(kind,
		"alias", "binding", "concept", "disposition", "harness", "implementation",
		"introduction", "raw-root", "reconstruction", "snapshot", "workload",
	)
}

func findLineageSourceFloor(states []lineageSourceFloorState, authority lineageSourceFloor) (lineageSourceFloorState, bool) {
	for _, state := range states {
		if state.Authority == authority {
			return state, true
		}
	}
	return lineageSourceFloorState{}, false
}

func lineageRecordKind(inventory lineageCatalog, id string) (string, bool) {
	records, err := lineageFloorRecords(inventory)
	if err != nil {
		return "", false
	}
	for _, record := range records {
		if record.ID == id {
			return record.Kind, true
		}
	}
	return "", false
}

func findLineageIntroduction(records []lineageIntroduction, id string) lineageIntroduction {
	for _, record := range records {
		if record.ID == id {
			return record
		}
	}
	panic("validated lineage introduction disappeared: " + strconv.Quote(id))
}

func findLineageSnapshot(records []lineageSnapshot, id string) lineageSnapshot {
	for _, record := range records {
		if record.ID == id {
			return record
		}
	}
	panic("validated lineage snapshot disappeared: " + strconv.Quote(id))
}

func findLineageImplementation(records []lineageImplementation, id string) lineageImplementation {
	for _, record := range records {
		if record.ID == id {
			return record
		}
	}
	panic("validated lineage implementation disappeared: " + strconv.Quote(id))
}

func findLineageRawRoot(records []lineageRawRoot, id string) lineageRawRoot {
	for _, record := range records {
		if record.ID == id {
			return record
		}
	}
	panic("validated lineage raw root disappeared: " + strconv.Quote(id))
}

func findLineageBinding(records []lineageBinding, id string) lineageBinding {
	for _, record := range records {
		if record.ID == id {
			return record
		}
	}
	panic("validated lineage binding disappeared: " + strconv.Quote(id))
}

func findLineageAlias(records []lineageAlias, id string) lineageAlias {
	for _, record := range records {
		if record.ID == id {
			return record
		}
	}
	panic("validated lineage alias disappeared: " + strconv.Quote(id))
}

func findLineageReconstruction(records []lineageReconstruction, id string) lineageReconstruction {
	for _, record := range records {
		if record.ID == id {
			return record
		}
	}
	panic("validated lineage reconstruction disappeared: " + strconv.Quote(id))
}

func findLineageDisposition(records []lineageDisposition, id string) lineageDisposition {
	for _, record := range records {
		if record.ID == id {
			return record
		}
	}
	panic("validated lineage disposition disappeared: " + strconv.Quote(id))
}

func cloneIntMap(values map[string]int) map[string]int {
	result := make(map[string]int, len(values))
	maps.Copy(result, values)
	return result
}

func samePhysicalFloorOutput(output, directory, name string) bool {
	outputPath, outputErr := filepath.Abs(output)
	directoryPath, directoryErr := filepath.Abs(directory)
	if outputErr != nil || directoryErr != nil || filepath.Base(outputPath) != name {
		return false
	}
	outputDirectory, outputErr := os.Stat(filepath.Dir(outputPath))
	floorDirectory, directoryErr := os.Stat(directoryPath)
	return outputErr == nil && directoryErr == nil && freezeComponentIdentity(outputDirectory) &&
		freezeComponentIdentity(floorDirectory) && outputDirectory.IsDir() && floorDirectory.IsDir() &&
		os.SameFile(outputDirectory, floorDirectory)
}
