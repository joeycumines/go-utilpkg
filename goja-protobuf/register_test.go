package gojaprotobuf

import (
	"fmt"
	"strings"
	"testing"

	"github.com/joeycumines/goja"
	gojarequire "github.com/joeycumines/goja_nodejs/require"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func TestRequire_LoadsProtobufModule(t *testing.T) {
	rt := goja.New()
	registry := gojarequire.NewRegistry()
	registry.RegisterNativeModule("protobuf", Require())
	registry.Enable(rt)

	v, err := rt.RunString(`
		var pb = require('protobuf');
		typeof pb.loadDescriptorSet === 'function' &&
		typeof pb.loadFileDescriptorProto === 'function' &&
		typeof pb.messageType === 'function' &&
		typeof pb.enumType === 'function' &&
		typeof pb.encode === 'function' &&
		typeof pb.decode === 'function' &&
		typeof pb.toJSON === 'function' &&
		typeof pb.fromJSON === 'function'
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestRequire_CustomModuleName(t *testing.T) {
	rt := goja.New()
	registry := gojarequire.NewRegistry()
	registry.RegisterNativeModule("my-pb", Require())
	registry.Enable(rt)

	v, err := rt.RunString(`
		var pb = require('my-pb');
		typeof pb.messageType === 'function'
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestRequireSnapshotsOptions(t *testing.T) {
	runtime := goja.New()
	types := new(protoregistry.Types)
	files := new(protoregistry.Files)
	options := []ModuleOption{WithResolver(types), WithFiles(files)}
	loader := Require(options...)
	options[0] = nil
	options[1] = nil

	module := runtime.NewObject()
	if err := module.Set("exports", runtime.NewObject()); err != nil {
		t.Fatal(err)
	}
	loader(runtime, module)
	value := runtime.GlobalObject().GetSymbol(runtimeStateSymbol)
	state, ok := value.Export().(*runtimeState)
	if !ok || state == nil {
		t.Fatal("Require did not install runtime state")
	}
	if state.typesIdentity != types || state.filesIdentity != files {
		t.Fatal("Require observed later option-slice mutation")
	}
}

func TestRequireRejectsForeignModuleBeforeStateCreation(t *testing.T) {
	runtime := goja.New()
	foreignRuntime := goja.New()
	foreignModule := foreignRuntime.NewObject()
	foreignExports := foreignRuntime.NewObject()
	if err := foreignExports.Set("marker", "preserved"); err != nil {
		t.Fatal(err)
	}
	if err := foreignModule.Set("exports", foreignExports); err != nil {
		t.Fatal(err)
	}
	recovered := captureRequirePanic(t, func() {
		Require()(runtime, foreignModule)
	})
	if !strings.Contains(fmt.Sprint(recovered), "module runtime mismatch") {
		t.Fatalf("panic = %v", recovered)
	}
	state := runtime.GlobalObject().GetSymbol(runtimeStateSymbol)
	if state != nil && !goja.IsUndefined(state) {
		t.Fatal("foreign module created protobuf runtime state")
	}
	if names := foreignExports.GetOwnPropertyNames(); len(names) != 1 ||
		names[0] != "marker" {
		t.Fatalf("foreign exports changed: %v", names)
	}
}

func TestRequireRejectsNonObjectExports(t *testing.T) {
	runtime := goja.New()
	module := runtime.NewObject()
	if err := module.Set("exports", 42); err != nil {
		t.Fatal(err)
	}
	recovered := captureRequirePanic(t, func() {
		Require()(runtime, module)
	})
	if !strings.Contains(
		fmt.Sprint(recovered),
		"module.exports must be an object",
	) {
		t.Fatalf("panic = %v", recovered)
	}
	state := runtime.GlobalObject().GetSymbol(runtimeStateSymbol)
	if state != nil && !goja.IsUndefined(state) {
		t.Fatal("invalid exports created protobuf runtime state")
	}
}

func captureRequirePanic(t *testing.T, run func()) (recovered any) {
	t.Helper()
	defer func() {
		recovered = recover()
		if recovered == nil {
			t.Fatal("expected Require to panic")
		}
	}()
	run()
	return nil
}
