package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type protocolIdentityVectorFile struct { // betteralign:ignore canonical JSON field order
	SchemaVersion  int                           `json:"schema_version"`
	Algorithm      string                        `json:"algorithm"`
	Domains        protocolIdentityVectorDomains `json:"domains"`
	FramingVectors []protocolFramingVector       `json:"framing_vectors"`
	Comparison     protocolComparisonVector      `json:"comparison"`
	Unit           protocolUnitVector            `json:"unit"`
	SelectionPlan  protocolSelectionPlanVector   `json:"selection_plan"`
	Execution      protocolExecutionVector       `json:"execution"`
}

type protocolIdentityVectorDomains struct { // betteralign:ignore canonical JSON field order
	Comparison    string `json:"comparison"`
	Unit          string `json:"unit"`
	SelectionPlan string `json:"selection_plan"`
	Execution     string `json:"execution"`
}

type protocolFramingVector struct { // betteralign:ignore canonical JSON field order
	Name   string   `json:"name"`
	Domain string   `json:"domain"`
	Fields []string `json:"fields"`
	SHA256 string   `json:"sha256"`
}

type protocolComparisonVector struct { // betteralign:ignore canonical JSON field order
	Input  comparisonIdentityInput `json:"input"`
	SHA256 string                  `json:"sha256"`
}

type protocolUnitVector struct { // betteralign:ignore canonical JSON field order
	Input  unitIdentityInput `json:"input"`
	SHA256 string            `json:"sha256"`
}

type protocolSelectionPlanVector struct { // betteralign:ignore canonical JSON field order
	Input  selectionPlanIdentityInput `json:"input"`
	SHA256 string                     `json:"sha256"`
}

type protocolExecutionVector struct { // betteralign:ignore canonical JSON field order
	Input  executionIdentityInput `json:"input"`
	SHA256 string                 `json:"sha256"`
}

func TestProtocolIdentityVectors(t *testing.T) {
	want := canonicalProtocolIdentityVectors(t)
	payload, err := os.ReadFile("../tournament/testdata/protocolidentityvectors.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var got protocolIdentityVectorFile
	if err := decoder.Decode(&got); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("protocol identity vectors have trailing JSON: %v", err)
	}
	if reflect.DeepEqual(got, want) {
		return
	}
	canonical, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	newline := append(canonical, '\n')
	if bytes.Equal(payload, newline) {
		t.Fatal("decoded protocol vectors differ despite byte-identical canonical JSON")
	}
	if got.SchemaVersion == want.SchemaVersion && got.Algorithm == want.Algorithm {
		t.Fatalf("protocol identity vectors differ from canonical definitions; replacement:\n%s", newline)
	}
	t.Fatalf("protocol identity vector header differs; replacement:\n%s", newline)
}

func TestProtocolIdentityRejectsMutations(t *testing.T) {
	vectors := canonicalProtocolIdentityVectors(t)

	comparison := vectors.Comparison.Input
	comparison.Configuration = slices.Clone(comparison.Configuration)
	comparison.Configuration[0], comparison.Configuration[1] = comparison.Configuration[1], comparison.Configuration[0]
	if _, err := comparisonIdentity(comparison); err == nil {
		t.Fatal("unsorted comparison configuration unexpectedly passed")
	}

	unit := vectors.Unit.Input
	unit.Bindings = slices.Clone(unit.Bindings)
	unit.Bindings[0].Results = slices.Clone(unit.Bindings[0].Results)
	unit.Bindings[0].Results[0].ComparisonID = strings.Repeat("0", 64)
	if _, err := unitIdentity(unit); err == nil {
		t.Fatal("wrong comparison identity unexpectedly passed")
	}

	plan := vectors.SelectionPlan.Input
	plan.UnitIDs = append(slices.Clone(plan.UnitIDs), plan.UnitIDs[0])
	if _, err := selectionPlanIdentity(plan); err == nil {
		t.Fatal("duplicate plan unit unexpectedly passed")
	}

	execution := vectors.Execution.Input
	execution.Environment = []string{"GOOS=linux", "GOOS=darwin"}
	if _, err := executionIdentity(execution); err == nil {
		t.Fatal("duplicate execution environment key unexpectedly passed")
	}
}

