---
name: bump-version
description: Bump the server version, commit, push, tag, and create a GitHub release. Use when ready to cut a new release.
argument-hint: "[major|minor|patch]"
allowed-tools: Read, Grep, Edit, Bash(git *), Bash(gh *), Bash(/usr/local/go/bin/go build *)
---

# Bump Version & Release

Cut a new release: bump version, commit, push, tag, and create a GitHub release.

## Arguments

`$ARGUMENTS` is the bump type: `major`, `minor`, or `patch` (default: `patch`).

## Process

### Step 1: Pre-flight checks

1. Ensure working directory is clean — no uncommitted changes. If there are uncommitted changes, **stop and ask the user** whether to commit them first or abort.
2. Ensure current branch is `main`.
3. Ensure local main is up to date with remote (`git fetch origin && git rev-list HEAD..origin/main --count` should be 0).

### Step 2: Determine new version

1. Find the current version in `main.go`:
   ```
   grep -n 'NewMCPServer' main.go
   ```
   The version string is the second argument to `server.NewMCPServer(...)`, e.g. `"1.1.0"`.

2. Parse as `MAJOR.MINOR.PATCH` and bump according to `$ARGUMENTS`:
   - `major` → increment MAJOR, reset MINOR and PATCH to 0
   - `minor` → increment MINOR, reset PATCH to 0
   - `patch` (default) → increment PATCH

3. Show the user: `Bumping v{old} → v{new}. Proceed?` Wait for confirmation.

### Step 3: Apply version bump

1. Edit the version string in `main.go`.
2. Run `/usr/local/go/bin/go build ./...` to verify.

### Step 4: Generate release notes

1. Find the previous version tag:
   ```
   git describe --tags --abbrev=0
   ```
2. Collect commits since that tag:
   ```
   git log {previous_tag}..HEAD --oneline
   ```
3. Group changes into categories by reading commit messages and the actual diffs:
   - **New features** — new tools, new parameters, new capabilities
   - **Bug fixes** — corrections to existing behavior
   - **Improvements** — refactors, performance, DX improvements
   - **Other** — docs, CI, chores

   Write concise, user-facing descriptions (not raw commit messages). Skip internal commits (plan docs, design docs, worktree cleanup).

4. Show the draft release notes to the user for approval.

### Step 5: Commit, tag, push, release

After user approves the release notes:

```bash
git add main.go
git commit -m "Bump version to v{new}"
git push origin main
git tag v{new}
git push origin v{new}
gh release create v{new} --title "v{new}" --notes "{release_notes}"
```

### Step 6: Report

Print the release URL returned by `gh release create`.
