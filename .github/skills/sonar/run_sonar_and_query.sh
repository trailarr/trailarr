#!/usr/bin/env bash
set -euo pipefail

# Simple helper to run sonar-scanner then query SonarQube API for a given rule or issue key.
# Usage: ./run_sonar_and_query.sh --project <projectKey> [--rule <ruleId>] [--issue <issueKey>]

PROJECT=""
RULE=""
ISSUE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --project) PROJECT="$2"; shift 2;;
    --rule) RULE="$2"; shift 2;;
    --issue) ISSUE="$2"; shift 2;;
    --help) echo "Usage: $0 --project <projectKey> [--rule <ruleId>] [--issue <issueKey>]"; exit 0;;
    *) echo "Unknown arg $1"; exit 1;;
  esac
done

if [[ -z "$PROJECT" ]]; then
  echo "--project is required"
  exit 2
fi

if [[ -z "${SONAR_HOST_URL:-}" || -z "${SONAR_TOKEN:-}" ]]; then
  echo "Environment must set SONAR_HOST_URL and SONAR_TOKEN" >&2
  exit 3
fi

echo "Running sonar-scanner..."
sonar-scanner

API_BASE="$SONAR_HOST_URL/api"

if [[ -n "$RULE" ]]; then
  echo "Querying issues for rule $RULE in project $PROJECT"
  curl -s -u "$SONAR_TOKEN:" "$API_BASE/issues/search?projectKeys=$PROJECT&rules=$RULE" | jq '.'
  exit 0
fi

if [[ -n "$ISSUE" ]]; then
  echo "Querying specific issue $ISSUE"
  curl -s -u "$SONAR_TOKEN:" "$API_BASE/issues/search?issue=$ISSUE" | jq '.'
  exit 0
fi

# Default: list all open issues for the project
curl -s -u "$SONAR_TOKEN:" "$API_BASE/issues/search?projectKeys=$PROJECT&statuses=OPEN" | jq '.'
