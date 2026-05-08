package oracle

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	identityTimeout = 10 * time.Second

	runtimeIdentityVCSWorktree   = "vcs-worktree"
	runtimeIdentityModuleArchive = "module-archive"
	oracleCommandPackage         = "github.com/joeycumines/goja-eventloop/cmd/goja-eventloop-oracle"
	oracleModule                 = "github.com/joeycumines/goja-eventloop"
)

type fixtureInput struct {
	Surfaces []surfaceInput `json:"surfaces,omitempty"`
	Audits   []auditInput   `json:"audits,omitempty"`
}

type surfaceInput struct {
	ID                    string   `json:"id"`
	Path                  string   `json:"path"`
	Root                  string   `json:"root"`
	ValueMode             string   `json:"valueMode,omitempty"`
	Segments              []string `json:"segments"`
	CompleteFunctionShape bool     `json:"completeFunctionShape"`
}

type surfaceObservation struct {
	Surfaces           []surfaceExpectedObservation `json:"surfaces"`
	ConstructionAudits []auditExpectedObservation   `json:"constructionAudits"`
	Audits             []auditExpectedObservation   `json:"audits"`
}

type surfaceExpectedObservation struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	SurfaceExpected
}

type auditInput struct {
	ID       string   `json:"id"`
	Path     string   `json:"path"`
	Root     string   `json:"root"`
	Segments []string `json:"segments"`
}

type auditExpectedObservation struct {
	ID      string   `json:"id"`
	Path    string   `json:"path"`
	Changes []string `json:"changes"`
}

type auditGroup struct {
	Changes map[string]bool
	Input   auditInput
}

type runnerServices struct {
	runtime  func(context.Context) (RuntimeIdentity, error)
	prepare  func(string) (*nodeArtifact, error)
	identify func(context.Context, *nodeArtifact) (NodeIdentity, error)
	execute  func(context.Context, *nodeArtifact, *LoadedManifest, Fixture) CaseRecord
	close    func(*nodeArtifact) error
}

// Run executes every authenticated fixture in a fresh Node process and a
// fresh Goja runtime, writing canonical JSONL evidence to output.
func Run(ctx context.Context, manifest *LoadedManifest, nodeArchive string, output, diagnostics io.Writer) int {
	return runEvidence(ctx, manifest, nodeArchive, output, diagnostics, runnerServices{
		runtime:  runtimeIdentity,
		prepare:  prepareNodeArtifact,
		identify: resolveNode,
		execute:  runCase,
		close: func(artifact *nodeArtifact) error {
			return artifact.Close()
		},
	})
}

