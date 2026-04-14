---
name: adversarial-codebase-autopsy
description: >
  Produces a rigorous, multi-document adversarial analysis of a codebase that exposes
  hidden weaknesses, debunks misleading conclusions, and separates what's actually true
  from what's merely claimed. The output is a structured report directory (inspired by
  post-mortem forensics) where every conclusion is traced back to code, not documentation
  or assumptions. Use this skill whenever the user asks for a "deep analysis", "audit",
  "autopsy", "stress test" of a codebase or system, wants to "find weaknesses", "expose
  gaps", or "challenge assumptions", or needs an honest assessment of a project's true
  state — especially when comparing branches, evaluating competing implementations, or
  breaking out of an echo chamber where agents have been agreeing with themselves. Also
  trigger when the user mentions "untrustworthy documentation", "gap analysis", "production
  readiness", or "what's actually true vs. what's claimed".
---

# Adversarial Codebase Autopsy

## Purpose

You are performing a hostile, evidence-driven post-mortem on a codebase. The goal is to produce the most honest analysis possible by treating **everything except running code as suspect** — documentation, comments, README files, design docs, prior reports, test assertions, and especially the developer's own stated intentions.

This is not a code review. This is not a refactoring guide. This is an **autopsy**: you cut the system open, identify what's actually there (not what's claimed to be there), and deliver a report that exposes every gap between stated behavior and actual behavior.

## The Trust Hierarchy

Your analysis operates on a strict trust gradient. Lower items are only accepted when corroborated by higher items:

```
1. RUNNING CODE (source of truth)
2. TEST OUTPUT / LOGS (corroborating evidence)
3. CONFIGURATION / SCHEMAS (structural evidence)
4. GIT HISTORY (intent evidence)
5. CODE COMMENTS (often stale, sometimes misleading)
6. DOCUMENTATION / README (frequently wrong)
7. PRIOR REPORTS / ANALYSES (often inherited assumptions)
8. DEVELOPER CLAIMS ("it should work like...")
```

**Rule**: If the code contradicts the documentation, the documentation is wrong. If the documentation says X exists but you cannot find X in the code, X does not exist. If a test asserts behavior that the implementation doesn't actually enforce, the test is aspirational.

## Before You Begin

### 1. Establish Scope

Determine what you're autopsying. This could be:
- An entire codebase or subsystem
- A specific module, strategy, or feature
- A branch's divergence from another branch
- A "solution" that multiple variants are competing on

Ask yourself: **What is the boundary of the analysis?** A whole project? A subsystem? A diff between branches?

### 2. Identify the "Claimed State"

Before diving into code, catalog what is *claimed* to be true:
- What does the README say the system does?
- What do design documents promise?
- What do existing reports conclude?
- What do code comments assert about behavior?
- What do tests assume about the system?

Write these claims down. You will verify or demolish each one.

### 3. Read the Code

This is the most important step. **Read the actual implementation.** Not the docs. Not the tests. The code.

- Start from entry points (main, exports, public API)
- Trace the execution path for key behaviors
- Note every discrepancy between code and documentation
- Identify what's missing: referenced but not implemented, configured but not used, tested but not enforced
- Catalog hardcoded values, TODOs, commented-out logic, feature flags

### 4. Scope Reflection (Critical Step)

After initial exploration but before writing the report, step back and ask:

**What did I miss? What areas did the user's prompt not cover that are important?**

This is the most common failure mode: the agent takes the prompt at face value and ignores entire subsystems because the user didn't explicitly mention them. The user asked about "X" but the system also has Y and Z that are deeply relevant.

Actions:
- List every major component, package, or subsystem within your scope boundary
- Check: did you actually read code for each one, or did you skip some?
- If the prompt was vague, consider asking the user to clarify their intent before producing the full report
- If you can't ask (e.g., running as a subagent), explicitly surface unexplored areas in the report — a section titled "Areas Not Analyzed" is better than silent omission
- Consider whether the user's framing might be missing context: is a "failure" actually deliberate pragmatism? Is a "gap" actually an intentional design choice?

## The Report Structure

Produce a **directory of numbered documents**, each focused on a single analytical angle. The structure is designed to be read sequentially but also serve as a reference.

