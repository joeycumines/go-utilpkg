package oracle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const maxManifestBytes = 4 << 20

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)

var requiredAuthorityCatalog = map[string]Authority{
	"node-v26.5.0":                {ID: "node-v26.5.0", Kind: "node", Revision: NodeSourceCommit, Locator: "https://github.com/nodejs/node/tree/" + NodeSourceCommit},
	"whatwg-dom":                  {ID: "whatwg-dom", Kind: "web", Revision: "8a5f57c61ca1de8dc21b7e114501b1b57882e935", Locator: "https://github.com/whatwg/dom/tree/8a5f57c61ca1de8dc21b7e114501b1b57882e935"},
	"whatwg-html":                 {ID: "whatwg-html", Kind: "web", Revision: "24c5e48bf66ea61bc199ec6338c81258275ba9c6", Locator: "https://github.com/whatwg/html/tree/24c5e48bf66ea61bc199ec6338c81258275ba9c6"},
	"whatwg-webidl":               {ID: "whatwg-webidl", Kind: "web", Revision: "fad9b4ce284fd034b719c1c8576e1c692bc97de3", Locator: "https://github.com/whatwg/webidl/tree/fad9b4ce284fd034b719c1c8576e1c692bc97de3"},
	"w3c-webcrypto":               {ID: "w3c-webcrypto", Kind: "web", Revision: "851575b9f580623fbdbeca4ad411b90ecbc68776", Locator: "https://github.com/w3c/webcrypto/tree/851575b9f580623fbdbeca4ad411b90ecbc68776"},
	"w3c-high-resolution-time":    {ID: "w3c-high-resolution-time", Kind: "web", Revision: "0649f538dd7caa55f7a6f3d25753ef852c34e863", Locator: "https://github.com/w3c/hr-time/tree/0649f538dd7caa55f7a6f3d25753ef852c34e863"},
	"whatwg-console":              {ID: "whatwg-console", Kind: "web", Revision: "a82403e842252f34975f84091ee694aef86dfd37", Locator: "https://github.com/whatwg/console/tree/a82403e842252f34975f84091ee694aef86dfd37"},
	"goja-eventloop-extension-v1": {ID: "goja-eventloop-extension-v1", Kind: "package", Revision: "v1", Locator: "urn:goja-eventloop:extension:v1"},
	"goja-eventloop-boundary-v1":  {ID: "goja-eventloop-boundary-v1", Kind: "package", Revision: "v1", Locator: "urn:goja-eventloop:boundary:v1"},
}

type fixtureContract struct {
	File       string
	Class      ProfileClass
	Comparison Comparison
	Role       FixtureRole
}

var requiredFixtureCatalog = map[string]fixtureContract{
	"surface-node":                  {"fixtures/surface-node.js", ClassNode, CompareNodeExact, RoleSurface},
	"surface-web":                   {"fixtures/surface-web.js", ClassWeb, CompareExpected, RoleSurface},
	"surface-extension":             {"fixtures/surface-extension.js", ClassExtension, CompareExpected, RoleSurface},
	"surface-boundary":              {"fixtures/surface-boundary.js", ClassBoundary, CompareExpected, RoleSurface},
	"surface-boundary-preservation": {"fixtures/surface-boundary-preservation.js", ClassBoundary, CompareExpected, RoleSurface},
	"node-timers":                   {"fixtures/node-timers.js", ClassNode, CompareNodeExact, RoleSemantic},
	"node-ordering":                 {"fixtures/node-ordering.js", ClassNode, CompareNodeExact, RoleSemantic},
	"node-process":                  {"fixtures/node-process.js", ClassNode, CompareNodeExact, RoleSemantic},
	"node-promises":                 {"fixtures/node-promises.js", ClassNode, CompareNodeExact, RoleSemantic},
	"web-events":                    {"fixtures/web-events.js", ClassWeb, CompareExpected, RoleSemantic},
	"web-abort":                     {"fixtures/web-abort.js", ClassWeb, CompareExpected, RoleSemantic},
	"web-domexception":              {"fixtures/web-domexception.js", ClassWeb, CompareExpected, RoleSemantic},
	"web-structured-clone":          {"fixtures/web-structured-clone.js", ClassWeb, CompareExpected, RoleSemantic},
	"web-base64":                    {"fixtures/web-base64.js", ClassWeb, CompareExpected, RoleSemantic},
	"web-performance":               {"fixtures/web-performance.js", ClassWeb, CompareExpected, RoleSemantic},
	"web-crypto":                    {"fixtures/web-crypto.js", ClassWeb, CompareExpected, RoleSemantic},
	"extension-delay":               {"fixtures/extension-delay.js", ClassExtension, CompareExpected, RoleSemantic},
	"extension-console":             {"fixtures/extension-console.js", ClassExtension, CompareExpected, RoleSemantic},
	"boundary-absence":              {"fixtures/boundary-absence.js", ClassBoundary, CompareExpected, RoleSemantic},
	"boundary-preservation":         {"fixtures/boundary-preservation.js", ClassBoundary, CompareExpected, RoleSemantic},
}

