package tournament

import (
	"strings"
	"testing"
)

func TestTimerReferenceWorkloadDefinitions(t *testing.T) {
	definitions := timerReferenceDefinitions()
	if len(definitions) != timerReferenceWorkloadCount || len(allTimerReferenceWorkloads) != timerReferenceWorkloadCount {
		t.Fatalf("reference definitions/workloads = (%d, %d), want %d", len(definitions), len(allTimerReferenceWorkloads), timerReferenceWorkloadCount)
	}
	seenHarnesses := make(map[string]struct{}, len(definitions))
	for index, definition := range definitions {
		if definition.ID != allTimerReferenceWorkloads[index] || !validTimerReferenceDefinition(definition) {
			t.Errorf("reference definition %d = %+v, want ordered valid %q", index, definition, allTimerReferenceWorkloads[index])
		}
		if len(definition.ParameterSHA256) != 64 || len(definition.DefinitionSHA256) != 64 {
			t.Errorf("reference definition %q digest lengths = (%d, %d)", definition.ID, len(definition.ParameterSHA256), len(definition.DefinitionSHA256))
		}
		harness := definition.SemanticHarness
		if harness.ID == "" || harness.Setup == "" || harness.Timed == "" || harness.Teardown == "" {
			t.Errorf("reference definition %q has incomplete semantic harness %+v", definition.ID, harness)
		}
		if strings.ContainsAny(harness.ID+harness.Setup+harness.Timed+harness.Teardown, "\r\n") {
			t.Errorf("reference definition %q semantic harness is not one-line", definition.ID)
		}
		if _, duplicate := seenHarnesses[harness.ID]; duplicate {
			t.Errorf("duplicate reference semantic harness %q", harness.ID)
		}
		parameters := definition.Parameters
		if parameters.OperationCount == 0 || parameters.OperationCount > uint8(len(parameters.Operations)) || parameters.ExpectedCount == 0 || parameters.ExpectedCount > uint8(len(parameters.Expected)) {
			t.Errorf("reference definition %q has invalid typed cardinalities %+v", definition.ID, parameters)
		}
		if definition.ID != timerReferenceAggregate && !parameters.Stationary {
			t.Errorf("executable reference definition %q is not stationary", definition.ID)
		}
		seenHarnesses[harness.ID] = struct{}{}
	}

	definitions[0].Parameters.Operations[0].ID = 99
	canonical, ok := canonicalTimerReferenceDefinition(timerReferenceMissing)
	if !ok || canonical.Parameters.Operations[0].ID != 1 || !validTimerReferenceDefinition(canonical) {
		t.Fatal("reference definitions share mutable state")
	}
	for _, corrupt := range []func(*timerReferenceDefinition){
		func(value *timerReferenceDefinition) { value.Parameters.Operations[0].ID++ },
		func(value *timerReferenceDefinition) { value.ParameterSHA256 = "forged" },
		func(value *timerReferenceDefinition) { value.SemanticHarness.Timed = "different timed path" },
		func(value *timerReferenceDefinition) { value.DefinitionSHA256 = "forged" },
	} {
		invalid := canonical
		corrupt(&invalid)
		if validTimerReferenceDefinition(invalid) {
			t.Errorf("accepted corrupted reference definition %+v", invalid)
		}
	}
}

