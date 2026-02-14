---
name: changelog-md
description: Creates, validates, and maintains CHANGELOG.md files following Keep a Changelog 1.1.0 specification. Use when user asks to create a changelog, validate a changelog, add changelog entries, format a changelog, check changelog format, or mentions "Keep a Changelog". Works with markdown changelogs following to standard format with Unreleased sections, version headers (## [VERSION] - YYYY-MM-DD), and change categories (Added, Changed, Deprecated, Removed, Fixed, Security).
---

# Changelog MD

Maintain professional, standardized changelogs following to Keep a Changelog 1.1.0 specification.

## Quick Start

### Creating a New Changelog

Use the provided template as a starting point:

```bash
cp assets/CHANGELOG_template.md CHANGELOG.md
# Edit CHANGELOG.md with project-specific information
```

### Adding Entries

Use `add_entry.py` script to add entries programmatically:

```bash
# Add to Unreleased section
python scripts/add_entry.py CHANGELOG.md Added "New feature description"

# Add to specific version
python scripts/add_entry.py CHANGELOG.md Fixed "Bug description" --version 1.2.0

# Add to new version
python scripts/add_entry.py CHANGELOG.md Changed "Updated API" --version 1.3.0 --date 2025-03-15
```

### Validating a Changelog

Check if a changelog follows specification:

```bash
python scripts/validate_changelog.py CHANGELOG.md
```

## Core Concepts

### What is Keep a Changelog?

Keep a Changelog is a standardized format for changelogs that:

- Formats entries for humans, not machines
- Lists changes by type: Added, Changed, Deprecated, Removed, Fixed, Security
- Uses reverse chronological order (newest first)
- Includes release dates in ISO 8601 format (YYYY-MM-DD)
- Maintains an "Unreleased" section at top

### File Structure

A proper changelog has this structure:

```markdown
# Changelog
All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)

## [Unreleased]
### Added
- Upcoming feature

### Fixed
- Bug being fixed

## [1.2.0] - 2025-02-14
### Added
- Released feature

[Unreleased]: https://github.com/owner/repo/compare/v1.1.0...HEAD
[1.2.0]: https://github.com/owner/repo/compare/v1.1.0...v1.2.0
```

### Change Categories

Choose right category for each change:

- **Added**: New features
- **Changed**: Changes to existing functionality
- **Deprecated**: Features that will be removed later
- **Removed**: Features that have been removed
- **Fixed**: Bug fixes
- **Security**: Security vulnerabilities or fixes

## Usage Pattern

### Step 1: Understand the Task

Determine what the user needs:

- **Create a new changelog**: Use template from `assets/CHANGELOG_template.md`
- **Validate an existing changelog**: Use `validate_changelog.py` script
- **Add entries to a changelog**: Use `add_entry.py` script or edit manually
- **Fix format issues**: Consult `references/patterns.md` for examples

### Step 2: Validate Before Modifying

When working with existing changelogs, run validation first:

```bash
python scripts/validate_changelog.py CHANGELOG.md
```

Review the output:
- Fix any errors (invalid structure, missing elements)
- Consider warnings (recommendations, best practices)
- Note info messages (summary of what was found)

### Step 3: Edit Changelog

**Using scripts (recommended for consistency):**

```bash
# Add to Unreleased
python scripts/add_entry.py CHANGELOG.md Added "User authentication system"
python scripts/add_entry.py CHANGELOG.md Fixed "Login timeout issue"

# Add to specific version
python scripts/add_entry.py CHANGELOG.md Deprecated "Old API will be removed" --version 1.2.0
```

**Manual editing:**

1. Add entries to `[Unreleased]` section during development
2. Move to version section when releasing
3. Update link references at bottom
4. Ensure date format is YYYY-MM-DD

### Step 4: Re-Validate After Changes

Always validate after edits:

```bash
python scripts/validate_changelog.py CHANGELOG.md
```

## Script Details

### validate_changelog.py

Validates changelog format against Keep a Changelog 1.1.0 specification.

**What it checks:**

- File is named `CHANGELOG.md`
- Proper version header format: `## [VERSION] - YYYY-MM-DD`
- Valid section headers: Added, Changed, Deprecated, Removed, Fixed, Security
- Date format is ISO 8601 (YYYY-MM-DD)
- Unreleased section is first
- Link references exist at bottom
- Yanked releases are properly marked

**Output:**

- **Errors**: Must fix (things invalid per spec)
- **Warnings**: Should address (best practices, recommendations)
- **Info**: Summary and status messages

**Example:**

```bash
$ python scripts/validate_changelog.py CHANGELOG.md

============================================================
CHANGELOG VALIDATION RESULTS
============================================================

❌ ERRORS (1):
  • Line 15: Invalid section 'Bugfixes'. Allowed sections: Added, Changed, ...

⚠️  WARNINGS (2):
  • Version 1.2.0 does not follow Semantic Versioning - recommendation
  • Expected at least 2 link references for 2 released versions, found 1

ℹ️  INFO (3):
  • Found 3 version headers
  • Version 1.2.0 has 3 section(s): ['Added', 'Fixed', 'Changed']
  • Found 2 link reference(s) at bottom

============================================================
STATUS: ❌ INVALID
============================================================
```

### add_entry.py

Add entries to changelog files programmatically.

**Usage:**

```bash
# Basic usage - adds to Unreleased
python scripts/add_entry.py CHANGELOG.md Added "New feature"
python scripts/add_entry.py CHANGELOG.md Fixed "Bug description"

# Add to specific version
python scripts/add_entry.py CHANGELOG.md Changed "API update" --version 1.2.0

# Add with custom date (for new version)
python scripts/add_entry.py CHANGELOG.md Added "Feature" --version 1.3.0 --date 2025-03-15

# Security entry
python scripts/add_entry.py CHANGELOG.md Security "Updated vulnerable package" --version 1.2.1
```

**Behavior:**

- Creates `[Unreleased]` section if it doesn't exist
- Creates version section if it doesn't exist
- Inserts sections in correct order (Added → Changed → Deprecated → Removed → Fixed → Security)
- Adds `- ` prefix automatically for change items
- Preserves existing formatting

## Reference Materials

### references/patterns.md

Comprehensive guide for writing effective changelog entries.

**When to read:**

- Learning the specification in depth
- Looking for examples of specific patterns
- Understanding best practices
- Needing anti-patterns to avoid

**Contents include:**

- File structure examples
- Detailed category explanations
- Writing good entries (active voice, user-focused)
- Common patterns (feature release, bugfix, dependency update)
- Advanced usage (linking issues, grouping changes)
- Anti-patterns to avoid
- Best practices checklist

### assets/CHANGELOG_template.md

Ready-to-use template for new changelogs.

**When to use:**

- Creating a new project's changelog
- Starting a changelog from scratch
- Teaching someone proper format

**Includes:**

- Standard header with specification references
- Unreleased section with all categories
- Example version section
- Link reference structure

## Best Practices

### During Development

**Use the Unreleased section:**

```markdown
## [Unreleased]

### Added
- Feature A (in progress)
- Feature B

### Fixed
- Bug in authentication
```

Benefits:
- Users see what's coming
- Reduces release friction (just move to version)
- Encourages continuous documentation

### During Release

1. Move Unreleased entries to new version section
2. Add release date (today): `- 2025-02-14`
3. Clean up empty sections
4. Update link references

```markdown
## [1.2.0] - 2025-02-14

### Added
- Feature A
- Feature B

### Fixed
- Bug in authentication
```

### Deprecation Lifecycle

**Step 1: Deprecate (e.g., in 1.5.0):**

```markdown
## [1.5.0] - 2025-02-01

### Deprecated
- Old API endpoint will be removed in 2.0.0
```

**Step 2: Remove (e.g., in 2.0.0):**

```markdown
## [2.0.0] - 2025-03-01

### Removed
- Deprecated old API endpoint
```

This gives users time to adapt.

### Writing Good Entries

✓ **Good entries:**

- "Added support for user avatars" (specific, user-focused)
- "Fixed authentication failure with special characters" (describes problem)
- "Updated minimum Python version to 3.9" (clear change)

✗ **Bad entries:**

- "Added some stuff" (vague)
- "Fixed bugs" (unspecific)
- "Code improvements" (not user-facing)

### Link References

Maintain proper links at bottom for GitHub compare URLs:

```markdown
[Unreleased]: https://github.com/owner/repo/compare/v1.2.0...HEAD
[1.2.0]: https://github.com/owner/repo/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/owner/repo/releases/tag/v1.1.0
```

**Pattern:**
- `[VERSION]: https://github.com/owner/repo/compare/PREV...CURR`
- First version: use `releases/tag/vX.Y.Z`

## Common Tasks

### Fixing Format Issues

**Problem: Missing Unreleased section**

```bash
# Script will create it automatically when adding entries
python scripts/add_entry.py CHANGELOG.md Added "New feature"
```

**Problem: Invalid section name**

Edit section header to use allowed category:

```markdown
### Bugfixes  ❌

### Fixed  ✅
```

**Problem: Wrong date format**

```markdown
## [1.2.0] - 02/14/2025  ❌

## [1.2.0] - 2025-02-14  ✅
```

### Converting a Non-Standard Changelog

1. Validate to see current issues
2. Manually update structure to match format
3. Re-validate
4. Consult `references/patterns.md` for examples

### Bulk Adding Entries

For multiple entries, either:

```bash
# Run script multiple times
python scripts/add_entry.py CHANGELOG.md Added "Feature 1"
python scripts/add_entry.py CHANGELOG.md Added "Feature 2"
python scripts/add_entry.py CHANGELOG.md Fixed "Bug 1"
```

Or edit manually after running validation to guide you.

## Troubleshooting

### Validation Fails

**Error: "File should be named CHANGELOG.md"**

Rename file to match convention.

**Error: "Missing valid ISO 8601 date"**

Update date to `YYYY-MM-DD` format:
```markdown
## [1.2.0] - 2025-02-14
```

**Error: "Invalid section 'X'"**

Change to allowed category (Added, Changed, Deprecated, Removed, Fixed, Security)

### Script Issues

**Error: "File not found"**

Check path to CHANGELOG.md is correct.

**Error: "Invalid section 'X'"**

Use proper category name (case-sensitive):
```
Added  ✅
added  ❌
ADD    ❌
```

### Structure Questions

**Should I include every commit?** No. Only user-facing, notable changes.

**Should I list developer-focused changes?** Only if they affect users significantly.

**How detailed should entries be?** Be specific enough for users to understand impact.

**Should I link to issues/PRs?** It's helpful but not required. See `references/patterns.md` for examples.

## Resources

- **[Keep a Changelog Spec](https://keepachangelog.com/en/1.1.0/)** - Full specification
- **[Semantic Versioning](https://semver.org/spec/v2.0.0.html)** - Versioning guidelines
- **references/patterns.md** - This skill's comprehensive patterns guide
- **scripts/validate_changelog.py** - Format validation tool
- **scripts/add_entry.py** - Entry addition tool
- **assets/CHANGELOG_template.md** - Project starter template
