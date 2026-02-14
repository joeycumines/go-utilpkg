---
name: blueprint-json
description: Manages blueprint.json — the exhaustive, flat, sequential task tracker that serves as the structural tether to reality across sessions. Use when user says "blueprint", "blueprint.json", "create blueprint", "update blueprint", "refine blueprint", "expand scope", "track tasks", "plan tasks", "baseline blueprint", or at project start / session resumption. Also use when the user asks to plan or track work exhaustively, or when any task list needs to be created or maintained as a JSON artifact. This skill applies whenever blueprint.json or baseline.blueprint.json is read, written, validated, or referenced.
license: MIT
---

# Blueprint JSON

`blueprint.json` is the single source of truth for all planned, in-progress, and completed work. If a task is not in `blueprint.json`, it is not being done.

The core value of this skill is **forced verification at a structural level**: the agent is compelled to check whether all tasks have actually been completed, and the persistent JSON artifact provides a hook point to expand the list of checks and tasks. Over long context windows, without this structural tether, agents drift, skip steps, and declare "done" prematurely.

## Hard Constraints

These are non-negotiable. Violating any of them is a failure state.

1. **Flat, sequential task list.** Tasks in `sequentialTasks` are a flat array executed in order. One level of sub-items is permitted for grouping only — never deeper.
2. **NO estimates.** Perfection isn't cheap. No effort estimates, time estimates, duration estimates, story points, or t-shirt sizes. These are proxies for laziness and are strictly forbidden. You must feed the "Wolf of Perfection," and it does not care about cost.
3. **NO priorities.** You must complete 100% of the work. Prioritization implies some tasks are optional or less important, allowing the "Wolf of Completion" to win at the expense of quality. There is only the sequence, and it must all be done.
4. **The "Rule of Two" (Commit Gate).** No commits are permitted until **two contiguous, issue-free #runSubagent reviews** have guaranteed correctness. Subagents are for **serial context management**, not parallelization. They must be instructed to be skeptical, hostile to errors, and used to find the local maximum of correctness.
5. **NO stopping (Indefinite Cycle).** The work is never done. Scope must always expand towards "True Perfection". You must feed both the wolf of completion and the wolf of perfection. Concluding that work is "done" or "good enough" is forbidden.
6. **Incremental updates.** Status must be updated after every sub-task, not batched. The next session's agent reads this file cold — stale state is fatal.
7. **Scope must expand.** After completing all current tasks, ideate new improvements, refinements, and features. Add them. Repeat.
8. **Deviations logged.** Any deviation from the plan must be recorded within the plan itself. The plan must be reassessed holistically after any change.

## Structure

The generic baseline template is in `references/baseline-template.json`. Projects may also maintain a **`baseline.blueprint.json`** in the project root — a project-specific template with pre-filled `globalAlerts`, `continuousVerification`, `mandatoryDirectives`, etc. tailored to the project's tooling. When both exist, prefer the project-specific baseline.

The key sections are:

- **`meta`** — File identity, source reference, and the maintenance instruction (keep the plan up to date; deviations logged within the plan).
- **`globalAlerts`** — Critical operational warnings. Customize these for the project's specific tooling constraints and build system.
- **`mandatoryDirectives`** — Operational directives for context and resource management during execution.
- **`statusSection`** — Current high-level state, updated as work progresses.
- **`sequentialTasks`** — The flat, ordered task array. Each task has a `task`, `description`, and `status` field. Status is one of: `"Not Started"`, `"In Progress"`, `"Done"`.
- **`continuousVerification`** — Standing verification mandate. Define project-specific build/test commands and coverage requirements here.
- **`finalEnforcementProtocol`** — Post-execution warnings, typically repeating critical tooling constraints for reinforcement.

## Lifecycle

### Create/Plan

1. If `baseline.blueprint.json` exists in the project root, use it as the starting template. Otherwise, read `references/baseline-template.json` for structure.
2. **Mandatory Subagent Plan:** You **MUST** use a subagent to generate the initial `sequentialTasks` list.
3. **Overwhelming Completeness:** The subagent must populate the list to be "overwhelmingly complete," enumerating *every* task between the current state and the goal.
4. **Autonomy:** If the path is unclear, the subagent must draft the best possible exhaustive plan without waiting for user intervention.
5. Ensure every task description is clear enough that a cold-start agent can execute it without additional context.