func TestTimerReferenceWorkloadPolicy(t *testing.T) {
	want := "EEEEE AAD"
	want = strings.ReplaceAll(want, " ", "")
	for _, descriptor := range timerReferenceDescriptors() {
		for index, id := range allTimerReferenceWorkloads {
			plan, err := resolveTimerReferenceWorkload(descriptor, id)
			if err != nil {
				t.Errorf("resolve reference %s/%s: %v", descriptor.ID, id, err)
				continue
			}
			if err := plan.validate(); err != nil {
				t.Errorf("validate reference %s/%s: %v", descriptor.ID, id, err)
			}
			rule := plan.rule
			switch want[index] {
			case 'E':
				if rule.Disposition != timerReferenceExecute || rule.CanonicalID != id || rule.Reason != "" {
					t.Errorf("execute reference rule %s/%s = %+v", descriptor.ID, id, rule)
				}
			case 'A':
				if rule.Disposition != timerReferenceAlias || rule.CanonicalID != timerReferenceMissing || rule.Reason != timerReferenceMissingAlias {
					t.Errorf("alias reference rule %s/%s = %+v", descriptor.ID, id, rule)
				}
			case 'D':
				if rule.Disposition != timerReferenceDiagnostic || rule.CanonicalID != id || rule.Reason != timerReferenceAggregateCheck {
					t.Errorf("diagnostic reference rule %s/%s = %+v", descriptor.ID, id, rule)
				}
			}
		}
	}
	if _, err := resolveTimerReferenceWorkload(timerReferenceDescriptor{}, timerReferenceMissing); err == nil {
		t.Fatal("reference resolver accepted empty descriptor")
	}
	if _, err := resolveTimerReferenceWorkload(timerReferenceDescriptors()[0], "invalid"); err == nil {
		t.Fatal("reference resolver accepted unknown workload")
	}
}

func TestTimerReferencePolicyRejectsCorruption(t *testing.T) {
	descriptor := timerReferenceDescriptors()[0]
	for name, corrupt := range map[string]func(*timerReferenceDescriptor){
		"driver":          func(value *timerReferenceDescriptor) { value.Driver = timerReferenceInt64 },
		"width":           func(value *timerReferenceDescriptor) { value.CounterBits = 64 },
		"source":          func(value *timerReferenceDescriptor) { value.Sources = nil },
		"source element":  func(value *timerReferenceDescriptor) { value.Sources[0].Path = "forged.go" },
		"materialization": func(value *timerReferenceDescriptor) { value.MaterializationSources = nil },
		"materialization element": func(value *timerReferenceDescriptor) {
			value.MaterializationSources[0].OriginBlob = "forged"
		},
		"materialization archive": func(value *timerReferenceDescriptor) {
			value.MaterializationArchive.PatchSHA256 = "forged"
		},
		"adaptation": func(value *timerReferenceDescriptor) { value.Adaptations = nil },
		"adaptation element": func(value *timerReferenceDescriptor) {
			value.Adaptations[0] = "forged"
		},
		"policy": func(value *timerReferenceDescriptor) { value.Policy.Rules[0].Disposition = timerReferenceDiagnostic },
	} {
		invalid := descriptor
		corrupt(&invalid)
		if _, err := resolveTimerReferenceWorkload(invalid, timerReferenceMissing); err == nil {
			t.Errorf("reference resolver accepted corrupted %s", name)
		}
	}

	descriptor = timerReferenceDescriptors()[0]
	plan, err := resolveTimerReferenceWorkload(descriptor, timerReferenceMissing)
	if err != nil {
		t.Fatal(err)
	}
	for name, corrupt := range map[string]func(*timerReferencePlan){
		"descriptor":      func(value *timerReferencePlan) { value.descriptorID = "timer.ref-core.map-swap-int64.v1" },
		"descriptor seal": func(value *timerReferencePlan) { value.descriptorSeal[0] ^= 1 },
		"driver":          func(value *timerReferencePlan) { value.driverID = timerReferenceInt64 },
		"definition":      func(value *timerReferencePlan) { value.definition.Parameters.Operations[0].ID++ },
		"rule":            func(value *timerReferencePlan) { value.rule.Disposition = timerReferenceDiagnostic },
		"seal":            func(value *timerReferencePlan) { value.seal[0] ^= 1 },
	} {
		invalid := plan
		corrupt(&invalid)
		if err := invalid.validate(); err == nil {
			t.Errorf("reference plan accepted corrupted %s", name)
		}
	}
}
