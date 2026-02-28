---
name: solution-analysis-master
description: Orchestrates rigorous, multi-pipeline solution analysis with subagent-based strict review gates. Use when user asks to "analyze a problem deeply", "produce optimal solution analysis", "run solution pipelines", "strict review gate", "quorum-based review", or when comprehensive technical analysis with verification is needed. Produces independently-generated analyses that pass strict review quorum. Designed for complex technical problems requiring high confidence.
license: MIT
---

# Solution Analysis Master

## Purpose

This skill orchestrates **rigorous, independently-generated solution analyses** that pass strict review gates. It is designed for complex technical problems where:

1. Multiple independent perspectives reduce blind spots
2. Strict review quorum ensures quality
3. User guidance MUST be incorporated (not just acknowledged)
4. Implementation code must be COMPLETE (no truncation)

## CRITICAL INVARIANTS

### Invariant 1: Independence Between Pipelines

Each pipeline MUST be completely isolated:

- **NO** reference to other pipeline outputs
- **NO** building on previous analyses
- **NO** shared findings between pipelines
- Each pipeline starts FRESH from the original problem statement

### Invariant 2: User Guidance is MANDATORY

If the user provides solution directions, they are NOT suggestions—they are REQUIREMENTS:

- Every user-specified solution direction MUST have an actionable implementation spec
- Analyses that ignore user guidance are AUTOMATICALLY REJECTED
- Technical accuracy without strategic alignment to user guidance is WORTHLESS

### Invariant 3: Quorum Requires IDENTICAL Review Prompts

For quorum to be meaningful, ALL strict reviewers MUST receive the IDENTICAL task prompt:

- Different review criteria = meaningless quorum
- Reviewers with different focuses (e.g., "technical" vs "completeness") will disagree for wrong reasons
- The prompt template in `references/subagent-prompt-templates.md` MUST be used verbatim for all reviewers

### Invariant 4: Complete Implementation Code

All proposed solutions MUST include:

- COMPLETE code (no `...` truncation)
- ALL helper methods defined
- ALL integration points specified
- Production-ready quality

---

## Workflow Overview

1. **Setup** — Read problem, extract user guidance, READ `references/subagent-prompt-templates.md` (CRITICAL), create pipeline dirs.
2. **Pipeline Execution** (repeat N times, each fully isolated):
   a. Initial Analysis (subagent) → `analysis-v1.md`
   b. Self-Critique (subagent) → `critique-v1.md`
   c. Refinement (subagent, may iterate) → `analysis-v2.md` (v3, v4…)
   d. Strict Review Gate (3 subagents, IDENTICAL prompts, quorum 2/3) → PASS or loop to (c)
3. **Synthesis** — Compare pipeline outputs, identify consensus/divergence, produce consolidated analysis

---

## Phase 1: Setup

### Step 1.1: Read Problem Statement

Read the problem statement file provided by the user. Extract:

- Core problem description
- Constraints and requirements
- Any existing attempted solutions
- User's explicit solution directions (MANDATORY)

### Step 1.2: Identify User Guidance

User guidance comes in several forms:

1. **Explicit solution directions** — "Consider these 3 approaches..."
2. **Constraints** — "Must use existing infrastructure..."
3. **Non-negotiables** — "Do NOT use approach X..."

Create a guidance document:

```
# User Guidance (MANDATORY)

## Solution Directions (MUST implement ALL)

1. [Direction 1]: [Description]
2. [Direction 2]: [Description]
3. [Direction 3]: [Description]

## Constraints

- [Constraint 1]
- [Constraint 2]

## Non-Negotiables

- [What to avoid]
```

### Step 1.3: Create Pipeline Structure

```
workspace/
├── meta/
│   ├── user-guidance.md          # Extracted user requirements
│   └── workflow-log.md           # Progress tracking
├── pipelines/
│   ├── pipeline-01/
│   │   ├── README.md             # Pipeline status
│   │   └── iterations/
│   │       ├── analysis-v1.md
│   │       ├── critique-v1.md
│   │       └── analysis-v2.md
│   ├── pipeline-02/
│   │   └── ...
│   └── ...
└── final/
    └── consolidated-analysis.md
```

### Step 1.4: Behavioral Refresh

If a behavioral configuration layer exists (directive files, task tracking documents):

- Re-read at session start
- Re-read periodically during long workflows
- Ensure all subagent prompts align with its constraints

---

## Phase 2: Pipeline Execution

1. Read `references/subagent-prompt-templates.md` to memory.
2. For each step, extract the exact template text and substitute variables.

### Step 2.1: Initial Analysis Subagent

**Role:** Senior engineer performing a completely fresh, independent analysis.

**Mandatory inputs:**
- Problem statement file path
- User guidance file path (MANDATORY)
- Codebase paths to explore

**Requirements:**
- Provide COMPLETE, production-ready implementation specs for ALL user solution directions
- Perform own codebase exploration — assume nothing
- Output: `analysis-v1.md`

**Automatic rejection if:** any user direction missing, code truncated, helpers undefined, user constraints violated.

See `references/subagent-prompt-templates.md` § Initial Analysis Subagent for the full prompt template.

### Step 2.2: Self-Critique Subagent

**Role:** Hostile reviewer of the pipeline's own analysis.

**Mandatory inputs:**
- Analysis file to critique
- User guidance file (MANDATORY)

**Primary criterion:** User guidance coverage. Technical accuracy is necessary but NOT sufficient.

