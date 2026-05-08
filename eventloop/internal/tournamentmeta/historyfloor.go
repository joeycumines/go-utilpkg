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
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	historyFloorSchemaVersion = 1
	historyRecordDigestDomain = "go-utilpkg/eventloop/tournament/source-history/record/v1"
	historySetDigestDomain    = "go-utilpkg/eventloop/tournament/source-history/set/v1"
)

type historyFloor struct { // betteralign:ignore canonical JSON field order
	SchemaVersion              int                    `json:"schema_version"`
	Sequence                   int                    `json:"sequence"`
	PreviousFloor              *historyFloorPrevious  `json:"previous_floor"`
	SourceHistorySchemaVersion int                    `json:"source_history_schema_version"`
	RecordDigestAlgorithm      string                 `json:"record_digest_algorithm"`
	Additions                  []historyFloorAddition `json:"additions"`
	CumulativeCounts           historyFloorCounts     `json:"cumulative_counts"`
	CumulativeRecordSetSHA256  string                 `json:"cumulative_record_set_sha256"`
}

type historyFloorPrevious struct { // betteralign:ignore canonical JSON field order
	Sequence                  int    `json:"sequence"`
	Path                      string `json:"path"`
	SHA256                    string `json:"sha256"`
	CumulativeRecordSetSHA256 string `json:"cumulative_record_set_sha256"`
}

type historyFloorAddition struct { // betteralign:ignore canonical JSON field order
	Kind         string `json:"kind"`
	ID           string `json:"id"`
	RecordSHA256 string `json:"record_sha256"`
}

type historyFloorCounts struct {
	AncestrySegments  int `json:"ancestry_segments"`
	SourceSnapshots   int `json:"source_snapshots"`
	SourceOccurrences int `json:"source_occurrences"`
	SourceTransitions int `json:"source_transitions"`
	CommitArchives    int `json:"commit_archives"`
	TreePatchArchives int `json:"tree_patch_archives"`
}

type historyFloorRecord struct {
	Kind   string
	ID     string
	Digest string
}

type historyFloorHead struct {
	Path                      string
	SHA256                    string
	CumulativeRecordSetSHA256 string
	Sequence                  int
}

