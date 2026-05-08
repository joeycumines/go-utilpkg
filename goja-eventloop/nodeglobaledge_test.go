package gojaeventloop

import (
	"bytes"
	"strings"
	"testing"
)

// Node console and process global edge coverage.

func TestConsoleTable_ObjectColumnsFilter_CoverGap(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table({
			alice: { age: 30, role: "admin", score: 95 },
			bob: { age: 25, role: "user", score: 85 }
		}, ["role"]);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "admin") {
		t.Error("Expected 'admin' in filtered object table")
	}
}

func TestConsoleTable_SingleRowSingleCol(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table([{x:1}]);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Expected non-empty table output")
	}
}

func TestConsoleTime_DuplicateLabel(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.time("dup");
		console.time("dup"); // duplicate - should warn
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsoleTimeEnd_MissingLabel(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.timeEnd("nonexistent");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !strings.Contains(buf.String(), "nonexistent") {
		t.Errorf("Expected warning about nonexistent timer, got: %s", buf.String())
	}
}

func TestConsoleTimeLog_MissingLabel(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.timeLog("missing");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !strings.Contains(buf.String(), "missing") {
		t.Errorf("Expected warning about missing timer, got: %s", buf.String())
	}
}

func TestConsoleTable_UndefinedData(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table(undefined);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !strings.Contains(buf.String(), "(index)") {
		t.Errorf("Expected (index) in output, got: %s", buf.String())
	}
}

func TestConsoleTable_NullData(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table(null);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsoleTable_PrimitiveString(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table("hello world");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !strings.Contains(buf.String(), "hello world") {
		t.Errorf("Expected 'hello world' in output, got: %s", buf.String())
	}
}

func TestConsoleTable_EmptyArray_CoverG3(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table([]);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestProcess_EnvAccess(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var hasProcess = typeof process !== "undefined";
		var envType = typeof process.env;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("hasProcess").ToBoolean() {
		t.Error("Expected process to be defined")
	}
}
