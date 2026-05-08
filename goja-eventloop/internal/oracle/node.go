package oracle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const (
	maxEngineOutputBytes = 4 << 20
	nodeFrame            = "GOJA_EVENTLOOP_ORACLE_V1:"
)

const nodeIdentityProgram = `
"use strict";
process.stdout.write(JSON.stringify({
  version: process.versions.node,
  v8: process.versions.v8,
  platform: process.platform,
  arch: process.arch,
  executable: process.execPath,
  release: process.release
}));
`

const nodeFixtureProgram = `
"use strict";
const fs = require("node:fs");
const request = JSON.parse(fs.readFileSync(0, "utf8"));
let observation;
let completed = false;
try {
  Object.defineProperty(globalThis, "__oracleCaptureConsole", {
    configurable: true,
    enumerable: false,
    writable: false,
    value: function captureConsole(callback) {
      if (typeof callback !== "function") throw new TypeError("console capture callback must be a function");
      const maximum = 4 << 20;
      const chunks = [];
      let size = 0;
      let overflow = false;
      const streams = process.stdout === process.stderr ? [process.stdout] : [process.stdout, process.stderr];
      const descriptors = [];
      function captureWrite(chunk, encoding, complete) {
        if (typeof encoding === "function") {
          complete = encoding;
          encoding = undefined;
        }
        const data = Buffer.isBuffer(chunk) ? Buffer.from(chunk) : Buffer.from(String(chunk), encoding);
        const remaining = maximum - size;
        if (remaining > 0) chunks.push(data.subarray(0, remaining));
        size += data.length;
        if (size > maximum) overflow = true;
        if (typeof complete === "function") complete();
        return true;
      }
      try {
        for (const stream of streams) {
          const descriptor = Object.getOwnPropertyDescriptor(stream, "write");
          descriptors.push([stream, descriptor]);
          Object.defineProperty(stream, "write", {
            configurable: true,
            enumerable: descriptor ? Boolean(descriptor.enumerable) : false,
            writable: true,
            value: captureWrite,
          });
        }
        callback();
      } finally {
        for (let index = descriptors.length - 1; index >= 0; index -= 1) {
          const [stream, descriptor] = descriptors[index];
          if (descriptor) Object.defineProperty(stream, "write", descriptor);
          else delete stream.write;
        }
      }
      if (overflow) throw new RangeError("console capture exceeded " + maximum + " bytes");
      return Buffer.concat(chunks).toString("utf8");
    },
  });
  (0, eval)(request.harnessSource);
  const oracle = globalThis.__gojaEventloopOracle;
  oracle.setup(request.setup || {}, request.input || {});
  oracle.checkpoint();
  const fixture = (0, eval)("(" + request.fixtureSource + "\n)");
  oracle.run(fixture, request.input || {}).then(
    function fulfilled(value) {
      observation = value;
      completed = true;
      oracle.restore();
      process.once("exit", function emitOracleObservation() {
        fs.writeSync(1, "GOJA_EVENTLOOP_ORACLE_V1:" + oracle.encode(observation) + "\n");
      });
    },
    function rejected(error) {
      process.stderr.write(String(error && error.stack || error) + "\n");
      process.exitCode = 70;
    }
  );
} catch (error) {
  process.stderr.write(String(error && error.stack || error) + "\n");
  process.exitCode = 70;
}
`

type nodeRequest struct {
	Input         any    `json:"input"`
	HarnessSource string `json:"harnessSource"`
	FixtureSource string `json:"fixtureSource"`
	Setup         Setup  `json:"setup"`
}

type cappedBuffer struct {
	buffer bytes.Buffer
	limit  int
	over   bool
}

func (w *cappedBuffer) Write(data []byte) (int, error) {
	original := len(data)
	remaining := w.limit - w.buffer.Len()
	if remaining <= 0 {
		w.over = true
		return original, nil
	}
	if len(data) > remaining {
		w.over = true
		data = data[:remaining]
	}
	_, _ = w.buffer.Write(data)
	return original, nil
}

func resolveNode(ctx context.Context, artifact *nodeArtifact) (NodeIdentity, error) {
	stdout, stderr, runErr := artifact.runProcess(ctx, []string{"--no-warnings", "-e", nodeIdentityProgram}, nil)
	if runErr != nil {
		return NodeIdentity{}, fmt.Errorf("query Node identity: %w%s", runErr, diagnosticSuffix(stderr))
	}
	var identity NodeIdentity
	if err := decodeStrict(stdout, &identity); err != nil {
		return NodeIdentity{}, fmt.Errorf("decode Node identity: %w", err)
	}
	if identity.Version != NodeVersion {
		return NodeIdentity{}, fmt.Errorf("node version is %q, want exactly %q", identity.Version, NodeVersion)
	}
	if identity.V8 == "" || identity.Platform == "" || identity.Arch == "" || identity.Release["name"] != "node" {
		return NodeIdentity{}, errors.New("node identity response is incomplete")
	}
	if err := validateSelectedNodeIdentity(artifact.pin, identity); err != nil {
		return NodeIdentity{}, err
	}
	wantReleasePart := "/release/" + NodeTag + "/node-" + NodeTag
	if !strings.Contains(identity.Release["sourceUrl"], wantReleasePart+".tar.gz") ||
		!strings.Contains(identity.Release["headersUrl"], wantReleasePart+"-headers.tar.gz") {
		return NodeIdentity{}, fmt.Errorf("node release metadata does not identify exact tag %s", NodeTag)
	}
	identity.Executable = artifact.pin.Entry
	identity.ExecutableSHA256 = artifact.executableSHA256
	identity.Artifact = artifact.identity()
	return identity, nil
}

