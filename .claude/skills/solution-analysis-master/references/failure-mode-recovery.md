# Failure Mode Recovery Guide

Failure modes observed in pipeline executions and their recovery procedures.

---

## Failure Mode 1: Ignoring User Guidance

### Symptoms
- Analysis is technically accurate but doesn't address user's solution directions
- Strict reviewers with "completeness" focus reject while "technical" reviewers approve
- User becomes frustrated despite "passing" technical verification

### Example

**What happened:**
- User provided 3 explicit solution directions
- Analysis focused on bug fixes in existing code instead of the requested directions
- Two reviewers (Technical, Architecture) APPROVED on technical grounds
- One reviewer (Completeness) REJECTED for ignoring user guidance
- User overrode the "passing" quorum because the approval was meaningless

**Root Cause:**
- Subagent prompts didn't emphasize user guidance as PRIMARY criterion
- Different review criteria made quorum meaningless
- Technical accuracy ≠ strategic alignment

### Prevention

1. **User guidance document is MANDATORY input for every subagent**
   ```markdown
   2. **User Guidance (MANDATORY):** [PATH]
   ```

2. **Self-critique PRIMARY criterion is user guidance coverage**
   ```markdown
   ### User Guidance Coverage (PRIMARY - any failure = REJECT)
   ```

3. **Strict review FIRST checks user guidance compliance**
   ```markdown
   ## YOUR MANDATE
   1. ALL user solution directions have actionable implementation specs
   [This must be FIRST]
   ```

4. **ALL reviewers get IDENTICAL prompts**
   - Do NOT create specialized reviewers
   - Different criteria = meaningless quorum

### Recovery

If a pipeline fails strict review for ignoring user guidance:

**DO NOT** attempt to patch the existing analysis. The analysis has fundamental misalignment that can't be fixed incrementally.

**Instead:**
1. Mark the pipeline as FAILED
2. Document the failure reason
3. Start a NEW pipeline with stronger user guidance emphasis in prompts
4. Add explicit checks to the new pipeline's subagent prompts

---

## Failure Mode 2: Different Review Criteria (Invalid Quorum)

### Symptoms
- Reviewers disagree but for different reasons
- One reviewer focuses on code quality, another on completeness, another on architecture
- "Quorum" passes but the approval doesn't mean what you think

### Example

**What happened:**
- Reviewer A (Technical): APPROVE - "All code claims verified"
- Reviewer B (Architecture): APPROVE - "Options are architecturally sound"
- Reviewer C (Completeness): REJECT - "Missing 2 of 3 user directions"

**Result:** 2/3 quorum "passed" but the approval was meaningless because reviewers evaluated different things.

### Root Cause

Reviewers had specialized focuses:
```markdown
# Reviewer A prompt (WRONG)
You are focused on TECHNICAL ACCURACY...

# Reviewer B prompt (WRONG)  
You are focused on ARCHITECTURAL SOUNDNESS...

# Reviewer C prompt (WRONG)
You are focused on COMPLETENESS...
```

### Prevention

**ALL reviewers MUST receive the IDENTICAL prompt.**

The template in `references/subagent-prompt-templates.md` under "Strict Review Subagent" must be used VERBATIM for all 3 reviewers. The only differences should be:
- File paths (same files for all)
- Output file names (if tracked separately)

### Recovery

If you've already run specialized reviewers:

1. **Discard ALL their results** - the quorum is invalid
2. **Re-run with IDENTICAL prompts** using the standard template
3. **Document the lesson** in the pipeline README

---

## Failure Mode 3: Code Truncation

### Symptoms
- Analysis contains phrases like:
  - `// ... rest of implementation similar to v1 ...`
  - `// ... existing code ...`
  - `[Similar helper methods omitted for brevity]`
- Strict review fails on "code completeness" criterion

### Example

**What happened:**
- Analysis-v1 had complete code for Solution A but truncated Solution B
- Self-critique found: "Code is INCOMPLETE - explicitly states '... rest of implementation similar to v1 ...'"
- Reviewer rejected: "Solution B lacks actual implementation code"

### Root Cause

Subagent ran out of context or tried to be "efficient" by not repeating similar patterns.

### Prevention

1. **Explicit word/line count requirements in prompts:**
   ```markdown
   ### COMPLETE Implementation (~300+ lines)
   [Full code, not truncated]
   ```

2. **Explicit ban on truncation:**
   ```markdown
   **AUTOMATIC REJECTION IF:**
   - Code is truncated with "..."
   - Phrases like "similar to above" or "rest omitted"
   ```

3. **Self-critique specifically checks for truncation:**
   ```markdown
   ## Code Completeness Issues
   | Issue | Severity | Description |
   | Truncated code | CRITICAL | [where] |
   ```

### Recovery

1. **Identify ALL truncated sections** in the critique
2. **Create specific fix instructions** for each:
   ```markdown
   ### Fix 2: Complete [ComponentName]
   **Problem:** Code truncated at line 150 with "... rest similar..."
   **Required Fix:** Provide complete implementation of:
   - methodA() (~50 lines)
   - methodB() (~30 lines)
   - cleanup/close method (~20 lines)
   ```
3. **Refinement subagent MUST output complete code** - verify before strict review

---

## Failure Mode 4: Fabricated Claims

