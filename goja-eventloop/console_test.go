//go:build linux || darwin

package gojaeventloop

import (
	"bytes"
	"context"
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
