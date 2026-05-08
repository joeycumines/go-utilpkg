//go:build unix

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunCommandExitStatus(t *testing.T) {
	for _, test := range []struct {
		name       string
		script     string
		wantCode   int
		wantReason string
		wantSignal string
	}{
		{name: "success", script: "exit 0", wantCode: 0, wantReason: "exit"},
		{name: "failure", script: "exit 7", wantCode: 7, wantReason: "exit"},
		{name: "literal 124", script: "exit 124", wantCode: 124, wantReason: "exit"},
		{name: "signal", script: "kill -TERM $$", wantCode: 143, wantReason: "signal", wantSignal: "terminated"},
	} {
		t.Run(test.name, func(t *testing.T) {
			code, status, _ := runShell(t, 5*time.Second, test.script)
			if code != test.wantCode {
				t.Fatalf("exit = %d, want %d", code, test.wantCode)
			}
			if status.Reason != test.wantReason || status.WrapperCode != test.wantCode ||
				!status.ContainmentOK || !status.ArtifactOK {
				t.Fatalf("status = %+v", status)
			}
			if status.ChildSignal != test.wantSignal {
				t.Fatalf("child signal = %q, want %q", status.ChildSignal, test.wantSignal)
			}
			if test.wantSignal != "" {
				if status.ChildCode != nil {
					t.Fatalf("signaled child code = %v, want absent", status.ChildCode)
				}
			} else if status.ChildCode == nil || *status.ChildCode != test.wantCode {
				t.Fatalf("child code = %v, want %d", status.ChildCode, test.wantCode)
			}
		})
	}
}

func TestRunCommandExactInputsAndOwnedStreams(t *testing.T) {
	arguments := []string{"", "space value", "quote'\"", "*?[", "line\nbreak", "世界", "equals=value"}
	script := "if IFS= read -r ignored; then exit 91; fi; " +
		"test \"$RUN_EXACT\" = \"value with spaces * ?\" || exit 92; " +
		"printf '%s\\0' \"$@\"; printf 'error\\0bytes' >&2"
	code, status, artifacts := runShellArguments(t, 5*time.Second, script, arguments,
		"RUN_EXACT=value with spaces * ?")
	if code != 0 || status.Reason != "exit" || !status.ContainmentOK || !status.ArtifactOK {
		t.Fatalf("result = %d, %+v", code, status)
	}
	wantStdout := make([]byte, 0)
	for _, argument := range arguments {
		wantStdout = append(wantStdout, argument...)
		wantStdout = append(wantStdout, 0)
	}
	assertRunOutput(t, artifacts, status.Stdout, wantStdout)
	assertRunOutput(t, artifacts, status.Stderr, []byte("error\x00bytes"))
}

func TestRunCommandTimeout(t *testing.T) {
	withProcessDeadlines(t, 100*time.Millisecond, time.Second)
	started := time.Now()
	code, status, _ := runShell(t, 50*time.Millisecond, "exec /bin/sleep 30")
	if code != 124 {
		t.Fatalf("timeout exit = %d, want 124", code)
	}
	if status.Reason != "timeout" || status.ChildCode != nil || !status.ContainmentOK || !status.ArtifactOK {
		t.Fatalf("timeout status = %+v", status)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("timeout took %s, want at most 3s", elapsed)
	}
}

func TestRunCommandKillsTermIgnoringDescendant(t *testing.T) {
	withProcessDeadlines(t, 100*time.Millisecond, time.Second)
	sourceRoot := tempPhysicalDir(t)
	pidPath := filepath.Join(sourceRoot, "descendant.pid")
	script := "trap 'exit 0' TERM; " +
		"(trap '' TERM; while :; do /bin/sleep 1; done) & " +
		"printf '%s' $! > " + shellQuote(pidPath) + "; " +
		"while :; do /bin/sleep 1; done"
	code, status, _ := runShellRoot(t, sourceRoot, 50*time.Millisecond, script, nil)
	if code != 124 || status.Reason != "timeout" || !status.ContainmentOK {
		t.Fatalf("timeout = %d, %+v", code, status)
	}
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("ReadFile descendant PID: %v", err)
	}
	pid, err := strconv.Atoi(string(pidData))
	if err != nil {
		t.Fatalf("descendant PID %q: %v", pidData, err)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("descendant PID %d remains live", pid)
	}
}

