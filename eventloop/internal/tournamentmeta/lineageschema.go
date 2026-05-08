package main

import (
	"fmt"
	"strings"
)

func validateLineageRecordSchema(inventory lineageCatalog, lineageVersion int, kind, id string) error {
	required := 1
	switch kind {
	case "introduction":
		record := findLineageIntroduction(inventory.Introductions, id)
		if record.SourceKind == "dynamic-frozen-filesystem" || record.SourceIdentityKind == "component-tree-sha256" {
			required = 3
		}
	case "snapshot":
		record := findLineageSnapshot(inventory.Snapshots, id)
		if record.SourceKind == "dynamic-frozen-filesystem" || record.IdentityKind == "component-tree-sha256" {
			required = 3
		}
	case "implementation":
		record := findLineageImplementation(inventory.Implementations, id)
		if lineageEnum(record.Kind, "eventtarget", "metrics", "poller", "timer") {
			required = 3
		}
	case "harness":
		record := findLineageHarness(inventory.Harnesses, id)
		if record.BuildSelection.GoDirective != "" {
			required = 3
		}
	case "workload":
		record := findLineageWorkload(inventory.Workloads, id)
		if record.SemanticHarness != nil {
			required = 3
		}
	case "binding":
		record := findLineageBinding(inventory.Bindings, id)
		if record.Applicability == "diagnostic" {
			required = 3
		}
	}
	if lineageVersion < required {
		return fmt.Errorf("lineage %s %q requires schema %d, floor uses %d", kind, id, required, lineageVersion)
	}
	return nil
}

func findLineageWorkload(records []lineageWorkload, id string) lineageWorkload {
	for _, record := range records {
		if record.ID == id {
			return record
		}
	}
	panic("validated lineage workload disappeared: " + id)
}

func validateLineageSourcePrefix(
	inventory lineageCatalog,
	kind string,
	id string,
	source lineageSourceFloorState,
) error {
	var sourceKind string
	var sourceID string
	switch kind {
	case "introduction":
		record := findLineageIntroduction(inventory.Introductions, id)
		sourceKind, sourceID = record.SourceKind, record.SourceID
	case "snapshot":
		record := findLineageSnapshot(inventory.Snapshots, id)
		sourceKind, sourceID = record.SourceKind, record.SourceID
	case "alias":
		record := findLineageAlias(inventory.Aliases, id)
		if record.Kind == "exact-source" && strings.HasPrefix(record.AliasSubjectID, "source.") {
			sourceKind, sourceID = "source-occurrence", record.AliasSubjectID
		}
	}
	if sourceKind != "source-occurrence" {
		return nil
	}
	key := historyFloorKey("source_occurrence", sourceID)
	if sequence, ok := source.RecordSequences[key]; !ok || sequence > source.Authority.Sequence {
		return fmt.Errorf("lineage %s %q references source occurrence %q after its source-history prefix", kind, id, sourceID)
	}
	return nil
}

func validateLineageCurrentSourceAliases(inventory lineageCatalog, history historyInventory) error {
	occurrences := make(map[string]historyOccurrence, len(history.SourceOccurrences))
	for _, occurrence := range history.SourceOccurrences {
		occurrences[occurrence.ID] = occurrence
	}
	snapshots := make(map[string]lineageSnapshot, len(inventory.Snapshots))
	for _, snapshot := range inventory.Snapshots {
		snapshots[snapshot.ID] = snapshot
	}
	for _, alias := range inventory.Aliases {
		if alias.Kind != "exact-source" {
			continue
		}
		occurrenceID := alias.AliasSubjectID
		if suffix, ok := strings.CutPrefix(occurrenceID, "stash.commit.sha1."); ok {
			occurrenceID = "source.commit.sha1." + suffix
		}
		if !strings.HasPrefix(occurrenceID, "source.") {
			continue
		}
		occurrence, ok := occurrences[occurrenceID]
		if !ok {
			return fmt.Errorf("lineage exact-source alias %q has unknown source occurrence %q", alias.ID, occurrenceID)
		}
		snapshot, ok := snapshots[alias.CanonicalSubjectID]
		if !ok || snapshot.SourcePath != "." || snapshot.IdentityKind != "git-tree-sha1" || snapshot.Identity != occurrence.RootTree {
			return fmt.Errorf("lineage exact-source alias %q does not match canonical root snapshot %q", alias.ID, alias.CanonicalSubjectID)
		}
	}
	return nil
}
