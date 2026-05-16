package gojaeventloop

import (
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func newBoundAdapterForNode26Test(t *testing.T) *Adapter {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	return adapter
}

func TestNode26AbortSignalReasonsAndThrowIdentity(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		const out = [];
		const objectReason = { marker: 1 };
		const aborted = AbortSignal.abort(objectReason);
		out.push(aborted.aborted === true);
		out.push(aborted.reason === objectReason);
		try { aborted.throwIfAborted(); out.push(false); } catch (err) { out.push(err === objectReason); }

		const controller = new AbortController();
		controller.abort(null);
		out.push(controller.signal.reason === null);
		try { controller.signal.throwIfAborted(); out.push(false); } catch (err) { out.push(err === null); }

		const defaultSignal = AbortSignal.abort();
		out.push(defaultSignal.reason instanceof DOMException);
		out.push(defaultSignal.reason.name === "AbortError");
		out.push(defaultSignal.reason.message === "This operation was aborted");
		out.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "true,true,true,true,true,true,true,true"; got != want {
		t.Fatalf("AbortSignal reason identity = %q, want %q", got, want)
	}
}