**Verification checklist:**
- User guidance coverage (each direction: spec present? code complete? helpers defined? integration specified?)
- Technical accuracy (code references verified? line numbers accurate? API claims correct? fabrications?)
- Completeness (action items concrete? estimates realistic? recommendation justified?)

**Output:** `critique-vN.md` with verdict: REJECT / NEEDS REVISION / READY FOR STRICT REVIEW.

See `references/subagent-prompt-templates.md` § Self-Critique Subagent for the full prompt template.

### Step 2.3: Refinement Subagent

**Role:** Fix ALL issues identified in the self-critique.

**Mandatory inputs:**
- Original analysis file
- Critique file
- User guidance file

**Requirements:**
- Address ALL issues from critique
- ALL code must be COMPLETE (no truncation)
- ALL helper methods must be defined
- Include explicit "Corrections Applied" table

**Output:** `analysis-v(N+1).md`

See `references/subagent-prompt-templates.md` § Refinement Subagent for the full prompt template.

### Step 2.4: Strict Review Gate

**CRITICAL: All reviewers MUST receive IDENTICAL prompts.** The quorum is meaningless if reviewers have different criteria. Do NOT create specialized reviewers.

**Role:** Strict reviewer conducting final quality verification.

**Mandatory inputs (same for all 3):**
- Analysis file to review
- User guidance file (MANDATORY)
- Codebase paths for verification

**Pass criteria (ALL must be met):**
1. ALL user solution directions have actionable implementation specs
2. ALL code is COMPLETE (no truncation, no undefined helpers)
3. ALL technical claims verified against actual source code
4. Recommendation is justified and aligns with user guidance

**Quorum rules:**
- Launch 3 reviewers with IDENTICAL prompts (in parallel)
- 2/3 = PASS (acceptable), 3/3 = PASS (ideal)
- 1/3 or 0/3 = FAIL → return to refinement

See `references/subagent-prompt-templates.md` § Strict Review Subagent for the full prompt template.

---

## Phase 3: Synthesis

After all pipelines pass their strict review gates:

### Step 3.1: Compare Outputs

For each user solution direction:

- What did each pipeline recommend?
- Where do they agree? (HIGH CONFIDENCE)
- Where do they diverge? (NEEDS INVESTIGATION)

### Step 3.2: Produce Consolidated Analysis

```
# Consolidated Solution Analysis

## Consensus Findings

[Findings that ALL pipelines agreed on]

## Divergent Findings

[Findings where pipelines disagreed, with analysis of why]

## Recommended Approach

[Synthesis of the best elements from each pipeline]

## Implementation Plan

[Unified action items drawing from all pipelines]
```

---

## Common Failure Modes

| Failure Mode | Symptom | Prevention | Recovery |
|---|---|---|---|
| Ignoring user guidance | Technically accurate but misses user directions | User guidance as MANDATORY input + PRIMARY review criterion | Restart pipeline — patching won't fix fundamental misalignment |
| Different review criteria | Reviewers disagree for wrong reasons | ALL reviewers get IDENTICAL prompts | Discard results, re-run with identical prompts |
| Code truncation | `// ... rest similar ...` or omitted helpers | Explicit ban + checks in critique and review | Return to refinement with specific completion instructions |
| Fabricated claims | Analysis claims code does something it doesn't | Require file paths + line numbers; reviewers spot-check 3+ claims | Mark fabrication explicitly, re-evaluate affected sections |
| Undefined helper methods | Integration code calls non-existent methods | Explicit check in critique and review checklists | Trace every method call, define complete implementations |
| Abstract action items | "explore", "consider", "evaluate" | Explicit ban in prompts; require concrete tasks with effort estimates | Replace each vague item with a concrete, actionable task |

For detailed recovery procedures and decision trees, see `references/failure-mode-recovery.md`.

---

## Time Management

See `references/time-estimation-guide.md` for detailed time estimates by problem complexity, parallel execution opportunities, and time budgeting strategies.

Key rule: **DO NOT skip the strict review gate** — it is the quality guarantee. If running behind, reduce pipeline count (minimum 3 for meaningful comparison) or skip synthesis if pipelines are highly consistent.

---

## Example Invocation

**User:** "Analyze this problem deeply. Consider these 3 approaches: (1) Approach A, (2) Approach B, (3) Approach C."

**Orchestrator response:**

1. Create user guidance document capturing all 3 approaches as MANDATORY
2. Create N pipeline directories (recommend 3–5)
3. For each pipeline:
   - Dispatch analysis subagent (with all 3 approaches in prompt)
   - Dispatch self-critique subagent
   - Dispatch refinement subagent (if needed)
   - Dispatch 3 IDENTICAL strict reviewers
   - Record pass/fail
4. Synthesize passing pipelines into final analysis

---

## Troubleshooting

**"Reviewers disagree for confusing reasons"** → Check if prompts differ. Re-run with IDENTICAL prompts.

**"Pipeline keeps failing strict review"** → Check which criterion fails. Usually user guidance coverage or code truncation.

**"Analysis drifts from user guidance"** → Subagent prompts aren't emphasizing user guidance strongly enough. Make it the FIRST thing mentioned.

**"Code is truncated despite instructions"** → Add explicit word/line count requirements and explicit ban on "..." truncation.

**"Fabricated claims getting through"** → Add spot-check verification to strict review prompt. Require citing file paths and line numbers.

---

## References

- `references/subagent-prompt-templates.md` — Full prompt templates for all subagent roles
- `references/failure-mode-recovery.md` — Detailed recovery procedures and decision trees
- `references/time-estimation-guide.md` — Time estimates by problem complexity
