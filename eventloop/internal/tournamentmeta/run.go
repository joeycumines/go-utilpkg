package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"time"
)

var (
	processGraceDeadline = 5 * time.Second
	processKillDeadline  = 5 * time.Second
)

type runScopeStatus struct {
	Kind             string `json:"kind"`
	Started          bool   `json:"started"`
	ResidualDetected bool   `json:"residual_detected"`
	Forced           bool   `json:"forced"`
	Dead             bool   `json:"dead"`
	Closed           bool   `json:"closed"`
}

type runStatus struct { // betteralign:ignore canonical JSON field order
	SchemaVersion int               `json:"schema_version"`
	Label         string            `json:"label"`
	Reason        string            `json:"reason"`
	WrapperCode   int               `json:"wrapper_code"`
	ChildCode     *int              `json:"child_code,omitempty"`
	ChildSignal   string            `json:"child_signal,omitempty"`
	Forwarded     string            `json:"forwarded_signal,omitempty"`
	ElapsedNS     int64             `json:"elapsed_ns"`
	Diagnostic    string            `json:"diagnostic,omitempty"`
	ContainmentOK bool              `json:"containment_ok"`
	ArtifactOK    bool              `json:"artifact_ok"`
	StopAction    string            `json:"stop_action,omitempty"`
	Scope         runScopeStatus    `json:"scope"`
	Argv          runInputArtifact  `json:"argv"`
	Environment   runInputArtifact  `json:"environment"`
	Stdout        runOutputArtifact `json:"stdout"`
	Stderr        runOutputArtifact `json:"stderr"`
}

func runCommand(arguments []string) int {
	return runCommandOperations(arguments, defaultRunOperations())
}

func runCommandOperations(arguments []string, operations runOperations) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	timeout := flags.Duration("timeout", 0, "hard process deadline")
	root := flags.String("root", "", "physical source-snapshot root")
	directory := flags.String("dir", "", "physical child working directory")
	artifactRoot := flags.String("artifact-root", "", "physical private single-use artifact root")
	label := flags.String("label", "command", "diagnostic command label")
	if err := flags.Parse(arguments); err != nil {
		return commandError(err)
	}
	config, err := validateRunArguments(
		*timeout,
		*root,
		*directory,
		*artifactRoot,
		*label,
		flags.Args(),
	)
	if err != nil {
		return commandError(err)
	}
	artifacts, inputs, err := acquireRunArtifacts(config.artifactRoot)
	if err != nil {
		return commandError(err)
	}
	started := time.Now()
	status := runStatus{
		SchemaVersion: 2,
		Label:         config.label,
		ContainmentOK: true,
		ArtifactOK:    true,
		Scope: runScopeStatus{
			Kind: scopeKind(),
			Dead: true,
		},
		Argv:        inputs.argvArtifact,
		Environment: inputs.environmentArtifact,
	}

	spec := processSpec{
		Executable:  inputs.argv[0],
		Arguments:   inputs.argv,
		Directory:   config.directory,
		Environment: inputs.environment,
		Stdin:       artifacts.stdin,
		Stdout:      artifacts.stdout,
		Stderr:      artifacts.stderr,
	}
	if err := validateProcessSpec(spec); err != nil {
		status.Reason = "environment-error"
		status.WrapperCode = 125
		status.ArtifactOK = false
		status.Diagnostic = err.Error()
		return finishRun(artifacts, nil, started, status, operations)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, wrapperSignals()...)
	defer signal.Stop(signals)
	scope, err := startOwnedProcess(spec)
	if err != nil {
		status.Reason = "start-error"
		status.WrapperCode = 125
		status.ContainmentOK = false
		status.Diagnostic = fmt.Sprintf("start %s: %v", config.label, err)
		return finishRun(artifacts, nil, started, status, operations)
	}
	status.Scope.Started = true
	status.Scope.Dead = false

	wait := make(chan error, 1)
	go func() { wait <- scope.wait() }()
	timer := time.NewTimer(config.timeout)
	defer timer.Stop()

	select {
	case waitErr := <-wait:
		alive, aliveErr := scope.alive()
		if aliveErr != nil || alive {
			status.Scope.ResidualDetected = true
			status.Scope.Forced = true
			shutdownErr := stopOwnedProcess(scope, wait, true)
			status.Reason = "containment-error"
			status.WrapperCode = 125
			status.ContainmentOK = false
			status.Scope.Dead = shutdownErr == nil
			diagnostic := errors.New("child left a live process scope")
			if aliveErr != nil {
				diagnostic = fmt.Errorf("inspect child process scope: %w", aliveErr)
			}
			status.Diagnostic = joinedDiagnostics(diagnostic, shutdownErr)
		} else {
			status.Scope.Dead = true
			status.ChildCode, status.ChildSignal, status.WrapperCode = childProcessResult(waitErr)
			status.Reason = "exit"
			if status.ChildSignal != "" {
				status.Reason = "signal"
			}
		}
	case received := <-signals:
		forwardErr := scope.forward(received)
		status.Scope.Forced = true
		shutdownErr := stopOwnedProcess(scope, wait, false)
		status.Reason = "signal"
		status.WrapperCode = wrapperSignalCode(received)
		status.StopAction = scopeInterruptAction()
		if status.StopAction == "forward-signal" {
			status.Forwarded = received.String()
		}
		status.Diagnostic = joinedDiagnostics(forwardErr, shutdownErr)
		status.ContainmentOK = forwardErr == nil && shutdownErr == nil
		status.Scope.Dead = shutdownErr == nil
	case <-timer.C:
		status.Scope.Forced = true
		terminateErr := scope.terminate()
		shutdownErr := stopOwnedProcess(scope, wait, false)
		status.Reason = "timeout"
		status.WrapperCode = 124
		status.StopAction = scopeTimeoutAction()
		status.Diagnostic = joinedDiagnostics(terminateErr, shutdownErr)
		status.ContainmentOK = terminateErr == nil && shutdownErr == nil
		status.Scope.Dead = shutdownErr == nil
	}
	return finishRun(artifacts, scope, started, status, operations)
}

