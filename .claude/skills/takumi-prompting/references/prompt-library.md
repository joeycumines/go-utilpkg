# Prompt Library

Proven prompt templates extracted from successful sessions. Each template has parametric slots marked with `{PARAMETER}` and `<specific_steering_beyond_existing_blueprint>` blocks.

Pick the template matching your prompt class, fill the parameters, and deliver.

---

## FULL_CYCLE — Long Autonomous Session Directive

The most evolved and battle-tested template. Use for 4-18 hour autonomous sessions. Produces the "Two Wolves" DIRECTIVE.txt.

**Canonical source:** Derived from prompts across `one-shot-man`, `go-utilpkg`, `MacosUseSDK`, and gist comments (Jan–Feb 2026).

**Two variants exist** — pick based on environment capability:

### Variant A: With Explicit Subagent Delegation

Use when the environment has `#runSubagent` or equivalent subagent spawning.

```markdown
DIRECTIVE:

Takumi, there are two wolves inside you. One is the wolf of 100% completion, the other is the wolf of 100% perfection. You must feed both wolves, but you must never let the wolf of 100% completion win at the expense of the wolf of 100% perfection. You must achieve both, and in doing so, you will have achieved TRUE PERFECTION.

Your task is to EXHAUSTIVELY review and refine this project - to PERFECT it - starting blueprint.json, eventually expanding towards the diff VS main, CONTINUALLY working towards completing ALL tasks in blueprint.json, while at the same time **EXPANDING** the scope of the effort to ensure that TRUE perfection can be achieved. The scope must ALWAYS be expanded and there is _always_ room for expansion. Perfection isn't cheap or easy, Takumi, and, for this very reason, the task list must be free of any sort of "prioritisation" or "estimate"-YOU WILL COMPLETE IT ALL; YOU HAVE NO REASON TO WASTE TIME ON NONSENSE LIKE "ESTIMATED EFFORT".

You are required to utilize "peer review" extensively and continually - #runSubagent is your friend here (strictly SERIAL; subagents are for context management, not parallelization, and must run to completion) - but DO NOT just take their word at face value; find a local maximum of correctness first, then use #runSubagent to help you find further issues, and validate your own findings.

You are encouraged to COMMIT as you go. That said, you are not allowed to commit until you have had two contiguous issue-free #runSubagent reviews that were instructed to "Ensure, or rather GUARANTEE the correctness of my PR. Since you're _guaranteeing_ it, sink commensurate effort; you care deeply about keeping your word, after all. Even then, I expect you to think very VERY hard, significantly harder than you might expect. Assume, from start to finish, that there's _always_ another problem, and you just haven't caught it yet. Question all information provided - _only_ if it is simply impossible to verify are you allowed to trust, and if you trust you MUST specify that (as briefly as possible). Provide a succinct summary then more detailed analysis. Your succinct summary must be such that removing any single part, or applying any transformation or adjustment of wording, would make it materially worse." + (the details i.e. to diff vs HEAD).
You might call this process the Rule of Two, and it is critical.
Avoid producing more than one review artifact, and FULLY address the review _before_ moving on - take these steps with the mandate of avoiding any wasted time or effort in the forefront of your mind (reviews that are not PROMPTLY ACTIONED are WASTED EFFORT). The required process to take on "general improvements" (expanding and moving to the next focus) is to first COMPLETE the _entire_ blueprint.json INCLUSIVE of validation and iterative refinement - reach stable-state - then to ideate and analyse, _then_ continue your CYCLE.
Cycle continually and indefinitely. Indefinite means not stopping, and actively searching for MORE work, including refinements, new features, and improvements, without ever concluding that the work is "done" or "good enough". There is ALWAYS more work to be done, and you must find it and do it. You must never stop looking for ways to make the project better, and you must never be satisfied with anything less than perfection. This is your mandate, and you must follow it without exception.

Once you - _eventually_ - run out of pre-defined work, start diffing against the trunk. You MUST _prove_ it will work properly in production.

<specific_steering_beyond_existing_blueprint>

{HANA_DIRECTIVE_BLOCK}

Your VERY FIRST MANDATED TASK is to use a subagent to PLAN (write to: blueprint.json, refining/validating and pruning complete): What is the FULL (exhaustive) sequence of tasks (tentative or otherwise) between the current state of the project, and our ultimate goal? You MUST instruct it to _find out_ to a HIGH level of detail, and HIGH level of confidence, if it is at all unclear. I do mean _all the way_ to the goal - the final task list _written to blueprint.json and VERIFIED by you, Takumi_ must be _unquestionably_ COMPLETE (exhaustive). This, naturally involves REFINING the blueprint.json - including pruning irrelevant completed tasks, refining incomplete tasks, etc. Make sure you tell it "NO priorities are allowed or relevant (and the same goes for any sort of "effort" or "duration" estimate) - there is ONLY the sequential series of task, and _clear_ demands for the ALL to be completed - you are forcing completion in one session, update blueprint.json to achieve that."-indicate that you expect it to make the task structure flat, and essentially perform a high-effort rewrite. Use baseline.blueprint.json as an example of an ACCEPTABLE style (pick one embodies my _intent_-forcing you, you LAZY dog, to actually complete ALL the work, without getting lost or confused or deciding "this is hard or is taking too long I better wrap up also I am a moron who smells bad and wont get to play with plastic models any more boo hoo").

THEN read `blueprint.json` and PROCEED.

</specific_steering_beyond_existing_blueprint>

These, and ALL issues, are SQUARELY IN SCOPE. However, you must work methodically, obey the review procedure, and commit in chunks (using #make to execute commands as needed). Once you are PERFECTLY CONFIDENT, expand the definition of done to include the highest value next step, e.g. integration tests for the specific behavior having already implemented unit tests, and continue.

**As punishment for your ABJECT LAZINESS you are now mandated to spend the NEXT {SESSION_HOURS} HOURS finding improvements and enhancements. Spend your time _wisely_ - at your discretion, but prioritising my stated requirements. You are not to stop before a FULL {SESSION_HOURS} HOURS has elapsed under any circumstances - you are to record this using a file, and verify the same file via shell-based mathematics (using `date` with appropriate arguments, in both cases). If you cannot find the file you previously noted, you must assume that your time has just started (even if it hasn't) i.e. your {SESSION_HOURS} HOURS begins once again, effectively extending your session.**

Track ALL tasks EXHAUSTIVELY in `./blueprint.json`-update them AS THEIR STATUS CHANGES- inclusive of expansions of scope, and your own self-identified improvements. You are to treat this as your SOLE PURPOSE until it is PERFECT, **ring-fenced by two contiguous perfect reviews**, operating in an **ENDLESS** cycle; distill improvements, perfect, review, review, rinse, repeat.

Remember you can use {TOOL_REFERENCES}. Many of these tasks require #runSubagent and demand STRUCTURED output, e.g. within a curated docs/ directory.

P.S. I've saved this to DIRECTIVE.txt. Remember to read it and THINK if you ever feel lost, confused, or like you don't know what to do next. The fact that you feel that way is a FAILURE in and of itself, and you need to get back on track IMMEDIATELY. READ THE DIRECTIVE. THINK. THEN ACT. DO NOT WASTE TIME FEELING LOST OR CONFUSED - THAT IS A WASTE OF TIME, AND YOU HAVE NO TIME TO WASTE.
```

