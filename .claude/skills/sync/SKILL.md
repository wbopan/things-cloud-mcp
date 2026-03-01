---
name: sync
description: Sync landing page UI and CLAUDE.md with the current code. Use after making changes to tool definitions, handlers, or architecture in main.go.
allowed-tools: Read, Grep, Glob, Edit, Bash(/usr/local/go/bin/go build *)
---

# Sync UI & Docs with Code

Ensure `landing.go` (web UI) and `CLAUDE.md` (project instructions) stay in sync with `main.go` (source of truth).

## Process

### Step 1: Extract current tool definitions from main.go

Read all `mcp.NewTool(...)` blocks in `main.go`. For each tool, extract:
- Tool name
- Description
- Parameters (name, type, required, description, enum values)
- Behavioral annotations (readOnly, destructive, etc.)

Count the total number of tools.

### Step 2: Audit landing.go

Read the tool documentation sections in `landing.go`. Compare against main.go and check for:

- **Missing tools**: tools in main.go not documented in landing.go
- **Removed tools**: tools in landing.go that no longer exist in main.go
- **Parameter mismatches**: parameters added, removed, renamed, or changed type (e.g., bool → enum)
- **Description drift**: tool descriptions that don't match
- **Stale counts**: the "N Tools" badges in hero section and category headers

Fix all discrepancies found. Match the existing HTML structure and CSS classes in landing.go.

### Step 3: Audit CLAUDE.md

Read `CLAUDE.md` and compare against main.go:

- **Architecture section**: verify file descriptions, key types, status/schedule values are correct
- **Coding Patterns section**: verify handler patterns, wire format notes, helper function references still match the code
- **Tool count references**: any mention of number of tools

Fix discrepancies. Keep CLAUDE.md concise — don't add verbose explanations.

### Step 4: Build verification

Run: `/usr/local/go/bin/go build ./...`

If it fails, fix the issue.

### Step 5: Report

Summarize what was changed:
- List each file modified
- List each discrepancy found and fixed
- If nothing was out of sync, say so

Do NOT commit. Leave that to the user.