func runEvidence(ctx context.Context, manifest *LoadedManifest, nodeArchive string, output, diagnostics io.Writer, services runnerServices) int {
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	runtimeValue, err := services.runtime(ctx)
	if err != nil {
		fmt.Fprintf(diagnostics, "goja-eventloop-oracle: runtime identity: %v\n", err)
		_ = encoder.Encode(invalidSummary(manifest, err))
		return ExitInvalidRun
	}
	attempt := AttemptRecord{
		Type:           "attempt",
		Schema:         ProtocolSchema,
		ManifestSchema: manifest.Manifest.Schema,
		ManifestSHA256: manifest.SHA256,
		NodePin:        manifest.Manifest.Node,
		Runtime:        runtimeValue,
		Cases:          len(manifest.Manifest.Fixtures),
		Surfaces:       len(manifest.Manifest.Surfaces),
	}
	if err := encoder.Encode(attempt); err != nil {
		fmt.Fprintf(diagnostics, "goja-eventloop-oracle: write attempt evidence: %v\n", err)
		return ExitInvalidRun
	}

	artifact, err := services.prepare(nodeArchive)
	if err != nil {
		fmt.Fprintf(diagnostics, "goja-eventloop-oracle: Node artifact: %v\n", err)
		_ = encoder.Encode(invalidSummary(manifest, err))
		return ExitInvalidRun
	}
	artifactOpen := true
	closeArtifact := func() error {
		if !artifactOpen {
			return nil
		}
		artifactOpen = false
		if closeErr := services.close(artifact); closeErr != nil {
			closeErr = fmt.Errorf("close Node artifact: %w", closeErr)
			fmt.Fprintf(diagnostics, "goja-eventloop-oracle: %v\n", closeErr)
			return closeErr
		}
		return nil
	}
	defer closeArtifact()

	identityCtx, cancel := context.WithTimeout(ctx, identityTimeout)
	nodeIdentity, err := services.identify(identityCtx, artifact)
	cancel()
	if err != nil {
		fmt.Fprintf(diagnostics, "goja-eventloop-oracle: Node identity: %v\n", err)
		err = errors.Join(err, closeArtifact())
		_ = encoder.Encode(invalidSummary(manifest, err))
		return ExitInvalidRun
	}

	header := HeaderRecord{
		Type:           "header",
		Schema:         ProtocolSchema,
		ManifestSchema: manifest.Manifest.Schema,
		ManifestSHA256: manifest.SHA256,
		NodePin:        manifest.Manifest.Node,
		Node:           nodeIdentity,
		Runtime:        runtimeValue,
		Cases:          len(manifest.Manifest.Fixtures),
		Surfaces:       len(manifest.Manifest.Surfaces),
	}
	if err := encoder.Encode(header); err != nil {
		return ExitInvalidRun
	}

	summary := SummaryRecord{
		Type:     "summary",
		Status:   "pass",
		Exit:     ExitPass,
		Cases:    len(manifest.Manifest.Fixtures),
		Surfaces: len(manifest.Manifest.Surfaces),
		Classes: map[string]ClassSummary{
			string(ClassNode): {}, string(ClassWeb): {}, string(ClassExtension): {}, string(ClassBoundary): {},
		},
	}
	for _, fixture := range manifest.Manifest.Fixtures {
		record := services.execute(ctx, artifact, manifest, fixture)
		updateSummary(&summary, record)
		if err := encoder.Encode(record); err != nil {
			fmt.Fprintf(diagnostics, "goja-eventloop-oracle: write case evidence: %v\n", err)
			return ExitInvalidRun
		}
	}
	summary.Conformance = addClassSummary(summary.Classes[string(ClassNode)], summary.Classes[string(ClassWeb)])
	if summary.Invalid != 0 {
		summary.Status = "invalid"
		summary.Exit = ExitInvalidRun
	} else if summary.Mismatch != 0 {
		summary.Status = "mismatch"
		summary.Exit = ExitMismatch
	}
	if err := closeArtifact(); err != nil {
		summary.Status = "invalid"
		summary.Exit = ExitInvalidRun
		summary.Error = err.Error()
	}
	if err := encoder.Encode(summary); err != nil {
		fmt.Fprintf(diagnostics, "goja-eventloop-oracle: write summary evidence: %v\n", err)
		return ExitInvalidRun
	}
	return summary.Exit
}

func invalidSummary(manifest *LoadedManifest, err error) SummaryRecord {
	return SummaryRecord{
		Type: "summary", Status: "invalid", Exit: ExitInvalidRun,
		Cases: len(manifest.Manifest.Fixtures), Surfaces: len(manifest.Manifest.Surfaces), Error: err.Error(),
	}
}

func runCase(ctx context.Context, artifact *nodeArtifact, manifest *LoadedManifest, fixture Fixture) CaseRecord {
	record := CaseRecord{
		Type:        "case",
		ID:          fixture.ID,
		Class:       fixture.Class,
		Comparison:  comparisonContract(fixture),
		Authorities: fixture.Authorities,
		Status:      "invalid",
	}
	caseInput := inputForFixture(manifest.Manifest, fixture, true)
	caseCtx, cancel := context.WithTimeout(ctx, time.Duration(fixture.TimeoutMillis)*time.Millisecond)
	nodeObservation, err := runNodeFixture(caseCtx, artifact, manifest, fixture, caseInput)
	cancel()
	if err != nil {
		record.Error = err.Error()
		return record
	}
	record.Node = nodeObservation
	gojaObservation, err := runGojaFixture(ctx, manifest, fixture, caseInput)
	if err != nil {
		record.Error = err.Error()
		return record
	}
	record.Goja = gojaObservation

	if err := compareCase(&record, manifest.Manifest, fixture); err != nil {
		record.Error = err.Error()
	}
	return record
}

func comparisonContract(fixture Fixture) ComparisonContract {
	if fixture.Comparison == CompareNodeExact && fixture.Role == RoleSurface {
		return ComparisonContract{
			NodeExact:        []string{"/ok", "/value/surfaces"},
			ExpectedContract: []string{"/value/audits", "/value/constructionAudits"},
		}
	}
	if fixture.Comparison == CompareNodeExact {
		return ComparisonContract{NodeExact: []string{""}, ExpectedContract: []string{}}
	}
	return ComparisonContract{NodeExact: []string{}, ExpectedContract: []string{""}}
}

