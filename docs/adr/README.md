# Architecture Decision Records (ADRs)

This directory contains ADRs that document significant architectural decisions affecting the go-utilpkg monorepo.

## Intent

ADRs capture important design decisions with context, rationale, and consequences. They serve as:

- **Documentation**: Why decisions were made, not just what was decided
- **Onboarding**: New contributors understand the reasoning behind code patterns
- **History**: Track evolution of architectural thinking over time

## Structure

ADRs follow a simple format:

1. **Title**: ADR-XXX: [Scope] Decision Title
2. **Context**: The problem or situation that prompted the decision
3. **Decision**: What was decided
4. **Rationale**: Why this approach was chosen

Additional sections like Implementation Example may be included as needed.

## Guidelines

- ADRs should be **module-agnostic** or clearly scoped to multiple modules
- Implementation-specific design for a single module belongs in that module's documentation, not here
- Number sequentially (ADR-001, ADR-002, etc.)
- Keep focused — one decision per ADR
