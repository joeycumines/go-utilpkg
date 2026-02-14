#!/usr/bin/env python3
"""
Add entries to a Keep a Changelog formatted CHANGELOG.md file

This script can:
- Add entries to the "Unreleased" section
- Add entries to a specific version
- Create a new version section if it doesn't exist
- Automatically update link references

Usage:
    python add_entry.py CHANGELOG.md Added "New feature description"
    python add_entry.py CHANGELOG.md Fixed "Fixed bug in authentication" --version 1.2.0
    python add_entry.py CHANGELOG.md Deprecated "Old API will be removed" --version 1.3.0 --date 2025-03-15
"""

import sys
import argparse
from datetime import datetime
from pathlib import Path
from typing import List, Tuple, Optional
import re


# Valid section types
VALID_SECTIONS = ['Added', 'Changed', 'Deprecated', 'Removed', 'Fixed', 'Security']

# Regex patterns
VERSION_PATTERN = r'^##\s+\[([^\]]+)\]'
LINK_REF_PATTERN = r'^\[([^\]]+)\]:\s+'


class ChangelogManager:
    """Manage changelog file operations"""

    def __init__(self, filepath: Path):
        self.filepath = filepath
        self.content: str = ""
        self.lines: List[str] = []

    def read(self) -> bool:
        """Read the changelog file"""
        try:
            self.content = self.filepath.read_text(encoding='utf-8')
            self.lines = self.content.splitlines()
            return True
        except FileNotFoundError:
            print(f"Error: File not found: {self.filepath}")
            return False
        except Exception as e:
            print(f"Error reading file: {e}")
            return False

    def write(self) -> bool:
        """Write the updated content back to the file"""
        try:
            self.content = '\n'.join(self.lines) + '\n'
            self.filepath.write_text(self.content, encoding='utf-8')
            return True
        except Exception as e:
            print(f"Error writing file: {e}")
            return False

    def find_unreleased_section(self) -> Optional[int]:
        """Find the line number of the Unreleased section header"""
        for i, line in enumerate(self.lines):
            if line.strip() == '## [Unreleased]':
                return i
        return None

    def find_version_section(self, version: str) -> Optional[int]:
        """Find the line number of a specific version section"""
        for i, line in enumerate(self.lines):
            if line.strip() == f'## [{version}]':
                return i
        return None

    def find_section_in_version(
        self,
        version_line: int,
        section: str
    ) -> Optional[int]:
        """Find or create a section header within a version"""
        # Search for existing section within this version
        for i in range(version_line + 1, len(self.lines)):
            line = self.lines[i].strip()
            # Stop at next version
            if line.startswith('## [') and line != '## [Unreleased]':
                break
            # Found the section
            if line == f'### {section}':
                return i

        # Section doesn't exist, we'll need to insert it
        return None

    def find_last_item_in_section(self, section_line: int) -> int:
        """Find the line number of the last item in a section"""
        i = section_line + 1
        while i < len(self.lines):
            line = self.lines[i].strip()
            # Stop at next section or version
            if line.startswith('### ') or line.startswith('## ['):
                return i - 1
            # Found an item
            if line.startswith('-'):
                i += 1
            else:
                # Empty line or non-item content
                return i - 1 if i > section_line + 1 else section_line
        return len(self.lines) - 1

    def find_link_ref_section(self) -> Optional[int]:
        """Find line number where link references start (or should start)"""
        # Link references start at the first line matching [X]: URL pattern
        for i, line in enumerate(self.lines):
            if re.match(LINK_REF_PATTERN, line.strip()):
                return i
        return None

    def extract_link_refs(self) -> dict:
        """Extract existing link references as a dictionary"""
        link_refs = {}
        link_start = self.find_link_ref_section()

        if link_start is not None:
            for i in range(link_start, len(self.lines)):
                line = self.lines[i].strip()
                match = re.match(LINK_REF_PATTERN, line)
                if match:
                    version = match.group(1)
                    url = line.split(']:', 1)[1].strip()
                    link_refs[version] = url
                elif line and not re.match(LINK_REF_PATTERN, line):
                    # Stop at non-link-ref line
                    break

        return link_refs

    def update_link_references(self, owner_repo: Optional[str] = None) -> bool:
        """
        Update link references at the bottom of the file.
        Detects owner/repo from existing links or requires it as parameter.
        Returns True if updated, False if skipped.
        """
        # Find all released versions in the file (in reverse chronological order)
        versions = []
        for line in self.lines:
            match = re.match(VERSION_PATTERN, line.strip())
            if match:
                version = match.group(1)
                if version.lower() != 'unreleased':
                    versions.append(version)

        if not versions:
            # No versions to create links for
            return False

        # Extract existing link references
        existing_refs = self.extract_link_refs()

        # Determine owner/repo from existing refs or require it as parameter
        owner_repo_pattern = None
        for version, url in existing_refs.items():
            if 'github.com/' in url:
                # Extract owner/repo from URL
                match = re.search(r'github\.com/([^/]+/[^/]+)/', url)
                if match:
                    owner_repo_pattern = match.group(1)
                    break

        if owner_repo_pattern is None:
            if owner_repo:
                owner_repo_pattern = owner_repo
            else:
                # Cannot determine repository, skip update
                return False

        # Generate updated link references
        # Versions are in reverse chronological order (newest first)
        # The first version (oldest) uses releases/tag/, others use compare/

        new_links = []
        for i, version in enumerate(versions):
            # Ensure version has 'v' prefix for URLs
            v_ref = version if version.startswith('v') else f'v{version}'

            if i == len(versions) - 1:
                # Last/oldest version uses releases/tag/
                url = f"https://github.com/{owner_repo_pattern}/releases/tag/{v_ref}"
                new_links.append(f"[{version}]: {url}")
            else:
                # Middle versions use compare/ URL to the next (older) version
                next_version = versions[i + 1]
                v_next = next_version if next_version.startswith('v') else f'v{next_version}'
                url = f"https://github.com/{owner_repo_pattern}/compare/{v_next}...{v_ref}"
                new_links.append(f"[{version}]: {url}")

        # Add Unreleased link reference if there's an Unreleased section
        if self.find_unreleased_section() is not None:
            latest_version = versions[0] if versions else 'v0.0.0'
            v_latest = latest_version if latest_version.startswith('v') else f'v{latest_version}'
            unreleased_url = f"https://github.com/{owner_repo_pattern}/compare/{v_latest}...HEAD"
            new_links.insert(0, f"[Unreleased]: {unreleased_url}")

        # Find or create link reference section
        link_start = self.find_link_ref_section()

        if link_start is None:
            # No link refs exist, add them at the end
            link_start = len(self.lines)
            # Ensure we have a blank line before link refs
            if link_start > 0 and self.lines[link_start - 1].strip():
                self.lines.append('')
                link_start = len(self.lines)
        else:
            # Remove old link refs
            # Find where they end
            link_end = link_start
            while link_end < len(self.lines) and self.lines[link_end].strip():
                link_end += 1

            # Remove the old link refs block
            del self.lines[link_start:link_end]

        # Insert new link refs at the appropriate position
        insert_pos = link_start
        for link in new_links:
            self.lines.insert(insert_pos, link)
            insert_pos += 1

        return True

    def add_unreleased_section_at_top(self) -> int:
        """Create an Unreleased section at the top of the file"""
        # Find the first existing version or end of file
        insert_pos = 0
        in_leading_content = True

        for i, line in enumerate(self.lines):
            stripped = line.strip()
            if stripped.startswith('## ['):
                insert_pos = i
                in_leading_content = False
                break
            # If there's non-heading content before versions
            if stripped and not stripped.startswith('#'):
                in_leading_content = False

        # Insert the Unreleased section
        unreleased_header = '## [Unreleased]'
        if insert_pos == 0:
            # Empty file or no content
            self.lines.insert(insert_pos, '')
            self.lines.insert(insert_pos + 1, unreleased_header)
            return 1
        else:
            self.lines.insert(insert_pos, '')
            self.lines.insert(insert_pos + 1, unreleased_header)
            if insert_pos > 0 and self.lines[insert_pos - 1].strip():
                self.lines.insert(insert_pos, '')
                return insert_pos + 1
            return insert_pos + 1

    def create_version_section(
        self,
        version: str,
        date: Optional[str] = None
    ) -> int:
        """Create a new version section"""
        if date is None:
            date = datetime.now().strftime('%Y-%m-%d')

        version_header = f'## [{version}] - {date}'

        # Find the best place to insert (after Unreleased, before other versions)
        insert_pos = 0
        unreleased_line = self.find_unreleased_section()

        if unreleased_line is not None:
            # Insert after Unreleased section
            insert_pos = unreleased_line + 1
            # Find the end of Unreleased content
            while insert_pos < len(self.lines) and not self.lines[insert_pos].strip().startswith('## ['):
                insert_pos += 1
        else:
            # No Unreleased section, insert before first version
            for i, line in enumerate(self.lines):
                if line.strip().startswith('## [') and line.strip() != '## [Unreleased]':
                    insert_pos = i
                    break
            else:
                # No versions found, append at end
                insert_pos = len(self.lines)

        # Insert the version header
        self.lines.insert(insert_pos, '')
        self.lines.insert(insert_pos + 1, version_header)

        return insert_pos + 1

    def add_entry(
        self,
        section: str,
        message: str,
        version: Optional[str] = None,
        date: Optional[str] = None
    ) -> bool:
        """Add a changelog entry"""

        # Validate section
        if section not in VALID_SECTIONS:
            print(f"Error: Invalid section '{section}'")
            print(f"Valid sections: {', '.join(VALID_SECTIONS)}")
            return False

        # Find or create version section
        if version is None:
            # Use Unreleased
            version_line = self.find_unreleased_section()
            if version_line is None:
                version_line = self.add_unreleased_section_at_top()
                print(f"Created [Unreleased] section")
        else:
            # Use specific version
            version_line = self.find_version_section(version)
            if version_line is None:
                version_line = self.create_version_section(version, date)
                print(f"Created version [{version}]")

        # Find or create section within version
        section_line = self.find_section_in_version(version_line, section)
        if section_line is None:
            # Create the section header
            # Find the right place to insert it
            insert_pos = version_line + 1

            # Skip past any existing sections (insert at correct position)
            current_sections = []
            priority_order = ['Added', 'Changed', 'Deprecated', 'Removed', 'Fixed', 'Security']

            temp_pos = version_line + 1
            while temp_pos < len(self.lines) and not self.lines[temp_pos].strip().startswith('## ['):
                stripped = self.lines[temp_pos].strip()
                if stripped.startswith('### '):
                    section_name = stripped[4:].strip()
                    if section_name in priority_order:
                        current_sections.append((temp_pos, section_name))
                temp_pos += 1

            # Insert in proper order
            section_priority = priority_order.index(section)
            insert_pos = version_line + 1

            for pos, existing_section in sorted(current_sections, key=lambda x: priority_order.index(x[1])):
                if priority_order.index(existing_section) > section_priority:
                    insert_pos = pos
                    break
                else:
                    insert_pos = pos + 1

            # Insert the section with a blank line before it
            if insert_pos > version_line + 1 and self.lines[insert_pos - 1].strip():
                self.lines.insert(insert_pos, '')
                insert_pos += 1

            self.lines.insert(insert_pos, f'### {section}')
            if insert_pos + 1 < len(self.lines) and not self.lines[insert_pos + 1].strip():
                insert_pos += 1
            else:
                self.lines.insert(insert_pos + 1, '')

            section_line = insert_pos

        # Add the entry item
        # Find the last item in this section
        last_item_line = self.find_last_item_in_section(section_line)

        # Insert after the last item
        insert_pos = last_item_line + 1

        # Check if we need to add a blank line
        if last_item_line < section_line:
            # Section had no items, insert right after header
            insert_pos = section_line + 1
        elif not self.lines[last_item_line].strip().startswith('-'):
            # No items yet after header (just blank lines)
            insert_pos = section_line + 1
            while insert_pos < len(self.lines) and not self.lines[insert_pos].strip():
                insert_pos += 1

        # Add the item
        entry = f"- {message}"
        if insert_pos >= len(self.lines):
            self.lines.append(entry)
        else:
            self.lines.insert(insert_pos, entry)

        return True


