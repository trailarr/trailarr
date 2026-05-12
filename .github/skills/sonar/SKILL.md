---
name: sonar
description: Interactive `/sonar` assistant command: run SonarQube analysis, query issues, and optionally prepare suggested fixes for specific Sonar issues.
argument-hint: sonar
user-invocable: true
---

This skill automates running `sonar-scanner`, querying SonarQube's API, and preparing repair suggestions for specific issues. It does not apply fixes automatically — the assistant will always ask for explicit confirmation before editing repository files.

Behavior:

1. Preconditions (required):
   - The environment must set `SONAR_TOKEN` (user token) and `SONAR_HOST_URL` (e.g. https://sonarqube.example.com).
   - `sonar-scanner` must be installed and available on PATH, or the repository provides a wrapper.

2. Primary actions:
   - Run `sonar-scanner` from the repository root.
   - Query SonarQube API for issues using `projectKeys`, `rules`, or `issues` query parameters. By default the skill will include `resolved=false` when querying so it returns only unfixed/open issues; a flag can be provided to include resolved issues when explicitly desired.
   - When no `--rule` is specified the skill will query the project for unresolved issues (equivalent to `resolved=false`) so maintainers can run a broad scan without specifying rules. Use `--include-resolved` to include resolved/closed issues in the results.
   - For each returned issue, gather `component`, `textRange`, `message`, and `quickFixAvailable` fields.

3. Repair assistance (interactive):
   - For issues where the skill knows a deterministic, safe textual replacement pattern (for example: replacing `parts[parts.length - 1]` with `parts.at(-1)`, or removing useless React fragment wrappers), the skill will prepare a patch and present it to the user for review.
   - The assistant will NOT apply patches automatically. It will prompt the user to approve each suggested patch before calling the repository edit flow.

4. Limitations and safety:
   - The skill does not attempt semantic refactors (complex AST transforms) — only small, pattern-based, reversible changes are suggested.
   - The skill will not run `git push` or modify remote branches.
   - The skill requires explicit confirmation for any file edits.

Usage examples:

- `/sonar scan --rule javascript:S7755` — run `sonar-scanner`, query for S7755 in the project (unresolved only by default), and prepare suggestions.
- `/sonar scan --rule javascript:S7755 --include-resolved` — query S7755 including resolved issues.
- `/sonar scan` — run `sonar-scanner`, then query the project for all unresolved issues (no `--rule` required).
- `/sonar repair --issue 6ec3b9f6-cf00-41c7-a021-4ac273de9e81` — query the specific issue by key and prepare a suggested fix if available.

Helper scripts shipped in this skill are templates to be executed locally by a maintainer or CI runner. They must be reviewed before running.
