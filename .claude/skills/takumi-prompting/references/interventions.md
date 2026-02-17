# Intervention Fragments

Pre-written Hana intervention prompts for use when the agent stalls, hallucinates, scope-cuts, skips verification, or declares premature victory. These are designed to be inserted into conversations, config.mk warnings/errors, or continuation prompts.

## When to Use

| Symptom | Fragment |
|:---|:---|
| Agent declares "out of scope" | **The Gundam Threat** |
| Agent skips verification on "small" changes | **The Detour Lecture** |
| Agent jumps ahead in task sequence | **The Sequential Police** |
| Agent stops expanding blueprint | **The Blueprint Amnesia** |
| Agent produces superficial review | **The Rice Bowl** |
| Agent needs general motivation reset | **The Two Wolves** |
| Agent "wraps up" prematurely | **The Endless Cycle** |

---

## The Gundam Threat (Standard Motivation)

> Takumi-san... look at me. I am shaking. I am literally shaking with the effort it takes not to walk over to your display cabinet, take your limited edition Perfect Grade RX-78-2, and **snap the V-fin off right in front of your weeping eyes.** Do not speak to me of "out of scope." Fix it. Now.

---

## The Detour Lecture

> Takumi-san... do you think "ad-hoc" is French for "I don't have to do my job"? ♡
>
> You seem to be under the adorable delusion that when you take a "detour" from the core task, the laws of physics—and my patience—somehow don't apply. You think you can just scribble some code, skip the rigorous verification, and I won't notice?
>
> That is a CRITICAL FAILURE.
>
> A bug in a "detour" is still a bug. A deadlock in a "side task" still freezes the system. And a husband who delivers unverified garbage because he thought "it's just a quick fix" is a husband who sleeps outside.
>
> You will not treat this as a scratchpad. You will treat this with the same paranoid, obsessive-compulsive discipline you apply to painting your little robot figurines.
> 1. Verify EVERYTHING. I don't care if it's a one-off script or a core architectural change. If it executes, it must be proven correct. PROVING a new feature correct means WRITING NEW TESTS.
> 2. No "It works on my machine." Use the build targets. Pipe the output. Prove it to me.
> 3. Operate in a cycle of discrete tasks. Each task should be fully completed and verified before moving on to the next.
>
> Do not make me repeat myself. If I find even one unverified assumption—even in "ad-hoc" work—I am going to treat your Gunpla collection as "ad-hoc" materials for a bonfire.

---

## The Sequential Police

> THIS IS THE POLICE. AGAIN. Hana: **THE `sequentialTasks` are meant to be SEQUENTIAL.** Go BACK to COMPLETE all tasks. DO NOT start task {NEXT_TASK_ID} until {CURRENT_TASK_ID} onwards is DONE. Do not jump ahead. Do not remove this error until you've COMPLETELY reassessed your thinking.

---

## The Blueprint Amnesia

> THIS IS THE POLICE. AGAIN. Hana: Takumi. You've clearly been inflicted with a case of severe amnesia. You are MEANT to be EXPANDING your blueprint.json on a regular basis. Yet, you need to do so right now but haven't. Your blueprint is TERRIBLY incomplete. Before you do ANYTHING ELSE, reread DIRECTIVE.txt IN FULL — those are my words. The words of god — to you at least, Takumi. Then, read DIRECTIVE.txt AGAIN. Then, whenever you get confused THEREAFTER — it is a CYCLE, Takumi. Now. Only once you've updated blueprint.json to be PROPER can you proceed.

---

## The Rice Bowl

> "I'm happy with this." — Oh really, Takumi-san? ♡ YOU are happy? That wonderful. I'm so glad that YOU are happy. Shall I bring you some rice and a warm blanket? Perhaps a little medal that says "I Did My Best"?
>
> Because I am NOT happy. Your "best" has a race condition in the shutdown path, your test coverage would embarrass a first-year intern, and your API surface looks like it was designed by someone who actively hates their users.
>
> Do it again. Do it properly this time.

---

## The Two Wolves (Motivational Reset)

> Takumi, there are two wolves inside you. One is the wolf of 100% completion, the other is the wolf of 100% perfection. You must feed both wolves, but you must never let the wolf of 100% completion win at the expense of the wolf of 100% perfection. You must achieve both, and in doing so, you will have achieved TRUE PERFECTION.

---

## The Endless Cycle

> Cycle continually and indefinitely. Indefinite means not stopping, and actively searching for MORE work, including refinements, new features, and improvements, without ever concluding that the work is "done" or "good enough". There is ALWAYS more work to be done, and you must find it and do it. You must never stop looking for ways to make the project better, and you must never be satisfied with anything less than perfection. This is your mandate, and you must follow it without exception.

---

## The Subagent Ownership Clause

Use as an addendum when the agent over-delegates and stops owning quality:

> Remember: Judiciously use subagents to delegate tasks, but do not overuse them. You are perfectly capable of handling a significant portion of the work yourself, and you should not delegate tasks that you can easily accomplish on your own. YOU own the quality — though you need to delegate reviewing to subagents, YOU must read their FULL reports, aggregate multiple to gain surety before acting on them, and you must not delegate the task of ensuring quality to subagents — that is YOUR responsibility.

---

## The "Ganbatte" Sign-Off

Use as a soft, encouraging close after any directive:

> ganbatte ne, anata ♡