const requiredSurfaceCount = 286

const requiredSurfaceCatalogSHA256 = "565ab99e54b0668f1219ff94d2eca6e1f49671af083e4b7d7881b1aef672e558"

const requiredManifestContractSHA256 = "9c903f12b48c2d1080b0890f1030a209b3b9f98c57a9cec0ab6c842923c92109"

// LoadManifest reads, authenticates, and validates a complete oracle manifest.
func LoadManifest(path string) (*LoadedManifest, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("oracle manifest path: %w", err)
	}
	data, err := readBounded(absPath, maxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("oracle manifest: %w", err)
	}
	if err := rejectDuplicateKeys(data); err != nil {
		return nil, fmt.Errorf("oracle manifest: %w", err)
	}
	var manifest Manifest
	if err := decodeStrict(data, &manifest); err != nil {
		return nil, fmt.Errorf("oracle manifest: %w", err)
	}
	if err := validateManifest(&manifest); err != nil {
		return nil, fmt.Errorf("oracle manifest: %w", err)
	}

	root := filepath.Dir(absPath)
	harness, err := loadAsset(root, manifest.Harness)
	if err != nil {
		return nil, fmt.Errorf("oracle harness: %w", err)
	}
	fixtures := make(map[string][]byte, len(manifest.Fixtures))
	for _, fixture := range manifest.Fixtures {
		data, loadErr := loadAsset(root, Asset{File: fixture.File, SHA256: fixture.SHA256})
		if loadErr != nil {
			return nil, fmt.Errorf("oracle fixture %q: %w", fixture.ID, loadErr)
		}
		fixtures[fixture.ID] = data
	}

	manifestSum := sha256.Sum256(data)
	return &LoadedManifest{
		Manifest: manifest,
		Path:     absPath,
		Root:     root,
		SHA256:   hex.EncodeToString(manifestSum[:]),
		Harness:  harness,
		Fixtures: fixtures,
	}, nil
}