func compareCase(record *CaseRecord, manifest Manifest, fixture Fixture) error {
	if record == nil {
		return errors.New("compare case: record is nil")
	}
	want := record.Node
	if fixture.Comparison == CompareExpected {
		var err error
		want, err = expectedForFixture(manifest, fixture)
		if err != nil {
			return err
		}
		record.Expected = want
	}
	comparisonWant := want
	comparisonGot := record.Goja
	var auditWant, auditGot json.RawMessage
	if fixture.Comparison == CompareNodeExact && fixture.Role == RoleSurface {
		var err error
		comparisonWant, _, err = splitSurfaceAudit(record.Node)
		if err != nil {
			return err
		}
		comparisonGot, auditGot, err = splitSurfaceAudit(record.Goja)
		if err != nil {
			return err
		}
		expected, expectedErr := expectedForFixture(manifest, fixture)
		if expectedErr != nil {
			return expectedErr
		}
		_, auditWant, err = splitSurfaceAudit(expected)
		if err != nil {
			return err
		}
		record.Expected = auditWant
	}
	differences, canonicalWant, canonicalGot, err := compareJSON(comparisonWant, comparisonGot)
	if err != nil {
		return err
	}
	if fixture.Comparison == CompareNodeExact {
		if fixture.Role != RoleSurface {
			record.Node = canonicalWant
			record.Goja = canonicalGot
		}
	} else {
		record.Expected = canonicalWant
		record.Goja = canonicalGot
	}
	if len(auditWant) != 0 {
		auditDifferences, canonicalAuditWant, _, auditErr := compareJSON(auditWant, auditGot)
		if auditErr != nil {
			return auditErr
		}
		record.Expected = canonicalAuditWant
		differences = append(differences, auditDifferences...)
	}
	if len(differences) != 0 {
		record.Status = "mismatch"
		record.Differences = differences
		return nil
	}
	record.Status = "pass"
	return nil
}

func inputForFixture(manifest Manifest, fixture Fixture, includeAudits bool) fixtureInput {
	input := fixtureInput{}
	if fixture.Role != RoleSurface {
		return input
	}
	for _, surface := range manifest.Surfaces {
		if surface.Class == fixture.Class && slices.Contains(surface.Fixtures, fixture.ID) {
			input.Surfaces = append(input.Surfaces, surfaceInput{
				ID: surface.ID, Path: surface.Path, Root: surface.Root, Segments: surface.Segments,
				ValueMode: surface.ValueMode, CompleteFunctionShape: true,
			})
		}
	}
	if includeAudits {
		for _, group := range auditGroups(manifest, fixture) {
			input.Audits = append(input.Audits, group.Input)
		}
	}
	return input
}

func expectedForFixture(manifest Manifest, fixture Fixture) (json.RawMessage, error) {
	var value any
	if fixture.Role == RoleSurface {
		observation := surfaceObservation{}
		for _, surface := range manifest.Surfaces {
			if surface.Class == fixture.Class && slices.Contains(surface.Fixtures, fixture.ID) {
				observation.Surfaces = append(observation.Surfaces, surfaceExpectedObservation{
					ID: surface.ID, Path: surface.Path, SurfaceExpected: surface.Expected,
				})
			}
		}
		for _, group := range auditGroups(manifest, fixture) {
			changes := make([]string, 0, len(group.Changes))
			for change := range group.Changes {
				changes = append(changes, change)
			}
			slices.Sort(changes)
			observation.Audits = append(observation.Audits, auditExpectedObservation{
				ID: group.Input.ID, Path: group.Input.Path, Changes: changes,
			})
			observation.ConstructionAudits = append(observation.ConstructionAudits, auditExpectedObservation{
				ID: group.Input.ID, Path: group.Input.Path, Changes: []string{},
			})
		}
		value = observation
	} else {
		if err := json.Unmarshal(fixture.Expected, &value); err != nil {
			return nil, fmt.Errorf("decode expected observation: %w", err)
		}
	}
	wrapper := map[string]any{"ok": true, "value": value}
	data, err := json.Marshal(wrapper)
	if err != nil {
		return nil, fmt.Errorf("encode expected observation: %w", err)
	}
	canonical, _, err := canonicalJSON(data)
	return canonical, err
}