func TestProtocolIdentityDomainsAndCardinality(t *testing.T) {
	vectors := canonicalProtocolIdentityVectors(t)
	identities := []string{
		vectors.Comparison.SHA256,
		vectors.Unit.SHA256,
		vectors.SelectionPlan.SHA256,
		vectors.Execution.SHA256,
	}
	unique := make(map[string]struct{}, len(identities))
	for _, identity := range identities {
		unique[identity] = struct{}{}
	}
	if len(unique) != len(identities) {
		t.Fatal("protocol identity domains did not separate identical-looking authority")
	}

	unit := vectors.Unit.Input
	unit.Applicability = "diagnostic"
	if _, err := unitIdentity(unit); err == nil {
		t.Fatal("diagnostic unit with performance results unexpectedly passed")
	}
	unit.Bindings = slices.Clone(unit.Bindings)
	unit.Bindings[0].Results = []unitIdentityResult{}
	if _, err := unitIdentity(unit); err != nil {
		t.Fatalf("status-only diagnostic unit: %v", err)
	}
	unit.Applicability = "executable"
	if _, err := unitIdentity(unit); err == nil {
		t.Fatal("executable unit without performance results unexpectedly passed")
	}
}

func TestProtocolIdentityRejectsOrderAndShapeDrift(t *testing.T) {
	vectors := canonicalProtocolIdentityVectors(t)

	unit := vectors.Unit.Input
	unit.Bindings = append(slices.Clone(unit.Bindings), unit.Bindings[0])
	unit.Bindings[1].BindingID = "binding.scheduler.main.zeta.linux-arm64"
	if _, err := unitIdentity(unit); err != nil {
		t.Fatalf("sorted two-binding unit: %v", err)
	}
	unit.Bindings[0], unit.Bindings[1] = unit.Bindings[1], unit.Bindings[0]
	if _, err := unitIdentity(unit); err == nil {
		t.Fatal("unsorted unit binding set unexpectedly passed")
	}
	unit = vectors.Unit.Input
	unit.Bindings = slices.Clone(unit.Bindings)
	unit.Bindings[0].Benchmark = "Benchmark"
	if _, err := unitIdentity(unit); err == nil {
		t.Fatal("invalid benchmark root name unexpectedly passed")
	}

	plan := vectors.SelectionPlan.Input
	plan.Dispositions = append(slices.Clone(plan.Dispositions), selectionPlanDisposition{
		Kind: "root-disposition", SubjectID: "raw-root.scheduler.disposed", AuthorityID: "disposition.scheduler.disposed",
	})
	if _, err := selectionPlanIdentity(plan); err != nil {
		t.Fatalf("sorted two-disposition plan: %v", err)
	}
	plan.Dispositions[0], plan.Dispositions[1] = plan.Dispositions[1], plan.Dispositions[0]
	if _, err := selectionPlanIdentity(plan); err == nil {
		t.Fatal("unsorted plan disposition set unexpectedly passed")
	}

	execution := vectors.Execution.Input
	execution.NativeAuthoritySHA256 = ""
	if _, err := executionIdentity(execution); err == nil {
		t.Fatal("implicit native authority unexpectedly passed")
	}
}

func TestProtocolIdentityChangesWithBoundAuthority(t *testing.T) {
	vectors := canonicalProtocolIdentityVectors(t)

	comparison := vectors.Comparison.Input
	comparison.StableLeaf = "Scheduler/Main/AutoV2"
	if got, err := comparisonIdentity(comparison); err != nil {
		t.Fatal(err)
	} else if got == vectors.Comparison.SHA256 {
		t.Fatal("comparison identity ignored stable leaf")
	}

	unit := vectors.Unit.Input
	unit.HarnessID = "harness.scheduler.roundtrip.linux-arm64-cgo0-v2"
	if got, err := unitIdentity(unit); err != nil {
		t.Fatal(err)
	} else if got == vectors.Unit.SHA256 {
		t.Fatal("unit identity ignored physical harness")
	}

	plan := vectors.SelectionPlan.Input
	plan.ManifestSHA256 = strings.Repeat("6", 64)
	if got, err := selectionPlanIdentity(plan); err != nil {
		t.Fatal(err)
	} else if got == vectors.SelectionPlan.SHA256 {
		t.Fatal("selection-plan identity ignored manifest")
	}

	execution := vectors.Execution.Input
	execution.Argv = slices.Clone(execution.Argv)
	execution.Argv[2] = "-test.bench=^BenchmarkSchedulerRoundTripV2$"
	if got, err := executionIdentity(execution); err != nil {
		t.Fatal(err)
	} else if got == vectors.Execution.SHA256 {
		t.Fatal("execution identity ignored argv")
	}
}