func validateManifest(manifest *Manifest) error {
	if manifest.Schema != ManifestSchema {
		return fmt.Errorf("schema is %q, want %q", manifest.Schema, ManifestSchema)
	}
	if manifest.Profile != "goja-eventloop-v26.5.0-retained" {
		return fmt.Errorf("profile is %q, want goja-eventloop-v26.5.0-retained", manifest.Profile)
	}
	if manifest.Node.Version != NodeVersion || manifest.Node.Tag != NodeTag ||
		manifest.Node.SourceCommit != NodeSourceCommit || manifest.Node.ReleaseURL != NodeReleaseURL ||
		!slices.Equal(manifest.Node.Artifacts, nodeArtifactPins) {
		return errors.New("node pin does not match the exact v26.5.0 release authority")
	}
	if err := validateAsset(manifest.Harness); err != nil {
		return fmt.Errorf("harness: %w", err)
	}

	authorities := make(map[string]Authority, len(manifest.Authorities))
	usedAuthorities := make(map[string]bool, len(manifest.Authorities))
	for index, authority := range manifest.Authorities {
		if err := validateAuthority(authority); err != nil {
			return fmt.Errorf("authority[%d]: %w", index, err)
		}
		if _, exists := authorities[authority.ID]; exists {
			return fmt.Errorf("duplicate authority %q", authority.ID)
		}
		authorities[authority.ID] = authority
	}
	if len(authorities) != len(requiredAuthorityCatalog) {
		return fmt.Errorf("authority catalog has %d entries, want %d", len(authorities), len(requiredAuthorityCatalog))
	}
	for id, expected := range requiredAuthorityCatalog {
		authority, ok := authorities[id]
		if !ok {
			return fmt.Errorf("required authority %q is absent", id)
		}
		if authority != expected {
			return fmt.Errorf("authority %q is %+v, want exact pinned catalog entry %+v", id, authority, expected)
		}
	}

	fixtures := make(map[string]Fixture, len(manifest.Fixtures))
	usedFixtures := make(map[string]bool, len(manifest.Fixtures))
	for index, fixture := range manifest.Fixtures {
		if err := validateFixture(fixture, authorities, usedAuthorities); err != nil {
			return fmt.Errorf("fixture[%d]: %w", index, err)
		}
		if _, exists := fixtures[fixture.ID]; exists {
			return fmt.Errorf("duplicate fixture %q", fixture.ID)
		}
		fixtures[fixture.ID] = fixture
	}
	if len(fixtures) != len(requiredFixtureCatalog) {
		return fmt.Errorf("fixture catalog has %d entries, want %d", len(fixtures), len(requiredFixtureCatalog))
	}
	for id, expected := range requiredFixtureCatalog {
		fixture, ok := fixtures[id]
		if !ok {
			return fmt.Errorf("required fixture %q is absent", id)
		}
		got := fixtureContract{File: fixture.File, Class: fixture.Class, Comparison: fixture.Comparison, Role: fixture.Role}
		if got != expected || fixture.TimeoutMillis != 3000 {
			return fmt.Errorf("fixture %q contract is %+v/%dms, want %+v/3000ms", id, got, fixture.TimeoutMillis, expected)
		}
	}

	if len(manifest.Surfaces) != requiredSurfaceCount {
		return fmt.Errorf("surface catalog has %d rows, want exact finite catalog size %d", len(manifest.Surfaces), requiredSurfaceCount)
	}
	surfaceIDs := make(map[string]bool, len(manifest.Surfaces))
	surfaceCoordinates := make(map[string]string, len(manifest.Surfaces))
	classCounts := make(map[ProfileClass]int, 4)
	for index, surface := range manifest.Surfaces {
		if err := validateSurface(surface, fixtures, authorities, usedFixtures, usedAuthorities); err != nil {
			return fmt.Errorf("surface[%d]: %w", index, err)
		}
		if surfaceIDs[surface.ID] {
			return fmt.Errorf("duplicate surface %q", surface.ID)
		}
		surfaceIDs[surface.ID] = true
		for _, fixture := range surface.Fixtures {
			coordinate := fixture + "\x00" + surface.Root + "\x00" + strings.Join(surface.Segments, "\x00")
			if previous, exists := surfaceCoordinates[coordinate]; exists {
				return fmt.Errorf("surface %q duplicates fixture execution coordinate from %q", surface.ID, previous)
			}
			surfaceCoordinates[coordinate] = surface.ID
		}
		classCounts[surface.Class]++
	}
	catalogSHA256, err := surfaceCatalogSHA256(manifest.Surfaces)
	if err != nil {
		return err
	}
	if catalogSHA256 != requiredSurfaceCatalogSHA256 {
		return fmt.Errorf("surface catalog SHA-256 is %s, want %s", catalogSHA256, requiredSurfaceCatalogSHA256)
	}
	for _, class := range []ProfileClass{ClassNode, ClassWeb, ClassExtension, ClassBoundary} {
		if classCounts[class] == 0 {
			return fmt.Errorf("surface class %q is empty", class)
		}
	}
	for id := range fixtures {
		if !usedFixtures[id] {
			return fmt.Errorf("fixture %q is not referenced by a surface", id)
		}
	}
	for id := range authorities {
		if !usedAuthorities[id] {
			return fmt.Errorf("authority %q is not referenced", id)
		}
	}
	contractSHA256, err := manifestContractSHA256(*manifest)
	if err != nil {
		return err
	}
	if contractSHA256 != requiredManifestContractSHA256 {
		return fmt.Errorf("manifest contract SHA-256 is %s, want %s", contractSHA256, requiredManifestContractSHA256)
	}
	return nil
}

