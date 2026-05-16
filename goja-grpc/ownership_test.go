package gojagrpc

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
)

func newRuntimeIdentityDependencies(t *testing.T) (*goeventloop.Loop, *goja.Runtime, *gojaeventloop.Adapter, *inprocgrpc.Channel, *gojaprotobuf.Module) {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	owner := goja.New()
	if _, err := owner.RunString(`
		(() => {
			class Performance {
				now() { return 0; }
				get timeOrigin() { return 0; }
				toJSON() { return {}; }
			}
			Object.defineProperty(Performance.prototype, Symbol.toStringTag, {
				value: "Performance", configurable: true,
			});
			class Crypto {
				randomUUID() { return "00000000-0000-4000-8000-000000000000"; }
				getRandomValues(value) { return value; }
			}
			Object.defineProperty(Crypto.prototype, Symbol.toStringTag, {
				value: "Crypto", configurable: true,
			});
			globalThis.Performance = Performance;
			globalThis.performance = new Performance();
			globalThis.Crypto = Crypto;
			globalThis.crypto = new Crypto();
		})();
	`); err != nil {
		t.Fatalf("install host singletons: %v", err)
	}
	adapter, err := gojaeventloop.New(loop, owner)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind adapter: %v", err)
	}
	channel := mustNewInprocChannel(t, inprocgrpc.WithLoop(loop))
	protobuf, err := gojaprotobuf.New(owner)
	if err != nil {
		t.Fatalf("New protobuf: %v", err)
	}
	t.Cleanup(func() { _ = loop.Shutdown(context.Background()) })
	return loop, owner, adapter, channel, protobuf
}

func TestNewRejectsAdapterRuntimeMismatch(t *testing.T) {
	_, _, adapter, channel, protobuf := newRuntimeIdentityDependencies(t)
	foreign := goja.New()
	requirePanicErrorIs(t, errAdapterRuntimeMismatch, func() {
		New(
			foreign,
			WithChannel(channel),
			WithProtobuf(protobuf),
			WithAdapter(adapter),
		)
	})
}

func TestNewRejectsTerminalAdapterBeforeAllocation(t *testing.T) {
	loop, runtime, adapter, channel, protobuf :=
		newRuntimeIdentityDependencies(t)
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
	<-adapter.Done()
	module, err := New(
		runtime,
		WithChannel(channel),
		WithProtobuf(protobuf),
		WithAdapter(adapter),
	)
	if module != nil || !errors.Is(err, goeventloop.ErrLoopTerminated) {
		t.Fatalf(
			"New terminal adapter = (%#v, %v), want nil ErrLoopTerminated",
			module,
			err,
		)
	}
}

func TestRequireRejectsAdapterRuntimeMismatchBeforeExports(t *testing.T) {
	_, _, adapter, channel, protobuf := newRuntimeIdentityDependencies(t)
	foreign := goja.New()
	module := foreign.NewObject()
	exports := foreign.NewObject()
	marker := foreign.NewObject()
	if err := exports.DefineDataProperty("marker", marker, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		t.Fatal(err)
	}
	if err := module.Set("exports", exports); err != nil {
		t.Fatal(err)
	}
	loader := Require(
		WithChannel(channel),
		WithProtobuf(protobuf),
		WithAdapter(adapter),
	)
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		loader(foreign, module)
	}()
	if recovered == nil {
		t.Fatal("Require runtime mismatch did not panic")
	}
	if !strings.Contains(recovered.(goja.Value).String(), errAdapterRuntimeMismatch.Error()) {
		t.Fatalf("Require panic = %v, want runtime mismatch", recovered)
	}
	if got := exports.GetOwnPropertyNames(); len(got) != 1 || got[0] != "marker" {
		t.Fatalf("exports changed before mismatch rejection: %v", got)
	}
	if !exports.Get("marker").SameAs(marker) {
		t.Fatal("exports marker identity changed before mismatch rejection")
	}
}

func TestNewRejectsProtobufRuntimeMismatch(t *testing.T) {
	_, runtime, adapter, channel, _ := newRuntimeIdentityDependencies(t)
	foreignProtobuf, err := gojaprotobuf.New(goja.New())
	if err != nil {
		t.Fatalf("New foreign protobuf: %v", err)
	}
	requirePanicErrorIs(t, errProtobufRuntimeMismatch, func() {
		New(
			runtime,
			WithChannel(channel),
			WithProtobuf(foreignProtobuf),
			WithAdapter(adapter),
		)
	})
}

func TestNewRejectsChannelLoopMismatch(t *testing.T) {
	_, runtime, adapter, _, protobuf := newRuntimeIdentityDependencies(t)
	foreignLoop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = foreignLoop.Shutdown(context.Background()) })
	foreignChannel := mustNewInprocChannel(t, inprocgrpc.WithLoop(foreignLoop))
	requirePanicErrorIs(t, errChannelLoopMismatch, func() {
		New(
			runtime,
			WithChannel(foreignChannel),
			WithProtobuf(protobuf),
			WithAdapter(adapter),
		)
	})
}

