---
name: blueprint-json
description: Manages blueprint.json — the exhaustive, flat, sequential task tracker that serves as the structural tether to reality across sessions. Mandates deep, context-gathered, vaporware-resistant planning with per-task acceptance criteria and structured replanning. Use when user says "blueprint", "blueprint.json", "create blueprint", "update blueprint", "refine blueprint", "replan", "expand scope", "track tasks", "plan tasks", "baseline blueprint", or at project start / session resumption. Also use when the user asks to plan or track work exhaustively, or when any task list needs to be created or maintained as a JSON artifact. This skill applies whenever blueprint.json or baseline.blueprint.json is read, written, validated, or referenced.
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
9. **Planning depth before execution.** No task may begin until the plan has been validated as deep enough to succeed. See "Planning Depth Standard" below.

## Planning Depth Standard

Volume of tasks is not depth. A hundred vague tasks is vaporware with line numbers. The planning bar is this:

> The plan must be **specific, contextualized, and sequenced well enough** that a team of **knowledgeable but easily confused juniors — with weak intuition and negligible applicable experience** — could follow it to the **perfect achievement of the desired outcome**, without requiring clarification, heroic inference, or "just figuring it out."

This is the planning standard because across context boundaries, **the executing agent _is_ that confused junior.** It wakes up with amnesia, reads the blueprint cold, and must make forward progress immediately — or it will make _backward_ progress, producing vaporware, trash code, or rework that poisons later sessions.

### What Depth Requires

1. **Domain model before tasks.** Before enumerating _what to do_, the planner must demonstrate understanding of _what exists_ and _how it works_. In complex domains, this means reading code, schemas, APIs, and documentation — not guessing. A plan built on assumptions about code that hasn't been read is a plan that will generate rework.
2. **Acceptance criteria per task.** Every task must include an `"acceptance"` field that answers: _"How do we know this task actually delivered value — not just that code exists?"_ Acceptance criteria are **not** about checks passing. Checks passing is a baseline implicit in every change, task or not — it is never acceptance criteria. Acceptance criteria must describe the **real-world, end-to-end effect** of the task: what becomes possible, what behavior changes, what the user/system/caller can now do that it couldn't before, and how this integrates with the project as a functioning whole. "Tests pass" is not acceptance. "The `/audit` endpoint returns paginated audit events for any entity, the admin dashboard renders them, and the retention policy in `config/audit.yaml` governs automatic cleanup" is acceptance — because it describes delivered value, not existence of code.
3. **Context pointers.** Each task must reference the specific files, functions, types, endpoints, or schemas it touches. A cold-start agent should not need to _search_ for what to change — the plan already told it.
4. **Dependency awareness.** If task N depends on the output of task M, that dependency must be explicit. If task N can only succeed after reading context produced by task M, the plan must say so.
5. **Unknowns surfaced.** If the planner encounters an area it cannot fully plan because context is missing, it must not paper over it with a vague task. It must create a _research task_ that explicitly gathers the missing context, followed by a _replanning checkpoint_ (see Replanning).
6. **Anti-vaporware test.** After generating the plan, apply this filter to every task: _"Could this task description appear verbatim in a plan for a completely different project?"_ If yes, it is too vague. Rewrite it with project-specific detail until it could only belong to _this_ plan.

## Structure

The generic baseline template is in `references/baseline-template.json`. Projects may also maintain a **`baseline.blueprint.json`** in the project root — a project-specific template with pre-filled `globalAlerts`, `continuousVerification`, `mandatoryDirectives`, etc. tailored to the project's tooling. When both exist, prefer the project-specific baseline.

The key sections are:

- **`meta`** — File identity, source reference, and the maintenance instruction (keep the plan up to date; deviations logged within the plan).
- **`globalAlerts`** — Critical operational warnings. Customize these for the project's specific tooling constraints and build system.
- **`mandatoryDirectives`** — Operational directives for context and resource management during execution.
- **`statusSection`** — Current high-level state, updated as work progresses.
- **`sequentialTasks`** — The flat, ordered task array. Each task has a `task`, `description`, `acceptance`, and `status` field. Status is one of: `"Not Started"`, `"In Progress"`, `"Done"`. See "Planning Depth Standard" for what constitutes a well-formed task.
- **`continuousVerification`** — Standing verification mandate. Define project-specific build/test commands and coverage requirements here.
- **`finalEnforcementProtocol`** — Post-execution warnings, typically repeating critical tooling constraints for reinforcement.

