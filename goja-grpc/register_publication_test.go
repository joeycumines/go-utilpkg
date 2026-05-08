package gojagrpc

import (
	"errors"
	"testing"

	"github.com/joeycumines/goja"
)

func TestPublishExportsRestoresOriginalWhenCloseWins(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	original := env.runtime.NewObject()
	if err := original.Set("marker", "original"); err != nil {
		t.Fatal(err)
	}
	if err := env.runtime.Set("__originalExports", original); err != nil {
		t.Fatal(err)
	}
	if err := env.runtime.Set("__closeDuringPublish", func() {
		if err := env.grpcMod.Close(); err != nil {
			panic(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	value, err := env.runtime.RunString(`
		(() => {
			let current = __originalExports;
			const module = {};
			Object.defineProperty(module, "exports", {
				configurable: true,
				get() { return current; },
				set(value) {
					current = value;
					__closeDuringPublish();
				},
			});
			return module;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	module, ok := value.(*goja.Object)
	if !ok {
		t.Fatalf("module = %T, want *goja.Object", value)
	}
	fresh := env.runtime.NewObject()
	err = env.grpcMod.publishExports(module, fresh, original)
	if !errors.Is(err, errModuleClosed) {
		t.Fatalf("publish error = %v, want module closed", err)
	}
	if got := module.Get("exports"); got != original {
		t.Fatalf("module.exports = %v, want original object", got)
	}
}