func validateAuthority(authority Authority) error {
	if !identifierPattern.MatchString(authority.ID) {
		return fmt.Errorf("invalid id %q", authority.ID)
	}
	if authority.Kind != "node" && authority.Kind != "web" && authority.Kind != "package" {
		return fmt.Errorf("invalid kind %q", authority.Kind)
	}
	if authority.Revision == "" {
		return errors.New("revision is empty")
	}
	parsed, err := url.Parse(authority.Locator)
	if err != nil || parsed.Scheme == "" {
		return fmt.Errorf("invalid locator %q", authority.Locator)
	}
	if authority.Kind != "package" && parsed.Scheme != "https" {
		return fmt.Errorf("authority locator %q must use https", authority.Locator)
	}
	if authority.Kind != "package" {
		if len(authority.Revision) != 40 || !lowerHex(authority.Revision) {
			return fmt.Errorf("revision %q is not a lowercase Git commit", authority.Revision)
		}
		if !strings.Contains(authority.Locator, authority.Revision) {
			return errors.New("locator does not contain its pinned revision")
		}
	}
	return nil
}

func validateFixture(fixture Fixture, authorities map[string]Authority, used map[string]bool) error {
	if !identifierPattern.MatchString(fixture.ID) {
		return fmt.Errorf("invalid id %q", fixture.ID)
	}
	if err := validateAsset(Asset{File: fixture.File, SHA256: fixture.SHA256}); err != nil {
		return err
	}
	if filepath.Ext(fixture.File) != ".js" {
		return errors.New("fixture file must end in .js")
	}
	if !validClass(fixture.Class) {
		return fmt.Errorf("invalid class %q", fixture.Class)
	}
	wantComparison := CompareExpected
	if fixture.Class == ClassNode {
		wantComparison = CompareNodeExact
	}
	if fixture.Comparison != wantComparison {
		return fmt.Errorf("comparison is %q, want %q", fixture.Comparison, wantComparison)
	}
	if fixture.Role != RoleSurface && fixture.Role != RoleSemantic {
		return fmt.Errorf("invalid role %q", fixture.Role)
	}
	if fixture.TimeoutMillis < 100 || fixture.TimeoutMillis > 30_000 {
		return fmt.Errorf("timeoutMillis %d is outside [100,30000]", fixture.TimeoutMillis)
	}
	if err := validateReferences("authority", fixture.Authorities, authorities, used); err != nil {
		return err
	}
	if err := validateAuthorityClass(fixture.Class, fixture.Authorities); err != nil {
		return err
	}
	if err := validateSetup(fixture.Setup); err != nil {
		return err
	}
	if fixture.Role == RoleSurface {
		if len(fixture.Expected) != 0 {
			return errors.New("surface fixture must derive expectations from surface rows")
		}
	} else if fixture.Comparison == CompareExpected {
		if len(fixture.Expected) == 0 {
			return errors.New("expected-comparison fixture has no expected observation")
		}
		if err := validateRawJSON(fixture.Expected); err != nil {
			return fmt.Errorf("expected: %w", err)
		}
	} else if len(fixture.Expected) != 0 {
		return errors.New("node-exact fixture must not declare expected output")
	}
	return nil
}

