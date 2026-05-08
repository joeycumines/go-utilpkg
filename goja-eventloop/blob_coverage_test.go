package gojaeventloop

import (
	"context"
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// Coverage for console.table, inspectValue, formatCellValue edge cases

func TestConsoleTable_WithObject(t *testing.T) {
	loop := goeventloop.New()
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// This exercises generateTableFromObject, formatCellValue, inspectValue
	_, err = runtime.RunString(`
		console.table({a: 1, b: "hello", c: true, d: null, e: undefined});
	`)
	if err != nil {
		t.Fatalf("console.table failed: %v", err)
	}
}

func TestConsoleTable_WithColumnFilter(t *testing.T) {
	loop := goeventloop.New()
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	_, err = runtime.RunString(`
		console.table([{a: 1, b: 2, c: 3}, {a: 4, b: 5, c: 6}], ["a", "c"]);
	`)
	if err != nil {
		t.Fatalf("console.table with columns failed: %v", err)
	}
}

func TestConsoleTable_NonArrayNonObject(t *testing.T) {
	loop := goeventloop.New()
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Passing a primitive to console.table
	_, err = runtime.RunString(`
		console.table("just a string");
		console.table(42);
	`)
	if err != nil {
		t.Fatalf("console.table with primitive failed: %v", err)
	}
}

func TestConsoleTable_NestedObjectsCoverage(t *testing.T) {
	loop := goeventloop.New()
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	_, err = runtime.RunString(`
		console.table([{name: "Alice", details: {age: 30}}, {name: "Bob", details: {age: 25}}]);
	`)
	if err != nil {
		t.Fatalf("console.table with nested objects failed: %v", err)
	}
}
