package tournament

import (
	"fmt"
	"reflect"
	"strconv"
)

type timerReferenceDisposition string

const (
	timerReferenceExecute    timerReferenceDisposition = "execute"
	timerReferenceAlias      timerReferenceDisposition = "alias"
	timerReferenceDiagnostic timerReferenceDisposition = "diagnostic"
)

type timerReferenceReason string

const (
	timerReferenceMissingAlias   timerReferenceReason = "removed-state-has-missing-timed-path"
	timerReferenceAggregateCheck timerReferenceReason = "aggregate-membership-correctness"
)

type timerReferenceRule struct {
	ID          timerReferenceWorkloadID
	Disposition timerReferenceDisposition
	CanonicalID timerReferenceWorkloadID
	Reason      timerReferenceReason
}

type timerReferencePolicy struct {
	Rules [timerReferenceWorkloadCount]timerReferenceRule
}

func newTimerReferencePolicy() timerReferencePolicy {
	var policy timerReferencePolicy
	for index, id := range allTimerReferenceWorkloads {
		rule := timerReferenceRule{ID: id, CanonicalID: id}
		switch id {
		case timerReferenceFired, timerReferenceCanceled:
			rule.Disposition = timerReferenceAlias
			rule.CanonicalID = timerReferenceMissing
			rule.Reason = timerReferenceMissingAlias
		case timerReferenceAggregate:
			rule.Disposition = timerReferenceDiagnostic
			rule.Reason = timerReferenceAggregateCheck
		default:
			rule.Disposition = timerReferenceExecute
		}
		policy.Rules[index] = rule
	}
	return policy
}

type timerReferencePlan struct {
	descriptorID   string
	descriptorSeal [32]byte
	driverID       timerReferenceDriverID
	definition     timerReferenceDefinition
	rule           timerReferenceRule
	seal           [32]byte
}

func resolveTimerReferenceWorkload(descriptor timerReferenceDescriptor, id timerReferenceWorkloadID) (timerReferencePlan, error) {
	if err := validateTimerReferenceDescriptor(descriptor); err != nil {
		return timerReferencePlan{}, err
	}
	definition, ok := canonicalTimerReferenceDefinition(id)
	if !ok {
		return timerReferencePlan{}, fmt.Errorf("unknown timer reference workload %q", id)
	}
	rule := descriptor.Policy.Rules[referenceWorkloadIndex(id)]
	plan := timerReferencePlan{
		descriptorID:   descriptor.ID,
		descriptorSeal: timerReferenceDescriptorSeal(descriptor),
		driverID:       descriptor.Driver,
		definition:     definition,
		rule:           rule,
	}
	plan.seal = plan.expectedSeal()
	return plan, nil
}

func (p timerReferencePlan) validate() error {
	descriptor, ok := timerReferenceDescriptorID(p.descriptorID)
	if !ok || descriptor.Driver != p.driverID || p.descriptorSeal != timerReferenceDescriptorSeal(descriptor) {
		return fmt.Errorf("invalid timer reference descriptor binding %q/%q", p.descriptorID, p.driverID)
	}
	if !validTimerReferenceDefinition(p.definition) {
		return fmt.Errorf("invalid timer reference definition %q", p.definition.ID)
	}
	index := referenceWorkloadIndex(p.definition.ID)
	if index < 0 || descriptor.Policy.Rules[index] != p.rule {
		return fmt.Errorf("invalid timer reference rule %q", p.definition.ID)
	}
	if p.seal != p.expectedSeal() {
		return fmt.Errorf("invalid timer reference plan seal")
	}
	return nil
}

func (p timerReferencePlan) expectedSeal() [32]byte {
	return framedTimerSeal(
		"go-utilpkg/eventloop/tournament/timer-reference-plan/v2",
		p.descriptorID,
		fmt.Sprintf("%x", p.descriptorSeal),
		string(p.driverID),
		string(p.definition.ID),
		p.definition.ParameterSHA256,
		p.definition.DefinitionSHA256,
		string(p.rule.Disposition),
		string(p.rule.CanonicalID),
		string(p.rule.Reason),
	)
}

func referenceWorkloadIndex(id timerReferenceWorkloadID) int {
	for index, candidate := range allTimerReferenceWorkloads {
		if candidate == id {
			return index
		}
	}
	return -1
}

func timerReferenceDescriptorID(id string) (timerReferenceDescriptor, bool) {
	for _, descriptor := range timerReferenceDescriptors() {
		if descriptor.ID == id {
			return descriptor, true
		}
	}
	return timerReferenceDescriptor{}, false
}

func timerReferenceDescriptorSeal(descriptor timerReferenceDescriptor) [32]byte {
	fields := []string{
		descriptor.ID,
		descriptor.AlgorithmFamily,
		descriptor.ImplementationRevision,
		descriptor.SourceStorageID,
		strconv.FormatUint(uint64(descriptor.CounterBits), 10),
		string(descriptor.Driver),
		strconv.Itoa(len(descriptor.Sources)),
	}
	for _, source := range descriptor.Sources {
		fields = append(fields, source.ProvenanceKind, source.Path, source.OriginCommit, source.BaseRevision, source.OriginBlob, source.SHA256)
	}
	fields = append(fields, strconv.Itoa(len(descriptor.MaterializationSources)))
	for _, source := range descriptor.MaterializationSources {
		fields = append(fields, source.ProvenanceKind, source.Path, source.OriginCommit, source.BaseRevision, source.OriginBlob, source.SHA256)
	}
	materializationArchive := descriptor.MaterializationArchive
	fields = append(fields,
		materializationArchive.PatchPath,
		materializationArchive.PatchSHA256,
		strconv.FormatInt(materializationArchive.PatchBytes, 10),
		materializationArchive.EmptyTree,
		materializationArchive.ReconstructedTree,
	)
	archive := descriptor.SourceArchive
	fields = append(fields,
		archive.PatchPath,
		archive.PatchSHA256,
		strconv.FormatInt(archive.PatchBytes, 10),
		archive.BaseRevision,
		archive.BaseTree,
		archive.BaseEventloopTree,
		archive.ReconstructedTree,
		archive.ReconstructedEventloopTree,
		archive.UnchangedGojaTree,
		strconv.Itoa(len(descriptor.Adaptations)),
	)
	fields = append(fields, descriptor.Adaptations...)
	fields = append(fields, strconv.Itoa(len(descriptor.Policy.Rules)))
	for _, rule := range descriptor.Policy.Rules {
		fields = append(fields, string(rule.ID), string(rule.Disposition), string(rule.CanonicalID), string(rule.Reason))
	}
	return framedTimerSeal("go-utilpkg/eventloop/tournament/timer-reference-descriptor/v1", fields...)
}

func validateTimerReferenceDescriptor(descriptor timerReferenceDescriptor) error {
	canonical, ok := timerReferenceDescriptorID(descriptor.ID)
	if !ok {
		return fmt.Errorf("unknown timer reference descriptor %q", descriptor.ID)
	}
	if !reflect.DeepEqual(descriptor, canonical) {
		return fmt.Errorf("invalid timer reference descriptor %q", descriptor.ID)
	}
	return nil
}