func TestRunCommandRejectsResidualDescendant(t *testing.T) {
	withProcessDeadlines(t, 100*time.Millisecond, time.Second)
	sourceRoot := tempPhysicalDir(t)
	pidPath := filepath.Join(sourceRoot, "descendant.pid")
	script := "(trap '' TERM; while :; do /bin/sleep 1; done) & " +
		"printf '%s' $! > " + shellQuote(pidPath) + "; exit 0"
	code, status, _ := runShellRoot(t, sourceRoot, 5*time.Second, script, nil)
	if code != 125 || status.Reason != "containment-error" || status.ContainmentOK ||
		!status.Scope.ResidualDetected || !status.Scope.Dead || !status.Scope.Closed {
		t.Fatalf("residual result = %d, %+v", code, status)
	}
}

func TestRunCommandArgumentValidation(t *testing.T) {
	shell := resolvedShell(t)
	for _, test := range []struct {
		name      string
		configure func(t *testing.T, sourceRoot, artifactRoot string) []string
	}{
		{
			name: "positional command",
			configure: func(_ *testing.T, sourceRoot, artifactRoot string) []string {
				return append(runFlags(sourceRoot, sourceRoot, artifactRoot), shell)
			},
		},
		{
			name: "directory outside source",
			configure: func(t *testing.T, sourceRoot, artifactRoot string) []string {
				return runFlags(sourceRoot, tempPhysicalDir(t), artifactRoot)
			},
		},
		{
			name: "bad label",
			configure: func(_ *testing.T, sourceRoot, artifactRoot string) []string {
				return append(runFlags(sourceRoot, sourceRoot, artifactRoot), "-label", "bad\nlabel")
			},
		},
		{
			name: "overlapping roots",
			configure: func(_ *testing.T, sourceRoot, _ string) []string {
				return runFlags(sourceRoot, sourceRoot, sourceRoot)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			sourceRoot := tempPhysicalDir(t)
			artifactRoot := tempPhysicalDir(t)
			writeRunInputs(t, artifactRoot, []string{shell, "-c", "exit 0"}, testRunEnvironment())
			if code := runCommand(test.configure(t, sourceRoot, artifactRoot)); code == 0 {
				t.Fatal("invalid run arguments unexpectedly passed")
			}
		})
	}
}

func TestRunCommandRejectsNonprivateAndUsedArtifactRoots(t *testing.T) {
	shell := resolvedShell(t)
	t.Run("nonprivate", func(t *testing.T) {
		sourceRoot := tempPhysicalDir(t)
		artifactRoot := tempPhysicalDir(t)
		if err := os.Chmod(artifactRoot, 0o755); err != nil {
			t.Fatalf("Chmod: %v", err)
		}
		writeRunInputs(t, artifactRoot, []string{shell, "-c", "exit 0"}, testRunEnvironment())
		if code := runCommand(runFlags(sourceRoot, sourceRoot, artifactRoot)); code == 0 {
			t.Fatal("nonprivate artifact root unexpectedly passed")
		}
	})
	t.Run("existing output", func(t *testing.T) {
		sourceRoot := tempPhysicalDir(t)
		artifactRoot := tempPhysicalDir(t)
		writeRunInputs(t, artifactRoot, []string{shell, "-c", "exit 0"}, testRunEnvironment())
		stdoutPath := filepath.Join(artifactRoot, runStdoutName)
		mustWriteFile(t, stdoutPath, []byte("preserve"), 0o640)
		if code := runCommand(runFlags(sourceRoot, sourceRoot, artifactRoot)); code == 0 {
			t.Fatal("used artifact root unexpectedly passed")
		}
		data, err := os.ReadFile(stdoutPath)
		if err != nil || string(data) != "preserve" {
			t.Fatalf("existing stdout = %q, %v", data, err)
		}
		info, err := os.Stat(stdoutPath)
		if err != nil || info.Mode().Perm() != 0o640 {
			t.Fatalf("existing stdout mode = %v, %v", info, err)
		}
	})
}