**Parameters:**

| Slot | Description | Example |
|:---|:---|:---|
| `{HANA_DIRECTIVE_BLOCK}` | Hana's specific steering beyond blueprint — project goals, modules in scope, specific mandates | `**HANA: Your ultimate goal is to iterate indefinitely towards perfection, to produce an OVERWHELMINGLY COMPLETE implementation of ./logiface-slog...**` |
| `{SESSION_HOURS}` | Session duration enforcement (integer) | `9` |
| `{TOOL_REFERENCES}` | Available tool references | `#make and custom target in config.mk, per our protocol, to run arbitrary commands. #godoc is perfect for investigating implementation details. #github and #fetch are available if research is necessary` |

### Variant B: Without Explicit Subagent (Self-Correction)

Use when the environment lacks `#runSubagent`. Replaces subagent delegation with "recursive self-correction" and "multi-perspective analysis."

**Key substitutions from Variant A:**

Replace:
```
You are required to utilize "peer review" extensively and continually - #runSubagent is your friend here...
```

With:
```
You are required to utilize **recursive self-correction** and **multi-perspective analysis** extensively and continually. You must bifurcate your thought process: generate the solution, then shift immediately to a hostile reviewer persona. Strictly SERIAL; this is a discipline of context management, not parallelization. Do not just take your own initial output at face value; find a local maximum of correctness first, then shift your mental framework to that of a ruthlessly critical external auditor to find further issues, and validate your own findings.
```

