# Time Estimation Guide

Realistic time estimates based on actual pipeline execution experience.

---

## Baseline: Single Pipeline

A single pipeline through strict review gate takes approximately:

| Phase | Time | Notes |
|-------|------|-------|
| Initial Analysis Subagent | 10-15 min | Includes codebase exploration |
| Self-Critique Subagent | 5-8 min | Reading analysis + verification |
| Refinement Subagent | 8-12 min | Fixing issues, completing code |
| Second Critique (if needed) | 5 min | Usually faster |
| Second Refinement (if needed) | 8 min | Usually faster |
| Strict Review (3 subagents) | 8-12 min | Run in parallel |
| Quorum Decision | 2 min | - |
| **Total (happy path)** | **35-45 min** | No re-work needed |
| **Total (1 iteration)** | **50-60 min** | One refinement cycle |
| **Total (2 iterations)** | **65-75 min** | Two refinement cycles |

---

## Multi-Pipeline Sessions

### 3 Pipelines (Minimum for Meaningful Comparison)

| Phase | Time |
|-------|------|
| Setup | 15 min |
| Pipeline 1 (learning curve) | 60 min |
| Pipeline 2 | 50 min |
| Pipeline 3 | 50 min |
| Synthesis | 20 min |
| **Total** | **3h 15min** |

### 5 Pipelines (Comprehensive Analysis)

| Phase | Time |
|-------|------|
| Setup | 15 min |
| Pipeline 1 (learning curve) | 60 min |
| Pipeline 2 | 50 min |
| Pipeline 3 | 45 min |
| Pipeline 4 | 45 min |
| Pipeline 5 | 45 min |
| Synthesis | 30 min |
| **Total** | **4h 50min** |

---

## Complexity Multipliers

### Problem Complexity

| Complexity | Multiplier | Example |
|------------|------------|---------|
| Low | 0.7x | Single component, clear solution |
| Medium | 1.0x | Multiple components, 2-3 solution directions |
| High | 1.5x | Cross-system, many components, 4+ directions |
| Very High | 2.0x | Novel problem, research required |

### Codebase Familiarity

| Familiarity | Multiplier |
|-------------|------------|
| Well-known | 0.8x |
| Somewhat familiar | 1.0x |
| Unfamiliar | 1.3x |
| Complex/Large | 1.5x |

### User Guidance Specificity

| Specificity | Multiplier | Example |
|-------------|------------|---------|
| Highly specific | 0.9x | "Use approach X with method Y" |
| Specific | 1.0x | "Consider approaches A, B, C" |
| General | 1.2x | "Find the best solution" |
| Vague | 1.5x | "Fix this problem" |

---

## Time Traps

### Trap 1: Fabrication Recovery

If a subagent fabricates claims about code behavior, recovery takes significant time:

| Activity | Time |
|----------|------|
| Detect fabrication in critique | 5 min |
| Verify actual code behavior | 10-15 min |
| Rewrite affected sections | 15-20 min |
| Re-run self-critique | 5 min |
| **Total Recovery** | **35-45 min** |

**Prevention:** Emphasize verification requirements in initial analysis prompt.

### Trap 2: Invalid Quorum Recovery

If you run specialized reviewers and realize quorum is meaningless:

| Activity | Time |
|----------|------|
| Realize problem | 5 min |
| Re-run 3 identical reviewers | 10 min |
| Process results | 5 min |
| **Total Recovery** | **20 min WASTED** |

**Prevention:** Use identical prompts from the start.

### Trap 3: User Guidance Misalignment

If pipeline ignores user guidance and needs restart:

| Activity | Time |
|----------|------|
| Complete pipeline (wasted) | 45-60 min |
| Realize misalignment | 5 min |
| Restart pipeline | 45-60 min |
| **Total Cost** | **90-120 min** |

**Prevention:** Make user guidance the FIRST thing in every subagent prompt.

---

## Parallel Execution Opportunities

### Safe to Parallelize

| Operations | When |
|------------|------|
| 3 Strict Reviewers | Same analysis, different subagents |
| Multiple Pipelines | After setup, if context allows |
| Codebase exploration | Read-only operations |

### Must Be Serial

| Operations | Reason |
|------------|--------|
| Analysis → Critique | Critique depends on analysis |
| Critique → Refinement | Refinement needs critique results |
| Refinement → Strict Review | Review needs refined analysis |

---

## Time Budgeting Strategy

### For a 4-Hour Session

**Aggressive (5 pipelines):**
- Risk: May not complete all pipelines
- Benefit: Maximum independent perspectives
- Strategy: Cut synthesis if running behind

**Conservative (3 pipelines):**
- Risk: Fewer perspectives
- Benefit: Guaranteed completion with synthesis
- Strategy: Spend more time per pipeline

**Recommended (4 pipelines):**
- Balance of breadth and depth
- Buffer for 1 pipeline failure
- Time for proper synthesis

### Budget Allocation

| Activity | % of Total Time |
|----------|-----------------|
| Setup | 5-10% |
| Pipelines | 70-80% |
| Synthesis | 10-15% |
| Buffer | 5-10% |

---

## When to Stop

### Stop Adding Pipelines If:

1. **Time pressure:** Less than 45 min remaining and synthesis not started
2. **Convergence:** Last 2 pipelines produced nearly identical recommendations
3. **Diminishing returns:** Each new pipeline finding the same issues

### Cut Scope If:

1. **Pipeline keeps failing:** After 2 strict review failures on same pipeline
2. **User guidance unclear:** Better to clarify than produce misaligned analysis
3. **Technical blockers:** Can't verify claims due to access issues