## Lifecycle

### Create/Plan

#### Phase 0: Context Gathering (Non-Negotiable)

Before generating _any_ tasks, the planner must build a sufficient mental model. This means:

1. **Read, don't guess.** The planner (subagent) must read the relevant source files, schemas, APIs, configs, and documentation in the target codebase. Plans based on assumptions about code that hasn't been inspected produce rework and vaporware.
2. **Identify the goal state.** Write a concrete, falsifiable statement of what the completed work looks like. This goes into `statusSection.goalState`. If you cannot write this statement, you do not understand the task.
3. **Map the domain.** Identify the key entities, interfaces, data flows, and constraints. For code changes, identify the specific modules, types, and functions involved. For infrastructure, identify the services, configs, and deployment targets. This context feeds the task descriptions.

#### Phase 1: Task Enumeration

1. If `baseline.blueprint.json` exists in the project root, use it as the starting template. Otherwise, read `references/baseline-template.json` for structure.
2. **Mandatory Subagent Plan:** You **MUST** use a subagent to generate the initial `sequentialTasks` list. The subagent prompt must include all context gathered in Phase 0.
3. **Overwhelming Completeness:** The subagent must populate the list to be "overwhelmingly complete," enumerating *every* task between the current state and the goal.
4. **Depth over breadth.** Each task must meet the Planning Depth Standard: specific to _this_ project, with `acceptance` criteria, context pointers (files/functions/types), and explicit dependencies. A plan full of generic tasks like "implement feature X" or "add tests" is vaporware — it will be rejected.
5. **Surface unknowns explicitly.** Where context is insufficient to plan precisely, insert a research task + replanning checkpoint rather than a vague implementation task.
6. **Autonomy:** If the path is unclear, the subagent must draft the best possible exhaustive plan without waiting for user intervention — but must flag uncertainty as unknowns, never paper over it.

#### Phase 2: Plan Stress-Test

After task enumeration, before any execution begins:

1. **Walk the plan end-to-end.** Read every task in sequence. For each, ask: _"If I woke up with amnesia and read only this task and the tasks before it, could I execute it correctly?"_ If no, the task is under-specified — fix it.
2. **Anti-vaporware sweep.** Apply the filter: _"Could this task description appear verbatim in a different project's plan?"_ If yes, rewrite with project-specific detail.
3. **Dependency audit.** Verify that no task references outputs, files, or state that haven't been produced by a preceding task.
4. **Goal-state check.** Verify that completing all tasks in order provably achieves the `goalState`. If there is a gap, add tasks.

### Maintain

1. Update task `status` immediately upon starting or completing each task.
2. Update `statusSection.currentState` to reflect the current high-level position.
3. Log deviations, blockers, or discoveries as amendments to affected tasks.

### Refine

1. Prune tasks whose status is `"Done"` only if they add no future value (e.g. one-off setup steps).
2. Rewrite unclear task descriptions — the next agent reads them cold.
3. Split tasks that are too coarse. Merge tasks that are redundant.
4. Perform a holistic reassessment: does the full sequence still make sense end-to-end?
5. Re-validate against the Planning Depth Standard. After any refinement, every task must still pass the anti-vaporware test and have concrete acceptance criteria.

### Replan

Replanning is not failure — it is the plan defending itself against discovered reality. Trigger a replan when:

1. **A research task reveals that prior assumptions were wrong.** The tasks that followed those assumptions are now suspect.
2. **Execution of a task uncovers a structural problem** that means subsequent tasks cannot succeed as written.
3. **The goal state itself changes** due to new requirements or discoveries.
4. **More than two tasks in a row require significant deviation** from their descriptions. This is a signal the plan has drifted from reality.

**Replanning protocol:**

