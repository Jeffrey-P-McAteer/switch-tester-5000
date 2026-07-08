#!/usr/bin/env python3
"""
create-release.py — bump patch version, tag, and push to origin.

Usage:
    python create-release.py           # bump patch (0.1.0 → 0.1.1)
    python create-release.py --minor   # bump minor (0.1.0 → 0.2.0)
    python create-release.py --major   # bump major (0.1.0 → 1.0.0)
"""

import argparse
import subprocess
import sys
from pathlib import Path

VERSION_FILE = Path(__file__).parent / "version.txt"


def run(cmd: list[str], check: bool = True) -> subprocess.CompletedProcess:
    print(f"  $ {' '.join(cmd)}")
    result = subprocess.run(cmd, capture_output=True, text=True)
    if check and result.returncode != 0:
        print(f"ERROR: {result.stderr.strip() or result.stdout.strip()}", file=sys.stderr)
        sys.exit(1)
    return result


def read_version() -> tuple[int, int, int]:
    raw = VERSION_FILE.read_text().strip()
    parts = raw.split(".")
    if len(parts) != 3:
        print(f"ERROR: version.txt must contain MAJOR.MINOR.PATCH, got {raw!r}", file=sys.stderr)
        sys.exit(1)
    try:
        return int(parts[0]), int(parts[1]), int(parts[2])
    except ValueError:
        print(f"ERROR: non-integer version component in {raw!r}", file=sys.stderr)
        sys.exit(1)


def write_version(major: int, minor: int, patch: int) -> str:
    v = f"{major}.{minor}.{patch}"
    VERSION_FILE.write_text(v + "\n")
    return v


def check_clean_working_tree():
    result = run(["git", "status", "--porcelain"], check=True)
    dirty = [
        line for line in result.stdout.splitlines()
        if not line.startswith("?? ")  # ignore untracked
    ]
    if dirty:
        print("ERROR: working tree has uncommitted changes:", file=sys.stderr)
        for line in dirty:
            print(f"  {line}", file=sys.stderr)
        print("Commit or stash changes before releasing.", file=sys.stderr)
        sys.exit(1)


def main():
    parser = argparse.ArgumentParser(description="Bump version, tag, and push a release.")
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--major", action="store_true", help="Bump major version")
    group.add_argument("--minor", action="store_true", help="Bump minor version")
    parser.add_argument("--dry-run", action="store_true", help="Show what would happen without doing it")
    args = parser.parse_args()

    check_clean_working_tree()

    major, minor, patch = read_version()
    old_version = f"{major}.{minor}.{patch}"

    if args.major:
        major += 1
        minor = 0
        patch = 0
    elif args.minor:
        minor += 1
        patch = 0
    else:
        patch += 1

    new_version = f"{major}.{minor}.{patch}"
    tag = f"v{new_version}"

    print(f"Releasing: {old_version} → {new_version}  (tag: {tag})")

    if args.dry_run:
        print("Dry run — no changes made.")
        return

    # 1. Write new version
    write_version(major, minor, patch)
    print(f"  Updated version.txt → {new_version}")

    # 2. Commit version bump
    run(["git", "add", "version.txt"])
    run(["git", "commit", "-m", f"chore: release {tag}"])

    # 3. Create annotated tag
    run(["git", "tag", "-a", tag, "-m", f"Release {tag}"])

    # 4. Push commit + tag
    print("Pushing commit and tag to origin…")
    run(["git", "push", "origin", "HEAD"])
    run(["git", "push", "origin", tag])

    print(f"\nDone! Tag {tag} pushed. GitHub Actions will build and publish the release.")


if __name__ == "__main__":
    main()
