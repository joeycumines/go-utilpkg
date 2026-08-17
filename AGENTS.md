# AGENTS.md / CLAUDE.md

This file provides guidance to AI agents.

## Guidance Scope

This file defines project facts, constraints, and outcomes—not routine methods or human preferences. Keep procedural recipes and structural snapshots in live tooling or task documentation; keep style, naming, and code-organization opinions in personal skills or system prompts. Add exceptions only when the user explicitly identifies one.

## Project Overview

This is a Go monorepo (`go-utilpkg`) that consolidates multiple related modules. Modules are synced to their individual published repositories using the `grit` tool. Every Go package belongs to its own subdirectory module; the package-free root module primarily pins project tool versions.

### Architectural Decision Records (ADRs)

ADR files are stored in `docs/adr/` and define cross-module architectural constraints. On beginning a session, list that directory as an enumerated list of filenames only; read an ADR when it applies to the task.

**Do not create, edit, rename, or delete an ADR unless the user directly and explicitly instructs you to do so.** A general request to improve code, documentation, standards, or architecture is not authorization to mutate ADRs.

### Live Repository Discovery

Do not add module inventories, package catalogs, dependency maps, directory trees, or similar structural snapshots to this file. They duplicate live sources of truth, become stale, and discourage agents from inspecting the repository. Discover structure from the checked-out files, `go.mod` files, GNU Make targets, `project.mk`, and module documentation relevant to the current task.

## Build System

GNU Make is the authoritative interface for repository build, test, analysis, coverage, and publication operations. On macOS, use `gmake`, never BSD `make`.

```bash
gmake help | head -n 170 # why = gmake:macOS head:long
```

## Development Workflow

1. **Always verify all tests pass** across all three OS platforms (Darwin, Linux, Windows). The complete repository gate must pass 100% before considering work complete.
2. **Never commit with failing tests** - timing-dependent failures are treated as bugs to be fixed, not as flakiness to be ignored.

### Grit Publishing

Grit targets are generated from `GRIT_DST` only. It maps a directory to a
destination repository, and the directory is converted to a slug using the
usual convention (`./logiface/logrus` becomes `logiface.logrus`). A Go module
without a `GRIT_DST` entry has no grit targets, and a `GRIT_DST` directory need
not be a Go module.

```bash
# Publish specific directory to its separate repo
gmake grit.eventloop

# Publish all configured destinations
gmake grit

# Sync one destination repo back into this monorepo (on demand, no "pull all")
gmake grit-pull.eventloop
```

See `project.mk` for the grit destination mappings.

## Code Quality Standards

N.B. This is just a subset.

- Implement **general-purpose solutions** that work for all valid inputs, not just test cases
- **No hard-coding** values or creating solutions that only work for specific test inputs
- **Zero tolerance** for test failures - fix them properly rather than working around
- Write **high-quality, principled implementations** following best practices
- All new features require **new tests** for verification
- **Do not use testify packages**. Only use the built-in `testing` package.
- Follow the applicable decisions in `docs/adr/`, including the repository logging contract.
- **Avoid prepositions in exposed symbols**. No prepositions (From, Into, To, By, On, In, Of, For, etc.) in method, function, type, or variable names — especially public APIs. Prefer `LoadConfig` over `LoadFromConfig`. **Allowed exception**: prepositions used with clear structural intent — e.g. `With*` option constructors convey a specific structural pattern; `ToJSON` matches an external API contract. The preposition must carry meaning beyond being part of a phrase that happens to be a symbol name.