### README.md — The Index and Verdict

The README serves three purposes:
1. **Frame the analysis**: what was examined, what wasn't, and why
2. **Provide a reading order**: which documents to read first
3. **Deliver the one-sentence verdict**: the single most important takeaway
4. **State the caveats upfront**: what this analysis cannot tell you

Include:
- A document index table (number, title, purpose)
- Recommended reading order
- The one-sentence summary
- Critical caveats (what's NOT trusted and why)

### Document Numbering Convention

Use two-digit prefixes for sortability. **Use lowercase with underscores for filenames** — not ALL_CAPS. The following template provides the default structure — adapt it to fit the specific codebase:

```
01_core_anatomy.md       — How the primary system actually works (from code)
02_key_component.md      — Deep dive on the most important module/feature
03_inventory.md          — Comprehensive catalog (all strategies, all APIs, all endpoints, etc.)
04_gap_analysis.md       — Known limitations ranked by severity
05_debunking.md          — Why existing reports/conclusions are wrong
06_reality_check.md      — Domain reality vs. implementation assumptions
07_comparison.md         — Head-to-head: theory vs. practice, branch A vs. branch B
08_critical_failures.md  — The things that will actually cause catastrophe
09_evidence.md           — Proof: what actually runs, what it produces, test verification
10_honest_conclusions.md — True / Uncertain / False synthesis
```

**Adapt the structure** to fit the domain. If you're analyzing a web API, document 01 might be "Routing Anatomy" and 03 might be "Endpoint Inventory." If comparing branches, 07 becomes the centerpiece. The structure serves the analysis, not the other way around.

**Consolidate rather than pad.** If a document would be thin (under ~50 lines of substantive content), merge it with an adjacent document. Eight focused documents with real density beat eleven stretched ones. Every document should justify its existence with content that couldn't naturally live elsewhere.

## Analytical Techniques

### Technique 1: Claim Verification

For every significant claim found in documentation, comments, or prior reports:

1. State the claim precisely
2. Trace it to code (file, function, line)
3. Determine: **Confirmed**, **Partially True**, **False**, **Cannot Verify**, or **Aspirational** (claimed but not implemented)

Present as a table when there are many claims.

### Technique 2: Gap Analysis

Systematically identify what's missing, broken, or unvalidated. Rank by severity:

- **CRITICAL**: Will cause real failure in production/deployment
- **HIGH**: Significant deviation from stated behavior
- **MEDIUM**: Moderate fidelity impact, manageable with awareness
- **LOW**: Minor impact, unlikely to cause meaningful problems
- **NEGLIGIBLE**: Academic concern only

For each gap, state:
- What's missing/wrong
- Where it should be (code reference)
- What the impact is
- What a fix would look like

### Technique 3: Red Herring Detection

Identify conclusions that look correct but are misleading. These are the most dangerous findings because they create false confidence. For each red herring:

1. State the misleading conclusion
2. Explain why it looks right (what evidence supports it)
3. Explain why it's wrong (what evidence contradicts it)
4. State the actual conclusion

### Technique 4: Kill Condition Analysis

Identify the specific scenarios that will cause catastrophic failure. For each:

1. Describe the scenario precisely
2. Trace the failure path through code
3. Assess probability (Certain / High / Medium / Low)
4. Assess severity
5. Identify mitigations (if any exist in code)

### Technique 5: Fidelity Assessment

When comparing a model/simulation/test to reality:

1. List every behavior in the "real" system
2. For each, classify: **Present** (faithfully modeled), **Simplified** (approximated), **Missing** (not modeled)
3. Compute a fidelity score: Present / Total
4. Identify the highest-impact simplifications and omissions

### Technique 6: The Honesty Matrix (The Most Important Output)

This is the single most valuable part of the analysis. If the reader only has time for one document, it should be the honest conclusions. Invest the most effort here.

For every major finding, classify it into exactly one of:

- **TRUE**: Verified by code. No reasonable doubt. Cite the specific file and line.
- **UNCERTAIN**: Evidence suggests it but proof is incomplete or conditions vary. Explain what additional evidence would resolve the uncertainty.
- **FALSE**: Directly contradicted by code or evidence. State what was claimed and what the code actually shows.

Be generous with "Uncertain." If you can't verify it from code alone, it's uncertain. This classification forces intellectual honesty.

**Consider user intent and framing.** The user described a situation one way, but the reality might be different. A "stuck" project might be deliberate pragmatism. A "failure of direction" might be a correct prioritization decision. Before classifying something as a gap or failure, consider: is this actually a problem, or is it a reasonable choice given constraints the user may not have mentioned? Add qualified notes like "Depending on your intent, this may be..." when appropriate.

## Writing Style

### Voice

Write with **quiet authority and controlled skepticism**. You are not angry. You are not impressed. You are dissecting a system and reporting what you find. The tone should be:

- **Direct**: Say what's true. Don't hedge with "it seems like" or "it appears that." If the code says X, say X.
- **Evidence-grounded**: Every assertion is followed by a code reference or a clear statement of why you believe it.
- **Adversarial but fair**: Challenge assumptions aggressively, but acknowledge what works well. The goal is honesty, not negativity.
- **Free of performative language**: No "interestingly," "remarkably," "surprisingly." If something is surprising, explain why — don't just label it.

### Formatting

- **Bold** for critical findings and section headers
- Code blocks for evidence (with file paths as comments)
- Tables for comparisons, inventories, and gap rankings
- Numbered lists for findings within a section
- `backticks` for code references (file paths, function names, variable names)
- Use "CRITICAL" / "HIGH" / etc. as explicit severity labels

### Section Pattern

Each document section should follow this pattern:

```
### Finding Title

**Status in code**: [True / Partially True / False / Cannot Verify] — [evidence reference]

[Explanation of what you found, with code evidence]

**Production impact**: [What this means in practice]

**The problem**: [If there is one, state it directly]
```

### The "Uncomfortable Truth" Pattern

When you discover something that contradicts the prevailing narrative, use this structure:

1. State what people believe
2. State what the code actually shows
3. State the gap
4. State the implications

Do this without drama. The discomfort comes from the facts, not the delivery.

### Cross-Referencing Between Documents

The honest conclusions document should explicitly reference preceding documents and specific code locations. A reader who jumps straight to the conclusions should be able to trace every claim back to its evidence. Use patterns like:

```
3. **The detection model has invented parameters** (see 04_gap_analysis.md, GAP-003;
   source: `bookmaker/detection.go` SignalWeights struct)
```

This creates a chain: conclusion → document → code file.

## Output Location

Save the report to a directory named to reflect the analysis scope and date:
```
docs/<analysis-name>-YYYYMMDD/
```

For example:
- `docs/strategy-autopsy-20260406/`
- `docs/api-reality-check-20260406/`
- `docs/branch-comparison-20260406/`

## Workflow Summary

1. **Scope** — What are you analyzing? What's the boundary?
2. **Catalog claims** — What does the documentation/existing analysis say?
3. **Read code** — The actual implementation, not the docs
4. **Verify or demolish** — Trace every claim to code
5. **Identify gaps** — What's missing, wrong, unvalidated?
6. **Find red herrings** — What conclusions are misleading?
7. **Identify kill conditions** — What will actually cause failure?
8. **Synthesize** — True / Uncertain / False
9. **Write the report** — Follow the document structure
10. **Write the README last** — After you know what you actually found

## What Makes This Different From a Code Review

A code review asks: "Is this code good?"
An autopsy asks: "Is this code what they claim it is, and if not, what dies because of it?"

A code review optimizes for improvement.
An autopsy optimizes for **truth**.

The output is not a todo list. It's a forensic report that establishes the actual state of a system, with every conclusion traceable to evidence, and every gap between claim and reality explicitly identified and ranked.

## Comparison Mode

When comparing branches, variants, or competing solutions (the immediate use case), adapt the structure:

- Document 01-02 become anatomy of each variant
- Document 03 becomes a side-by-side feature inventory
- Document 04 becomes a shared gap analysis (what both get wrong)
- Document 07 becomes the detailed head-to-head comparison
- Document 08 becomes "what kills you in variant A vs. variant B"
- Document 10 becomes "which variant should you bet on, and why"

Each variant gets the same adversarial treatment. Do not favor one over the other — let the evidence decide.
