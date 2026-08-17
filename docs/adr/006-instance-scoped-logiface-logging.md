# ADR-006: Instance-Scoped Logiface Logging

## Context

Modules in this repository need a consistent logging contract without coupling callers to process-wide state or competing logging APIs. Package-global loggers make independent instances interfere with one another, complicate tests, and prevent applications from routing each component's events according to their own policy.

The repository already provides `logiface` as its logging abstraction. Allowing new implementations to use the standard library `log` or `log/slog` packages directly would duplicate that abstraction and make composition inconsistent.

## Decision

New implementations that log must use the repository's `logiface` module. They must not emit through the standard library `log` or `log/slog` packages, package-global logger variables, or an implicit global default. A `logiface` backend adapter may depend on the logging API it integrates, but that dependency must remain inside the adapter boundary; it does not permit application or utility modules to bypass `logiface`.

Logging configuration must be instance-scoped. Where logging is appropriate, accept the logger as constructor configuration, typically through an option, and store it for internal use by that instance. It is acceptable for a module's public API to reference `logiface` so callers can provide the logger.

The logger is an inward dependency, not normally part of an implementation's observable state. Do not expose it as a public field, accessor, or return value unless callers have a concrete use case that cannot be met by configuration alone.

Existing implementations should migrate to this contract whenever doing so does not require a breaking API or behavioral change. Compatibility constraints must not be used to justify new uses of standard-library, global, or otherwise competing logging paths.

## Rationale

- One logging abstraction gives applications consistent event construction, filtering, and backend integration.
- Instance-scoped configuration supports independent module instances, deterministic tests, and caller-controlled routing.
- Accepting `logiface` in public configuration is preferable to hiding a global dependency.
- Keeping the configured logger internal preserves encapsulation and avoids making logging machinery part of the module's operational API.

## Consequences

- A new module that needs logging must provide an instance-level configuration path before emitting logs.
- A module that has no useful events does not need to accept a logger merely for uniformity.
- Changes to existing modules must preserve compatibility unless the user explicitly authorizes a breaking migration.
- Reviews must reject new direct uses of `log`, `log/slog`, global logger state, and fallback logging paths outside `logiface`, except for the backend API used within its dedicated `logiface` adapter.