func finishRun(
	artifacts *runArtifacts,
	scope *ownedProcess,
	started time.Time,
	status runStatus,
	operations runOperations,
) int {
	var containmentErr error
	if scope != nil {
		containmentErr = operations.closeScope(scope)
		status.Scope.Closed = containmentErr == nil
		if containmentErr != nil {
			status.ContainmentOK = false
			status.Reason = "containment-error"
			status.WrapperCode = 125
		}
	}
	stdout, stderr, artifactErr := artifacts.finalize(operations)
	status.Stdout = stdout
	status.Stderr = stderr
	if artifactErr != nil {
		status.ArtifactOK = false
		if status.Reason != "containment-error" {
			status.Reason = "artifact-error"
		}
		status.WrapperCode = 125
	}
	status.Diagnostic = joinedDiagnostics(
		diagnosticError(status.Diagnostic),
		annotateError("close process scope", containmentErr),
		artifactErr,
	)
	status.ElapsedNS = time.Since(started).Nanoseconds()
	if err := operations.writeStatus(artifacts.statusPath, status); err != nil {
		return commandError(errors.Join(diagnosticError(status.Diagnostic), err))
	}
	return status.WrapperCode
}

func stopOwnedProcess(scope *ownedProcess, wait <-chan error, waitAlreadyReceived bool) error {
	var errs []error
	if !waitAlreadyReceived {
		select {
		case <-wait:
			waitAlreadyReceived = true
		case <-time.After(processGraceDeadline):
		}
	}
	if err := scope.kill(); err != nil {
		errs = append(errs, fmt.Errorf("force process scope: %w", err))
	}
	if !waitAlreadyReceived {
		select {
		case <-wait:
			waitAlreadyReceived = true
		case <-time.After(processKillDeadline):
			errs = append(errs, errors.New("direct child wait exceeded post-kill deadline"))
		}
	}
	deadline := time.Now().Add(processKillDeadline)
	for {
		alive, err := scope.alive()
		if err != nil {
			errs = append(errs, fmt.Errorf("inspect killed process scope: %w", err))
			break
		}
		if !alive {
			break
		}
		if time.Now().After(deadline) {
			errs = append(errs, errors.New("process scope remained live after force deadline"))
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.Join(errs...)
}

func joinedDiagnostics(values ...error) string {
	values = slices.DeleteFunc(values, func(value error) bool { return value == nil })
	if len(values) == 0 {
		return ""
	}
	return errors.Join(values...).Error()
}

func diagnosticError(value string) error {
	if value == "" {
		return nil
	}
	return errors.New(value)
}

func annotateError(label string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", label, err)
}

type runConfig struct {
	root         string
	directory    string
	artifactRoot string
	label        string
	timeout      time.Duration
}

func validateRunArguments(
	timeout time.Duration,
	root,
	directory,
	artifactRoot,
	label string,
	arguments []string,
) (runConfig, error) {
	if timeout <= 0 || len(arguments) != 0 {
		return runConfig{}, errors.New("run requires a positive -timeout and accepts no positional arguments")
	}
	if label == "" || strings.TrimSpace(label) != label || strings.ContainsAny(label, "\x00\r\n") {
		return runConfig{}, errors.New("run requires a nonempty single-line -label")
	}
	resolvedRoot, err := physicalDirectory(root, "run source root")
	if err != nil {
		return runConfig{}, err
	}
	resolvedDirectory, err := physicalDirectory(directory, "run directory")
	if err != nil {
		return runConfig{}, err
	}
	if !containedPath(resolvedRoot, resolvedDirectory) {
		return runConfig{}, fmt.Errorf("run directory %q escapes source root %q", directory, root)
	}
	resolvedArtifacts, err := physicalDirectory(artifactRoot, "run artifact root")
	if err != nil {
		return runConfig{}, err
	}
	if containedPath(resolvedRoot, resolvedArtifacts) || containedPath(resolvedArtifacts, resolvedRoot) {
		return runConfig{}, errors.New("run source and artifact roots must not overlap")
	}
	if err := validatePrivateRunRoot(resolvedArtifacts); err != nil {
		return runConfig{}, err
	}
	return runConfig{
		timeout:      timeout,
		root:         resolvedRoot,
		directory:    resolvedDirectory,
		artifactRoot: resolvedArtifacts,
		label:        label,
	}, nil
}

func physicalDirectory(path, label string) (string, error) {
	if path == "" || !isAbsoluteCleanPath(path) {
		return "", fmt.Errorf("%s must be an absolute clean path", label)
	}
	resolved, err := evalPhysicalPath(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", label, err)
	}
	if path != resolved {
		return "", fmt.Errorf("%s must be its resolved physical path: %q != %q", label, path, resolved)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", label, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s %q is not a directory", label, resolved)
	}
	return resolved, nil
}