func TestRunCommandDoesNotInheritHostileEnvironment(t *testing.T) {
	t.Setenv("TOURNAMENT_HOSTILE_INHERITED", "present")
	code, status, _ := runShell(t, 5*time.Second, `test -z "${TOURNAMENT_HOSTILE_INHERITED+x}"`)
	if code != 0 || status.Reason != "exit" {
		t.Fatalf("clean environment result = %d, %+v", code, status)
	}
}

func runShell(t *testing.T, timeout time.Duration, script string) (int, runStatus, string) {
	t.Helper()
	return runShellRoot(t, tempPhysicalDir(t), timeout, script, nil)
}

func runShellArguments(
	t *testing.T,
	timeout time.Duration,
	script string,
	arguments []string,
	environment ...string,
) (int, runStatus, string) {
	t.Helper()
	return runShellRoot(t, tempPhysicalDir(t), timeout, script, append(environment, arguments...))
}

func runShellRoot(
	t *testing.T,
	sourceRoot string,
	timeout time.Duration,
	script string,
	extra []string,
) (int, runStatus, string) {
	t.Helper()
	artifactRoot := tempPhysicalDir(t)
	shell := resolvedShell(t)
	var arguments []string
	var environment []string
	for _, value := range extra {
		if strings.Contains(value, "=") && len(arguments) == 0 {
			environment = append(environment, value)
			continue
		}
		arguments = append(arguments, value)
	}
	argv := append([]string{shell, "-c", script, "test-script"}, arguments...)
	writeRunInputs(t, artifactRoot, argv, testRunEnvironment(environment...))
	code := runCommand(append(runFlags(sourceRoot, sourceRoot, artifactRoot), "-timeout", timeout.String()))
	statusPath := filepath.Join(artifactRoot, runStatusName)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("ReadFile status: %v", err)
	}
	var status runStatus
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatalf("Unmarshal status: %v\n%s", err, data)
	}
	if status.SchemaVersion != 2 || status.ElapsedNS <= 0 || status.Label != "test child" {
		t.Fatalf("status framing = %+v", status)
	}
	return code, status, artifactRoot
}

func runFlags(sourceRoot, directory, artifactRoot string) []string {
	return []string{
		"-timeout", "5s",
		"-root", sourceRoot,
		"-dir", directory,
		"-artifact-root", artifactRoot,
		"-label", "test child",
	}
}

func writeRunInputs(t *testing.T, artifactRoot string, argv, environment []string) {
	t.Helper()
	mustWriteNULFile(t, filepath.Join(artifactRoot, runArgvName), argv)
	mustWriteNULFile(t, filepath.Join(artifactRoot, runEnvironmentName), environment)
}

func resolvedShell(t *testing.T) string {
	t.Helper()
	shell, err := filepath.EvalSymlinks("/bin/sh")
	if err != nil {
		t.Fatalf("resolve /bin/sh: %v", err)
	}
	return shell
}

func tempPhysicalDir(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatalf("make temporary directory private: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatalf("resolve temporary directory: %v", err)
	}
	return resolved
}

func assertRunOutput(t *testing.T, root string, artifact runOutputArtifact, want []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, artifact.Name))
	if err != nil {
		t.Fatalf("ReadFile %s: %v", artifact.Name, err)
	}
	if string(data) != string(want) {
		t.Fatalf("%s = %q, want %q", artifact.Name, data, want)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	if artifact.Size != int64(len(data)) || artifact.SHA256 != digest {
		t.Fatalf("%s metadata = %+v, want size %d digest %s", artifact.Name, artifact, len(data), digest)
	}
	info, err := os.Stat(filepath.Join(root, artifact.Name))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("%s mode = %v, %v", artifact.Name, info, err)
	}
}

func withProcessDeadlines(t *testing.T, grace, kill time.Duration) {
	t.Helper()
	oldGrace := processGraceDeadline
	oldKill := processKillDeadline
	processGraceDeadline = grace
	processKillDeadline = kill
	t.Cleanup(func() {
		processGraceDeadline = oldGrace
		processKillDeadline = oldKill
	})
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
