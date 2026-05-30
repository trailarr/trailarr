---
name: checks
description: Orchestrator command to run repository checks (formatters, linters, dependencies, secret-scan, large-files, eslint-provision). Accepts an optional check name to run a single check.
argument-hint: checks
user-invocable: true
---

Purpose

- Provide a single entrypoint skill `checks` that invokes the per-check skills located in the `checks/` folder.

Behavior

- When invoked, `checks` will:
  1. By default run checks across the entire repository codebase (all tracked source files). This ensures CI-like, comprehensive scans are the default behavior.
  2. Accept optional arguments to tune invocation:
     - `check`: name of a single sub-skill to run (examples: `formatters`, `linters`, `dependencies`, `secret-scan`, `large-files`, `eslint-provision`).
     - `scope`: when set to `changed` (for example when `/commit` orchestrator invokes `checks`), the skill will compute the repository `changed_set` (Added/Modified and untracked files) and run sub-skills only against those files.
  3. If `check=all` or no `check` argument is provided, sequentially invoke the following skills (using the selected `scope`):
     - `formatters`
     - `linters`
     - `dependencies`
     - `secret-scan`
     - `large-files`
     - `eslint-provision`
  3. Collect exit codes and logs from each sub-skill and return a consolidated result. When a single check is requested, only that check's exit code/logs are returned.

Exit codes and behavior

- `0` — all checks passed.
- `2` — linters reported non-fixable errors (pause and require confirmation).
- `3` — secret scanner found probable secrets (pause and require confirmation).
- `4` — large files detected (pause and require confirmation).
- other non-zero — execution error; return logs and abort.

Notes

- `checks` is intended as a convenience to call the individual check skills in order; repositories may still provide a single wrapper script if they prefer.
- Skill names for subskills are expected to match their folder names under `.github/skills/checks/`.

Commit and push policy

- The `checks` skill MUST NOT create commits or push changes to the repository under any circumstance. It may run fix commands in a working tree when explicitly requested (for example when `check=dependencies --fix` is provided), but it must leave any resulting file modifications uncommitted.
- When fixes would modify files (lockfiles, manifests, source files), the skill MUST produce a clear diff, list of changed files, and exact remediation commands. The skill MUST then pause and request explicit user approval before creating any commit or performing a push. In CI/non-interactive mode the skill may optionally return the planned changes and exit with a non-zero status unless an explicit `--yes`/`--ci-approve` flag was provided by the caller.
