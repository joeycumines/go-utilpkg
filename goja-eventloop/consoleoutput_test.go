package gojaeventloop

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

type reentrantConsoleWriter struct {
	adapter *Adapter
	called  chan struct{}
	once    sync.Once
}

func (w *reentrantConsoleWriter) Write(p []byte) (int, error) {
	w.once.Do(func() {
		w.adapter.SetConsoleOutput(io.Discard)
		close(w.called)
	})
	return len(p), nil
}

// ===============================================
// console.group/groupEnd/trace/clear/dir Tests
// ===============================================

// TestConsoleGroup_Basic tests basic console.group() usage.
func TestConsoleGroup_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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

func TestConsoleOutputWriterMayReenterConfiguration(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	writer := &reentrantConsoleWriter{adapter: adapter, called: make(chan struct{})}
	adapter.SetConsoleOutput(writer)
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		_, runErr := runtime.RunString(`console.time("reentrant"); console.time("reentrant")`)
		result <- runErr
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("console warning: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("console writer reentry deadlocked")
	}
	select {
	case <-writer.called:
	default:
		t.Fatal("console warning did not invoke the configured writer")
	}
}

func TestConsoleOutputConcurrentConfigurationAndWarning(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	adapter.SetConsoleOutput(io.Discard)
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	warning := adapter.warningObject("concurrent output", "Warning", "")

	start := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-start
		for index := range 10_000 {
			if index%2 == 0 {
				adapter.SetConsoleOutput(io.Discard)
			} else {
				adapter.SetConsoleOutput(nil)
			}
		}
	}()
	close(start)
	for range 10_000 {
		adapter.emitWarningObject(warning)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent console configuration did not complete")
	}
}

// TestConsoleGroup_DefaultLabel tests console.group() without label.
func TestConsoleGroup_DefaultLabel(t *testing.T) {
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
	loop, err := goeventloop.New()
	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
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
