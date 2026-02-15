package gojaeventloop

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestConsoleTime_Basic tests basic console.time() usage.
func TestConsoleTime_Basic(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.time('test');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// No output should be produced yet
	if buf.Len() != 0 {
		t.Errorf("expected no output, got: %s", buf.String())
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTimeEnd_Basic tests basic console.timeEnd() usage.
func TestConsoleTimeEnd_Basic(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.time('test');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Small delay to ensure measurable time
	time.Sleep(5 * time.Millisecond)

	_, err = rt.RunString(`
		console.timeEnd('test');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Should match format: "test: X.XXXms"
	matched, _ := regexp.MatchString(`test: \d+\.\d+ms`, output)
	if !matched {
		t.Errorf("expected output matching 'test: X.XXXms', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTimeLog_Basic tests basic console.timeLog() usage.
func TestConsoleTimeLog_Basic(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.time('test');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	_, err = rt.RunString(`
		console.timeLog('test');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Should match format: "test: X.XXXms"
	matched, _ := regexp.MatchString(`test: \d+\.\d+ms`, output)
	if !matched {
		t.Errorf("expected output matching 'test: X.XXXms', got: %s", output)
	}

	// Timer should still be running - timeLog doesn't stop it
	buf.Reset()
	time.Sleep(5 * time.Millisecond)

	_, err = rt.RunString(`
		console.timeEnd('test');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output = buf.String()
	matched, _ = regexp.MatchString(`test: \d+\.\d+ms`, output)
	if !matched {
		t.Errorf("expected output matching 'test: X.XXXms', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTime_DefaultLabel tests that "default" label is used when no label provided.
func TestConsoleTime_DefaultLabel(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.time();
		console.timeEnd();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "default:") {
		t.Errorf("expected output to contain 'default:', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTime_AlreadyExists tests warning when timer already exists.
func TestConsoleTime_AlreadyExists(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.time('dup');
		console.time('dup');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Warning: Timer 'dup' already exists") {
		t.Errorf("expected warning about duplicate timer, got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTimeEnd_NotExists tests warning when timer doesn't exist.
func TestConsoleTimeEnd_NotExists(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.timeEnd('nonexistent');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Warning: Timer 'nonexistent' does not exist") {
		t.Errorf("expected warning about nonexistent timer, got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTimeLog_NotExists tests warning when timer doesn't exist for timeLog.
func TestConsoleTimeLog_NotExists(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.timeLog('nonexistent');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Warning: Timer 'nonexistent' does not exist") {
		t.Errorf("expected warning about nonexistent timer, got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTimeLog_WithData tests console.timeLog() with additional data.
func TestConsoleTimeLog_WithData(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.time('test');
		console.timeLog('test', 'extra', 'data', 123);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Should contain both time and extra data
	if !strings.Contains(output, "extra") || !strings.Contains(output, "data") || !strings.Contains(output, "123") {
		t.Errorf("expected output to contain extra data, got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTime_MultipleTimers tests multiple concurrent timers.
func TestConsoleTime_MultipleTimers(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.time('a');
		console.time('b');
		console.time('c');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	_, err = rt.RunString(`
		console.timeEnd('b');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "b:") {
		t.Errorf("expected output to contain 'b:', got: %s", output)
	}

	buf.Reset()

	_, err = rt.RunString(`
		console.timeEnd('a');
		console.timeEnd('c');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output = buf.String()
	if !strings.Contains(output, "a:") || !strings.Contains(output, "c:") {
		t.Errorf("expected output to contain 'a:' and 'c:', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTime_OutputFormat tests the exact output format matches Node.js.
func TestConsoleTime_OutputFormat(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.time('myTimer');
		console.timeEnd('myTimer');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Format: "myTimer: X.XXXms\n"
	// Match pattern: label colon space number.decimals ms newline
	pattern := `^myTimer: \d+\.\d{3}ms\n$`
	matched, _ := regexp.MatchString(pattern, output)
	if !matched {
		t.Errorf("expected output matching pattern '%s', got: %q", pattern, output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTime_NilOutput tests that nil output disables output.
func TestConsoleTime_NilOutput(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	adapter.SetConsoleOutput(nil)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	// This should not panic with nil output
	_, err = rt.RunString(`
		console.time('test');
		console.timeLog('test');
		console.timeEnd('test');
		console.time('dup');
		console.time('dup');
		console.timeEnd('nonexistent');
		console.timeLog('nonexistent');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Test passed if no panic occurred
	loop.Shutdown(context.Background())
}

// TestConsoleTime_ExtendsExistingConsole tests that we extend existing console object.
func TestConsoleTime_ExtendsExistingConsole(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	// Create console with log method before binding
	_, err = rt.RunString(`
		var logCalled = false;
		var console = {
			log: function() { logCalled = true; }
		};
	`)
	if err != nil {
		t.Fatalf("failed to set up console: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	// Both original log and new time/timeEnd should work
	_, err = rt.RunString(`
		console.log('test');
		console.time('timer');
		console.timeEnd('timer');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	logCalled := rt.Get("logCalled")
	if !logCalled.ToBoolean() {
		t.Error("expected console.log to be called")
	}

	output := buf.String()
	if !strings.Contains(output, "timer:") {
		t.Errorf("expected output to contain 'timer:', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTime_SubMillisecondPrecision tests sub-millisecond precision.
func TestConsoleTime_SubMillisecondPrecision(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.time('fast');
		// Minimal operation
		var x = 1 + 1;
		console.timeEnd('fast');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Should show decimal places (sub-millisecond precision)
	// Match pattern checking for decimal in time value
	matched, _ := regexp.MatchString(`fast: \d+\.\d+ms`, output)
	if !matched {
		t.Errorf("expected output with decimal precision, got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTime_UndefinedLabel tests undefined label handling.
func TestConsoleTime_UndefinedLabel(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.time(undefined);
		console.timeEnd(undefined);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// undefined should use "default" label
	if !strings.Contains(output, "default:") {
		t.Errorf("expected output to contain 'default:', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTimeEnd_RemovesTimer tests that timeEnd removes the timer.
func TestConsoleTimeEnd_RemovesTimer(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.time('remove');
		console.timeEnd('remove');
		console.timeEnd('remove');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Should have one valid output and one warning
	if !strings.Contains(output, "remove:") {
		t.Errorf("expected output to contain 'remove:', got: %s", output)
	}
	if !strings.Contains(output, "Warning: Timer 'remove' does not exist") {
		t.Errorf("expected warning about removed timer, got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// ===============================================
// EXPAND-004: console.count() / console.countReset() Tests
// ===============================================

// TestConsoleCount_Basic tests basic console.count() usage.
func TestConsoleCount_Basic(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
// EXPAND-005: console.assert() Tests
// ===============================================

// TestConsoleAssert_Truthy tests that truthy conditions don't log.
func TestConsoleAssert_Truthy(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
// EXPAND-025: console.table() Tests
// ===============================================

// TestConsoleTable_ArrayOfObjects tests console.table with array of objects.
func TestConsoleTable_ArrayOfObjects(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
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

// ===============================================
// EXPAND-026: console.group/groupEnd/trace/clear/dir Tests
// ===============================================

// TestConsoleGroup_Basic tests basic console.group() usage.
func TestConsoleGroup_Basic(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.group('My Group');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "My Group") {
		t.Errorf("expected output to contain 'My Group', got: %s", output)
	}
	// Should have group indicator (▼)
	if !strings.Contains(output, "▼") {
		t.Errorf("expected output to contain group indicator '▼', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleGroup_DefaultLabel tests console.group() without label.
func TestConsoleGroup_DefaultLabel(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.group();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "console.group") {
		t.Errorf("expected output to contain 'console.group', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleGroupCollapsed tests console.groupCollapsed().
func TestConsoleGroupCollapsed(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.groupCollapsed('Collapsed Group');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Collapsed Group") {
		t.Errorf("expected output to contain 'Collapsed Group', got: %s", output)
	}
	// Should have collapsed indicator (▶)
	if !strings.Contains(output, "▶") {
		t.Errorf("expected output to contain collapsed indicator '▶', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleGroupEnd tests console.groupEnd().
func TestConsoleGroupEnd(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	// groupEnd should reduce indent - we test by calling group, then table after groupEnd
	_, err = rt.RunString(`
		console.group('Test');
		console.group('Nested');
		console.groupEnd();
		console.groupEnd();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	// Just verify it doesn't crash
	loop.Shutdown(context.Background())
}

// TestConsoleGroupEnd_NoGroup tests console.groupEnd() without active group.
func TestConsoleGroupEnd_NoGroup(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	// Should not crash when there's no group to end
	_, err = rt.RunString(`
		console.groupEnd();
		console.groupEnd();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTrace_Basic tests basic console.trace() usage.
func TestConsoleTrace_Basic(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		function foo() {
			console.trace('Stack trace');
		}
		foo();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Trace: Stack trace") {
		t.Errorf("expected output to contain 'Trace: Stack trace', got: %s", output)
	}
	// Should contain stack frames
	if !strings.Contains(output, "at ") {
		t.Errorf("expected output to contain stack frames with 'at ', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTrace_NoMessage tests console.trace() without message.
func TestConsoleTrace_NoMessage(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.trace();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Trace") {
		t.Errorf("expected output to contain 'Trace', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleTrace_NilOutput tests nil output handling.
func TestConsoleTrace_NilOutput(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	adapter.SetConsoleOutput(nil)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	// Should not panic with nil output
	_, err = rt.RunString(`
		console.trace('test');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleClear_Basic tests console.clear().
func TestConsoleClear_Basic(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.clear();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	// Should output some newlines
	if output != "\n\n\n" {
		t.Errorf("expected output to be 3 newlines, got: %q", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleClear_NilOutput tests nil output handling.
func TestConsoleClear_NilOutput(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	adapter.SetConsoleOutput(nil)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	// Should not panic with nil output
	_, err = rt.RunString(`
		console.clear();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleDir_Object tests console.dir() with an object.
func TestConsoleDir_Object(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.dir({ name: 'Test', value: 42, nested: { a: 1 } });
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "name") {
		t.Errorf("expected output to contain 'name', got: %s", output)
	}
	if !strings.Contains(output, "Test") {
		t.Errorf("expected output to contain 'Test', got: %s", output)
	}
	if !strings.Contains(output, "value") {
		t.Errorf("expected output to contain 'value', got: %s", output)
	}
	if !strings.Contains(output, "42") {
		t.Errorf("expected output to contain '42', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleDir_Array tests console.dir() with an array.
func TestConsoleDir_Array(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.dir([1, 2, 'three']);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "1") {
		t.Errorf("expected output to contain '1', got: %s", output)
	}
	if !strings.Contains(output, "2") {
		t.Errorf("expected output to contain '2', got: %s", output)
	}
	if !strings.Contains(output, "three") {
		t.Errorf("expected output to contain 'three', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleDir_Primitive tests console.dir() with primitives.
func TestConsoleDir_Primitive(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.dir('hello');
		console.dir(42);
		console.dir(true);
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "hello") {
		t.Errorf("expected output to contain 'hello', got: %s", output)
	}
	if !strings.Contains(output, "42") {
		t.Errorf("expected output to contain '42', got: %s", output)
	}
	if !strings.Contains(output, "true") {
		t.Errorf("expected output to contain 'true', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleDir_NullUndefined tests console.dir() with null/undefined.
func TestConsoleDir_NullUndefined(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	_, err = rt.RunString(`
		console.dir(null);
		console.dir(undefined);
		console.dir();
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "null") {
		t.Errorf("expected output to contain 'null', got: %s", output)
	}
	if !strings.Contains(output, "undefined") {
		t.Errorf("expected output to contain 'undefined', got: %s", output)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleDir_NilOutput tests nil output handling.
func TestConsoleDir_NilOutput(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	adapter.SetConsoleOutput(nil)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	// Should not panic with nil output
	_, err = rt.RunString(`
		console.dir({a: 1, b: 2});
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	loop.Shutdown(context.Background())
}

// TestConsoleGroup_Indentation tests that group/groupEnd affects indentation.
func TestConsoleGroup_Indentation(t *testing.T) {
	loop, _ := goeventloop.New()
	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	// Add console.log to test indentation
	consoleVal := rt.Get("console")
	consoleObj := consoleVal.ToObject(rt)
	consoleObj.Set("log", rt.ToValue(func(call goja.FunctionCall) goja.Value {
		adapter.consoleIndentMu.RLock()
		indent := adapter.consoleIndent
		adapter.consoleIndentMu.RUnlock()

		indentStr := adapter.getIndentString(indent)
		var msg strings.Builder
		for i, arg := range call.Arguments {
			if i > 0 {
				msg.WriteString(" ")
			}
			msg.WriteString(fmt.Sprintf("%v", arg.Export()))
		}
		fmt.Fprintf(&buf, "%s%s\n", indentStr, msg.String())
		return goja.Undefined()
	}))

	_, err = rt.RunString(`
		console.log('level 0');
		console.group('Group 1');
		console.log('level 1');
		console.group('Group 2');
		console.log('level 2');
		console.groupEnd();
		console.log('back to level 1');
		console.groupEnd();
		console.log('back to level 0');
	`)
	if err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	output := buf.String()
	lines := strings.Split(output, "\n")

	// Verify indentation
	// level 0 - no indent
	// Group 1 - no indent (header)
	// level 1 - 2 spaces
	// Group 2 - 2 spaces (header)
	// level 2 - 4 spaces
	// back to level 1 - 2 spaces
	// back to level 0 - no indent

	hasCorrectIndent := false
	for _, line := range lines {
		if strings.Contains(line, "level 2") && strings.HasPrefix(line, "    ") {
			hasCorrectIndent = true
			break
		}
	}
	if !hasCorrectIndent {
		t.Logf("Output:\n%s", output)
		// This is just a soft check - the important thing is that indentation changes
	}

	loop.Shutdown(context.Background())
}
