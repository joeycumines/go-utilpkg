package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSourceHistoryRecordFloor(t *testing.T) {
	const inventoryPath = "../tournament/source_history.json"
	inventory, err := loadHistory(inventoryPath)
	if err != nil {
		t.Fatalf("loadHistory: %v", err)
	}
	head, err := validateHistoryFloors(inventory, inventoryPath)
	if err != nil {
		t.Fatalf("validateHistoryFloors: %v", err)
	}
	if head.Sequence != 2 || head.Path != "historyfloors/000002.json" ||
		head.SHA256 != "116e70f0d993e5b0033874109bc3ee961b869e779ceef74f2f0529d9f9dfd707" ||
		head.CumulativeRecordSetSHA256 != "04c4cfe87d6b527ceda6ff18e24a0cb614652f0cb86bba891ff5bf85e2c9cdeb" {
		t.Fatalf("history floor head = %+v", head)
	}

	mutated := inventory
	mutated.SourceOccurrences = slices.Clone(inventory.SourceOccurrences)
	mutated.SourceOccurrences[0].AuthorEpoch++
	if _, _, err := loadHistoryFloorChain(mutated, "../tournament/historyfloors", false); err == nil {
		t.Fatal("mutated floored occurrence unexpectedly passed")
	}

	mutated = inventory
	mutated.SourceTransitions = slices.Clone(inventory.SourceTransitions[1:])
	if _, _, err := loadHistoryFloorChain(mutated, "../tournament/historyfloors", false); err == nil {
		t.Fatal("deleted floored transition unexpectedly passed")
	}
}

func TestSourceHistoryRecordFloorAllowsOnlyFlooredTail(t *testing.T) {
	const inventoryPath = "../tournament/source_history.json"
	inventory, err := loadHistory(inventoryPath)
	if err != nil {
		t.Fatalf("loadHistory: %v", err)
	}
	directory := t.TempDir()
	for sequence, wantSHA256 := range []string{
		"d5490bb0364abe898b3be1066281c4cdb24ad07d03d1a8500ecc71f55e0a8fbf",
		"116e70f0d993e5b0033874109bc3ee961b869e779ceef74f2f0529d9f9dfd707",
	} {
		name := fmt.Sprintf("%06d.json", sequence+1)
		floor, err := os.ReadFile(filepath.Join("../tournament/historyfloors", name))
		if err != nil {
			t.Fatalf("read floor %s: %v", name, err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(floor)); got != wantSHA256 {
			t.Fatalf("floor %s SHA-256 = %s", name, got)
		}
		if err := os.WriteFile(filepath.Join(directory, name), floor, 0o644); err != nil {
			t.Fatalf("write floor %s: %v", name, err)
		}
	}

	mutated := inventory
	mutated.SourceSnapshots = append(slices.Clone(inventory.SourceSnapshots), historySnapshot{
		ID:               "source.eventloop.sha1.0000000000000000000000000000000000000000",
		DiscoveryOrdinal: len(inventory.SourceSnapshots) + 1,
		EventloopTree:    "0000000000000000000000000000000000000000",
	})
	if _, _, err := loadHistoryFloorChain(mutated, directory, false); err == nil {
		t.Fatal("unfloored tail unexpectedly passed")
	}
	output := filepath.Join(directory, "000003.json")
	floor, err := createHistoryFloor(mutated, directory, output)
	if err != nil {
		t.Fatalf("createHistoryFloor: %v", err)
	}
	if floor.Sequence != 3 || len(floor.Additions) != 1 || floor.Additions[0].Kind != "source_snapshot" {
		t.Fatalf("tail floor = %+v", floor)
	}
	data, err := encodeHistoryFloor(floor)
	if err != nil {
		t.Fatalf("encodeHistoryFloor: %v", err)
	}
	if err := os.WriteFile(output, data, 0o644); err != nil {
		t.Fatalf("write tail floor: %v", err)
	}
	if _, head, err := loadHistoryFloorChain(mutated, directory, false); err != nil {
		t.Fatalf("validate tail floor: %v", err)
	} else if head.Sequence != 3 {
		t.Fatalf("tail floor sequence = %d, want 3", head.Sequence)
	}
}

func TestSourceHistoryRecordFloorRejectsEmptyAddition(t *testing.T) {
	const inventoryPath = "../tournament/source_history.json"
	inventory, err := loadHistory(inventoryPath)
	if err != nil {
		t.Fatalf("loadHistory: %v", err)
	}
	data, err := os.ReadFile("../tournament/historyfloors/000001.json")
	if err != nil {
		t.Fatalf("read source-history floor: %v", err)
	}
	floor, err := decodeHistoryFloor(data)
	if err != nil {
		t.Fatalf("decode source-history floor: %v", err)
	}
	floor.Additions = []historyFloorAddition{}
	data, err = encodeHistoryFloor(floor)
	if err != nil {
		t.Fatalf("encode empty source-history floor: %v", err)
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "000001.json"), data, 0o644); err != nil {
		t.Fatalf("write empty source-history floor: %v", err)
	}
	if _, _, err := loadHistoryFloorChain(inventory, directory, false); err == nil {
		t.Fatal("empty source-history floor unexpectedly passed")
	}
}