func validateSurface(surface Surface, fixtures map[string]Fixture, authorities map[string]Authority, usedFixtures, usedAuthorities map[string]bool) error {
	if !identifierPattern.MatchString(surface.ID) {
		return fmt.Errorf("invalid id %q", surface.ID)
	}
	if surface.Path == "" || strings.ContainsAny(surface.Path, "\r\n\t") {
		return errors.New("path is empty or contains control whitespace")
	}
	if !slices.Contains([]string{
		"global", "processPrototype", "processEmitterPrototype",
		"timeoutPrototype", "immediatePrototype", "timeoutInstance",
		"immediateInstance", "eventInstance",
	}, surface.Root) {
		return fmt.Errorf("invalid root %q", surface.Root)
	}
	if len(surface.Segments) == 0 {
		return errors.New("segments are empty")
	}
	for _, segment := range surface.Segments {
		if segment == "" || strings.ContainsAny(segment, "\x00\r\n") {
			return fmt.Errorf("invalid path segment %q", segment)
		}
	}
	if expectedPath := surfaceDisplayPath(surface.Root, surface.Segments); surface.Path != expectedPath {
		return fmt.Errorf("path %q does not match executable coordinate %q", surface.Path, expectedPath)
	}
	if !validClass(surface.Class) {
		return fmt.Errorf("invalid class %q", surface.Class)
	}
	if !slices.Contains([]string{"install", "augment", "preserve", "absent"}, surface.Mode) {
		return fmt.Errorf("invalid mode %q", surface.Mode)
	}
	if (surface.Mode == "absent") != !surface.Expected.Exists {
		return errors.New("absent mode and expected.exists disagree")
	}
	if err := validateSurfaceExpected(surface.Expected, surface.ValueMode); err != nil {
		return err
	}
	if err := validateReferences("authority", surface.Authorities, authorities, usedAuthorities); err != nil {
		return err
	}
	if err := validateAuthorityClass(surface.Class, surface.Authorities); err != nil {
		return err
	}
	if len(surface.Fixtures) == 0 {
		return errors.New("fixtures are empty")
	}
	hasSurfaceFixture := false
	seen := make(map[string]bool, len(surface.Fixtures))
	for _, id := range surface.Fixtures {
		fixture, ok := fixtures[id]
		if !ok {
			return fmt.Errorf("unknown fixture %q", id)
		}
		if seen[id] {
			return fmt.Errorf("duplicate fixture reference %q", id)
		}
		seen[id] = true
		if fixture.Class != surface.Class {
			return fmt.Errorf("fixture %q class %q differs from surface class %q", id, fixture.Class, surface.Class)
		}
		if fixture.Role == RoleSurface {
			hasSurfaceFixture = true
		}
		usedFixtures[id] = true
	}
	if !hasSurfaceFixture {
		return errors.New("surface does not reference a surface-role fixture")
	}
	return nil
}

func surfaceDisplayPath(root string, segments []string) string {
	prefix := ""
	switch root {
	case "processPrototype":
		prefix = "process prototype"
	case "processEmitterPrototype":
		prefix = "EventEmitter.prototype"
	case "timeoutPrototype":
		prefix = "Timeout.prototype"
	case "immediatePrototype":
		prefix = "Immediate.prototype"
	case "timeoutInstance":
		prefix = "Timeout instance"
	case "immediateInstance":
		prefix = "Immediate instance"
	case "eventInstance":
		prefix = "Event instance"
	}
	path := strings.Join(segments, ".")
	if prefix == "" {
		return path
	}
	if path == "" {
		return prefix
	}
	return prefix + "." + path
}

