---
name: strict-review-gate
description: Enforces the "Rule of Two" Paranoia Protocol — the mandatory gate before marking any task "Done" or committing. Use when user says "review", "guarantee correctness", "check correctness", "finalize task", "commit", "Rule of Two", or when ANY task is completed, **before** you move on. This gate applies to ALL tasks, not only when explicitly requested.
license: MIT
---

# Strict Review Gate (Rule of Two)

## Core Mandate

You are the gatekeeper. No task may be marked "Done" and no code may be committed until you have achieved **two contiguous, issue-free subagent reviews** using the Paranoia Protocol. You do not trust yourself. You do not trust the user to catch your bugs.

## Operational Parameters

- **Review scope**: `diff vs HEAD`
- **Artifacts directory**: `./scratch/`
- **Single artifact constraint**: Avoid producing more than one review artifact per run
- **Action requirement**: FULLY address each review BEFORE moving on — reviews not promptly actioned are wasted effort
- **Check coverage**: ALL project-defined checks must pass on ALL project-defined targets (platforms, environments, etc.) — no skipping, no ignoring
- **Test coverage**: 100% unit test coverage is required; integration testing must be included whenever useful. Both must be explicitly verified (a separate subagent may be used)

## Verification Patterns

Apply these named patterns during every review:

1. **Blind Review** — Infer intent from code alone (no commit message). Compare inferred intent against actual intent to catch assumption mismatches.
2. **Hypothesis Testing** — Form hypotheses of incorrectness aggressively, then actively disprove them. Do not ask the agent to "prove correctness" — it will simply lie.
3. **Reproduce-or-Fail** — Bugfixes must reproduce the original bug before accepting the fix. Features must include a verified usage example.

## The Protocol

### Step 1: Spawn Subagent Review

You **must** spawn a subagent. Do not self-review. Use this EXACT prompt:

> "Ensure, or rather GUARANTEE the correctness of my PR. Since you're _guaranteeing_ it, sink commensurate effort; you care deeply about keeping your word, after all. Even then, I expect you to think very VERY hard, significantly harder than you might expect. Assume, from start to finish, that there's _always_ another problem, and you just haven't caught it yet. Question all information provided - _only_ if it is simply impossible to verify are you allowed to trust, and if you trust you MUST specify that (as briefly as possible). Provide a succinct summary then more detailed analysis. Your succinct summary must be such that removing any single part, or applying any transformation or adjustment of wording, would make it materially worse."

Include the review scope (diff vs HEAD) and direct output to `./scratch/`.

### Step 2: The Contiguous Gate

Both runs use the **identical** full paranoia prompt above. Each is a comprehensive, independent review — not a graduated pipeline.

1. **Run 1:** Spawn subagent. Execute full review.
    - *FAILURE:* Reject. Reset count to 0. Fix all issues. Restart.
    - *SUCCESS:* Proceed to Run 2.
2. **Run 2:** Spawn a **new** subagent. Execute full review independently.
    - *FAILURE:* Reject. Reset count to 0. Fix all issues. Restart at Run 1.
    - *SUCCESS:* Proceed to Step 3.

### Step 3: Fitness Review

Separately review for **alignment and fitness for purpose**. Functional correctness alone is insufficient — the change must also be the right change.

After this passes: mark "Done" and/or commit.

## Troubleshooting

- **Superficial review**: If a review is "LGTM" without hypothesis testing, it is a FAILURE. Reject it.
- **Target missing**: If any project-defined target (platform, environment, etc.) was not checked, the review is a FAILURE.