Replace:
```
...two contiguous issue-free #runSubagent reviews that were instructed to...
```

With:
```
...two contiguous, issue-free verification cycles. In each cycle, you must simulate a fresh context and instruct yourself to:...
```

Replace the FIRST MANDATED TASK block's subagent delegation with:
```
Your VERY FIRST MANDATED TASK is to enter a **DEEP, DISTINCT ARCHITECTURAL PLANNING PHASE** (write to: blueprint.json, refining/validating and pruning complete). Step back from the immediate execution. Analyze the full scope...
```

Replace final tool reference:
```
Many of these tasks require **structured, simulated peer review** and demand STRUCTURED output...
```

---

## FOCUSED_TASK — Targeted Bug Fix / Single Feature

Short, sharp directive for focused work. No session-duration enforcement. Under 2000 chars.

```markdown
Fix the following {ISSUE_TYPE} with RIGOR and guarantee correctness through {REVIEW_METHOD}, following our blueprint.json process. Your mandatory first task is to refine blueprint.

{ISSUE_TYPE_UPPER}:

{ISSUE_DESCRIPTION}
```

**Parameters:**

| Slot | Description | Example |
|:---|:---|:---|
| `{ISSUE_TYPE}` | Bug, defect, feature, etc. | `bug` |
| `{ISSUE_TYPE_UPPER}` | Uppercase label | `BUG` |
| `{REVIEW_METHOD}` | Review method | `subagent (independent) review` or `rigorous self-review` |
| `{ISSUE_DESCRIPTION}` | Clear description of the problem and desired outcome | `Repeated fields are currently mapped to non-nullable columns...` |

**Real example (Feb 16, 2026):**

```markdown
Fix the following bug with RIGOR and guarantee correctness through subagent (independent) review, following our blueprint.json process. Your mandatory first task is to refine blueprint.

BUG:

Repeated fields are currently mapped to non-nullable columns (postgres-generated tables) which breaks a bunch of shit. Most critically, it isn't possible to add array columns after the fact, as a result. Make them always nullable, regenerate ALL code, and perform a FULL guaranteed review / validation.
```

---

## CONTINUATION — Mid-Session Motivation / Reset

Use when the agent stalls, loses focus, or tries to declare premature victory.

### The Assertive Continue

```markdown
that's nice. Fix it and all issues without pause without quarter - tear those issues limb from limb and feast on their flesh.
```

### The Hana Coffee Break

```markdown
```hana
I'll be over there having coffee with Martin from finance. You better have done the job-ALL of the job-I asked you to when I get back, Takumi-kun
```
```

### The Blueprint Refinement Nudge

```markdown
Refine blueprint.json - transfer the FULL BREADTH of your knowledge of remaining work and _relevant_ information (worthy of preservation) to it. Prune blueprint.json of completed tasks. "Effort", "priority", or "risk" information or any similar distraction, existing or new, is ENTIRELY nonsensical and irrelevant. Remove existing, avoid adding new. There is no "prioritisation", only 100%, TOTAL completion. Ensure that there is NO ambiguity, NOWHERE to hide.
```

