package tournament

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"
)

type timerWorkloadDigestVectorFile struct {
	SchemaVersion   uint8                        `json:"schema_version"`
	Domain          string                       `json:"domain"`
	FramingVectors  []timerWorkloadFramingVector `json:"framing_vectors"`
	WorkloadVectors []timerWorkloadDigestVector  `json:"workload_vectors"`
}

type timerWorkloadFramingVector struct {
	Name   string   `json:"name"`
	Domain string   `json:"domain"`
	Fields []string `json:"fields"`
	SHA256 string   `json:"sha256"`
}

type timerWorkloadDigestVector struct {
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	ParameterType string `json:"parameter_type"`
	ParameterJSON string `json:"parameter_json"`
	SHA256        string `json:"sha256"`
}

func TestTimerWorkloadDigestVectors(t *testing.T) {
	want := canonicalTimerWorkloadDigestVectors(t)
	payload, err := os.ReadFile("testdata/timerworkloaddigestvectors.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var got timerWorkloadDigestVectorFile
	if err := decoder.Decode(&got); err != nil {
		t.Fatal(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("timer workload digest vectors have trailing JSON: %v", err)
	}
	if reflect.DeepEqual(got, want) {
		return
	}
	canonical, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("timer workload digest vectors differ from canonical definitions; replacement:\n%s", canonical)
}

func canonicalTimerWorkloadDigestVectors(t *testing.T) timerWorkloadDigestVectorFile {
	t.Helper()
	file := timerWorkloadDigestVectorFile{
		SchemaVersion: 1,
		Domain:        timerParameterDigestDomain,
		FramingVectors: []timerWorkloadFramingVector{
			newTimerWorkloadFramingVector("unicode-empty-nul", "go-utilpkg/eventloop/tournament/framing-test/v1", "", "µs", "\x00"),
			newTimerWorkloadFramingVector("concatenation-a-bc", "a", "bc"),
			newTimerWorkloadFramingVector("concatenation-ab-c", "ab", "c"),
		},
	}
	seen := make(map[string]struct{}, timerStorageWorkloadCount+timerQualificationWorkloadCount)
	for _, definition := range timerStorageDefinitions() {
		file.WorkloadVectors = append(file.WorkloadVectors, newTimerWorkloadDigestVector(t, "storage", string(definition.ID), definition.Parameters, definition.ParameterSHA256, seen))
	}
	for _, definition := range timerQualificationDefinitions() {
		file.WorkloadVectors = append(file.WorkloadVectors, newTimerWorkloadDigestVector(t, "qualification", string(definition.ID), definition.Parameters, definition.ParameterSHA256, seen))
	}
	if len(file.WorkloadVectors) != timerStorageWorkloadCount+timerQualificationWorkloadCount {
		t.Fatalf("timer workload digest vector count = %d", len(file.WorkloadVectors))
	}
	return file
}

func newTimerWorkloadFramingVector(name, domain string, fields ...string) timerWorkloadFramingVector {
	return timerWorkloadFramingVector{
		Name: name, Domain: domain, Fields: fields,
		SHA256: fmt.Sprintf("%x", framedTimerSeal(domain, fields...)),
	}
}

func newTimerWorkloadDigestVector(t *testing.T, kind, id string, parameters any, digest string, seen map[string]struct{}) timerWorkloadDigestVector {
	t.Helper()
	key := kind + "\x00" + id
	if _, ok := seen[key]; ok {
		t.Fatalf("duplicate timer workload digest vector %q/%q", kind, id)
	}
	seen[key] = struct{}{}
	payload, err := json.Marshal(parameters)
	if err != nil {
		t.Fatal(err)
	}
	parameterType := reflect.TypeOf(parameters).String()
	if got := fmt.Sprintf("%x", framedTimerSeal(timerParameterDigestDomain, kind, id, parameterType, string(payload))); got != digest {
		t.Fatalf("timer workload digest %q/%q = %s, want %s", kind, id, got, digest)
	}
	return timerWorkloadDigestVector{
		Kind: kind, ID: id, ParameterType: parameterType,
		ParameterJSON: string(payload), SHA256: digest,
	}
}