func auditGroups(manifest Manifest, fixture Fixture) []auditGroup {
	groups := make([]auditGroup, 0)
	indexes := make(map[string]int)
	preexisting := make(map[string]bool)
	preservedSubtree := make(map[string]bool)
	for _, name := range fixture.Setup.Globals {
		preexisting[name] = true
		preservedSubtree[name] = true
	}
	for _, member := range fixture.Setup.Members {
		preexisting[member.Object] = true
	}
	for _, pair := range fixture.Setup.BrandPairs {
		preexisting[pair.Constructor] = true
		preservedSubtree[pair.Constructor] = true
		preexisting[pair.Singleton] = true
		preservedSubtree[pair.Singleton] = true
	}
	for _, surface := range manifest.Surfaces {
		// Handle/Event instances are created after Bind. They are observable
		// surface roots, not stable owners in the Bind mutation journal.
		if surface.Root == "timeoutInstance" || surface.Root == "immediateInstance" || surface.Root == "eventInstance" {
			continue
		}
		root, owner, installedOwner := surfaceAuditOwner(surface)
		key := root + "\x00" + strings.Join(owner, "\x00")
		index, ok := indexes[key]
		if !ok {
			path := root
			if len(owner) != 0 {
				path += "." + strings.Join(owner, ".")
			}
			index = len(groups)
			indexes[key] = index
			groups = append(groups, auditGroup{
				Input:   auditInput{ID: fmt.Sprintf("audit-%03d", index+1), Path: path, Root: root, Segments: slices.Clone(owner)},
				Changes: make(map[string]bool),
			})
		}
		changed := (surface.Mode == "install" || surface.Mode == "augment") &&
			surface.Expected.Descriptor != nil &&
			(surface.Expected.Descriptor.Depth == 0 || installedOwner)
		if surface.Segments[len(surface.Segments)-1] == "constructor" || surface.Segments[len(surface.Segments)-1] == "prototype" {
			changed = false
		}
		if surface.Root == "global" && len(surface.Segments) != 0 {
			if preservedSubtree[surface.Segments[0]] ||
				(len(owner) == 0 && preexisting[surface.Segments[0]] && surface.Segments[0] != "process") {
				changed = false
			}
		}
		if changed {
			change := surface.Segments[len(surface.Segments)-1]
			switch change {
			case "@@shapeMode":
				change = "Symbol(shapeMode)"
			case "@@kCapture":
				change = "Symbol(kCapture)"
			}
			groups[index].Changes[change] = true
		}
	}
	return groups
}

func surfaceAuditOwner(surface Surface) (root string, owner []string, installedOwner bool) {
	root = surface.Root
	owner = surface.Segments[:len(surface.Segments)-1]
	if surface.Root != "global" || len(surface.Segments) != 2 ||
		surface.Segments[0] != "process" || surface.Expected.Descriptor == nil {
		return root, owner, false
	}
	switch surface.Expected.Descriptor.Depth {
	case 1:
		return "processPrototype", []string{}, true
	case 2:
		return "processEmitterPrototype", []string{}, true
	default:
		return root, owner, false
	}
}

func splitSurfaceAudit(data []byte) (withoutAudit, audit json.RawMessage, err error) {
	canonical, value, err := canonicalJSON(data)
	if err != nil {
		return nil, nil, fmt.Errorf("surface observation: %w", err)
	}
	_ = canonical
	wrapper, ok := value.(map[string]any)
	if !ok {
		return nil, nil, errors.New("surface observation wrapper is not an object")
	}
	valueObject, ok := wrapper["value"].(map[string]any)
	if !ok {
		return nil, nil, errors.New("surface observation value is not an object")
	}
	auditValue, ok := valueObject["audits"]
	if !ok {
		return nil, nil, errors.New("surface observation has no audits")
	}
	constructionAuditValue, ok := valueObject["constructionAudits"]
	if !ok {
		return nil, nil, errors.New("surface observation has no construction audits")
	}
	delete(valueObject, "audits")
	delete(valueObject, "constructionAudits")
	withoutData, marshalErr := json.Marshal(wrapper)
	if marshalErr != nil {
		return nil, nil, marshalErr
	}
	auditData, marshalErr := json.Marshal(map[string]any{"value": map[string]any{"audits": auditValue, "constructionAudits": constructionAuditValue}})
	if marshalErr != nil {
		return nil, nil, marshalErr
	}
	withoutAudit, _, err = canonicalJSON(withoutData)
	if err != nil {
		return nil, nil, err
	}
	audit, _, err = canonicalJSON(auditData)
	return withoutAudit, audit, err
}