1. Mark the trigger event and affected tasks in the blueprint.
2. Re-run Phase 0 (Context Gathering) for the affected area — re-read the code/state, do not rely on stale mental models.
3. Rewrite affected tasks to reflect discovered reality. Apply the full Planning Depth Standard.
4. Re-run Phase 2 (Stress-Test) on the modified segment of the plan.
5. Log the replan event in `statusSection.replanLog` with: trigger, affected task range, and summary of changes.

### Expand (Indefinite Cycle)

1. **Feed the Wolves:** You must never let the wolf of completion win at the expense of perfection.
2. **True Perfection:** After reaching stable-state (all current tasks complete, verified), you must immediately identify the next frontier.
3. **Scope Expansion:** Add new tasks: improvements, integration tests, performance optimizations, documentation, new features.
4. The task list must never be empty. If it is, you have failed to imagine "True Perfection."

### Verify (Rule of Two)

**Top-level orchestrator only.** If you are a subagent, complete your
task and return results — the orchestrator handles review gating.

Before marking a task "Done" or committing any code, the orchestrator
must strictly follow the `strict-review-gate` skill protocol:

1. Invoke a subagent to review the work (using the exact prompt from
   `strict-review-gate`).
2. **Read the review yourself** — every word. Verify each finding.
3. Fully address all issues raised.
4. Invoke a *new* subagent for the second review.
5. **Read the second review yourself** — every word.
6. Only if two contiguous reviews confirm the code is at the local
   maximum of quality may you proceed.
7. Subagents run serially, each to completion.

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
  "description": "Create auth/jwt.go with Sign() and Verify() functions using the claims struct in auth/claims.go. Wire into the middleware chain in server/middleware.go (append to the handlerChain slice). Token expiry must match the TTL in config/auth.yaml.",
  "acceptance": "All protected API endpoints (listed in server/routes.go) reject requests without a valid token and return 401. Authenticated requests carry the parsed claims through to handlers via the context key defined in auth/context.go. Expired tokens are rejected — not silently re-issued. The admin and user roles defined in auth/roles.go gate access to their respective route groups end-to-end.",
  "status": "Not Started"
}
```

```json lines
// WRONG — vaporware (could appear in any project's plan)
{
  "task": "Set up the database",
  "description": "Create the database schema and migrations.",
  "status": "Not Started"
}

// RIGHT — project-specific, verifiable
{
  "task": "Create PostgreSQL migration 003: add audit_log table",
  "description": "New migration in db/migrations/003_audit_log.up.sql. Table audit_log with columns: id (uuid, PK), entity_type (text), entity_id (uuid), action (text), actor_id (uuid FK to users.id), payload (jsonb), created_at (timestamptz). Down migration drops the table. Must pass sqlc generate without errors.",
  "acceptance": "The audit_log table is populated by the event publisher in events/publisher.go for every entity mutation. The GET /admin/audit-log endpoint returns paginated, filterable audit entries. The cleanup job in jobs/audit_retention.go respects the TTL in config/audit.yaml and prunes old records. The table is a functioning part of the audit trail — not an orphaned schema.",
  "status": "Not Started"
}
```

## Troubleshooting

* **Empty `sequentialTasks`**: The blueprint was not populated. Use a subagent to generate an *overwhelmingly complete* list immediately.
* **Stale task statuses**: You forgot incremental updates. Audit every task — does its status reflect reality?
* **Tasks contain estimates or priorities**: Remove them immediately. They prevent you from feeding the wolf of perfection.
* **Agent concludes work is "done"**: It is not. Expand scope towards True Perfection. Read the Hard Constraints.
* **Tasks are vague / generic**: The plan failed the anti-vaporware test. Re-read the code, re-run Context Gathering, and rewrite every task that could belong to a different project.
* **Repeated rework on the same area**: The plan was built on assumptions instead of inspected reality. Trigger a Replan: re-read the affected code, rewrite the tasks, stress-test again.
* **Agent produces code that doesn't integrate**: Acceptance criteria were missing or described checks passing instead of value delivered. Every task must specify _what becomes possible_ when it's done — not just that code exists and compiles.
* **Plan looks complete but outcome is wrong**: The goal state was never written down or was too vague. Check `statusSection.goalState`. If it's empty or unfalsifiable, fix it before continuing.