### Symptoms
- Analysis claims code does something it doesn't
- Line numbers are way off (more than 10 lines)
- Method/class names don't exist
- Capabilities are attributed to wrong components

### Example

**What happened:**
- Analysis-v1 claimed a component overrides a method to implement custom behavior
- Self-critique verified: the method was actually a NO-OP (returns a default/false value)
- Analysis also attributed a standard library feature as a project-specific innovation

### Root Cause

Subagent made assumptions about what code "probably" does based on naming conventions rather than actually reading it.

### Prevention

1. **Require explicit verification in analysis:**
   ```markdown
   **VERIFIED via:** read_file
   **FILE:** /exact/path/to/file.java
   **LINE:** [exact number]
   **CODE:** [actual snippet]
   ```

2. **Strict review spot-checks claims:**
   ```markdown
   ### Technical Accuracy (any fabrication = REJECT)
   Spot-check at least 3 claims against actual source files
   ```

3. **Include source code snippets** as evidence

### Recovery

1. **Mark fabrication explicitly:**
   ```markdown
   ## Corrections Applied
   
   ### CORRECTION: [Component] Capabilities
   **v1 FABRICATION:** "[Component] implements [claimed behavior]"
   **v2 CORRECTION:** Method actually [actual behavior]
   **Evidence:** [actual code snippet]
   ```

2. **Re-evaluate affected sections** - if fabrication was foundational, the entire solution built on it is suspect

3. **Add explicit "What X ACTUALLY Does" section** in refinement

---

## Failure Mode 5: Undefined Helper Methods

### Symptoms
- Analysis uses methods that don't exist in the codebase
- Integration code calls helpers without defining them
- Strict review fails on "helpers defined" criterion

### Example

**What happened:**
- Integration code used helper methods (e.g., `isTypeMatch(info)` and `extractValue(info)`)
- These methods didn't exist in the codebase
- Reviewer rejected: "Integration code uses undefined helper methods"

### Root Cause

Analysis assumed helpers would be "obvious to implement" and didn't provide them.

### Prevention

1. **Explicit check in self-critique:**
   ```markdown
   - [ ] Are all helper methods defined?
   ```

2. **Require full method signatures AND bodies:**
   ```markdown
   ### Helper Methods
   
   private boolean isTypeMatch(TypeInfo info) {
       // COMPLETE implementation, not signature only
   }
   ```

3. **Strict review specifically checks:**
   ```markdown
   | Direction | ... | Helpers Defined | ... |
   ```

### Recovery

1. **Trace every method call** in integration code
2. **For each undefined method:**
   - Examine existing codebase for similar patterns
   - Define complete implementation
   - Include error handling
3. **Add to refinement with explicit instructions:**
   ```markdown
   ### Fix: Define [helperMethodName]()
   
   Examine [relevant class/record] in [source file]
   The method must [specific behavior]
   Include null safety and error handling
   ```

---

## Failure Mode 6: Abstract/Vague Action Items

### Symptoms
- Action items use words like "explore", "consider", "evaluate", "investigate"
- No effort estimates
- No clear acceptance criteria
- Items can't be directly turned into tasks

### Example

**Bad action items:**
```markdown
## Action Items
- Explore [approach A]
- Consider implementing [approach B]
- Evaluate performance implications
```

### Prevention

1. **Explicit ban in prompts:**
   ```markdown
   **AUTOMATIC REJECTION IF:**
   - ANY "explore" or "consider" in action items
   ```

2. **Require specific format:**
   ```markdown
   ## Action Items
   | ID | Task | Effort | Acceptance Criteria |
   |----|------|--------|---------------------|
   | 1  | Create CacheComponent.java | 4h | Class compiles, tests pass |
   ```

### Recovery

1. **Replace each vague item with concrete task:**
   - "Explore [approach A]" → "Read [SourceComponent].java lines 100-200, document actual behavior"
   - "Consider [approach B]" → "Create CacheComponent.java with populate() and resolve() methods"
   - "Evaluate performance" → "Add metrics for cache hit/miss rate, run load test with representative data"

---

## Recovery Decision Tree

```
Pipeline failed strict review
            │
            ▼
┌──────────────────────────────────────────────┐
│ What criterion failed?                        │
└──────────────────────────────────────────────┘
            │
    ┌───────┴───────────────────────────┐
    │                                   │
    ▼                                   ▼
User Guidance                    Technical Issues
(Missing solutions)              (truncation, fabrication, helpers)
    │                                   │
    ▼                                   ▼
RESTART PIPELINE              REFINE EXISTING ANALYSIS
(Fundamental misalignment     (Fixable issues)
 can't be patched)                     │
                                       ▼
                              Create specific fix instructions
                                       │
                                       ▼
                              Run refinement subagent
                                       │
                                       ▼
                              Re-run strict review
                                       │
                              ┌────────┴────────┐
                              │                 │
                              ▼                 ▼
                           PASS              FAIL
                              │                 │
                              ▼                 ▼
                        Pipeline          Loop back to
                        Complete          refinement
                                          (max 3 iterations)
```

---

## Iteration Limits

To prevent infinite loops:

- **Self-critique → Refinement:** Max 3 iterations
- **Strict Review failures:** Max 2 re-submissions

If limits reached:
1. Document what's blocking progress
2. Escalate to user with specific questions
3. Consider simplifying requirements or splitting into smaller analyses
