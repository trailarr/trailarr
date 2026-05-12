#!/usr/bin/env bash
set -euo pipefail

# Helper script used by the /commit assistant workflow.
# Usage:
#   echo "Commit message" | .cursor/commands/commit
#   or
#   .cursor/commands/commit "Commit message"

if [ "$#" -ge 1 ]; then
  MSG="$*"
elif [ -t 0 ]; then
  echo "Usage: $(basename "$0") \"commit message\" or pipe a message to stdin" >&2
  exit 2
else
  MSG="$(cat -)"
fi

# Basic safety check for obvious secrets or env files
if git status --porcelain | rg --hidden --fixed-strings --line-number --quiet ".env" >/dev/null 2>&1; then
  echo "Warning: .env appears in git status. Ensure secrets are not being committed." >&2
fi

git add -A
git commit -m "$MSG"
echo "Committed: $(git rev-parse --short HEAD)"
git status --porcelain --branch

