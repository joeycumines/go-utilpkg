#!/usr/bin/env python3
"""
Validate CHANGELOG.md files according to Keep a Changelog spec 1.1.0

This script validates:
- File naming conventions
- Proper section structure with allowed categories
- Version format and ordering
- Date format (ISO 8601: YYYY-MM-DD)
- Link references at the bottom
- Yanked release formatting

Usage:
    python validate_changelog.py CHANGELOG.md
"""

import re
import sys
from pathlib import Path
from typing import List, Tuple, Optional


# Regex patterns
VERSION_PATTERN = r'^\[(?P<version>[^\]]+)\]$'
DATE_PATTERN = r'^\d{4}-\d{2}-\d{2}$'
VERSION_HEADER_PATTERN = r'^##\s+\[.+\]\s*[-]\s*\d{4}-\d{2}-\d{2}(\s+\[YANKED\])?$'
UNRELEASED_HEADER_PATTERN = r'^##\s+\[Unreleased\]$'
LINK_REF_PATTERN = r'^\[[^\]]+\]:\s+https?://\S+'
SECTION_HEADER_PATTERN = r'^###\s+(Added|Changed|Deprecated|Removed|Fixed|Security)$'
CHANGE_ITEM_PATTERN = r'^-\s+.+$'
# URL regex patterns for link references
COMPARE_URL_PATTERN = r'^\[[^\]]+\]:\s+https://[^/]+/[^/]+/compare/[^.]+\.\.\.[^.]+$'
RELEASES_URL_PATTERN = r'^\[[^\]]+\]:\s+https://[^/]+/[^/]+/releases/tag/[^.]+$'

# Valid section types (matching Keep a Changelog specification)
ALLOWED_TYPES = ['Added', 'Changed', 'Deprecated', 'Removed', 'Fixed', 'Security']


class ValidationResult:
    """Track validation results"""
    def __init__(self):
        self.errors: List[str] = []
        self.warnings: List[str] = []
        self.info: List[str] = []

    def add_error(self, message: str, line_num: Optional[int] = None):
        location = f"Line {line_num}: " if line_num else ""
        self.errors.append(f"{location}{message}")

    def add_warning(self, message: str, line_num: Optional[int] = None):
        location = f"Line {line_num}: " if line_num else ""
        self.warnings.append(f"{location}{message}")

    def add_info(self, message: str):
        self.info.append(message)

    @property
    def is_valid(self) -> bool:
        return len(self.errors) == 0


def validate_file_path(filepath: Path) -> ValidationResult:
    """Validate file path and name"""
    result = ValidationResult()

    # Check if file exists
    if not filepath.exists():
        result.add_error(f"File does not exist: {filepath}")
        return result

    # Check file name (Should be CHANGELOG.md)
    if filepath.name.lower() != 'changelog.md':
        result.add_error(
            f"File should be named 'CHANGELOG.md' (case-insensitive), "
            f"got: {filepath.name}"
        )

    return result