def main():
    parser = argparse.ArgumentParser(
        description='Add entries to a Keep a Changelog formatted CHANGELOG.md',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  Add to Unreleased section:
    %(prog)s CHANGELOG.md Added "New user authentication feature"
    %(prog)s CHANGELOG.md Fixed "Fixed bug in password reset"

  Add to specific version:
    %(prog)s CHANGELOG.md Changed "Updated API documentation" --version 1.2.0
    %(prog)s CHANGELOG.md Deprecated "Old auth endpoint" --version 1.3.0

  Add to new version with specific date:
    %(prog)s CHANGELOG.md Added "New feature" --version 2.0.0 --date 2025-03-15

Valid sections: Added, Changed, Deprecated, Removed, Fixed, Security
        """
    )

    parser.add_argument(
        'changelog_file',
        type=Path,
        help='Path to CHANGELOG.md file'
    )

    parser.add_argument(
        'section',
        choices=VALID_SECTIONS,
        help='Section type for the entry'
    )

    parser.add_argument(
        'message',
        help='The changelog entry message'
    )

    parser.add_argument(
        '--version',
        help='Version to add entry to (default: Unreleased section)'
    )

    parser.add_argument(
        '--date',
        help='Date for new version (YYYY-MM-DD, default: today)',
        metavar='YYYY-MM-DD'
    )

    parser.add_argument(
        '--owner-repo',
        help='Repository owner/repo for link references (e.g., owner/repo). Detected from existing links if omitted.',
        metavar='owner/repo'
    )

    args = parser.parse_args()

    # Validate date format if provided
    if args.date:
        try:
            datetime.strptime(args.date, '%Y-%m-%d')
        except ValueError:
            print(f"Error: Invalid date format '{args.date}'. Use YYYY-MM-DD")
            sys.exit(1)

    # Create changelog manager
    manager = ChangelogManager(args.changelog_file)

    # Read the file
    if not manager.read():
        sys.exit(1)

    # Add the entry
    target = f"[Unreleased]" if not args.version else f"[{args.version}]"
    print(f"Adding entry to {target} > {args.section}")

    if manager.add_entry(
        section=args.section,
        message=args.message,
        version=args.version,
        date=args.date
    ):
        # Update link references when adding entries to specific versions
        updated_links = False
        if args.version and manager.find_version_section(args.version) is not None:
            updated_links = manager.update_link_references(args.owner_repo)
        elif args.version is None and manager.find_unreleased_section() is None:
            # Created Unreleased section, try to update links
            updated_links = manager.update_link_references(args.owner_repo)

        # Write the file
        if manager.write():
            print(f"✅ Entry added successfully")
            print(f"   {args.section}: {args.message}")
            if args.version:
                print(f"   Version: {args.version}")
            else:
                print(f"   Version: [Unreleased]")
            sys.exit(0)
        else:
            print("❌ Failed to write changes")
            sys.exit(1)
    else:
        print("❌ Failed to add entry")
        sys.exit(1)


if __name__ == "__main__":
    main()