func canonicalProtocolIdentityVectors(t *testing.T) protocolIdentityVectorFile {
	t.Helper()
	measurement := strings.Repeat("a", 64)
	configuration := []lineageSetting{{Key: "fast_path", Value: "auto"}, {Key: "producers", Value: "1"}}
	comparisonInput := comparisonIdentityInput{
		ImplementationID: "implementation.scheduler.main.auto", WorkloadID: "workload.scheduler.roundtrip.v2",
		Configuration: configuration, SemanticHarnessID: "semantic-harness.scheduler.roundtrip.v2",
		MeasurementContractSHA256: measurement, StableLeaf: "Scheduler/Main/Auto",
	}
	comparisonID, err := comparisonIdentity(comparisonInput)
	if err != nil {
		t.Fatal(err)
	}
	unitInput := unitIdentityInput{
		LaneID: "lane.scheduler", BuildCellID: "build-cell.eventloop.linux-arm64-cgo0",
		ModuleID: "eventloop", Package: "internal/tournament", RawRootID: "raw-root.scheduler.roundtrip",
		HarnessID: "harness.scheduler.roundtrip.linux-arm64-cgo0", Applicability: "executable",
		MeasurementContractSHA256: measurement,
		Bindings: []unitIdentityBinding{{
			BindingID: "binding.scheduler.main.auto.linux-arm64", Benchmark: "BenchmarkSchedulerRoundTrip",
			ImplementationID: "implementation.scheduler.main.auto", SnapshotIDs: []string{"snapshot.scheduler.main.current"},
			WorkloadID: "workload.scheduler.roundtrip.v2", Configuration: configuration,
			SemanticHarnessID: "semantic-harness.scheduler.roundtrip.v2",
			Results: []unitIdentityResult{{
				EmittedLeaf: "Scheduler/Main/Auto", StableLeaf: "Scheduler/Main/Auto", ComparisonID: comparisonID,
			}},
		}},
	}
	unitID, err := unitIdentity(unitInput)
	if err != nil {
		t.Fatal(err)
	}
	planInput := selectionPlanIdentityInput{
		ManifestSHA256: strings.Repeat("b", 64), LineageSHA256: strings.Repeat("c", 64),
		LineageFloorSHA256: strings.Repeat("d", 64), SharedSourceID: strings.Repeat("e", 64),
		SourceCaptureID: strings.Repeat("f", 64), HostAuthorityID: strings.Repeat("1", 64),
		MeasurementContractSHA256: measurement,
		BuildCellIDs:              []string{"build-cell.eventloop.linux-arm64-cgo0"}, UnitIDs: []string{unitID},
		Dispositions: []selectionPlanDisposition{{
			Kind: "alias-only", SubjectID: "binding.scheduler.handoff.alias", AuthorityID: "alias.scheduler.handoff",
		}},
	}
	planID, err := selectionPlanIdentity(planInput)
	if err != nil {
		t.Fatal(err)
	}
	executionInput := executionIdentityInput{
		PlanID: planID, UnitID: unitID, SharedSourceID: strings.Repeat("e", 64),
		SourceCaptureID: strings.Repeat("f", 64), BinarySHA256: strings.Repeat("2", 64),
		ToolchainAuthoritySHA256: strings.Repeat("3", 64), ModuleGraphSHA256: strings.Repeat("4", 64),
		NativeAuthoritySHA256: "none", HostAuthorityID: strings.Repeat("1", 64),
		MeasurementProfileSHA256: measurement, ExecutionProfileSHA256: strings.Repeat("5", 64),
		Argv:        []string{"/frozen/bin/scheduler.test", "-test.run=^$", "-test.bench=^BenchmarkSchedulerRoundTrip$"},
		Environment: []string{"GOARCH=arm64", "GOMAXPROCS=1", "GOOS=linux"},
	}
	executionID, err := executionIdentity(executionInput)
	if err != nil {
		t.Fatal(err)
	}
	return protocolIdentityVectorFile{
		SchemaVersion: 1, Algorithm: "sha256-domain-length-framed-v1",
		Domains: protocolIdentityVectorDomains{
			Comparison: comparisonIdentityDomain, Unit: unitIdentityDomain,
			SelectionPlan: selectionPlanDomain, Execution: executionIdentityDomain,
		},
		FramingVectors: []protocolFramingVector{
			newProtocolFramingVector("unicode-empty-nul", "go-utilpkg/eventloop/tournament/framing-test/v1", "", "µs", "\x00"),
			newProtocolFramingVector("concatenation-a-bc", "a", "bc"),
			newProtocolFramingVector("concatenation-ab-c", "ab", "c"),
		},
		Comparison:    protocolComparisonVector{Input: comparisonInput, SHA256: comparisonID},
		Unit:          protocolUnitVector{Input: unitInput, SHA256: unitID},
		SelectionPlan: protocolSelectionPlanVector{Input: planInput, SHA256: planID},
		Execution:     protocolExecutionVector{Input: executionInput, SHA256: executionID},
	}
}

func newProtocolFramingVector(name, domain string, fields ...string) protocolFramingVector {
	return protocolFramingVector{
		Name: name, Domain: domain, Fields: fields,
		SHA256: protocolIdentityDigest(domain, fields),
	}
}