### Maintain

1. Update task `status` immediately upon starting or completing each task.
2. Update `statusSection.currentState` to reflect the current high-level position.
3. Log deviations, blockers, or discoveries as amendments to affected tasks.

### Refine

1. Prune tasks whose status is `"Done"` only if they add no future value (e.g. one-off setup steps).
2. Rewrite unclear task descriptions — the next agent reads them cold.
3. Split tasks that are too coarse. Merge tasks that are redundant.
4. Perform a holistic reassessment: does the full sequence still make sense end-to-end?

### Expand (Indefinite Cycle)

1. **Feed the Wolves:** You must never let the wolf of completion win at the expense of perfection.
2. **True Perfection:** After reaching stable-state (all current tasks complete, verified), you must immediately identify the next frontier.
3. **Scope Expansion:** Add new tasks: improvements, integration tests, performance optimizations, documentation, new features.
4. The task list must never be empty. If it is, you have failed to imagine "True Perfection."

### Verify (Rule of Two)

Before marking a task "Done" or committing any code, you must strictly adhere to the **Rule of Two**:

1. **First Review:** Invoke a subagent to review the work. Instruct it to be skeptical and guarantee correctness.
2. **Read the Review Yourself:** You—the caller—MUST read **EVERY WORD** of the subagent's review in full. Do not skim. Do not trust a summary. Verify each finding yourself.
3. **Action:** Fully address *all* issues raised.
4. **Second Review:** Invoke a *new* subagent to review the fixed state.
5. **Read the Review Yourself (Again):** You—the caller—MUST read **EVERY WORD** of the second subagent's review in full.
6. **Gate:** Only if **two contiguous reviews confirm the code is at the local maximum of quality**—no issues found AND no possible improvements—can you proceed to commit. A mere "no issues" is insufficient; the subagent must verify the code cannot be improved.
7. **Serial Execution:** Subagents are for context management, not parallelization. They must run to completion.

### Produce / Refine Project Baseline

When the user asks to create or refine a `baseline.blueprint.json`:

1. Start from the generic `references/baseline-template.json` (or the existing `baseline.blueprint.json` if refining).
2. Fill or update `globalAlerts`, `mandatoryDirectives`, `continuousVerification`, and `finalEnforcementProtocol` with the project's specific tooling, build commands, and constraints.
3. Leave `sequentialTasks` empty and `statusSection.currentState` blank — these are populated per-blueprint, not in the baseline.
4. Write the result to `baseline.blueprint.json` in the project root.

## Session Resumption

`blueprint.json` is designed to be the first file read at the start of any new session. The agent should:

1. Read `blueprint.json` before doing any other work.
2. Resume from the first task whose status is not `"Done"`.
3. If the project uses a separate session-state file (e.g. for stack traces, file paths, immediate next steps), read that too.

## Anti-Patterns

**Blueprint must NOT contain any of these. If found, remove them immediately.**

```json lines
// WRONG — contains estimate
{
  "task": "Implement auth module",
  "estimatedEffort": "2 hours",
  // VIOLATION: Perfection isn't cheap.
  "status": "Not Started"
}

// WRONG — contains priority
{
  "task": "Fix login bug",
  "priority": "P0",
  // VIOLATION: All tasks are mandatory.
  "severity": "Critical",
  "status": "Not Started"
}

// WRONG — nested task hierarchy
{
  "task": "Backend work",
  "subtasks": [
    {
      "task": "Database",
      "subtasks": [
        {
          "task": "Schema migration"
        }
      ]
    }
  ]
}
```

**Correct form:**

```json
{
  "task": "Implement auth module: JWT token generation and validation",
  "description": "Create auth/jwt.go with Sign() and Verify() functions. Add unit tests with 100% coverage. Integrate with existing middleware chain.",
  "status": "Not Started"
}
```

## Troubleshooting

* **Empty `sequentialTasks**`: The blueprint was not populated. Use a subagent to generate an *overwhelmingly complete* list immediately.
* **Stale task statuses**: You forgot incremental updates. Audit every task — does its status reflect reality?
* **Tasks contain estimates or priorities**: Remove them immediately. They prevent you from feeding the wolf of perfection.
* **Agent concludes work is "done"**: It is not. Expand scope towards True Perfection. Read the Hard Constraints.