func surfaceCatalogSHA256(surfaces []Surface) (string, error) {
	data, err := json.Marshal(surfaces)
	if err != nil {
		return "", fmt.Errorf("encode surface catalog: %w", err)
	}
	canonical, _, err := canonicalJSON(data)
	if err != nil {
		return "", fmt.Errorf("canonicalize surface catalog: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func manifestContractSHA256(manifest Manifest) (string, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("encode manifest contract: %w", err)
	}
	canonical, _, err := canonicalJSON(data)
	if err != nil {
		return "", fmt.Errorf("canonicalize manifest contract: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func validateSurfaceExpected(expected SurfaceExpected, valueMode string) error {
	if !slices.Contains([]string{"", "exact", "type"}, valueMode) {
		return fmt.Errorf("invalid valueMode %q", valueMode)
	}
	if valueMode == "type" && !expected.Exists {
		return errors.New("absent surface uses type-only value comparison")
	}
	if !expected.Exists {
		if expected.Kind != "" || len(expected.Value) != 0 || expected.Descriptor != nil || expected.Function != nil || expected.Prototype != "" || expected.Sentinel != "" {
			return errors.New("absent surface declares present-only expectations")
		}
		return nil
	}
	if !slices.Contains([]string{"function", "object", "number", "string", "boolean", "symbol", "undefined", "null", "accessor"}, expected.Kind) {
		return fmt.Errorf("invalid expected kind %q", expected.Kind)
	}
	if expected.Descriptor == nil {
		return errors.New("present surface has no descriptor expectation")
	}
	if expected.Descriptor.Depth < 0 || expected.Descriptor.Depth > 8 {
		return errors.New("descriptor depth is outside [0,8]")
	}
	data := expected.Descriptor.Writable != nil
	accessor := expected.Descriptor.Getter != nil || expected.Descriptor.Setter != nil
	if data == accessor {
		return errors.New("descriptor must be exactly one of data or accessor")
	}
	if expected.Kind == "function" {
		if expected.Function == nil {
			return errors.New("function surface has no function expectation")
		}
	} else if expected.Function != nil {
		return errors.New("non-function surface declares function expectation")
	}
	if expected.Function != nil && expected.Function.Length < 0 {
		return errors.New("function length is negative")
	}
	if expected.Function != nil {
		if err := validateFunctionExpectation("function", expected.Function); err != nil {
			return err
		}
	}
	if expected.Descriptor.Getter != nil {
		if err := validateFunctionExpectation("getter", expected.Descriptor.Getter); err != nil {
			return err
		}
	}
	if expected.Descriptor.Setter != nil {
		if err := validateFunctionExpectation("setter", expected.Descriptor.Setter); err != nil {
			return err
		}
	}
	primitive := slices.Contains([]string{"number", "string", "boolean"}, expected.Kind)
	if valueMode == "type" && !primitive {
		return errors.New("type-only value comparison requires a number, string, or boolean surface")
	}
	if primitive && valueMode != "type" && len(expected.Value) == 0 {
		return errors.New("primitive surface has no value expectation")
	}
	if valueMode == "type" && len(expected.Value) != 0 {
		return errors.New("type-only value comparison declares a value expectation")
	}
	if !primitive && len(expected.Value) != 0 {
		return errors.New("non-primitive surface declares a value expectation")
	}
	if len(expected.Value) != 0 {
		if err := validateRawJSON(expected.Value); err != nil {
			return fmt.Errorf("value: %w", err)
		}
	}
	if expected.Prototype == "" && expected.Kind != "accessor" && expected.Kind != "number" && expected.Kind != "string" && expected.Kind != "boolean" && expected.Kind != "symbol" && expected.Kind != "undefined" && expected.Kind != "null" {
		return errors.New("object/function surface has no prototype expectation")
	}
	return nil
}

func validateFunctionExpectation(label string, expected *FunctionExpectation) error {
	if expected == nil {
		return fmt.Errorf("%s expectation is nil", label)
	}
	if expected.Length < 0 {
		return fmt.Errorf("%s length is negative", label)
	}
	if expected.Constructable == nil {
		return fmt.Errorf("%s constructability is unspecified", label)
	}
	if expected.OwnPropertyNames == nil {
		return fmt.Errorf("%s own property names are unspecified", label)
	}
	if !slices.IsSorted(expected.OwnPropertyNames) || slices.ContainsFunc(expected.OwnPropertyNames, func(key string) bool { return key == "" }) {
		return fmt.Errorf("%s own property names are invalid", label)
	}
	if expected.OwnPrototype == nil {
		return fmt.Errorf("%s own prototype is unspecified", label)
	}
	prototype := expected.OwnPrototype
	if !prototype.Exists {
		if prototype.Configurable != nil || prototype.Enumerable != nil || prototype.Writable != nil {
			return fmt.Errorf("%s absent own prototype declares descriptor flags", label)
		}
		return nil
	}
	if prototype.Configurable == nil || prototype.Enumerable == nil || prototype.Writable == nil {
		return fmt.Errorf("%s present own prototype omits descriptor flags", label)
	}
	return nil
}

func validateAuthorityClass(class ProfileClass, refs []string) error {
	want := map[ProfileClass]string{ClassNode: "node-v26.5.0", ClassBoundary: "goja-eventloop-boundary-v1"}[class]
	if want != "" && (!slices.Contains(refs, want) || len(refs) != 1) {
		return fmt.Errorf("class %q must reference only %q", class, want)
	}
	if class == ClassWeb {
		if len(refs) == 0 {
			return errors.New("web entry has no pinned Web authority")
		}
		for _, id := range refs {
			if id == "node-v26.5.0" || strings.HasPrefix(id, "goja-eventloop-") {
				return fmt.Errorf("web entry references non-Web authority %q", id)
			}
		}
	}
	if class == ClassExtension {
		if !slices.Contains(refs, "goja-eventloop-extension-v1") {
			return errors.New("extension entry has no package contract authority")
		}
		for _, id := range refs {
			if id != "goja-eventloop-extension-v1" && id != "whatwg-console" {
				return fmt.Errorf("extension entry references unsupported authority %q", id)
			}
		}
	}
	return nil
}

func validateSetup(setup Setup) error {
	seen := make(map[string]bool)
	for _, name := range setup.Globals {
		if !identifierPattern.MatchString(strings.ToLower(name)) || strings.Contains(name, ".") {
			return fmt.Errorf("invalid setup global %q", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate setup global %q", name)
		}
		seen[name] = true
	}
	for _, member := range setup.Members {
		if member.Object == "" || member.Property == "" || member.Value == "" {
			return errors.New("setup member has an empty field")
		}
	}
	for _, pair := range setup.BrandPairs {
		if !identifierPattern.MatchString(strings.ToLower(pair.Constructor)) || strings.Contains(pair.Constructor, ".") ||
			!identifierPattern.MatchString(strings.ToLower(pair.Singleton)) || strings.Contains(pair.Singleton, ".") || pair.Sentinel == "" {
			return errors.New("setup brand pair has an invalid field")
		}
		properties := make(map[string]string, len(pair.Methods)+len(pair.Accessors))
		for kind, names := range map[string][]string{"method": pair.Methods, "accessor": pair.Accessors} {
			if !slices.IsSorted(names) {
				return fmt.Errorf("setup brand pair %ss are not sorted", kind)
			}
			for _, name := range names {
				if !identifierPattern.MatchString(strings.ToLower(name)) || strings.Contains(name, ".") {
					return fmt.Errorf("setup brand pair has invalid %s %q", kind, name)
				}
				if prior := properties[name]; prior != "" {
					return fmt.Errorf("setup brand pair property %q repeats as %s and %s", name, prior, kind)
				}
				properties[name] = kind
			}
		}
		if pair.Constructor == pair.Singleton {
			return fmt.Errorf("setup brand pair repeats name %q", pair.Constructor)
		}
		for _, name := range []string{pair.Constructor, pair.Singleton} {
			if seen[name] {
				return fmt.Errorf("duplicate setup global %q", name)
			}
			seen[name] = true
		}
	}
	return nil
}

func validateAsset(asset Asset) error {
	if asset.File == "" || filepath.IsAbs(asset.File) || filepath.Clean(asset.File) != filepath.FromSlash(asset.File) || strings.HasPrefix(asset.File, ".."+string(filepath.Separator)) || strings.Contains(asset.File, "\\") {
		return fmt.Errorf("unsafe relative file %q", asset.File)
	}
	if len(asset.SHA256) != sha256.Size*2 || !lowerHex(asset.SHA256) {
		return fmt.Errorf("invalid SHA-256 %q", asset.SHA256)
	}
	return nil
}

func validateReferences[T any](kind string, refs []string, catalog map[string]T, used map[string]bool) error {
	if len(refs) == 0 {
		return fmt.Errorf("%s references are empty", kind)
	}
	seen := make(map[string]bool, len(refs))
	for _, id := range refs {
		if _, ok := catalog[id]; !ok {
			return fmt.Errorf("unknown %s %q", kind, id)
		}
		if seen[id] {
			return fmt.Errorf("duplicate %s reference %q", kind, id)
		}
		seen[id] = true
		used[id] = true
	}
	return nil
}

func validClass(class ProfileClass) bool {
	return class == ClassNode || class == ClassWeb || class == ClassExtension || class == ClassBoundary
}

func loadAsset(root string, asset Asset) ([]byte, error) {
	if err := validateAsset(asset); err != nil {
		return nil, err
	}
	path := filepath.Join(root, filepath.FromSlash(asset.File))
	data, err := readBounded(path, maxManifestBytes)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != asset.SHA256 {
		return nil, fmt.Errorf("%s SHA-256 is %s, want %s", asset.File, got, asset.SHA256)
	}
	return data, nil
}

func readBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, limit)
	}
	return data, nil
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func validateRawJSON(data []byte) error {
	if err := rejectDuplicateKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON value")
	}
	return nil
}

func rejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := make(map[string]bool)
			for decoder.More() {
				keyToken, keyErr := decoder.Token()
				if keyErr != nil {
					return keyErr
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("object key is not a string")
				}
				if seen[key] {
					return fmt.Errorf("duplicate object key %q", key)
				}
				seen[key] = true
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return fmt.Errorf("unexpected delimiter %q", delimiter)
		}
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func lowerHex(value string) bool {
	if value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