def parse_and_validate(filepath: Path) -> ValidationResult:
    """Parse and validate changelog content"""
    result = validate_file_path(filepath)
    if not result.is_valid:
        return result

    try:
        content = filepath.read_text(encoding='utf-8')
    except Exception as e:
        result.add_error(f"Failed to read file: {e}")
        return result

    lines = content.splitlines()

    # Check for empty file
    if not content.strip():
        result.add_error("File is empty")
        return result

    # Track structure
    current_version: Optional[str] = None
    current_section: Optional[str] = None
    versions_found: List[Tuple[str, int]] = []
    sections_found: List[Tuple[str, int]] = []
    link_refs: List[int] = []

    # Parse line by line
    for i, line in enumerate(lines, 1):
        stripped = line.strip()

        # Check for version headers
        if re.match(VERSION_HEADER_PATTERN, stripped):
            # Extract version
            match = re.search(r'\[([^\]]+)\]', stripped)
            if match:
                version = match.group(1)
                current_version = version
                versions_found.append((version, i))
                current_section = None

                # Check for YANKED tag
                is_yanked = '[YANKED]' in stripped.upper()

                if version.lower() == 'unreleased':
                    result.add_info(f"Found [Unreleased] section at line {i}")
                    if is_yanked:
                        result.add_warning("[Unreleased] should not be marked as [YANKED]", i)
                else:
                    # Check date format
                    date_match = re.search(r'\d{4}-\d{2}-\d{2}', stripped)
                    if not date_match:
                        result.add_error(
                            f"Version {version} missing valid ISO 8601 date (YYYY-MM-DD)",
                            i
                        )

        # Check for section headers
        elif re.match(SECTION_HEADER_PATTERN, stripped):
            match = re.match(r'^###\s+(\w+)$', stripped)
            if match:
                section = match.group(1)
                current_section = section
                sections_found.append((section, i))

                # Check if section is within a version context
                if not current_version:
                    result.add_warning(
                        f"Section '{section}' found outside version context",
                        i
                    )

        # Check for change items
        elif re.match(CHANGE_ITEM_PATTERN, stripped):
            if not current_section:
                result.add_warning(
                    "Change item found outside of a section (Added/Changed/etc.)",
                    i
                )

        # Check for link references (at bottom of file)
        elif re.match(LINK_REF_PATTERN, stripped):
            link_refs.append(i)

    # Validate version ordering (reverse chronological)
    if versions_found:
        result.add_info(f"Found {len(versions_found)} version headers")

        # Check if Unreleased is first
        if versions_found[0][0].lower() != 'unreleased':
            result.add_warning(
                "[Unreleased] section should be first (not found at top of file)"
            )

        # Check for semantic versioning in versions (optional recommendation)
        for version, line in versions_found:
            if version.lower() != 'unreleased' and not re.match(r'^v?\d+\.\d+\.\d+', version):
                result.add_info(
                    f"Version '{version}' does not follow Semantic Versioning "
                    f"(vX.Y.Z or X.Y.Z) - this is just a recommendation"
                )

    # Validate sections used (only allowed types)
    for section, line in sections_found:
        if section not in ALLOWED_TYPES:
            result.add_error(
                f"Invalid section '{section}' at line {line}. "
                f"Allowed sections: {', '.join(ALLOWED_TYPES)}",
                line
            )

    # Validate link references structure
    if link_refs:
        result.add_info(f"Found {len(link_refs)} link reference(s) at bottom")

        # Validate URL patterns for link references
        for line_num in link_refs:
            line_content = lines[line_num - 1].strip()
            version_match = re.match(r'^\[([^\]]+)\]:', line_content)
            if version_match:
                version = version_match.group(1)

                # Check if version matches URL pattern
                is_compare = re.match(COMPARE_URL_PATTERN, line_content)
                is_releases = re.match(RELEASES_URL_PATTERN, line_content)

                if not is_compare and not is_releases:
                    result.add_warning(
                        f"Link reference for '{version}' has invalid URL pattern. "
                        f"Expected 'https://github.com/owner/repo/compare/vA...vB' or "
                        f"'https://github.com/owner/repo/releases/tag/vX.Y.Z'",
                        line_num
                    )

        # Link refs should be at the very bottom (ignore blank lines)
        if link_refs[-1] < len(lines):
            non_link_content = [i for i in range(link_refs[-1] + 1, len(lines) + 1)
                               if i not in link_refs]
            # Filter out blank lines - they're not real content
            non_content_with_text = [i for i in non_link_content
                                     if lines[i-1].strip()]
            if non_content_with_text:
                result.add_warning(
                    f"Link references should be at the very bottom. "
                    f"Found content after last link reference"
                )

    # Check for proper link reference format (version links)
    version_refs = [v for v in versions_found if v[0].lower() != 'unreleased']
    if version_refs and len(link_refs) < len(version_refs):
        result.add_warning(
            f"Expected at least {len(version_refs)} link references "
            f"for {len(version_refs)} released versions, found {len(link_refs)}"
        )

    # Check for section consistency
    for version, version_line in versions_found:
        # Find sections for this version
        version_index = versions_found.index((version, version_line))
        is_last_version = version_index + 1 >= len(versions_found)

        version_sections = [
            (s, l) for s, l in sections_found
            if l > version_line and (is_last_version or l < versions_found[version_index + 1][1])
        ]

        if version_sections:
            result.add_info(
                f"Version {version} has {len(version_sections)} "
                f"section(s): {[s for s, _ in version_sections]}"
            )

        # Check if version has no changes at all
        version_end_line = (versions_found[versions_found.index((version, version_line)) + 1][1]
                           if versions_found.index((version, version_line)) + 1 < len(versions_found)
                           else len(lines))

        has_content = any(
            re.match(SECTION_HEADER_PATTERN, lines[i].strip()) or
            re.match(CHANGE_ITEM_PATTERN, lines[i].strip())
            for i in range(version_line, version_end_line - 1)
        )

        if not has_content and version.lower() != 'unreleased':
            result.add_warning(
                f"Version {version} has no change items or sections"
            )

    # Final summary
    if result.is_valid:
        result.add_info("✅ Changelog follows Keep a Changelog 1.1.0 specification")

    return result


def print_results(result: ValidationResult):
    """Print validation results"""
    print("\n" + "="*60)
    print("CHANGELOG VALIDATION RESULTS")
    print("="*60)

    if result.errors:
        print(f"\n❌ ERRORS ({len(result.errors)}):")
        for error in result.errors:
            print(f"  • {error}")

    if result.warnings:
        print(f"\n⚠️  WARNINGS ({len(result.warnings)}):")
        for warning in result.warnings:
            print(f"  • {warning}")

    if result.info:
        print(f"\nℹ️  INFO ({len(result.info)}):")
        for info in result.info:
            print(f"  • {info}")

    print("\n" + "="*60)
    if result.is_valid:
        print("STATUS: ✅ VALID")
    else:
        print("STATUS: ❌ INVALID")
    print("="*60 + "\n")


def main():
    if len(sys.argv) != 2:
        print("Usage: python validate_changelog.py CHANGELOG.md")
        print("\nValidates a CHANGELOG.md file against Keep a Changelog 1.1.0 spec")
        sys.exit(1)

    filepath = Path(sys.argv[1])

    result = parse_and_validate(filepath)
    print_results(result)

    sys.exit(0 if result.is_valid else 1)


if __name__ == "__main__":
    main()