func historyFloorGenerateCommand(arguments []string) int {
	flags := flag.NewFlagSet("history floor-generate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	inventoryPath := flags.String("inventory", "", "canonical source-history inventory")
	floorDirectory := flags.String("floor-directory", "", "history floor directory")
	output := flags.String("output", "", "new floor path")
	if err := flags.Parse(arguments); err != nil {
		return commandError(err)
	}
	if flags.NArg() != 0 || *inventoryPath == "" || *floorDirectory == "" || *output == "" ||
		!filepath.IsAbs(*inventoryPath) || !filepath.IsAbs(*floorDirectory) || !filepath.IsAbs(*output) {
		return commandError(errors.New("history floor-generate requires absolute -inventory, -floor-directory, and -output"))
	}
	inventory, err := loadHistory(*inventoryPath)
	if err != nil {
		return commandError(err)
	}
	floor, err := createHistoryFloor(inventory, *floorDirectory, *output)
	if err != nil {
		return commandError(err)
	}
	data, err := encodeHistoryFloor(floor)
	if err != nil {
		return commandError(err)
	}
	if err := writeAtomicNew(*output, data, 0o644); err != nil {
		return commandError(fmt.Errorf("write history floor: %w", err))
	}
	return 0
}

func createHistoryFloor(inventory historyInventory, directory, output string) (historyFloor, error) {
	records, err := historyFloorRecords(inventory)
	if err != nil {
		return historyFloor{}, err
	}
	frozen, head, err := loadHistoryFloorChain(inventory, directory, true)
	if err != nil {
		return historyFloor{}, err
	}
	sequence := head.Sequence + 1
	wantOutput := filepath.Join(directory, fmt.Sprintf("%06d.json", sequence))
	resolvedOutput, err := filepath.Abs(output)
	if err != nil {
		return historyFloor{}, fmt.Errorf("resolve history floor output: %w", err)
	}
	resolvedWant, err := filepath.Abs(wantOutput)
	if err != nil {
		return historyFloor{}, fmt.Errorf("resolve expected history floor output: %w", err)
	}
	if resolvedOutput != resolvedWant {
		return historyFloor{}, fmt.Errorf("history floor output = %q, want %q", resolvedOutput, resolvedWant)
	}
	additions := make([]historyFloorAddition, 0)
	for key, record := range records {
		if _, ok := frozen[key]; ok {
			continue
		}
		additions = append(additions, historyFloorAddition{
			Kind: record.Kind, ID: record.ID, RecordSHA256: record.Digest,
		})
	}
	slices.SortFunc(additions, compareHistoryFloorAdditions)
	if len(additions) == 0 {
		return historyFloor{}, errors.New("history floor has no new records")
	}
	for _, addition := range additions {
		frozen[historyFloorKey(addition.Kind, addition.ID)] = historyFloorRecord{
			Kind: addition.Kind, ID: addition.ID, Digest: addition.RecordSHA256,
		}
	}
	var previous *historyFloorPrevious
	if head.Sequence != 0 {
		previous = &historyFloorPrevious{
			Sequence: head.Sequence, Path: head.Path, SHA256: head.SHA256,
			CumulativeRecordSetSHA256: head.CumulativeRecordSetSHA256,
		}
	}
	return historyFloor{
		SchemaVersion:              historyFloorSchemaVersion,
		Sequence:                   sequence,
		PreviousFloor:              previous,
		SourceHistorySchemaVersion: historySchemaVersion,
		RecordDigestAlgorithm:      "sha256-domain-length-framed-v1",
		Additions:                  additions,
		CumulativeCounts:           countHistoryFloorRecords(frozen),
		CumulativeRecordSetSHA256:  digestHistoryFloorSet(frozen),
	}, nil
}

func validateHistoryFloors(inventory historyInventory, inventoryPath string) (historyFloorHead, error) {
	directory := filepath.Join(filepath.Dir(inventoryPath), "historyfloors")
	_, head, err := loadHistoryFloorChain(inventory, directory, false)
	return head, err
}

func loadHistoryFloorChain(
	inventory historyInventory,
	directory string,
	allowUnfloored bool,
) (map[string]historyFloorRecord, historyFloorHead, error) {
	records, err := historyFloorRecords(inventory)
	if err != nil {
		return nil, historyFloorHead{}, err
	}
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) && allowUnfloored {
		return make(map[string]historyFloorRecord), historyFloorHead{}, nil
	}
	if err != nil {
		return nil, historyFloorHead{}, fmt.Errorf("read history floor directory: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, historyFloorHead{}, fmt.Errorf("unexpected history floor entry %q", entry.Name())
		}
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	if len(names) == 0 && !allowUnfloored {
		return nil, historyFloorHead{}, errors.New("history has no immutable record floor")
	}
	frozen := make(map[string]historyFloorRecord, len(records))
	var head historyFloorHead
	for index, name := range names {
		sequence := index + 1
		wantName := fmt.Sprintf("%06d.json", sequence)
		if name != wantName {
			return nil, historyFloorHead{}, fmt.Errorf("history floor %q, want %q", name, wantName)
		}
		path := filepath.Join(directory, name)
		if err := requireHistoryRegularFile(path, 0o644); err != nil {
			return nil, historyFloorHead{}, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, historyFloorHead{}, fmt.Errorf("read history floor %q: %w", name, err)
		}
		floor, err := decodeHistoryFloor(data)
		if err != nil {
			return nil, historyFloorHead{}, fmt.Errorf("decode history floor %q: %w", name, err)
		}
		if floor.Sequence != sequence || floor.SchemaVersion != historyFloorSchemaVersion ||
			floor.SourceHistorySchemaVersion != historySchemaVersion ||
			floor.RecordDigestAlgorithm != "sha256-domain-length-framed-v1" || len(floor.Additions) == 0 {
			return nil, historyFloorHead{}, fmt.Errorf("history floor %q authority changed", name)
		}
		if sequence == 1 {
			if floor.PreviousFloor != nil {
				return nil, historyFloorHead{}, errors.New("genesis history floor has a predecessor")
			}
		} else {
			wantPrevious := historyFloorPrevious{
				Sequence: head.Sequence, Path: head.Path, SHA256: head.SHA256,
				CumulativeRecordSetSHA256: head.CumulativeRecordSetSHA256,
			}
			if floor.PreviousFloor == nil || *floor.PreviousFloor != wantPrevious {
				return nil, historyFloorHead{}, fmt.Errorf("history floor %q predecessor changed", name)
			}
		}
		for additionIndex, addition := range floor.Additions {
			if additionIndex != 0 && compareHistoryFloorAdditions(floor.Additions[additionIndex-1], addition) >= 0 {
				return nil, historyFloorHead{}, fmt.Errorf("history floor %q additions are not a sorted set", name)
			}
			key := historyFloorKey(addition.Kind, addition.ID)
			current, ok := records[key]
			if !ok || current.Digest != addition.RecordSHA256 {
				return nil, historyFloorHead{}, fmt.Errorf("history floor %q record %q changed or disappeared", name, key)
			}
			if _, duplicate := frozen[key]; duplicate {
				return nil, historyFloorHead{}, fmt.Errorf("history floor %q repeats record %q", name, key)
			}
			frozen[key] = current
		}
		if floor.CumulativeCounts != countHistoryFloorRecords(frozen) ||
			floor.CumulativeRecordSetSHA256 != digestHistoryFloorSet(frozen) {
			return nil, historyFloorHead{}, fmt.Errorf("history floor %q cumulative authority changed", name)
		}
		head = historyFloorHead{
			Sequence:                  sequence,
			Path:                      "historyfloors/" + name,
			SHA256:                    fmt.Sprintf("%x", sha256.Sum256(data)),
			CumulativeRecordSetSHA256: floor.CumulativeRecordSetSHA256,
		}
	}
	if !allowUnfloored && len(frozen) != len(records) {
		return nil, historyFloorHead{}, fmt.Errorf("history has %d floored records/%d current records", len(frozen), len(records))
	}
	return frozen, head, nil
}

func historyFloorRecords(inventory historyInventory) (map[string]historyFloorRecord, error) {
	records := make(map[string]historyFloorRecord)
	add := func(kind, id string, value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode history floor record %s/%s: %w", kind, id, err)
		}
		digest := digestHistoryFloorRecord(kind, id, data)
		key := historyFloorKey(kind, id)
		if _, duplicate := records[key]; duplicate {
			return fmt.Errorf("duplicate history floor record %q", key)
		}
		records[key] = historyFloorRecord{Kind: kind, ID: id, Digest: digest}
		return nil
	}
	for _, record := range inventory.AncestrySegments {
		if err := add("ancestry_segment", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.SourceSnapshots {
		if err := add("source_snapshot", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.SourceOccurrences {
		if err := add("source_occurrence", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.SourceTransitions {
		if err := add("source_transition", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.CommitArchives {
		if err := add("commit_archive", record.ID, record); err != nil {
			return nil, err
		}
	}
	for _, record := range inventory.TreePatchArchives {
		if err := add("tree_patch_archive", record.ID, record); err != nil {
			return nil, err
		}
	}
	return records, nil
}

func digestHistoryFloorRecord(kind, id string, data []byte) string {
	hash := sha256.New()
	writeHistoryDigestFrame(hash, []byte(historyRecordDigestDomain))
	writeHistoryDigestFrame(hash, []byte(kind))
	writeHistoryDigestFrame(hash, []byte(id))
	writeHistoryDigestFrame(hash, data)
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func digestHistoryFloorSet(records map[string]historyFloorRecord) string {
	values := make([]historyFloorRecord, 0, len(records))
	for _, record := range records {
		values = append(values, record)
	}
	slices.SortFunc(values, func(left, right historyFloorRecord) int {
		if result := strings.Compare(left.Kind, right.Kind); result != 0 {
			return result
		}
		return strings.Compare(left.ID, right.ID)
	})
	hash := sha256.New()
	writeHistoryDigestFrame(hash, []byte(historySetDigestDomain))
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(values)))
	_, _ = hash.Write(count[:])
	for _, record := range values {
		writeHistoryDigestFrame(hash, []byte(record.Kind))
		writeHistoryDigestFrame(hash, []byte(record.ID))
		digest, err := hex.DecodeString(record.Digest)
		if err != nil || len(digest) != sha256.Size {
			panic("validated history record digest became invalid")
		}
		writeHistoryDigestFrame(hash, digest)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func writeHistoryDigestFrame(writer io.Writer, data []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(data)))
	_, _ = writer.Write(size[:])
	_, _ = writer.Write(data)
}

func countHistoryFloorRecords(records map[string]historyFloorRecord) historyFloorCounts {
	var counts historyFloorCounts
	for _, record := range records {
		switch record.Kind {
		case "ancestry_segment":
			counts.AncestrySegments++
		case "source_snapshot":
			counts.SourceSnapshots++
		case "source_occurrence":
			counts.SourceOccurrences++
		case "source_transition":
			counts.SourceTransitions++
		case "commit_archive":
			counts.CommitArchives++
		case "tree_patch_archive":
			counts.TreePatchArchives++
		default:
			panic("unknown history floor record kind " + strconv.Quote(record.Kind))
		}
	}
	return counts
}

func encodeHistoryFloor(floor historyFloor) ([]byte, error) {
	data, err := json.MarshalIndent(floor, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode history floor: %w", err)
	}
	return append(data, '\n'), nil
}

func decodeHistoryFloor(data []byte) (historyFloor, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return historyFloor{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var floor historyFloor
	if err := decoder.Decode(&floor); err != nil {
		return historyFloor{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return historyFloor{}, errors.New("history floor has trailing JSON")
	}
	canonical, err := encodeHistoryFloor(floor)
	if err != nil {
		return historyFloor{}, err
	}
	if !bytes.Equal(data, canonical) {
		return historyFloor{}, errors.New("history floor is not canonical JSON")
	}
	return floor, nil
}

func compareHistoryFloorAdditions(left, right historyFloorAddition) int {
	if result := strings.Compare(left.Kind, right.Kind); result != 0 {
		return result
	}
	return strings.Compare(left.ID, right.ID)
}

func historyFloorKey(kind, id string) string {
	return kind + "\x00" + id
}
