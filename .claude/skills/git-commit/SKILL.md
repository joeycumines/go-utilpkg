---
name: git-commit
description: Orchestrate the complete git commit workflow - review staged changes, generate Kubernetes-style commit messages from diffs, and guarantee correctness before committing. Use when user says "commit", "commit these changes", or asks about making a git commit. Do NOT use for general git operations like branch management, merging, or pull requests.
license: MIT
---

# Git Commit Workflow

## Critical Mandate

**GUARANTEE CORRECTNESS** before executing any commit. You must verify the staged changes are exactly what should be committed. Committing incorrect changes is worse than not committing at all.

## Commit Execution

### Step 1: Review Staged Changes

Identify what is staged for commit:

```bash
git diff --staged
```

Examine the diff carefully. Understand the scope and intent of the changes.

### Step 2: Generate Commit Message

Produce a commit message following **Kubernetes-style guidelines** (NOT conventional commits):

**Subject Line (first line):**
- 50 characters or less, never exceeding 72 characters
- Use imperative mood: "Add feature", "Fix bug" (not "Added" or "Adds")
- Capitalize first word
- No period at end

**Body:**
- Single blank line between subject and body
- Explain "what" and "why" (not "how" — the code shows that)
- Wrap lines at 72 characters

**General Guidelines:**
- Do NOT include GitHub keywords (fixes #123) or @mentions
- Do NOT include pull request references
- Squash minor changes (typos, style fixes) together
- Be specific: "fixed stuff" or "updated code" are unacceptable

For curated good and bad examples, see [references/kubernetes-commit-examples.md](references/kubernetes-commit-examples.md).

### Step 3: Guarantee Correctness

**Use the strict-review-gate skill** to execute the "Rule of Two" protocol:

1. **Guarantee correctness** of the changes by spawning subagents to review
2. Require **two contiguous, issue-free reviews** before proceeding
3. Only after passing both reviews may you commit

The review must verify:
- Changes match the intended work
- No unintended files are included
- No syntax errors, type errors, or obvious bugs
- Tests pass (if applicable)
- The changes are ready for history

### Step 4: Execute Commit

After both reviews pass successfully:

```bash
git commit -m "<subject line>" -m "<body>"
```

Alternative with full message:

```bash
git commit -F /path/to/message.txt
```

## Tool Availability

If neither a dedicated git tool nor a shell is directly available, use a more ad-hoc mechanism — for example, executing shell scripts via another tool that supports code execution.

- **Shell available**: Use `git` commands directly (preferred)
- **MCP git server available**: Use MCP tools in place of shell commands
- **Neither available**: Execute git commands via any tool capable of running shell scripts (e.g., a code execution tool, a file-writing tool that can produce and invoke a `.sh` file)

The method matters less than the result: **a correct, well-formed commit**.

## Examples

**Example 1: Simple bugfix**

User says: "Commit the bugfix"

Steps:
1. Check `git diff --staged` to see what's staged
2. Review diff: fixes null pointer in auth module
3. Generate message: `Fix null pointer in auth module`
4. Guarantee correctness via strict-review-gate (2 reviews)
5. Execute: `git commit -m "Fix null pointer in auth module"`

**Example 2: Feature with context**

User says: "Commit the new caching layer"

Steps:
1. Check `git diff --staged`
2. Review diff: adds Redis caching, updates config, includes tests
3. Generate message:
   ```
   Add Redis caching layer

   Implements distributed caching using Redis for improved
   performance on read-heavy operations. Includes cache
   invalidation logic and configuration options.
   ```
4. Guarantee correctness via strict-review-gate (2 reviews)
5. Execute: `git commit -m "Add Redis caching layer" -m "Implements distributed caching..."`

## Troubleshooting

**Issue**: Nothing staged for commit

Check status: `git status`
Stage changes: `git add <files>` or `git add -A` for all changes

**Issue**: Wrong files included in commit

Unstage wrong files: `git reset HEAD <wrong-files>`
Stage correct files: Review `git status` and add only intended changes

**Issue**: Review finds critical bugs

Fix the bugs. Do not commit known broken code. Re-review fixed code.

**Issue**: Review surfaces unrelated problems

If the review finds issues outside the scope of this commit (e.g., pre-existing bugs, style violations in untouched code), document them but do not block the commit. Only staged changes must be correct. File separate issues or follow-up commits for unrelated findings.

**Issue**: Generated message is unclear

Refine the message. A good commit message should be understandable years later. Focus on "why" the change matters.