func updateSummary(summary *SummaryRecord, record CaseRecord) {
	class := summary.Classes[string(record.Class)]
	class.Total++
	switch record.Status {
	case "pass":
		summary.Passed++
		class.Passed++
	case "mismatch":
		summary.Mismatch++
		class.Mismatch++
	default:
		summary.Invalid++
		class.Invalid++
	}
	summary.Classes[string(record.Class)] = class
}

func addClassSummary(left, right ClassSummary) ClassSummary {
	return ClassSummary{
		Total: left.Total + right.Total, Passed: left.Passed + right.Passed,
		Mismatch: left.Mismatch + right.Mismatch, Invalid: left.Invalid + right.Invalid,
	}
}

func runtimeIdentity(ctx context.Context) (RuntimeIdentity, error) {
	executable, err := os.Executable()
	if err != nil {
		return RuntimeIdentity{}, fmt.Errorf("resolve oracle executable: %w", err)
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return RuntimeIdentity{}, errors.New("resolve oracle source path")
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return RuntimeIdentity{}, errors.New("read oracle build identity")
	}
	return buildRuntimeIdentity(ctx, executable, source, info)
}

func buildRuntimeIdentity(ctx context.Context, executable, source string, info *debug.BuildInfo) (RuntimeIdentity, error) {
	identity := RuntimeIdentity{GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	if info == nil || info.Path == "" || info.Main.Path == "" || info.Main.Version == "" {
		return identity, errors.New("oracle main module identity is incomplete")
	}
	if info.Path != oracleCommandPackage {
		return identity, fmt.Errorf("oracle package is %q, want %q", info.Path, oracleCommandPackage)
	}
	if info.Main.Path != oracleModule {
		return identity, fmt.Errorf("oracle main module is %q, want %q", info.Main.Path, oracleModule)
	}
	executableSHA256, err := regularFileSHA256(executable)
	if err != nil {
		return identity, fmt.Errorf("hash oracle executable: %w", err)
	}
	identity.Package = info.Path
	identity.ExecutableSHA256 = executableSHA256
	identity.Module = info.Main.Path + "@" + info.Main.Version

	goja, err := runtimeDependency(info, "github.com/joeycumines/goja")
	if err != nil {
		return identity, err
	}
	eventloop, err := runtimeDependency(info, "github.com/joeycumines/go-eventloop")
	if err != nil {
		return identity, err
	}

	switch {
	case info.Main.Version == "(devel)" && info.Main.Sum == "":
		identity.IdentityMode = runtimeIdentityVCSWorktree
	case info.Main.Version == "(devel)":
		return identity, errors.New("development oracle main module unexpectedly has an archive sum")
	case info.Main.Sum == "":
		return identity, errors.New("versioned oracle main module has no archive sum")
	default:
		identity.IdentityMode = runtimeIdentityModuleArchive
		identity.ModuleSum = info.Main.Sum
		if err := validateModuleArchive(info.Main.Path, info.Main.Version, info.Main.Sum, nil); err != nil {
			return identity, err
		}
		if err := validateModuleArchive(goja.Path, goja.Version, goja.Sum, goja.Replace); err != nil {
			return identity, err
		}
		if err := validateModuleArchive(eventloop.Path, eventloop.Version, eventloop.Sum, eventloop.Replace); err != nil {
			return identity, err
		}
		identity.GojaVersion = goja.Version
		identity.GojaSum = goja.Sum
		identity.EventloopVersion = eventloop.Version
		identity.EventloopSum = eventloop.Sum
		return identity, nil
	}

	settings := make(map[string]string, len(info.Settings))
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	identity.VCS = settings["vcs"]
	identity.VCSRevision = settings["vcs.revision"]
	dirtyValue, dirtyOK := settings["vcs.modified"]
	if identity.VCS == "" || identity.VCSRevision == "" || !dirtyOK {
		return identity, errors.New("oracle VCS revision or dirty state is unavailable")
	}
	identity.VCSDirty, err = strconv.ParseBool(dirtyValue)
	if err != nil {
		return identity, fmt.Errorf("parse oracle VCS dirty state %q: %w", dirtyValue, err)
	}

	identity.GojaVersion, identity.GojaSum = moduleVersion(goja)
	if identity.GojaVersion == "" {
		return identity, errors.New("goja module identity is unavailable")
	}
	identity.EventloopVersion = eventloop.Version
	identity.EventloopSum = eventloop.Sum
	if identity.EventloopVersion == "" {
		return identity, errors.New("eventloop module identity is unavailable")
	}
	if eventloop.Replace == nil {
		return identity, nil
	}
	identity.EventloopReplacement = eventloop.Replace.Path
	if !localReplacement(eventloop.Replace.Path) {
		if eventloop.Replace.Version == "" {
			return identity, fmt.Errorf("eventloop replacement %q has neither a version nor a local path", eventloop.Replace.Path)
		}
		identity.EventloopVersion, identity.EventloopSum = moduleVersion(eventloop)
		return identity, nil
	}
	mainRoot, err := sourceModuleRoot(source, info.Main.Path)
	if err != nil {
		return identity, err
	}
	candidateRoot := filepath.FromSlash(eventloop.Replace.Path)
	if !filepath.IsAbs(candidateRoot) {
		candidateRoot = filepath.Join(mainRoot, candidateRoot)
	}
	candidateRoot, err = filepath.Abs(candidateRoot)
	if err != nil {
		return identity, fmt.Errorf("resolve eventloop candidate root: %w", err)
	}
	modulePath, err := readModulePath(filepath.Join(candidateRoot, "go.mod"))
	if err != nil {
		return identity, fmt.Errorf("read eventloop candidate module: %w", err)
	}
	if modulePath != eventloop.Path {
		return identity, fmt.Errorf("eventloop candidate module is %q, want %q", modulePath, eventloop.Path)
	}
	identity.EventloopCandidateFormat = eventloopCandidateFormat
	identity.EventloopCandidateSHA256, identity.EventloopCandidateRecords, err = candidateModuleSHA256(ctx, candidateRoot)
	if err != nil {
		return identity, fmt.Errorf("hash eventloop candidate: %w", err)
	}
	return identity, nil
}

func runtimeDependency(info *debug.BuildInfo, path string) (*debug.Module, error) {
	var result *debug.Module
	for _, dependency := range info.Deps {
		if dependency.Path != path {
			continue
		}
		if result != nil {
			return nil, fmt.Errorf("duplicate %s module identity", path)
		}
		result = dependency
	}
	if result == nil {
		return nil, fmt.Errorf("%s module identity is unavailable", path)
	}
	return result, nil
}

func validateModuleArchive(path, version, sum string, replacement *debug.Module) error {
	if replacement != nil {
		return fmt.Errorf("archive module %s has a replacement", path)
	}
	if version == "" || version == "(devel)" {
		return fmt.Errorf("archive module %s has invalid version %q", path, version)
	}
	if !strings.HasPrefix(sum, "h1:") {
		return fmt.Errorf("archive module %s has invalid content sum %q", path, sum)
	}
	digest, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sum, "h1:"))
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("archive module %s has invalid content sum %q", path, sum)
	}
	return nil
}

