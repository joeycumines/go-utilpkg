package debug

import (
	"log"
	"os"
	"strings"
	"testing"
)

// resetGlobals restores global state mutated during tests.
func resetGlobals() {
	enableAssert = false
	logger = log.New(os.Stdout, "", log.LstdFlags)
	if logfile != nil {
		_ = logfile.Close()
	}
	logfile = nil
}

func resetEnv() {
	_ = os.Unsetenv(envAssertPanic)
	_ = os.Unsetenv(envEnableLog)
}

func TestAssertPanicsWhenEnabled(t *testing.T) {
	t.Cleanup(resetGlobals)
	enableAssert = true
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic when assertions enabled")
		}
	}()
	Assert(false, "boom")
}

func TestAssertNoOpWhenConditionTrue(t *testing.T) {
	t.Cleanup(resetGlobals)
	enableAssert = true
	// Should not panic when condition is true
	Assert(true, "this should not panic")
}

func TestAssertWritesToStderrWhenDisabled(t *testing.T) {
	t.Cleanup(resetGlobals)
	enableAssert = false
	// Should not panic, just write to stderr
	Assert(false, "should not panic but log")
}

func TestAssertNoErrorNoOpWhenNil(t *testing.T) {
	t.Cleanup(resetGlobals)
	enableAssert = true
	// Should not panic when error is nil
	AssertNoError(nil)
}

func TestAssertNoErrorWritesToStderrWhenDisabled(t *testing.T) {
	t.Cleanup(resetGlobals)
	enableAssert = false
	// Should not panic, just write to stderr
	AssertNoError(os.ErrClosed)
}

func TestAssertNoErrorPanicsWhenEnabled(t *testing.T) {
	t.Cleanup(resetGlobals)
	enableAssert = true
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic on error with assertions enabled")
		}
	}()
	AssertNoError(os.ErrClosed)
}

func TestLogWritesWhenLoggerPresent(t *testing.T) {
	t.Cleanup(resetGlobals)
	t.Cleanup(resetEnv)

	tmp, err := os.CreateTemp("", "debug-log-*.log")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	logfile = tmp
	logger = log.New(logfile, "", 0)

	Log("hello-world")

	_ = logfile.Sync()
	data, err := os.ReadFile(logfile.Name())
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(data), "hello-world") {
		t.Fatalf("log output missing message, got %q", string(data))
	}
}

type mockStringer struct{ s string }

func (m mockStringer) String() string { return m.s }

func TestToStringVariants(t *testing.T) {
	t.Cleanup(resetGlobals)

	stringer := mockStringer{s: "stringer"}

	cases := map[string]string{
		"func":     toString(func() string { return "fn" }),
		"string":   toString("plain"),
		"stringer": toString(stringer),
		"fallback": toString(123),
	}

	if cases["func"] != "fn" {
		t.Fatalf("expected func case to return fn, got %q", cases["func"])
	}
	if cases["string"] != "plain" {
		t.Fatalf("expected string case to return plain, got %q", cases["string"])
	}
	if cases["stringer"] != "stringer" {
		t.Fatalf("expected stringer case to return stringer, got %q", cases["stringer"])
	}
	if cases["fallback"] == "" {
		t.Fatalf("fallback should produce message, got empty string")
	}
}

func TestInitEnablesFlagsFromEnv(t *testing.T) {
	t.Cleanup(resetGlobals)
	t.Cleanup(resetEnv)

	if err := os.Setenv(envAssertPanic, "1"); err != nil {
		t.Fatalf("setenv assert: %v", err)
	}
	if err := os.Setenv(envEnableLog, "true"); err != nil {
		t.Fatalf("setenv log: %v", err)
	}
	logfile = nil
	logger = nil
	enableAssert = false

	loadAssertEnv()
	loadLoggerEnv()
	if !enableAssert {
		t.Fatalf("expected assertions enabled from env")
	}
	if logger == nil {
		t.Fatalf("expected logger initialized")
	}
	Close()
}

func TestCloseWithNilLogfile(t *testing.T) {
	t.Cleanup(resetGlobals)
	logfile = nil
	// Should not panic
	Close()
}

func TestCloseWithOpenLogfile(t *testing.T) {
	t.Cleanup(resetGlobals)
	t.Cleanup(resetEnv)

	tmp, err := os.CreateTemp("", "debug-close-*.log")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	logfile = tmp
	logger = log.New(logfile, "", 0)

	// Write something
	Log("test message")

	// Close should not error
	Close()

	// After close, writing to the file should fail or be a no-op
	// The file handle is closed but logfile pointer is not nil
	// This is expected behavior based on the implementation
}

func TestWriteWithSyncError(t *testing.T) {
	t.Cleanup(resetGlobals)

	// Use an invalid fd to trigger write error
	// This should not panic
	writeWithSync(-1, "test message")
}

func TestLoadAssertEnvWithTrueValue(t *testing.T) {
	t.Cleanup(resetGlobals)
	t.Cleanup(resetEnv)

	_ = os.Setenv(envAssertPanic, "true")
	loadAssertEnv()
	if !enableAssert {
		t.Fatalf("expected assertions enabled with 'true' value")
	}
}

func TestLoadAssertEnvWithFalseValue(t *testing.T) {
	t.Cleanup(resetGlobals)
	t.Cleanup(resetEnv)

	_ = os.Setenv(envAssertPanic, "false")
	loadAssertEnv()
	if enableAssert {
		t.Fatalf("expected assertions disabled with 'false' value")
	}
}