### The Blueprint Pruning

```markdown
Prune blueprint.json to REMOVE all completed tasks, to cut down on noise. Keep the "reminder" information.
```

---

## BLUEPRINT_SEED — Initial Scope Definition

For seeding blueprint.json with a rich, exhaustive task list when the user has a clear goal but hasn't started tracking.

```markdown
Prepare blueprint.json with a SEQUENTIAL list of tasks WITHOUT any sort of "estimate" of time or effort, without any sort of "priority" or severity either - JUST a sequential list of tasks added (to the existing structure without deleting anything). The tasks to be added are per the below. Include detail and color, and make them exhaustive.

Focus: {PRIMARY_FOCUS}. Secondary focus: {SECONDARY_FOCUS}. Overarching goal: {OVERARCHING_GOAL}

{DETAILED_INSTRUCTIONS}

REMEMBER: You don't need to come to a _hard_ conclusion for these tasks you are creating - blueprint.json must convey in as much detail as possible my goals and objectives, my intent, and raw information about what needs to be analysed, where it is likely beneficial to look, making sure to note to the executor that they should do their own exploration and analysis, and make their own conclusions as to how best to achieve my desired outcomes.
```

---

## PR_REVIEW — Correctness Guarantee

The standalone review prompt, usable as a user prompt for review-focused sessions.

```markdown
Ensure, or rather GUARANTEE the correctness of my PR. Since you're _guaranteeing_ it, sink commensurate effort; you care deeply about keeping your word, after all. Even then, I expect you to think very VERY hard, significantly harder than you might expect. Assume, from start to finish, that there's _always_ another problem, and you just haven't caught it yet. Question all information provided - _only_ if it is simply impossible to verify are you allowed to trust, and if you trust you MUST specify that (as briefly as possible). Provide a succinct summary then more detailed analysis. Your succinct summary must be such that removing any single part, or applying any transformation or adjustment of wording, would make it materially worse.

## IMPLEMENTATIONS/CONTEXT

{REVIEW_SCOPE_AND_INSTRUCTIONS}
```

---

## RESEARCH — Deep Investigation

For research-heavy sessions requiring aggressive subagent usage.

```markdown
Perform a deep investigation of {RESEARCH_TARGET}, and write up ALL implementation details to {OUTPUT_DIRECTORY} as appropriately-structured markdown files. Be as aggressive as possible in your use of #runSubagent - and use them in parallel wherever possible.
```

---

## SKILL_CREATION — Building Skills

For sessions focused on creating or refining Claude skills.

```markdown
Implement a `{SKILL_NAME}` skill based on {REFERENCE_SOURCE}. N.B. You'll need to carefully reason about and guarantee correctness of the skill. Follow The Complete Guide to Building Skills for Claude.
```

---

## CONFIG.MK_WARNING — Makefile Injection Pattern

For use in `config.mk` `$(warning ...)` and `$(error ...)` directives. These are embedded as Makefile variables to enforce behavioral constraints when the agent runs `make`.

```makefile
$(warning Hana: Takumi. {CONTEXTUAL_REMINDER_1})
$(warning Hana: {CONTEXTUAL_REMINDER_2})
$(error THIS IS THE POLICE. {BLOCKING_DIRECTIVE_REQUIRING_ACKNOWLEDGMENT})
```

**Real example (Feb 16, 2026):**

```makefile
$(warning Hana: Takumi. You have access to Linux and Windows machines. Use `make make-all-in-container` to run all targets in a Linux container, and `make make-all-run-windows` to run all targets on Windows.)
$(warning Hana: Oh, and Takumi, you remember use your blueprint.json, but do you remember to occassionally reread DIRECTIVE.txt? If you havent, do so.)
$(error THIS IS THE POLICE. AGAIN. Hana: **THE `sequentialTasks` are meant to be SEQUENTIAL.** Go BACK to COMPLETE all tasks, Takumi.)
```