func TestRequireRejectsDependencyIdentityBeforeExports(t *testing.T) {
	tests := []struct {
		name    string
		options func(*testing.T, *goeventloop.Loop, *goja.Runtime, *gojaeventloop.Adapter, *inprocgrpc.Channel, *gojaprotobuf.Module) []ModuleOption
		want    error
	}{
		{
			name: "protobuf runtime",
			options: func(t *testing.T, _ *goeventloop.Loop, _ *goja.Runtime, adapter *gojaeventloop.Adapter, channel *inprocgrpc.Channel, _ *gojaprotobuf.Module) []ModuleOption {
				t.Helper()
				protobuf, err := gojaprotobuf.New(goja.New())
				if err != nil {
					t.Fatalf("New foreign protobuf: %v", err)
				}
				return []ModuleOption{WithChannel(channel), WithProtobuf(protobuf), WithAdapter(adapter)}
			},
			want: errProtobufRuntimeMismatch,
		},
		{
			name: "channel loop",
			options: func(t *testing.T, _ *goeventloop.Loop, _ *goja.Runtime, adapter *gojaeventloop.Adapter, _ *inprocgrpc.Channel, protobuf *gojaprotobuf.Module) []ModuleOption {
				t.Helper()
				foreignLoop, err := goeventloop.New()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = foreignLoop.Shutdown(context.Background()) })
				channel := mustNewInprocChannel(t, inprocgrpc.WithLoop(foreignLoop))
				return []ModuleOption{WithChannel(channel), WithProtobuf(protobuf), WithAdapter(adapter)}
			},
			want: errChannelLoopMismatch,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, runtime, adapter, channel, protobuf := newRuntimeIdentityDependencies(t)
			module := runtime.NewObject()
			exports := runtime.NewObject()
			marker := runtime.NewObject()
			if err := exports.DefineDataProperty("marker", marker, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
				t.Fatal(err)
			}
			if err := module.Set("exports", exports); err != nil {
				t.Fatal(err)
			}
			loader := Require(test.options(t, loop, runtime, adapter, channel, protobuf)...)
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				loader(runtime, module)
			}()
			if recovered == nil {
				t.Fatal("Require dependency mismatch did not panic")
			}
			value, ok := recovered.(goja.Value)
			if !ok || !strings.Contains(value.String(), test.want.Error()) {
				t.Fatalf("Require panic = %v, want %v", recovered, test.want)
			}
			if got := exports.GetOwnPropertyNames(); len(got) != 1 || got[0] != "marker" {
				t.Fatalf("exports changed before mismatch rejection: %v", got)
			}
			if !exports.Get("marker").SameAs(marker) {
				t.Fatal("exports marker identity changed before mismatch rejection")
			}
		})
	}
}

type countingModuleOption struct {
	applications *atomic.Int64
}

func (o *countingModuleOption) applyModuleOption(*moduleConfig) error {
	o.applications.Add(1)
	return errors.New("counting option applied")
}

func TestRequireValidatesOptionsImmediately(t *testing.T) {
	var applications atomic.Int64
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = Require(&countingModuleOption{applications: &applications})
	}()
	if recovered == nil {
		t.Fatal("invalid options did not panic")
	}
	if applications.Load() != 1 {
		t.Fatalf(
			"option applications = %d, want one immediate validation",
			applications.Load(),
		)
	}
}

func TestRequireRejectsMissingOptionsImmediately(t *testing.T) {
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = Require()
	}()
	if recovered == nil {
		t.Fatal("missing options did not panic")
	}
}

func TestRequireTerminalAdapterPreservesExports(t *testing.T) {
	loop, runtime, adapter, channel, protobuf :=
		newRuntimeIdentityDependencies(t)
	module := runtime.NewObject()
	exports := runtime.NewObject()
	marker := runtime.NewObject()
	if err := exports.Set("marker", marker); err != nil {
		t.Fatal(err)
	}
	if err := module.Set("exports", exports); err != nil {
		t.Fatal(err)
	}
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
	<-adapter.Done()
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		Require(
			WithChannel(channel),
			WithProtobuf(protobuf),
			WithAdapter(adapter),
		)(runtime, module)
	}()
	if recovered == nil {
		t.Fatal("terminal adapter Require did not panic")
	}
	if value, ok := recovered.(goja.Value); !ok ||
		!strings.Contains(value.String(), goeventloop.ErrLoopTerminated.Error()) {
		t.Fatalf("Require panic = %v, want ErrLoopTerminated", recovered)
	}
	if module.Get("exports") != exports || exports.Get("marker") != marker {
		t.Fatal("terminal Require changed module.exports")
	}
}

func TestSetupExportsAuthenticatesRuntimeBeforeMutation(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	foreign := goja.New().NewObject()
	if err := foreign.Set("marker", "preserved"); err != nil {
		t.Fatal(err)
	}
	if err := env.grpcMod.SetupExports(foreign); err == nil {
		t.Fatal("foreign exports object was accepted")
	}
	if names := foreign.GetOwnPropertyNames(); len(names) != 1 ||
		names[0] != "marker" {
		t.Fatalf("foreign exports changed: %v", names)
	}
	owner := env.runtime.NewObject()
	if err := env.grpcMod.SetupExports(owner); err != nil {
		t.Fatalf("owner exports rejected after foreign target: %v", err)
	}
}

func TestSetupExportsReturnsAbruptPropertyInspection(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	value, err := env.runtime.RunString(`new Proxy({}, {
		ownKeys() { throw new Error("ownKeys failed"); }
	})`)
	if err != nil {
		t.Fatal(err)
	}
	exports, ok := value.(*goja.Object)
	if !ok {
		t.Fatalf("proxy = %T, want object", value)
	}
	if err := env.grpcMod.SetupExports(exports); err == nil ||
		!strings.Contains(err.Error(), "ownKeys failed") {
		t.Fatalf("SetupExports error = %v", err)
	}
}