func moduleVersion(module *debug.Module) (string, string) {
	if module.Replace != nil && module.Replace.Version != "" {
		return module.Replace.Path + "@" + module.Replace.Version, module.Replace.Sum
	}
	return module.Version, module.Sum
}

func localReplacement(path string) bool {
	path = filepath.FromSlash(path)
	separator := string(filepath.Separator)
	return filepath.IsAbs(path) || filepath.VolumeName(path) != "" || path == "." || path == ".." ||
		strings.HasPrefix(path, "."+separator) || strings.HasPrefix(path, ".."+separator)
}

func sourceModuleRoot(source, modulePath string) (string, error) {
	if !filepath.IsAbs(source) {
		return "", fmt.Errorf("oracle source path %q is not absolute", source)
	}
	directory := filepath.Dir(source)
	for {
		path := filepath.Join(directory, "go.mod")
		value, err := readModulePath(path)
		switch {
		case err == nil:
			if value != modulePath {
				return "", fmt.Errorf("oracle source module is %q, want %q", value, modulePath)
			}
			return directory, nil
		case !errors.Is(err, os.ErrNotExist):
			return "", fmt.Errorf("read oracle source module: %w", err)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("oracle source module %q is unavailable", modulePath)
		}
		directory = parent
	}
}

func readModulePath(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != "module" {
			continue
		}
		value := fields[1]
		if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "`") {
			value, err = strconv.Unquote(value)
			if err != nil {
				return "", fmt.Errorf("decode module path: %w", err)
			}
		}
		if value == "" {
			break
		}
		return value, nil
	}
	return "", errors.New("module directive is unavailable")
}

func regularFileSHA256(path string) (_ string, resultErr error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		resultErr = errors.Join(resultErr, file.Close())
	}()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
