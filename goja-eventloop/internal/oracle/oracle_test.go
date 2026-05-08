package oracle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

var oracleProcessHelper = flag.String("oracle-process-helper", "", "internal oracle process helper")

func manifestForTest(t *testing.T) *LoadedManifest {
	t.Helper()
	manifest, err := LoadManifest(filepath.Join("..", "..", "testdata", "oracle", "surface.json"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	return manifest
}

func TestProtocolSchemaVersion(t *testing.T) {
	if ProtocolSchema != "goja-eventloop.oracle.result/v6" {
		t.Fatalf("protocol schema = %q, want result/v6", ProtocolSchema)
	}
}

func TestComparisonContract(t *testing.T) {
	tests := []struct {
		name    string
		fixture Fixture
		want    string
	}{
		{
			name:    "Node semantic",
			fixture: Fixture{Comparison: CompareNodeExact, Role: RoleSemantic},
			want:    `{"nodeExact":[""],"expectedContract":[]}`,
		},
		{
			name:    "expected semantic",
			fixture: Fixture{Comparison: CompareExpected, Role: RoleSemantic},
			want:    `{"nodeExact":[],"expectedContract":[""]}`,
		},
		{
			name:    "expected surface",
			fixture: Fixture{Comparison: CompareExpected, Role: RoleSurface},
			want:    `{"nodeExact":[],"expectedContract":[""]}`,
		},
		{
			name:    "Node surface",
			fixture: Fixture{Comparison: CompareNodeExact, Role: RoleSurface},
			want:    `{"nodeExact":["/ok","/value/surfaces"],"expectedContract":["/value/audits","/value/constructionAudits"]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(comparisonContract(test.fixture))
			if err != nil {
				t.Fatal(err)
			}
			if got := string(data); got != test.want {
				t.Fatalf("comparison contract = %s, want %s", got, test.want)
			}
		})
	}
}

func TestCompareCaseNodeSurfaceEvidence(t *testing.T) {
	loaded := manifestForTest(t)
	var fixture Fixture
	for _, candidate := range loaded.Manifest.Fixtures {
		if candidate.ID == "surface-node" {
			fixture = candidate
			break
		}
	}
	if fixture.ID == "" {
		t.Fatal("surface-node fixture is unavailable")
	}
	expected, err := expectedForFixture(loaded.Manifest, fixture)
	if err != nil {
		t.Fatal(err)
	}
	mutateAudit := func(data json.RawMessage, changes []any) json.RawMessage {
		t.Helper()
		var wrapper map[string]any
		if err := json.Unmarshal(data, &wrapper); err != nil {
			t.Fatal(err)
		}
		value, ok := wrapper["value"].(map[string]any)
		if !ok {
			t.Fatal("expected surface value is unavailable")
		}
		audits, ok := value["audits"].([]any)
		if !ok || len(audits) == 0 {
			t.Fatal("expected surface audits are unavailable")
		}
		audit, ok := audits[0].(map[string]any)
		if !ok {
			t.Fatal("expected surface audit is invalid")
		}
		audit["changes"] = changes
		result, err := json.Marshal(wrapper)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	t.Run("Node audit is observation only", func(t *testing.T) {
		record := CaseRecord{
			Node:       mutateAudit(expected, []any{"node-only"}),
			Goja:       expected,
			Comparison: comparisonContract(fixture),
			Status:     "invalid",
		}
		if err := compareCase(&record, loaded.Manifest, fixture); err != nil {
			t.Fatal(err)
		}
		if record.Status != "pass" {
			t.Fatalf("status = %q, differences = %+v", record.Status, record.Differences)
		}
		if bytes.Equal(record.Node, record.Goja) {
			t.Fatal("mixed comparison test did not retain unequal full observations")
		}
		if len(record.Expected) == 0 {
			t.Fatal("mixed comparison omitted expected audit evidence")
		}
		var auditEvidence map[string]any
		if err := json.Unmarshal(record.Expected, &auditEvidence); err != nil {
			t.Fatal(err)
		}
		if _, exists := auditEvidence["ok"]; exists {
			t.Fatal("expected audit evidence overlaps the Node-exact /ok scope")
		}
		value, ok := auditEvidence["value"].(map[string]any)
		if !ok || value["audits"] == nil || value["constructionAudits"] == nil || value["surfaces"] != nil {
			t.Fatalf("expected audit evidence = %#v", auditEvidence)
		}
	})

	t.Run("Goja audit mismatch fails", func(t *testing.T) {
		record := CaseRecord{
			Node:       expected,
			Goja:       mutateAudit(expected, []any{"goja-mismatch"}),
			Comparison: comparisonContract(fixture),
			Status:     "invalid",
		}
		if err := compareCase(&record, loaded.Manifest, fixture); err != nil {
			t.Fatal(err)
		}
		if record.Status != "mismatch" || len(record.Differences) == 0 {
			t.Fatalf("record = %+v, want audit mismatch", record)
		}
	})
}

func TestManifestAuthenticatesCompleteCatalog(t *testing.T) {
	manifest := manifestForTest(t)
	if got, want := len(manifest.Manifest.Fixtures), len(requiredFixtureCatalog); got != want {
		t.Fatalf("fixture count = %d, want %d", got, want)
	}
	if got := len(manifest.Manifest.Surfaces); got < 150 {
		t.Fatalf("surface count = %d, want an explicit retained catalog", got)
	}
	for _, fixture := range manifest.Manifest.Fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			data, ok := manifest.Fixtures[fixture.ID]
			if !ok || len(data) == 0 {
				t.Fatal("authenticated fixture bytes are absent")
			}
		})
	}
}

func TestManifestRetainedSurfaceContractIsComplete(t *testing.T) {
	manifest := manifestForTest(t)
	index := make(map[string][]Surface, len(manifest.Manifest.Surfaces))
	for _, surface := range manifest.Manifest.Surfaces {
		key := string(surface.Class) + "\x00" + surface.Root + "\x00" + surface.Path
		index[key] = append(index[key], surface)
	}
	must := func(class ProfileClass, root, path string) Surface {
		t.Helper()
		key := string(class) + "\x00" + root + "\x00" + path
		values := index[key]
		if len(values) == 0 {
			t.Fatalf("missing retained surface %s/%s/%s", class, root, path)
		}
		if len(values) != 1 {
			t.Fatalf("retained surface %s/%s/%s has %d declarations, want 1", class, root, path, len(values))
		}
		return values[0]
	}

	interfaceParents := map[string]string{
		"EventTarget": "Object", "Event": "Object", "CustomEvent": "Event",
		"AbortController": "Object", "AbortSignal": "EventTarget", "DOMException": "Error",
		"Performance": "EventTarget", "Crypto": "Object",
	}
	for name, parent := range interfaceParents {
		prototype := must(ClassWeb, "global", name+".prototype")
		if prototype.Expected.Prototype != parent || prototype.Expected.Descriptor == nil ||
			prototype.Expected.Descriptor.Writable == nil || *prototype.Expected.Descriptor.Writable ||
			prototype.Expected.Descriptor.Enumerable || prototype.Expected.Descriptor.Configurable {
			t.Errorf("%s.prototype expectation = %+v, want non-writable WebIDL prototype inheriting %s", name, prototype.Expected, parent)
		}
		constructor := must(ClassWeb, "global", name+".prototype.constructor")
		if constructor.Expected.Function == nil || constructor.Expected.Function.Name != name {
			t.Errorf("%s prototype constructor = %+v", name, constructor.Expected)
		}
	}

	performanceJSON := must(ClassWeb, "global", "performance.toJSON")
	if performanceJSON.Mode != "install" || performanceJSON.Expected.Function == nil || performanceJSON.Expected.Function.Name != "toJSON" || performanceJSON.Expected.Descriptor.Depth != 1 {
		t.Errorf("performance.toJSON expectation = %+v", performanceJSON)
	}
	for _, path := range []string{"Performance.prototype.now", "Performance.prototype.timeOrigin", "Performance.prototype.toJSON", "Performance.prototype.@@toStringTag", "performance.constructor", "performance.@@toStringTag"} {
		must(ClassWeb, "global", path)
	}
	for _, path := range []string{"Crypto", "Crypto.prototype", "Crypto.prototype.getRandomValues", "Crypto.prototype.randomUUID", "Crypto.prototype.@@toStringTag", "crypto.constructor", "crypto.@@toStringTag"} {
		must(ClassWeb, "global", path)
	}
	for _, path := range []string{"crypto.subtle", "Crypto.prototype.subtle"} {
		if value := must(ClassWeb, "global", path); value.Mode != "absent" || value.Expected.Exists {
			t.Errorf("%s must be explicitly absent: %+v", path, value)
		}
	}

	for _, path := range []string{
		"Event.prototype.srcElement", "Event.prototype.composedPath", "Event.prototype.returnValue",
		"Event.prototype.cancelBubble", "Event.prototype.composed", "Event.prototype.initEvent",
		"CustomEvent.prototype.initCustomEvent",
	} {
		must(ClassWeb, "global", path)
	}
	trusted := must(ClassWeb, "eventInstance", "Event instance.isTrusted")
	if trusted.Expected.Descriptor == nil || trusted.Expected.Descriptor.Depth != 0 || trusted.Expected.Descriptor.Configurable || !trusted.Expected.Descriptor.Enumerable {
		t.Errorf("LegacyUnforgeable isTrusted descriptor = %+v", trusted.Expected.Descriptor)
	}
	for _, constant := range []string{"NONE", "CAPTURING_PHASE", "AT_TARGET", "BUBBLING_PHASE"} {
		for _, path := range []string{"Event." + constant, "Event.prototype." + constant} {
			value := must(ClassWeb, "global", path)
			descriptor := value.Expected.Descriptor
			if descriptor == nil || descriptor.Writable == nil || *descriptor.Writable || !descriptor.Enumerable || descriptor.Configurable {
				t.Errorf("%s descriptor = %+v, want W=false E=true C=false", path, descriptor)
			}
		}
	}

	timeoutConstructor := must(ClassNode, "timeoutPrototype", "Timeout.prototype.constructor")
	immediateConstructor := must(ClassNode, "immediatePrototype", "Immediate.prototype.constructor")
	if timeoutConstructor.Expected.Function.Length != 5 || immediateConstructor.Expected.Function.Length != 2 {
		t.Errorf("handle constructor lengths = %d, %d", timeoutConstructor.Expected.Function.Length, immediateConstructor.Expected.Function.Length)
	}
	must(ClassNode, "timeoutInstance", "Timeout instance._idleTimeout")
	for _, path := range []string{
		"Timeout instance._idlePrev", "Timeout instance._idleNext", "Timeout instance._idleStart",
		"Timeout instance._onTimeout", "Timeout instance._timerArgs", "Timeout instance._repeat",
		"Timeout instance._destroyed", "Timeout instance.@@refed", "Timeout instance.@@kHasPrimitive",
		"Timeout instance.@@asyncId", "Timeout instance.@@triggerId", "Timeout instance.@@kAsyncContextFrame",
	} {
		must(ClassNode, "timeoutInstance", path)
	}
	for _, path := range []string{
		"Immediate instance._idleNext", "Immediate instance._idlePrev", "Immediate instance._onImmediate",
		"Immediate instance._argv", "Immediate instance._destroyed", "Immediate instance.@@refed",
		"Immediate instance.@@asyncId", "Immediate instance.@@triggerId", "Immediate instance.@@kAsyncContextFrame",
	} {
		must(ClassNode, "immediateInstance", path)
	}
	for _, path := range []string{
		"process._events.newListener", "process._events.removeListener", "process.@@kCapture",
	} {
		must(ClassNode, "global", path)
	}
	for _, path := range []string{
		"EventEmitter.prototype._events", "EventEmitter.prototype._eventsCount",
		"EventEmitter.prototype._maxListeners", "EventEmitter.prototype.@@kCapture",
	} {
		must(ClassNode, "processEmitterPrototype", path)
	}
	for _, path := range []string{
		"Timeout instance._idleStart", "Timeout instance.@@asyncId", "Timeout instance.@@triggerId",
		"Immediate instance.@@asyncId", "Immediate instance.@@triggerId",
	} {
		root := "timeoutInstance"
		if strings.HasPrefix(path, "Immediate") {
			root = "immediateInstance"
		}
		if surface := must(ClassNode, root, path); surface.ValueMode != "type" || len(surface.Expected.Value) != 0 {
			t.Errorf("%s dynamic value contract = mode %q/value %s", path, surface.ValueMode, surface.Expected.Value)
		}
	}
	inspectPath := "Timeout.prototype.@@nodejs.util.inspect.custom"
	if inspect := must(ClassBoundary, "timeoutPrototype", inspectPath); inspect.Mode != "absent" || inspect.Expected.Exists {
		t.Errorf("unclaimed timeout inspect hook = %+v", inspect)
	}
	if values := index[string(ClassNode)+"\x00timeoutPrototype\x00"+inspectPath]; len(values) != 0 {
		t.Fatal("timeout inspect hook leaked into Node conformance surface")
	}
	exitCode := must(ClassNode, "global", "process.exitCode")
	if exitCode.Expected.Descriptor == nil || exitCode.Expected.Descriptor.Configurable || !exitCode.Expected.Descriptor.Enumerable || exitCode.Expected.Descriptor.Getter == nil || exitCode.Expected.Descriptor.Setter == nil {
		t.Errorf("process.exitCode descriptor = %+v", exitCode.Expected.Descriptor)
	}
}

func TestAuditGroupsExpectOwnedProcessReplacement(t *testing.T) {
	descriptor := &DescriptorExpectation{Depth: 0}
	manifest := Manifest{Surfaces: []Surface{
		{Root: "global", Segments: []string{"console"}, Mode: "augment", Expected: SurfaceExpected{Descriptor: descriptor}},
		{Root: "global", Segments: []string{"process"}, Mode: "augment", Expected: SurfaceExpected{Descriptor: descriptor}},
	}}
	fixture := Fixture{Setup: Setup{Members: []SetupMember{
		{Object: "console", Property: "host", Value: "console-host"},
		{Object: "process", Property: "host", Value: "process-host"},
	}}}

	groups := auditGroups(manifest, fixture)
	if len(groups) != 1 {
		t.Fatalf("audit group count = %d, want 1", len(groups))
	}
	if groups[0].Changes["console"] {
		t.Fatal("foreign console identity must remain preserved")
	}
	if !groups[0].Changes["process"] {
		t.Fatal("adapter-owned detached process replacement was omitted from the audit")
	}
}

func TestAuditGroupsMapProcessPrototypeOwners(t *testing.T) {
	manifest := Manifest{Surfaces: []Surface{
		{
			Root: "global", Segments: []string{"process", "constructor"}, Mode: "augment",
			Expected: SurfaceExpected{Descriptor: &DescriptorExpectation{Depth: 1}},
		},
		{
			Root: "global", Segments: []string{"process", "on"}, Mode: "augment",
			Expected: SurfaceExpected{Descriptor: &DescriptorExpectation{Depth: 2}},
		},
		{
			Root: "processEmitterPrototype", Segments: []string{"_events"}, Mode: "install",
			Expected: SurfaceExpected{Descriptor: &DescriptorExpectation{Depth: 0}},
		},
		{
			Root: "processEmitterPrototype", Segments: []string{"@@kCapture"}, Mode: "install",
			Expected: SurfaceExpected{Descriptor: &DescriptorExpectation{Depth: 0}},
		},
	}}

	groups := auditGroups(manifest, Fixture{})
	byRoot := make(map[string]auditGroup, len(groups))
	for _, group := range groups {
		byRoot[group.Input.Root] = group
	}
	if group, ok := byRoot["processPrototype"]; !ok {
		t.Fatal("process prototype ownership group is absent")
	} else if len(group.Changes) != 0 {
		t.Fatalf("process prototype changes = %v, want constructor ignored", group.Changes)
	}
	group, ok := byRoot["processEmitterPrototype"]
	if !ok {
		t.Fatal("process EventEmitter prototype ownership group is absent")
	}
	for _, change := range []string{"on", "_events", "Symbol(kCapture)"} {
		if !group.Changes[change] {
			t.Errorf("process EventEmitter prototype change %q is absent from %v", change, group.Changes)
		}
	}
}

func TestValidateSurfaceExpectedValueModes(t *testing.T) {
	writable := true
	descriptor := &DescriptorExpectation{Depth: 0, Writable: &writable}
	number := SurfaceExpected{Exists: true, Kind: "number", Descriptor: descriptor}
	if err := validateSurfaceExpected(number, "type"); err != nil {
		t.Fatalf("type-only number: %v", err)
	}
	if err := validateSurfaceExpected(number, ""); err == nil {
		t.Fatal("exact number without a value was accepted")
	}
	number.Value = json.RawMessage("1")
	if err := validateSurfaceExpected(number, "type"); err == nil {
		t.Fatal("type-only number with a value was accepted")
	}
	nullValue := SurfaceExpected{Exists: true, Kind: "null", Descriptor: descriptor}
	if err := validateSurfaceExpected(nullValue, ""); err != nil {
		t.Fatalf("null value: %v", err)
	}
	if err := validateSurfaceExpected(nullValue, "type"); err == nil {
		t.Fatal("type-only null was accepted")
	}
}

func TestHarnessBrandPairSetupIsCoherentAndRestorable(t *testing.T) {
	manifest := manifestForTest(t)
	runtime := goja.New()
	if _, err := runtime.RunString(string(manifest.Harness)); err != nil {
		t.Fatal(err)
	}
	value, err := runtime.RunString(`
      (() => {
        const originalConstructor = function OriginalCrypto() {};
        const originalSingleton = Object.create(originalConstructor.prototype);
        globalThis.Crypto = originalConstructor;
        globalThis.crypto = originalSingleton;
        const oracle = globalThis.__gojaEventloopOracle;
        oracle.setup({brandPairs: [{
          constructor: "Crypto",
          singleton: "crypto",
          sentinel: "crypto",
          methods: ["getRandomValues", "randomUUID"],
          accessors: ["subtle"],
        }]}, {});
        const during = [
          Crypto.name,
          Crypto.__oracleSentinel,
          crypto.__oracleSentinel,
          crypto instanceof Crypto,
          Object.getPrototypeOf(crypto) === Crypto.prototype,
          Object.prototype.toString.call(crypto),
          typeof Crypto.prototype.getRandomValues,
          typeof Object.getOwnPropertyDescriptor(Crypto.prototype, "subtle").get,
        ];
        oracle.restore();
        return [during, Crypto === originalConstructor, crypto === originalSingleton];
      })()
    `)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("__oracleBrandResult", value); err != nil {
		t.Fatal(err)
	}
	encoded, err := runtime.RunString(`JSON.stringify(__oracleBrandResult)`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := encoded.String(), `[["Crypto","Crypto","crypto",true,true,"[object Crypto]","function","function"],true,true]`; got != want {
		t.Fatalf("brand pair result = %s, want %s", got, want)
	}
	if err := validateSetup(Setup{
		Globals:    []string{"crypto"},
		BrandPairs: []SetupBrandPair{{Constructor: "Crypto", Singleton: "crypto", Sentinel: "crypto"}},
	}); err == nil || !strings.Contains(err.Error(), "duplicate setup global") {
		t.Fatalf("overlapping brand setup error = %v", err)
	}
}

func TestStrictJSONRejectsDuplicateUnknownAndTrailingValues(t *testing.T) {
	if err := rejectDuplicateKeys([]byte(`{"a":1,"a":2}`)); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate-key error = %v", err)
	}
	var target struct {
		A int `json:"a"`
	}
	if err := decodeStrict([]byte(`{"a":1,"extra":2}`), &target); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown-field error = %v", err)
	}
	if _, _, err := canonicalJSON([]byte(`{} {}`)); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing-value error = %v", err)
	}
}

func TestAssetHashDriftFailsClosed(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "fixture.js")
	if err := os.WriteFile(path, []byte("function fixture() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadAsset(root, Asset{File: "fixture.js", SHA256: strings.Repeat("0", 64)})
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("hash drift error = %v", err)
	}
}

func TestNodeProtocolFailsClosed(t *testing.T) {
	valid := []byte(nodeFrame + `{"ok":true}` + "\n")
	if observation, err := parseNodeObservation(valid, nil); err != nil || string(observation) != `{"ok":true}` {
		t.Fatalf("valid observation = %s, %v", observation, err)
	}
	tests := []struct {
		name   string
		stdout []byte
		stderr []byte
	}{
		{name: "no frame", stdout: []byte(`{"ok":true}`)},
		{name: "stdout noise", stdout: append([]byte("noise\n"), valid...)},
		{name: "two frames", stdout: append(append([]byte{}, valid...), valid...)},
		{name: "stderr noise", stdout: valid, stderr: []byte("warning")},
		{name: "duplicate observation key", stdout: []byte(nodeFrame + `{"ok":true,"ok":false}` + "\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseNodeObservation(test.stdout, test.stderr); err == nil {
				t.Fatal("protocol input unexpectedly passed")
			}
		})
	}
}

func TestCappedBufferRejectsOversize(t *testing.T) {
	buffer := &cappedBuffer{limit: 4}
	if written, err := buffer.Write([]byte("12345")); err != nil || written != 5 {
		t.Fatalf("Write = %d, %v", written, err)
	}
	if !buffer.over || buffer.buffer.String() != "1234" {
		t.Fatalf("capped buffer = %q, over=%v", buffer.buffer.String(), buffer.over)
	}
}

func TestGojaConsoleCaptureIsScopedAndPreservesExceptionIdentity(t *testing.T) {
	loop := goeventloop.New(goeventloop.WithAutoExit(true))
	t.Cleanup(func() {
		if err := loop.Close(); err != nil && !errors.Is(err, goeventloop.ErrLoopTerminated) {
			t.Errorf("close loop: %v", err)
		}
	})
	runtime := goja.New()
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := installConsoleCapture(runtime, adapter); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	value, err := runtime.RunString(`
      (() => {
        const inside = __oracleCaptureConsole(() => console.count("inside"));
        console.count("outside");
        const outside = __oracleCaptureConsole(() => console.count("outside"));
        const sequentialFirst = __oracleCaptureConsole(() => console.count("sequential"));
        const sequentialSecond = __oracleCaptureConsole(() => console.count("sequential"));
        let nestedInner;
        const nestedOuter = __oracleCaptureConsole(() => {
          console.count("nested");
          nestedInner = __oracleCaptureConsole(() => console.count("nested"));
          console.count("nested");
        });
        const thrown = {};
        let same = false;
        try {
          __oracleCaptureConsole(() => { throw thrown; });
        } catch (error) {
          same = error === thrown;
        }
        let callbackTypeError = false;
        try {
          __oracleCaptureConsole(1);
        } catch (error) {
          callbackTypeError = error instanceof TypeError;
        }
        const descriptor = Object.getOwnPropertyDescriptor(globalThis, "__oracleCaptureConsole");
        return JSON.stringify([
          inside,
          outside,
          sequentialFirst,
          sequentialSecond,
          nestedOuter,
          nestedInner,
          same,
          callbackTypeError,
          [descriptor.writable, descriptor.configurable, descriptor.enumerable],
        ]);
      })()
    `)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := value.String(), `["inside: 1\n","outside: 2\n","sequential: 1\n","sequential: 2\n","nested: 1\nnested: 3\n","nested: 2\n",true,true,[false,true,false]]`; got != want {
		t.Fatalf("capture result = %s, want %s", got, want)
	}
}

func TestRunProcessTimeoutCrashAndOversize(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("timeout cleanup", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, _, err := runProcess(ctx, executable, []string{"-test.run=^TestOracleProcessHelper$", "-oracle-process-helper=sleep"}, nil)
		if err == nil || !strings.Contains(err.Error(), "timeout") {
			t.Fatalf("timeout error = %v", err)
		}
	})
	t.Run("crash", func(t *testing.T) {
		_, _, err := runProcess(context.Background(), executable, []string{"-test.run=^TestOracleProcessHelper$", "-oracle-process-helper=crash"}, nil)
		if err == nil {
			t.Fatal("crash unexpectedly passed")
		}
	})
	t.Run("oversize", func(t *testing.T) {
		_, _, err := runProcess(context.Background(), executable, []string{"-test.run=^TestOracleProcessHelper$", "-oracle-process-helper=oversize"}, nil)
		if err == nil || !strings.Contains(err.Error(), "exceeded") {
			t.Fatalf("oversize error = %v", err)
		}
	})
}

func TestOracleProcessHelper(t *testing.T) {
	switch *oracleProcessHelper {
	case "":
		return
	case "sleep":
		time.Sleep(5 * time.Second)
	case "crash":
		os.Exit(7)
	case "oversize":
		_, _ = os.Stdout.WriteString(strings.Repeat("x", maxEngineOutputBytes+1))
	default:
		t.Fatalf("unknown helper %q", *oracleProcessHelper)
	}
}

func TestSurfaceAuditFindsUndeclaredDeltaAndIgnoresRunnerTemporaries(t *testing.T) {
	manifest := manifestForTest(t)
	runtime := goja.New()
	if _, err := runtime.RunString(string(manifest.Harness)); err != nil {
		t.Fatal(err)
	}
	value, err := runtime.RunString(`
      (() => {
        const oracle = globalThis.__gojaEventloopOracle;
        const input = {audits: [{id: "audit", path: "global", root: "global", segments: []}]};
        oracle.setup({}, input);
        globalThis.declaredMutation = 1;
        globalThis.__oracleRunnerTemporary = 2;
        return oracle.surfaceFixture({surfaces: [], audits: input.audits});
      })()
    `)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("__oracleAuditResult", value); err != nil {
		t.Fatal(err)
	}
	encoded, err := runtime.RunString(`JSON.stringify(__oracleAuditResult.audits[0].changes)`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := encoded.String(), `["declaredMutation"]`; got != want {
		t.Fatalf("audit changes = %s, want %s", got, want)
	}
}

func TestCommandValidationEvidence(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "testdata", "oracle", "surface.json")
	var stdout, stderr bytes.Buffer
	if exit := Command(context.Background(), []string{"-validate", "-manifest", manifestPath}, &stdout, &stderr); exit != ExitPass {
		t.Fatalf("validation exit = %d, stderr = %s", exit, stderr.String())
	}
	var validation map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &validation); err != nil || validation["status"] != "pass" {
		t.Fatalf("validation record = %v, %v", validation, err)
	}
}
