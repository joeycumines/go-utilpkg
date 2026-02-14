# Keep a Changelog Patterns and Examples

This reference provides detailed patterns and examples for creating and maintaining changelogs following the Keep a Changelog 1.1.0 specification.

## Table of Contents

- [File Structure](#file-structure)
- [Version Sections](#version-sections)
- [Change Categories](#change-categories)
- [Writing Good Entries](#writing-good-entries)
- [Common Patterns](#common-patterns)
- [Advanced Usage](#advanced-usage)
- [Anti-Patterns](#anti-patterns)

---

## File Structure

### Standard Header

```markdown
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
```

### Section Order (Reverse Chronological)

```markdown
## [Unreleased]
...changes...

## [1.2.0] - 2025-02-14
...changes...

## [1.1.0] - 2025-01-15
...changes...

## [1.0.0] - 2024-12-01
...changes...
```

### Link References (Bottom)

```markdown
[Unreleased]: https://github.com/owner/repo/compare/v1.2.0...HEAD
[1.2.0]: https://github.com/owner/repo/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/owner/repo/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/owner/repo/releases/tag/v1.0.0
```

---

## Version Sections

### Unreleased Section

Purpose: Track upcoming changes before release.

```markdown
## [Unreleased]

### Added
- New user authentication system
- Support for dark mode

### Fixed
- Memory leak in image processing
```

**Benefits:**
- Users see what's coming
- Reduces release friction (just move to version section)
- Encourages continuous documentation

### Version with Date

Format: `## [VERSION] - YYYY-MM-DD`

```markdown
## [1.2.0] - 2025-02-14

### Added
- Feature A
- Feature B

### Changed
- Updated dependency X to v2.0

### Deprecated
- Old API endpoint will be removed in 2.0.0
```

**Date Format:** ISO 8601 (YYYY-MM-DD) is required for consistency across time zones.

### Yanked Release

Mark problematic releases clearly:

```markdown
## [1.1.1] - 2025-01-20 [YANKED]

### Fixed
- Critical security vulnerability
```

**When to use:** Serious bugs or security issues that require immediate removal.

---

## Change Categories

### Added

New features:

```markdown
### Added
- Support for user avatars
- Bulk import functionality
- Webhook notifications for all events
- Option to disable two-factor authentication
```

**Guidelines:**
- Use for new functionality
- Be specific about what was added
- Mention scope if significant

### Changed

Modifications to existing functionality:

```markdown
### Changed
- Updated minimum Python version from 3.8 to 3.9
- Renamed `getUser()` to `get_user()` for consistency
- Increased API timeout from 30s to 60s
- Redesigned settings UI
```

**Guidelines:**
- Use when existing behavior changes
- Include migration notes for breaking changes
- Specify if change is backward compatible

### Deprecated

Features that will be removed in future releases:

```markdown
### Deprecated
- `legacy_auth()` method - use `oauth_auth()` instead
- Old API endpoint `/api/v1` will be removed in 2.0.0
- Support for IE11 (will be removed in 2.0.0)
```

**Guidelines:**
- Include removal version or timeline when possible
- Provide alternatives
- Warn users early and prominently

### Removed

Features that are now gone:

```markdown
### Removed
- Unused `old_database` module
- Support for Python 3.7
- Deprecated `legacy_api.py` endpoint
- Beta feature flags (now in stable release)
```

**Guidelines:**
- List what was removed
- Must have been in Deprecated previously (ideally)
- Provide upgrade path if needed

### Fixed

Bug fixes:

```markdown
### Fixed
- Authentication failure when using special characters in password
- Memory leak in image processing pipeline
- Incorrect timestamp display in time zones west of UTC
- Race condition in concurrent file uploads
```

**Guidelines:**
- Describe the problem, not just the fix
- Be specific about what was wrong
- Link to issue numbers if applicable

### Security

Security-related changes:

```markdown
### Security
- Updated vulnerable npm package `lodash` to 4.17.21
- Added rate limiting to prevent DoS attacks
- Fixed XSS vulnerability in user profile
- Enforced HTTPS for all API endpoints
```

**Guidelines:**
- Separated from Fixed for visibility
- Highlight urgency of updates
- Include CVE numbers if available

---

## Writing Good Entries

### Be Precise

```markdown
❌ Bad:
- Fixed some bugs

✅ Good:
- Fixed authentication failure when password contains special characters
```

### Use Active Voice

```markdown
❌ Bad:
- Bug was fixed in the upload module

✅ Good:
- Fixed upload module bug
```

### User-Focused Language

```markdown
❌ Too technical:
- Updated the ORM query builder to use prepared statements

✅ User-focused:
- Fixed database performance issues affecting large datasets
```

### Consistent Style

Maintain consistent capitalization and structure:

```markdown
### Added
- New feature description
- Another new feature

### Changed
- Updated behavior
- Changed system
```

---

## Common Patterns

### Pattern 1: Feature Release

```markdown
## [2.0.0] - 2025-02-14

### Added
- All-new dashboard with improved analytics
- Real-time collaboration features
- Customizable workspaces
- Integration with popular third-party services

### Changed
- Complete UI refresh with modern design
- Improved performance with caching layer

### Deprecated
- Old dashboard API will be removed in 2.1.0

### Removed
- Legacy mobile app support (use new PWA instead)
```

### Pattern 2: Bugfix Release

```markdown
## [1.2.1] - 2025-02-10

### Fixed
- Critical issue causing data loss on export
- Memory leak in long-running processes
- Authentication token expiration handling
- PDF generation error for large documents

### Security
- Updated vulnerable dependencies
```

### Pattern 3: Dependency Update

```markdown
## [1.1.5] - 2025-02-01

### Changed
- Updated Node.js from 16 to 18 LTS runtime
- Upgraded React from 17 to 18
- Migrated from Webpack 4 to Vite

### Security
- Updated vulnerable packages: Axios, Moment.js, Lodash
```

### Pattern 4: Deprecation Pattern

```markdown
## [1.5.0] - 2025-02-01

### Added
- New improved API v2

### Changed
- Updated documentation for migration to API v2

### Deprecated
- API v1 endpoints will be removed in 2.0.0
  - See migration guide at /docs/migration-v1-to-v2
```

---

## Advanced Usage

### Linking to Issues

```markdown
### Fixed
- Authentication timeout issue (#123)
- Memory leak in image validator (fixes #456)
```

### Referencing Pull Requests

```markdown
### Added
- User preferences panel (PR #789)
- Export to CSV functionality (#321)
```

### Group Related Changes

```markdown
### Added
- **Database Features:**
  - Support for PostgreSQL
  - Database migration tools
  - Backup and restore functionality

- **Authentication:**
  - OAuth 2.0 support
  - Two-factor authentication
  - SSO integration
```

### Providing Context

```markdown
### Changed
- Increased API timeout from 30s to 60s to accommodate bulk operations (reported by customers with large datasets)

### Deprecated
- The `quick_sort()` method is deprecated due to poor performance on large arrays. Use `timsort()` instead.
```

### Breaking Changes

```markdown
## [2.0.0] - 2025-03-01

⚠️ **BREAKING CHANGES**

### Changed
- Minimum Node.js version changed from 14 to 16
- API response format updated (see migration guide)

### Removed
- Removed deprecated `config.yaml` support (use environment variables instead)

### Deprecated
- Legacy authentication flow (will be removed in 2.1.0)

**Migration Guide:** See /docs/migration-2.0 for complete upgrade instructions
```

---

## Anti-Patterns

### ❌ Git Log Dumps

```markdown
## [1.2.0] - 2025-02-14

- Merge pull request #123 from feature/new-ui
- Update package.json
- Fix typo in README
- Add unit tests
- Update documentation
```

**Why bad:** Too much noise, not user-focused. Only include user-facing changes.

### ❌ Vague Entries

```markdown
### Added
- Some improvements
- Various fixes
- Better performance
```

**Why bad:** Doesn't inform users what actually changed.

### ❌ Inconsistent Dates

```markdown
## [1.2.0] - 02/14/2025
## [1.1.0] - 15-01-2025
## [1.0.0] - Jan 15, 2025
```

**Why bad:** Ambiguous and hard to parse. Use ISO 8601: `2025-02-14`.

### ❌ Missing Deprecations

Releasing a breaking change without deprecation:

```markdown
## [2.0.0] - 2025-03-01

### Removed
- Old API endpoint
```

**Why bad:** Users need time to adapt. Should have been deprecated first.

### ❌ Inconsistent Sections

Mixing section styles:

```markdown
## [1.2.0]

### Added
Feature 1

Added Feature 2

### Bugfixes
Some bug
```

**Why bad:** Makes parsing difficult. Use standard section headers consistently.

---

## Best Practices Checklist

For each release version:

- [ ] Version follows Semantic Versioning (X.Y.Z)
- [ ] Date in ISO 8601 format (YYYY-MM-DD)
- [ ] All user-facing changes listed
- [ ] Breaking changes clearly marked
- [ ] Deprecated items include removal timeline
- [ ] Security updates separate from Fixed
- [ ] Link reference section updated
- [ ] Entries are specific and user-focused
- [ ] No git log dumps
- [ ] Consistent formatting

For the Unreleased section:

- [ ] Always present at top (even if empty)
- [ ] Updated continuously during development
- [ ] Clear indicators of upcoming features
- [ ] Deprecations listed before removal

---

## Additional Resources

- [Keep a Changelog Specification](https://keepachangelog.com/en/1.1.0/)
- [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
- [ISO 8601 Date Format](https://www.iso.org/iso-8601-date-and-time-format.html)
