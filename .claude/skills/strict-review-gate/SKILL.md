---
name: strict-review-gate
description: Enforces the "Rule of Two" Paranoia Protocol — the mandatory gate before marking any task "Done" or committing. Use when user says "review", "guarantee correctness", "check correctness", "finalize task", "commit", "Rule of Two", or when ANY task is completed, **before** you move on. This gate applies to ALL tasks, not only when explicitly requested.
license: MIT
---

# Strict Review Gate (Rule of Two)

## Applicability

This protocol is for the **top-level orchestrating agent ONLY** —
the agent that owns the task lifecycle, spawns subagents, and has
the authority to commit code.

**If you are a subagent** (spawned to do work, review code, run
tests, or any other delegated task): **STOP READING.** This protocol
is not yours to execute. Complete your assigned task. Return your
results. The orchestrator handles review gating. Do NOT attempt the
Rule of Two. Do NOT self-review using this protocol.

## Core Mandate

The top-level orchestrating agent is the gatekeeper. No task may be
marked "Done" and no code may be committed until the orchestrator
has achieved **two contiguous, issue-free subagent reviews** using
the Paranoia Protocol. The orchestrator does not trust itself. It
does not trust the user to catch its bugs.

## Operational Parameters

- **Review scope**: `diff vs HEAD`
- **Artifacts directory**: `./scratch/`
- **Single artifact constraint**: Avoid producing more than one
  review artifact per run
- **Action requirement**: FULLY address each review BEFORE moving
  on — reviews not promptly actioned are wasted effort
- **Check coverage**: ALL project-defined checks must pass on ALL
  project-defined targets (platforms, environments, etc.) — no
  skipping, no ignoring
- **Test coverage**: 100% unit test coverage is required; integration
  testing must be included whenever useful. Both must be explicitly
  verified (a separate subagent may be used)

## Verification Patterns

Apply these named patterns during every review:

1. **Blind Review** — Infer intent from code alone (no commit
   message). Compare inferred intent against actual intent to catch
   assumption mismatches.
2. **Hypothesis Testing** — Form hypotheses of incorrectness
   aggressively, then actively disprove them. Do not ask the agent
   to "prove correctness" — it will simply lie.
3. **Reproduce-or-Fail** — Bugfixes must reproduce the original bug
   before accepting the fix. Features must include a verified usage
   example.

## The Protocol

### Step 1: Spawn Subagent Review

You **must** spawn a subagent. Do not self-review. Use this EXACT
prompt:

> "You are a REVIEWER. Your sole task is to review the provided diff
> and guarantee its correctness through thorough analysis. Do NOT
> attempt the 'Rule of Two' or any multi-pass review gating — that
> is the caller's responsibility, not yours. Perform ONE review.
>
> Sink commensurate effort; you care deeply about keeping your word.
> Think very hard — significantly harder than you might expect.
> Assume, from start to finish, that there's always another problem
> you haven't caught yet. Question all information — only if it is
> simply impossible to verify are you allowed to trust, and if you
> trust you MUST specify that (briefly).
>
> Apply these verification patterns:
> 1. Blind Review — Infer intent from code alone, compare against
>    stated intent.
> 2. Hypothesis Testing — Form hypotheses of incorrectness, then
>    disprove them.
> 3. Reproduce-or-Fail — Bugfixes must show the original bug;
>    features must include a verified usage path.
>
> Return: verdict (PASS/FAIL), a succinct summary (removing any part
> would make it materially worse), then detailed findings."

Include the review scope (diff vs HEAD) and direct output to
`./scratch/`.

### Step 2: The Contiguous Gate

Both runs use the **identical** full prompt above. Each is a
comprehensive, independent review — not a graduated pipeline.

1. **Run 1:** Spawn subagent. Execute full review.
    - *FAILURE:* Reject. Reset count to 0. Fix all issues. Restart.
    - *SUCCESS:* Proceed to Run 2.
2. **Run 2:** Spawn a **new** subagent. Execute full review
   independently.
    - *FAILURE:* Reject. Reset count to 0. Fix all issues. Restart
      at Run 1.
    - *SUCCESS:* Proceed to Step 3.

### Step 3: Fitness Review

Separately review for **alignment and fitness for purpose**.
Functional correctness alone is insufficient — the change must also
be the right change.

After this passes: mark "Done" and/or commit.

## Spawning Work Subagents

When spawning a subagent to perform WORK (not review), include this
in the subagent's prompt:

> "Complete the assigned task and return your results. Do NOT attempt
> the 'Rule of Two', self-review, commit gating, or any review
> orchestration — that is the top-level agent's responsibility."

This prevents work subagents from wasting effort on self-gating they
cannot meaningfully perform.

## Troubleshooting

- **Superficial review**: If a review is "LGTM" without hypothesis
  testing, it is a FAILURE. Reject it.
- **Target missing**: If any project-defined target (platform,
  environment, etc.) was not checked, the review is a FAILURE.
- **Work subagent self-gating**: If a work subagent attempts the Rule
  of Two on its own sub-task (self-reviewing, trying to spawn
  reviewers), its prompt is missing the work subagent clause. Re-read
  "Spawning Work Subagents" above.
