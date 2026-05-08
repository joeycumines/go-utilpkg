package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSourceHistoryInventory(t *testing.T) {
	const path = "../tournament/source_history.json"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got, want := fmt.Sprintf("%x", sha256.Sum256(data)), "0638cb2b64a2c9f5a7d2b1a46310d2c345ecf45919945df28219414318357d60"; got != want {
		t.Fatalf("source history SHA-256 = %s, want %s", got, want)
	}
	inventory, err := loadHistory(path)
	if err != nil {
		t.Fatalf("loadHistory: %v", err)
	}
	wantOccurrences := map[string][2]string{
		"source.commit.sha1.bcadd4aaa61c7a9c1068a493c948bb88bf5fa038": {
			"root-alias",
			"source.commit.sha1.8bbefe5623c5b94cd85aa8dda2f3ebe9007d3eba",
		},
		"source.commit.sha1.5f691abe5d4557c13eb7c16d42f60f203df24336": {
			"root-alias",
			"source.commit.sha1.fa68be1139e0fac25349b3e7644ffd73a22d6616",
		},
		"source.commit.sha1.f7ef4c86843e1790bf0528975ab2c92ba3351702": {
			"tree-patch",
			"source.tree-patch.sha256.1ae62529924d910ba7f038a8afdd7d5bbf54c2876100b033ccc3e152ab80f48a",
		},
		"source.commit.sha1.53e2f662adc245c9b63e06bb64977b0751dcff82": {
			"root-alias",
			"source.commit.sha1.0bc4ad0ae702ce2205615c31dcf37992d67ff9c8",
		},
		"source.commit.sha1.1396868d29689c659ff7782760e89423aa478cf4": {
			"root-alias",
			"source.commit.sha1.0bc4ad0ae702ce2205615c31dcf37992d67ff9c8",
		},
	}
	for _, occurrence := range inventory.SourceOccurrences {
		expected, ok := wantOccurrences[occurrence.ID]
		if !ok {
			continue
		}
		if occurrence.TreeMaterializationKind != expected[0] || occurrence.TreeMaterializationID != expected[1] {
			t.Errorf(
				"occurrence %q materialization = %q/%q, want %q/%q",
				occurrence.ID,
				occurrence.TreeMaterializationKind,
				occurrence.TreeMaterializationID,
				expected[0],
				expected[1],
			)
		}
		delete(wantOccurrences, occurrence.ID)
	}
	if len(wantOccurrences) != 0 {
		t.Fatalf("source history omitted governed occurrences: %v", wantOccurrences)
	}

	wantTransitions := map[string][2]string{
		"source.transition.eventloop.sha1.4b88649daa06b54b1d3116d9d34ff6cb02b2898c.55a1295d0d2e9a262a07950257d155227dc9279c": {
			"source.commit.sha1.8bbefe5623c5b94cd85aa8dda2f3ebe9007d3eba",
			"source.commit.sha1.8023c2d1f01528922d8e98c17f286240fdb543be",
		},
		"source.transition.eventloop.sha1.50f6f34396121b78a9d48f2481d8ccefc9d08596.6ba4238ee231457b81e7f71e82a21615fa044678": {
			"source.commit.sha1.3302f879a6cc3f52774a3d6ebf442b80e24a660b",
			"source.commit.sha1.eac8da394f98db1932e83de00b2f295b9cc813ec",
		},
	}
	for _, transition := range inventory.SourceTransitions {
		expected, ok := wantTransitions[transition.ID]
		if !ok {
			continue
		}
		if transition.WitnessParentOccurrenceID != expected[0] || transition.WitnessChildOccurrenceID != expected[1] {
			t.Errorf("transition %q witness = %q/%q, want %q/%q", transition.ID,
				transition.WitnessParentOccurrenceID, transition.WitnessChildOccurrenceID, expected[0], expected[1])
		}
		delete(wantTransitions, transition.ID)
	}
	if len(wantTransitions) != 0 {
		t.Fatalf("source history omitted non-introduction transitions: %v", wantTransitions)
	}

	mutated := inventory
	mutated.SourceOccurrences = append([]historyOccurrence(nil), inventory.SourceOccurrences...)
	mutated.SourceOccurrences[0].DiscoveryOrdinal = 2
	if err := validateHistory(mutated); err == nil {
		t.Fatal("history with a duplicate ordinal unexpectedly passed")
	}
	mutated = inventory
	mutated.SourceTransitions = append([]historyTransition(nil), inventory.SourceTransitions[1:]...)
	if err := validateHistory(mutated); err == nil {
		t.Fatal("history without one all-occurrence transition unexpectedly passed")
	}
}

func TestOrderHistoryCommitsAppendsLateDiscoveries(t *testing.T) {
	commits := map[string]*historyCommit{
		"base":    {oid: "base", commitEpoch: 10, authorEpoch: 10},
		"regular": {oid: "regular", parents: []string{"base"}, commitEpoch: 20, authorEpoch: 20},
		"late-1":  {oid: "late-1", parents: []string{"base"}, commitEpoch: 15, authorEpoch: 15, lateDiscovery: 1},
		"late-2":  {oid: "late-2", parents: []string{"late-1"}, commitEpoch: 15, authorEpoch: 15, lateDiscovery: 2},
	}
	ordered, err := orderHistoryCommits(commits)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"base", "regular", "late-1", "late-2"}
	if len(ordered) != len(want) {
		t.Fatalf("ordered commits = %d, want %d", len(ordered), len(want))
	}
	for index, commit := range ordered {
		if commit.oid != want[index] {
			t.Errorf("ordered commit %d = %q, want %q", index, commit.oid, want[index])
		}
	}
}

func TestOrderHistoryCommitsRejectsInvalidLateDiscovery(t *testing.T) {
	tests := map[string]map[string]*historyCommit{
		"duplicate": {
			"late-a": {oid: "late-a", lateDiscovery: 1},
			"late-b": {oid: "late-b", lateDiscovery: 1},
		},
		"gap": {
			"late": {oid: "late", lateDiscovery: 2},
		},
		"ancestor": {
			"base":    {oid: "base"},
			"late":    {oid: "late", parents: []string{"base"}, lateDiscovery: 1},
			"regular": {oid: "regular", parents: []string{"late"}},
		},
	}
	for name, commits := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := orderHistoryCommits(commits); err == nil {
				t.Fatal("invalid late-discovery order passed")
			}
		})
	}
}

func TestHistoryArchiveRehydratesAnchoredOnlyAuthority(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("repository path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repository, ".git")); os.IsNotExist(err) {
		t.Skip("anchored-only regeneration requires the monorepo Git authority")
	} else if err != nil {
		t.Fatalf("inspect repository: %v", err)
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("Git unavailable: %v", err)
	}
	gitPath, err = filepath.Abs(gitPath)
	if err != nil {
		t.Fatalf("Git path: %v", err)
	}
	inventory, err := generateHistory(gitPath, repository)
	if err != nil {
		t.Fatalf("generateHistory: %v", err)
	}
	generated, err := encodeHistory(inventory)
	if err != nil {
		t.Fatalf("encodeHistory: %v", err)
	}
	tracked, err := os.ReadFile("../tournament/source_history.json")
	if err != nil {
		t.Fatalf("read tracked history: %v", err)
	}
	if !bytes.Equal(generated, tracked) {
		t.Fatal("anchored-only rehydration differs from tracked source history")
	}
}
