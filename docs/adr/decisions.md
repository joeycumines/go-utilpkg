# Architecture Decision Records

## ADR-001: Require Pattern for Module Initialization

**Status**: Accepted

**Context**: Module initialization in Goja (JavaScript) needs a clean API pattern.

**Decision**: Use exported `Require(runtime *goja.Runtime, module *goja.Object)` for
nodejs-compatible `require()` pattern, and `New(runtime, ...options)` constructor for
direct Go usage.

**Rationale**: Consistent with established Goja ecosystem patterns. `New()` returns
`(*Module, error)` allowing flexible initialization. `Require()` panics on setup errors
since `require()` in JavaScript doesn't have error returns.

---

## ADR-002: Interface-Based Option Pattern

**Status**: Accepted

**Context**: Configurable components need extensible initialization.

**Decision**: All configurable types use interface-based options (study
`inprocgrpc/options.go`). Options are functions that return errors for invariant
violations. Factory functions (`New`, `NewChannel`) panic on option errors.

**Rationale**: Panic on configuration error is appropriate because invalid configuration
is always a programming error, not a runtime condition. Interface-based options allow
future extension without breaking compatibility.

---

## ADR-003: Event-Loop-Native Handlers

**Status**: Accepted

**Context**: gRPC handlers in JS must interact with the event loop correctly.

**Decision**: Server handlers use `StreamHandlerFunc` which runs directly on the event
loop goroutine. All stream I/O uses non-blocking callbacks (`Recv`/`Send`).

**Rationale**: JavaScript is single-threaded. Running handlers on the event loop
goroutine ensures thread-safety without locks. The callback-based `RPCStream` API matches
the non-blocking nature of the event loop.

---

## ADR-004: dynamicpb Over Generated Stubs

**Status**: Accepted

**Context**: JS doesn't have Go's generated protobuf types.

**Decision**: Use `dynamicpb.Message` for all protobuf messages in the JS bridge.
Descriptors are loaded at runtime from `FileDescriptorSet` bytes.

**Rationale**: Generated stubs require code generation tooling and Go type awareness.
`dynamicpb` provides full protobuf wire format compatibility with runtime descriptor
loading, matching the protobuf-es approach of dynamic message construction.

---

## ADR-005: NewChannel Panics on Nil Loop

**Status**: Accepted

**Context**: `inprocgrpc.NewChannel(nil)` is always a bug.

**Decision**: Changed from `(channel, error)` return to panic on nil loop.

**Rationale**: A nil loop paramater is a programming error (not a runtime condition).
Panicking immediately provides a clear stack trace at the call site rather than
propagating an error that must be handled at every caller.

---

## ADR-006: Prepositions Banned in Public API Names

**Status**: Accepted

**Context**: API naming consistency across the project.

**Decision**: No prepositions in public API names. Examples:
- `WaitForMessage` → `Recv`
- `IsClosed` → `Closed`
- `SetToValue` → `SetValue`

**Rationale**: Preposition-free names are more idiomatic Go. They're shorter, clearer,
and consistent with the standard library (`io.Reader.Read`, not `io.Reader.ReadFrom`).

---

## ADR-007: Three-Platform Testing Mandatory

**Status**: Accepted

**Context**: Cross-platform compatibility is critical for a library.

**Decision**: All changes must pass on macOS, Linux, and Windows before merging.
Platform-specific code uses build tags (e.g., `fd_unix.go`, `fd_windows.go`).

**Rationale**: Platform-specific bugs (especially around I/O, timing, and epoll/kqueue/IOCP
differences) are common in event loop and networking code. Continuous three-platform
testing catches these before release.
