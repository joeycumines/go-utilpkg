package gojagrpc

import (
	"testing"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
)

func TestModuleOptionsRejectTypedNilAndNilValues(t *testing.T) {
	tests := []struct {
		name   string
		option ModuleOption
	}{
		{name: "typed nil channel", option: (*ChannelOption)(nil)},
		{name: "typed nil protobuf", option: (*ProtobufOption)(nil)},
		{name: "typed nil adapter", option: (*AdapterOption)(nil)},
		{name: "nil channel", option: WithChannel(nil)},
		{name: "nil protobuf", option: WithProtobuf(nil)},
		{name: "nil adapter", option: WithAdapter(nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := resolveOptions(
				[]ModuleOption{test.option},
			); err == nil {
				t.Fatal("invalid module option was accepted")
			}
		})
	}
}

func TestModuleOptionsLastWin(t *testing.T) {
	_, runtime, adapter, channel, protobuf :=
		newRuntimeIdentityDependencies(t)
	cfg, err := resolveOptions([]ModuleOption{
		WithChannel(new(inprocgrpc.Channel)),
		WithChannel(channel),
		WithProtobuf(new(gojaprotobuf.Module)),
		WithProtobuf(protobuf),
		WithAdapter(new(gojaeventloop.Adapter)),
		WithAdapter(adapter),
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.channel != channel || cfg.protobuf != protobuf ||
		cfg.adapter != adapter {
		t.Fatal("last option did not win")
	}
	if !cfg.adapter.OwnsRuntime(runtime) {
		t.Fatal("resolved adapter does not own runtime")
	}
}

func TestRequireSnapshotsOptions(t *testing.T) {
	_, runtime, adapter, channel, protobuf :=
		newRuntimeIdentityDependencies(t)
	options := []ModuleOption{
		WithChannel(channel),
		WithProtobuf(protobuf),
		WithAdapter(adapter),
	}
	loader := Require(options...)
	for index := range options {
		options[index] = nil
	}
	module := runtime.NewObject()
	if err := module.Set("exports", runtime.NewObject()); err != nil {
		t.Fatal(err)
	}
	loader(runtime, module)
	exports, ok := module.Get("exports").(*goja.Object)
	if !ok {
		t.Fatalf("exports = %T, want object", module.Get("exports"))
	}
	if _, ok := goja.AssertFunction(exports.Get("createClient")); !ok {
		t.Fatal("snapshotted loader did not install exports")
	}
}