func runNodeFixture(ctx context.Context, artifact *nodeArtifact, manifest *LoadedManifest, fixture Fixture, input any) (json.RawMessage, error) {
	request := nodeRequest{
		HarnessSource: string(manifest.Harness),
		FixtureSource: string(manifest.Fixtures[fixture.ID]),
		Setup:         fixture.Setup,
		Input:         input,
	}
	requestData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	stdout, stderr, runErr := artifact.runProcess(ctx, []string{"--no-warnings", "-e", nodeFixtureProgram}, requestData)
	if runErr != nil {
		return nil, fmt.Errorf("node fixture: %w%s", runErr, diagnosticSuffix(stderr))
	}
	return parseNodeObservation(stdout, stderr)
}

func parseNodeObservation(stdout, stderr []byte) (json.RawMessage, error) {
	if len(bytes.TrimSpace(stderr)) != 0 {
		return nil, fmt.Errorf("node protocol: unexpected stderr%s", diagnosticSuffix(stderr))
	}
	lines := bytes.Split(bytes.TrimSpace(stdout), []byte{'\n'})
	if len(lines) != 1 || !bytes.HasPrefix(lines[0], []byte(nodeFrame)) {
		return nil, fmt.Errorf("node protocol: expected one framed line, got %q%s", truncate(stdout, 512), diagnosticSuffix(stderr))
	}
	observation, _, err := canonicalJSON(bytes.TrimPrefix(lines[0], []byte(nodeFrame)))
	if err != nil {
		return nil, fmt.Errorf("node protocol observation: %w", err)
	}
	return observation, nil
}

func runProcess(ctx context.Context, path string, args []string, stdin []byte) ([]byte, []byte, error) {
	command := exec.CommandContext(ctx, path, args...)
	return captureProcess(ctx, command, stdin)
}

func (a *nodeArtifact) runProcess(ctx context.Context, args []string, stdin []byte) ([]byte, []byte, error) {
	if err := a.verify(); err != nil {
		return nil, nil, err
	}
	var command *exec.Cmd
	if a.launchMode == "proc-self-fd" {
		command = exec.CommandContext(ctx, "/proc/self/fd/3", args...)
		command.ExtraFiles = []*os.File{a.executable}
	} else {
		command = exec.CommandContext(ctx, a.executablePath, args...)
	}
	stdout, stderr, runErr := captureProcess(ctx, command, stdin)
	if verifyErr := a.verify(); verifyErr != nil {
		runErr = errors.Join(runErr, verifyErr)
	}
	return stdout, stderr, runErr
}

func captureProcess(ctx context.Context, command *exec.Cmd, stdin []byte) ([]byte, []byte, error) {
	command.Env = oracleEnvironment()
	command.Stdin = bytes.NewReader(stdin)
	stdout := &cappedBuffer{limit: maxEngineOutputBytes}
	stderr := &cappedBuffer{limit: maxEngineOutputBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return stdout.buffer.Bytes(), stderr.buffer.Bytes(), fmt.Errorf("timeout or cancellation: %w", ctxErr)
	}
	if stdout.over || stderr.over {
		return stdout.buffer.Bytes(), stderr.buffer.Bytes(), errors.New("engine output exceeded 4 MiB")
	}
	return stdout.buffer.Bytes(), stderr.buffer.Bytes(), err
}

func oracleEnvironment() []string {
	environment := []string{"LANG=C", "LC_ALL=C", "TZ=UTC", "NODE_DISABLE_COLORS=1", "NO_COLOR=1"}
	for _, name := range []string{"HOME", "TMPDIR", "SystemRoot", "WINDIR"} {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}

func diagnosticSuffix(data []byte) string {
	value := strings.TrimSpace(string(data))
	if value == "" {
		return ""
	}
	return ": " + truncate([]byte(value), 1024)
}

func truncate(data []byte, limit int) string {
	if len(data) <= limit {
		return string(data)
	}
	return string(data[:limit]) + "..."
}

var _ io.Writer = (*cappedBuffer)(nil)
