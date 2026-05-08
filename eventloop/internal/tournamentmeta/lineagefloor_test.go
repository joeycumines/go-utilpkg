package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTournamentLineageRecordFloor(t *testing.T) {
	inventoryPath, err := filepath.Abs("../tournament/lineage.json")
	if err != nil {
		t.Fatalf("resolve lineage path: %v", err)
	}
	historyPath, err := filepath.Abs("../tournament/source_history.json")
	if err != nil {
		t.Fatalf("resolve history path: %v", err)
	}
	inventory, history, err := loadLineageAuthority(inventoryPath, historyPath)
	if err != nil {
		t.Fatalf("loadLineageAuthority: %v", err)
	}
	head, err := validateLineageFloors(inventory, inventoryPath, history, historyPath)
	if err != nil {
		t.Fatalf("validateLineageFloors: %v", err)
	}
	if head.Sequence != 3 || head.Path != "lineagefloors/000003.json" ||
		head.SHA256 != "0f428e302885daf3273a352a57e420d27e54554db5970fde0f5d8cf4d50706b5" ||
		head.CumulativeRecordSetSHA256 != "e85f6a46344c7311d4d6fe8cb9f89b3bcb0551f7529513039284b0e12d415865" {
		t.Fatalf("lineage floor head = %+v", head)
	}
	data, err := readRegularStable(inventoryPath, 0o644)
	if err != nil {
		t.Fatalf("read lineage inventory: %v", err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "f69586930dd0fb993be2d58709a3029d49dde5a2214fae4cf6e951c718777d6c" {
		t.Fatalf("lineage inventory SHA-256 = %s", got)
	}

	mutated := inventory
	mutated.Snapshots = slices.Clone(inventory.Snapshots)
	mutated.Snapshots[0].Adaptations = []string{"changed"}
	if _, _, err := loadLineageFloorChain(
		mutated,
		inventoryPath,
		mustSourceFloors(t, history, historyPath),
		"../tournament/lineagefloors",
		false,
	); err == nil {
		t.Fatal("mutated floored tournament lineage unexpectedly passed")
	}
}

func TestLineageCurrentSourceAliases(t *testing.T) {
	inventory, err := loadLineage("../tournament/lineage.json")
	if err != nil {
		t.Fatal(err)
	}
	history, err := loadHistory("../tournament/source_history.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateLineageCurrentSourceAliases(inventory, history); err != nil {
		t.Fatalf("validate current source aliases: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*lineageAlias)
	}{
		{name: "missing legacy occurrence", mutate: func(alias *lineageAlias) {
			alias.AliasSubjectID = "stash.commit.sha1.0000000000000000000000000000000000000000"
		}},
		{name: "wrong canonical root", mutate: func(alias *lineageAlias) {
			alias.CanonicalSubjectID = "snapshot.repository.sha1.955e4f445c5d7be00f40e660f24535885e6055b1"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutated := inventory
			mutated.Aliases = slices.Clone(inventory.Aliases)
			for index := range mutated.Aliases {
				if mutated.Aliases[index].ID == "alias.source.1396868d.0bc4ad0a" {
					test.mutate(&mutated.Aliases[index])
					if err := validateLineageCurrentSourceAliases(mutated, history); err == nil {
						t.Fatal("invalid legacy source alias passed")
					}
					return
				}
			}
			t.Fatal("legacy source alias is absent")
		})
	}
}

func TestLineageFloorGenesisAndTail(t *testing.T) {
	historyPath, err := filepath.Abs("../tournament/source_history.json")
	if err != nil {
		t.Fatalf("resolve history path: %v", err)
	}
	history, err := loadHistory(historyPath)
	if err != nil {
		t.Fatalf("loadHistory: %v", err)
	}
	historyData, err := readRegularStable(historyPath, 0o644)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	inventory := testLineage(fmt.Sprintf("%x", sha256.Sum256(historyData)))
	directory := t.TempDir()
	inventoryPath := filepath.Join(directory, "lineage.json")
	writeLineageFixture(t, inventoryPath, inventory)
	floorDirectory := filepath.Join(directory, "lineagefloors")
	if err := os.Mkdir(floorDirectory, 0o755); err != nil {
		t.Fatalf("create floor directory: %v", err)
	}
	genesisPath := filepath.Join(floorDirectory, "000001.json")
	genesis, err := createLineageFloor(inventory, inventoryPath, history, historyPath, floorDirectory, genesisPath)
	if err != nil {
		t.Fatalf("create genesis: %v", err)
	}
	if genesis.Sequence != 1 || genesis.PreviousFloor != nil || len(genesis.Additions) != 5 {
		t.Fatalf("genesis authority = %+v", genesis)
	}
	if got := genesis.CumulativeCounts; !slices.Equal(got, []lineageFloorCount{
		{Kind: "concept", Count: 1},
		{Kind: "disposition", Count: 1},
		{Kind: "implementation", Count: 1},
		{Kind: "introduction", Count: 1},
		{Kind: "snapshot", Count: 1},
	}) {
		t.Fatalf("genesis counts = %+v", got)
	}
	writeLineageFloorFixture(t, genesisPath, genesis)
	if head, err := validateLineageFloors(inventory, inventoryPath, history, historyPath); err != nil {
		t.Fatalf("validate genesis: %v", err)
	} else if head.Sequence != 1 || head.Path != "lineagefloors/000001.json" {
		t.Fatalf("genesis head = %+v", head)
	}
	if _, err := createLineageFloor(inventory, inventoryPath, history, historyPath, floorDirectory, filepath.Join(floorDirectory, "000002.json")); err == nil {
		t.Fatal("no-op lineage floor unexpectedly passed")
	}

	mutated := inventory
	mutated.Concepts = slices.Clone(inventory.Concepts)
	mutated.Concepts[0].Name = "Changed concept"
	if _, _, err := loadLineageFloorChain(mutated, inventoryPath, mustSourceFloors(t, history, historyPath), floorDirectory, false); err == nil {
		t.Fatal("mutated floored lineage record unexpectedly passed")
	}

	extended := inventory
	extended.Concepts = append(slices.Clone(inventory.Concepts), lineageConcept{
		ID: "concept.scheduler.second", Name: "Second concept",
		SourcePath:   "docs/tournament/2026-01-18/ANALYSIS_ALTERNATETWO_HYBRID.md",
		SourceSHA256: fmt.Sprintf("%064d", 2), Status: "concept-only", Disposition: "Not implemented.",
	})
	writeLineageFixture(t, inventoryPath, extended)
	if _, _, err := loadLineageFloorChain(extended, inventoryPath, mustSourceFloors(t, history, historyPath), floorDirectory, false); err == nil {
		t.Fatal("unfloored lineage tail unexpectedly passed")
	}
	tailPath := filepath.Join(floorDirectory, "000002.json")
	tail, err := createLineageFloor(extended, inventoryPath, history, historyPath, floorDirectory, tailPath)
	if err != nil {
		t.Fatalf("create tail: %v", err)
	}
	if tail.Sequence != 2 || len(tail.Additions) != 1 || tail.Additions[0].Kind != "concept" {
		t.Fatalf("tail authority = %+v", tail)
	}
	writeLineageFloorFixture(t, tailPath, tail)
	if _, head, err := loadLineageFloorChain(extended, inventoryPath, mustSourceFloors(t, history, historyPath), floorDirectory, false); err != nil {
		t.Fatalf("validate tail: %v", err)
	} else if head.Sequence != 2 {
		t.Fatalf("tail head = %+v", head)
	}
}

func TestLineageFloorRejectsEmptyAndSourceTamper(t *testing.T) {
	historyPath, err := filepath.Abs("../tournament/source_history.json")
	if err != nil {
		t.Fatalf("resolve history path: %v", err)
	}
	history, err := loadHistory(historyPath)
	if err != nil {
		t.Fatalf("loadHistory: %v", err)
	}
	historyData, err := readRegularStable(historyPath, 0o644)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	inventory := testLineage(fmt.Sprintf("%x", sha256.Sum256(historyData)))
	directory := t.TempDir()
	inventoryPath := filepath.Join(directory, "lineage.json")
	writeLineageFixture(t, inventoryPath, inventory)
	floorDirectory := filepath.Join(directory, "lineagefloors")
	if err := os.Mkdir(floorDirectory, 0o755); err != nil {
		t.Fatalf("create floor directory: %v", err)
	}
	floorPath := filepath.Join(floorDirectory, "000001.json")
	floor, err := createLineageFloor(inventory, inventoryPath, history, historyPath, floorDirectory, floorPath)
	if err != nil {
		t.Fatalf("create lineage floor: %v", err)
	}
	floor.Additions = []lineageFloorAddition{}
	writeLineageFloorFixture(t, floorPath, floor)
	if _, _, err := loadLineageFloorChain(inventory, inventoryPath, mustSourceFloors(t, history, historyPath), floorDirectory, false); err == nil {
		t.Fatal("empty lineage floor unexpectedly passed")
	}
	if err := os.Remove(floorPath); err != nil {
		t.Fatalf("remove empty floor: %v", err)
	}
	floor, err = createLineageFloor(inventory, inventoryPath, history, historyPath, floorDirectory, floorPath)
	if err != nil {
		t.Fatalf("recreate lineage floor: %v", err)
	}
	floor.SourceHistoryFloor.SHA256 = fmt.Sprintf("%064d", 1)
	writeLineageFloorFixture(t, floorPath, floor)
	if _, _, err := loadLineageFloorChain(inventory, inventoryPath, mustSourceFloors(t, history, historyPath), floorDirectory, false); err == nil {
		t.Fatal("tampered source-history floor pin unexpectedly passed")
	}
}

func TestLineageFloorRejectsSchemaJumpAndPrematureFeatures(t *testing.T) {
	historyPath, err := filepath.Abs("../tournament/source_history.json")
	if err != nil {
		t.Fatalf("resolve history path: %v", err)
	}
	history, err := loadHistory(historyPath)
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	historyData, err := readRegularStable(historyPath, 0o644)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	inventory := testLineage(fmt.Sprintf("%x", sha256.Sum256(historyData)))
	directory := t.TempDir()
	inventoryPath := filepath.Join(directory, "lineage.json")
	writeLineageFixture(t, inventoryPath, inventory)
	floorDirectory := filepath.Join(directory, "lineagefloors")
	if err := os.Mkdir(floorDirectory, 0o755); err != nil {
		t.Fatalf("create floor directory: %v", err)
	}
	genesisPath := filepath.Join(floorDirectory, "000001.json")
	genesis, err := createLineageFloor(inventory, inventoryPath, history, historyPath, floorDirectory, genesisPath)
	if err != nil {
		t.Fatalf("create genesis: %v", err)
	}
	genesis.LineageSchemaVersion = 1
	writeLineageFloorFixture(t, genesisPath, genesis)

	schema3 := inventory
	schema3.SchemaVersion = 3
	schema3.Concepts = append(slices.Clone(inventory.Concepts), lineageConcept{
		ID: "concept.scheduler.schema-three", Name: "Schema three concept",
		SourcePath:   "docs/tournament/2026-01-18/ANALYSIS_ALTERNATETWO_HYBRID.md",
		SourceSHA256: strings.Repeat("f", 64), Status: "concept-only", Disposition: "Not implemented.",
	})
	writeLineageFixture(t, inventoryPath, schema3)
	if _, err := createLineageFloor(
		schema3,
		inventoryPath,
		history,
		historyPath,
		floorDirectory,
		filepath.Join(floorDirectory, "000002.json"),
	); err == nil {
		t.Fatal("lineage schema jump from 1 to 3 unexpectedly passed")
	}

	dynamic := testLineageHelperAlias()
	dynamic.SchemaVersion = 3
	dynamic.Introductions[0].SourceKind = "dynamic-frozen-filesystem"
	dynamic.Introductions[0].SourceID = "source.eventloop.current"
	dynamic.Introductions[0].SourceIdentityKind = "component-tree-sha256"
	dynamic.Introductions[0].SourceIdentity = strings.Repeat("d", 64)
	if err := validateLineageRecordSchema(dynamic, 2, "introduction", dynamic.Introductions[0].ID); err == nil {
		t.Fatal("schema 3 dynamic introduction admitted to schema 2 floor")
	}
	if err := validateLineageRecordSchema(dynamic, 3, "introduction", dynamic.Introductions[0].ID); err != nil {
		t.Fatalf("schema 3 dynamic introduction rejected from schema 3 floor: %v", err)
	}

	diagnostic := testLineageDiagnostic()
	diagnosticID := "binding.scheduler.diagnostic"
	if err := validateLineageRecordSchema(diagnostic, 2, "binding", diagnosticID); err == nil {
		t.Fatal("schema 3 diagnostic binding admitted to schema 2 floor")
	}
	if err := validateLineageRecordSchema(diagnostic, 3, "binding", diagnosticID); err != nil {
		t.Fatalf("schema 3 diagnostic binding rejected from schema 3 floor: %v", err)
	}
	workloadID := diagnostic.Workloads[0].ID
	if err := validateLineageRecordSchema(diagnostic, 2, "workload", workloadID); err == nil {
		t.Fatal("schema 3 semantic harness admitted to schema 2 floor")
	}
	if err := validateLineageRecordSchema(diagnostic, 3, "workload", workloadID); err != nil {
		t.Fatalf("schema 3 semantic harness rejected from schema 3 floor: %v", err)
	}
}

func TestLineageFloorRejectsDiagnosticBindingUnderSchemaTwo(t *testing.T) {
	historyPath, err := filepath.Abs("../tournament/source_history.json")
	if err != nil {
		t.Fatalf("resolve history path: %v", err)
	}
	history, err := loadHistory(historyPath)
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	historyData, err := readRegularStable(historyPath, 0o644)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	historySHA := fmt.Sprintf("%x", sha256.Sum256(historyData))
	directory := t.TempDir()
	inventoryPath := filepath.Join(directory, "lineage.json")
	floorDirectory := filepath.Join(directory, "lineagefloors")
	if err := os.Mkdir(floorDirectory, 0o755); err != nil {
		t.Fatalf("create floor directory: %v", err)
	}

	base := testLineage(historySHA)
	writeLineageFixture(t, inventoryPath, base)
	genesisPath := filepath.Join(floorDirectory, "000001.json")
	genesis, err := createLineageFloor(base, inventoryPath, history, historyPath, floorDirectory, genesisPath)
	if err != nil {
		t.Fatalf("create schema-2 genesis: %v", err)
	}
	writeLineageFloorFixture(t, genesisPath, genesis)

	extended := testLineageDiagnostic()
	extended.SourceHistorySHA256 = historySHA
	writeLineageFixture(t, inventoryPath, extended)
	schemaThreePath := filepath.Join(floorDirectory, "000002.json")
	schemaThree, err := createLineageFloor(
		extended,
		inventoryPath,
		history,
		historyPath,
		floorDirectory,
		schemaThreePath,
	)
	if err != nil {
		t.Fatalf("create schema-3 diagnostic floor: %v", err)
	}
	schemaThree.LineageSchemaVersion = 2
	writeLineageFloorFixture(t, schemaThreePath, schemaThree)
	if _, _, err := loadLineageFloorChain(
		extended,
		inventoryPath,
		mustSourceFloors(t, history, historyPath),
		floorDirectory,
		false,
	); err == nil {
		t.Fatal("diagnostic binding attributed to schema-2 floor unexpectedly passed")
	}
}

func TestLineageDigestGolden(t *testing.T) {
	record := lineageFloorRecord{
		Kind: "concept", ID: "concept.scheduler.example",
		Digest: digestLineageFloorRecord("concept", "concept.scheduler.example", []byte(`{"id":"concept.scheduler.example"}`)),
	}
	if record.Digest != "2821fcf9f15eafbcec407cdf7183e3c1ec620eefad123a5dc7c3d0e967708248" {
		t.Fatalf("record digest = %s", record.Digest)
	}
	got := digestLineageFloorSet(map[string]lineageFloorRecord{lineageFloorKey(record.Kind, record.ID): record})
	if got != "259443a0a7a760f1834a99d4ed625af030257822fb1b35a14a15a47306057a31" {
		t.Fatalf("set digest = %s", got)
	}
}

func writeLineageFixture(t *testing.T, path string, inventory lineageCatalog) {
	t.Helper()
	data, err := encodeLineage(inventory)
	if err != nil {
		t.Fatalf("encode lineage fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write lineage fixture: %v", err)
	}
}

func writeLineageFloorFixture(t *testing.T, path string, floor lineageFloor) {
	t.Helper()
	data, err := encodeLineageFloor(floor)
	if err != nil {
		t.Fatalf("encode lineage floor fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write lineage floor fixture: %v", err)
	}
}

func mustSourceFloors(t *testing.T, history historyInventory, historyPath string) []lineageSourceFloorState {
	t.Helper()
	floors, err := loadStableHistoryFloors(history, historyPath)
	if err != nil {
		t.Fatalf("load source floors: %v", err)
	}
	return floors
}

func TestLineageSourceHistoryFloorRemainsImmutable(t *testing.T) {
	data, err := os.ReadFile("../tournament/historyfloors/000001.json")
	if err != nil {
		t.Fatalf("read source-history floor: %v", err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "d5490bb0364abe898b3be1066281c4cdb24ad07d03d1a8500ecc71f55e0a8fbf" {
		t.Fatalf("source-history floor SHA-256 = %s", got)
	}
	if !bytes.Contains(data, []byte(`"cumulative_record_set_sha256": "699124f31d4dfb12558b3e486d3d0730c5dafef44d57cc5861995a587fa7d8f9"`)) {
		t.Fatal("source-history cumulative floor identity changed")
	}
}
