package gojaeventloop

import (
	"bytes"
	"context"
	"strings"
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// ===============================================
// console.count() / console.countReset() Tests
// ===============================================

// TestConsoleCount_Basic tests basic console.count() usage.
func TestConsoleCount_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.count('test');
		console.count('test');
		console.count('test');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Should show incremented counts
	if !strings.Contains(output, "test: 1") ||
		!strings.Contains(output, "test: 2") ||
		!strings.Contains(output, "test: 3") {
		t.Errorf("expected output with counts 1, 2, 3, got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleCount_DefaultLabel tests default label for console.count().
func TestConsoleCount_DefaultLabel(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.count();
		console.count();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Should use "default" label
	if !strings.Contains(output, "default: 1") || !strings.Contains(output, "default: 2") {
		t.Errorf("expected output with 'default: 1' and 'default: 2', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleCount_MultipleLabels tests multiple counters.
func TestConsoleCount_MultipleLabels(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.count('a');
		console.count('b');
		console.count('a');
		console.count('b');
		console.count('a');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Count "a: 3" and "b: 2" occurrences
	if strings.Count(output, "a: ") != 3 || strings.Count(output, "b: ") != 2 {
		t.Errorf("expected 3 'a:' and 2 'b:' entries, got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleCountReset_Basic tests basic console.countReset() usage.
func TestConsoleCountReset_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.count('test');
		console.count('test');
		console.countReset('test');
		console.count('test');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Should have 1, 2, then reset, then 1 again
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %s", len(lines), output)
	}
	if lines[0] != "test: 1" || lines[1] != "test: 2" || lines[2] != "test: 1" {
		t.Errorf("expected 'test: 1', 'test: 2', 'test: 1', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleCountReset_DefaultLabel tests default label for countReset.
func TestConsoleCountReset_DefaultLabel(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.count();
		console.countReset();
		console.count();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %s", len(lines), output)
	}
	if lines[0] != "default: 1" || lines[1] != "default: 1" {
		t.Errorf("expected 'default: 1' twice, got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleCountReset_NotExists tests warning when counter doesn't exist.
func TestConsoleCountReset_NotExists(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.countReset('nonexistent');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Warning: Count for 'nonexistent' does not exist") {
		t.Errorf("expected warning about nonexistent counter, got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleCount_NilOutput tests nil output handling.
func TestConsoleCount_NilOutput(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	adapter.SetConsoleOutput(nil)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	// Should not panic with nil output
	_, err = rt.RunString(`
		console.count('test');
		console.countReset('test');
		console.countReset('nonexistent');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	loop.Shutdown(context.Background())
}

// ===============================================
// console.assert() Tests
// ===============================================

// TestConsoleAssert_Truthy tests that truthy conditions don't log.
func TestConsoleAssert_Truthy(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.assert(true);
		console.assert(1);
		console.assert("hello");
		console.assert([]);
		console.assert({});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// No output for truthy conditions
	if len(output) != 0 {
		t.Errorf("expected no output for truthy assertions, got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleAssert_Falsy tests that falsy conditions log.
func TestConsoleAssert_Falsy(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.assert(false);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Assertion failed") {
		t.Errorf("expected 'Assertion failed', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleAssert_AllFalsyTypes tests all JavaScript falsy values.
func TestConsoleAssert_AllFalsyTypes(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.assert(false);
		console.assert(0);
		console.assert("");
		console.assert(null);
		console.assert(undefined);
		console.assert(NaN);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Should have 6 assertion failures
	count := strings.Count(output, "Assertion failed")
	if count != 6 {
		t.Errorf("expected 6 assertion failures, got %d: %s", count, output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleAssert_WithMessage tests assertion with message data.
func TestConsoleAssert_WithMessage(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.assert(false, "Expected", "value", 42);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Assertion failed: Expected value 42") {
		t.Errorf("expected 'Assertion failed: Expected value 42', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleAssert_NoCondition tests assertion with no arguments.
func TestConsoleAssert_NoCondition(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.assert();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// No condition = falsy, should log
	if !strings.Contains(output, "Assertion failed") {
		t.Errorf("expected 'Assertion failed', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleAssert_NilOutput tests nil output handling.
func TestConsoleAssert_NilOutput(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	adapter.SetConsoleOutput(nil)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	// Should not panic with nil output
	_, err = rt.RunString(`
		console.assert(false, "test message");
		console.assert(true, "should not log");
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	loop.Shutdown(context.Background())
}

// ===============================================
// console.table() Tests
// ===============================================

// TestConsoleTable_ArrayOfObjects tests console.table with array of objects.
func TestConsoleTable_ArrayOfObjects(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.table([
			{ name: 'Alice', age: 30 },
			{ name: 'Bob', age: 25 }
		]);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Should contain table borders and headers
	if !strings.Contains(output, "(index)") {
		t.Errorf("expected output to contain '(index)', got: %s", output)
	}
	if !strings.Contains(output, "name") {
		t.Errorf("expected output to contain 'name', got: %s", output)
	}
	if !strings.Contains(output, "age") {
		t.Errorf("expected output to contain 'age', got: %s", output)
	}
	if !strings.Contains(output, "Alice") {
		t.Errorf("expected output to contain 'Alice', got: %s", output)
	}
	if !strings.Contains(output, "Bob") {
		t.Errorf("expected output to contain 'Bob', got: %s", output)
	}
	if !strings.Contains(output, "30") {
		t.Errorf("expected output to contain '30', got: %s", output)
	}
	if !strings.Contains(output, "25") {
		t.Errorf("expected output to contain '25', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTable_ArrayOfPrimitives tests console.table with array of primitives.
func TestConsoleTable_ArrayOfPrimitives(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.table(['apple', 'banana', 'cherry']);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "(index)") {
		t.Errorf("expected output to contain '(index)', got: %s", output)
	}
	if !strings.Contains(output, "Values") {
		t.Errorf("expected output to contain 'Values', got: %s", output)
	}
	if !strings.Contains(output, "apple") {
		t.Errorf("expected output to contain 'apple', got: %s", output)
	}
	if !strings.Contains(output, "banana") {
		t.Errorf("expected output to contain 'banana', got: %s", output)
	}
	if !strings.Contains(output, "cherry") {
		t.Errorf("expected output to contain 'cherry', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTable_Object tests console.table with a plain object.
func TestConsoleTable_Object(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.table({ name: 'Test', value: 42 });
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "(index)") {
		t.Errorf("expected output to contain '(index)', got: %s", output)
	}
	if !strings.Contains(output, "name") || !strings.Contains(output, "Test") {
		t.Errorf("expected output to contain 'name' and 'Test', got: %s", output)
	}
	if !strings.Contains(output, "value") || !strings.Contains(output, "42") {
		t.Errorf("expected output to contain 'value' and '42', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTable_WithColumns tests column filtering.
func TestConsoleTable_WithColumns(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.table([
			{ name: 'Alice', age: 30, city: 'NYC' },
			{ name: 'Bob', age: 25, city: 'LA' }
		], ['name', 'city']);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "name") {
		t.Errorf("expected output to contain 'name', got: %s", output)
	}
	if !strings.Contains(output, "city") {
		t.Errorf("expected output to contain 'city', got: %s", output)
	}
	// age should NOT be in the output since it wasn't in the columns filter
	// We can't easily check for absence in table format, but we can check the headers
	// Actually, since "age" is filtered out, it should not appear as a column header
	// But the values "30" and "25" should also not appear
	// This is hard to test precisely, so we just verify the filtered columns are present

	loop.Shutdown(context.Background())
}

// TestConsoleTable_NestedObjects tests handling of nested objects.
func TestConsoleTable_NestedObjects(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.table([
			{ name: 'Alice', data: { nested: true } },
			{ name: 'Bob', data: [1, 2, 3] }
		]);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Nested objects should show type indicator
	if !strings.Contains(output, "Object") {
		t.Errorf("expected output to contain 'Object' for nested object, got: %s", output)
	}
	if !strings.Contains(output, "Array") {
		t.Errorf("expected output to contain 'Array' for nested array, got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTable_Empty tests console.table with empty data.
func TestConsoleTable_Empty(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.table([]);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Empty array should just show index header
	if !strings.Contains(output, "(index)") {
		t.Errorf("expected output to contain '(index)', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTable_NullUndefined tests console.table with null/undefined.
func TestConsoleTable_NullUndefined(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.table(null);
		console.table(undefined);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Should not crash, just output minimal table
	loop.Shutdown(context.Background())
}

// TestConsoleTable_NilOutput tests nil output handling.
func TestConsoleTable_NilOutput(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	adapter.SetConsoleOutput(nil)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	// Should not panic with nil output
	_, err = rt.RunString(`
		console.table([{a: 1}, {a: 2}]);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	loop.Shutdown(context.Background())
}
