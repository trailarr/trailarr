#!/usr/bin/env bash
set -euo pipefail

# run_checks.sh
# Runs formatters, auto-fix linters and a secret scan on the deterministic "changed_set" (Added/Modified + untracked).
# Designed as a concrete helper the assistant can invoke; configurable via environment variables when the project prefers different tools.

# Configuration (override with env vars):
: ${PRETTIER_CMD:="npx prettier --write"}
: ${PRETTIER_FLAGS:=""}
: ${ESLINT_CMD:="npx eslint --fix --ext .js,.jsx,.ts,.tsx"}
: ${ESLINT_FLAGS:=""}
: ${ESLINT_CHECK_CMD:="npx eslint --ext .js,.jsx,.ts,.tsx"}
: ${ESLINT_CHECK_FLAGS:=""}
: ${PY_BLACK_CMD:="python -m black"}
: ${BLACK_FLAGS:=""}
: ${PY_ISORT_CMD:="python -m isort"}
: ${ISORT_FLAGS:=""}
: ${PY_FLAKE_CMD:="python -m flake8"}
: ${FLAKE_FLAGS:=""}
: ${GRADLEW_CMD:="./gradlew --no-daemon"}
: ${GRADLE_FLAGS:=""}
: ${SECRET_SCANNER_CMD:="gitleaks detect --stdin"}

# By default enable verbose flags for formatters/linters so outputs are shown to user
: ${VERBOSE:=1}
if [ "$VERBOSE" -ne 0 ]; then
  PRETTIER_FLAGS=""
  ESLINT_FLAGS="--debug"
  ESLINT_CHECK_FLAGS="--debug"
  BLACK_FLAGS="-v"
  ISORT_FLAGS="-v"
  FLAKE_FLAGS="-v"
  GRADLE_FLAGS="--info"
fi

# Helper: run a command but don't exit script on failure (useful for formatters)
run_nonfatal() {
  echo "[run_checks] + $*"
  # evaluate and preserve stdout/stderr so user sees verbose output
  if ! eval "$*"; then
    echo "[run_checks] Command failed (non-fatal): $*"
  fi
}

echo "[run_checks] Computing changed_set (Added/Modified + untracked)"
changed=$(git diff --name-only --diff-filter=AM HEAD || true)
untracked=$(git ls-files --others --exclude-standard || true)

# combine and deduplicate
changed_set=$(printf "%s\n%s" "$changed" "$untracked" | awk 'NF' | sort -u)
if [ -z "$(echo "$changed_set" | tr -d '\n')" ]; then
  echo "[run_checks] No added/modified or untracked files to process. Exiting success."
  exit 0
fi

echo "[run_checks] Files to process:"
echo "$changed_set"

# helpers to filter by extension
prettier_files=$(echo "$changed_set" | grep -E '\.(js|jsx|ts|tsx|json|css|scss|html)$' || true)
eslint_files=$(echo "$changed_set" | grep -E '\.(js|jsx|ts|tsx)$' || true)
java_files=$(echo "$changed_set" | grep -E '\.(java)$' || true)
py_files=$(echo "$changed_set" | grep -E '\.(py)$' || true)
doc_files=$(echo "$changed_set" | grep -E '\.(md|rst)$' || true)

echo "[run_checks-debug] prettier_files=<<EOF\n$prettier_files\nEOF"
echo "[run_checks-debug] eslint_files=<<EOF\n$eslint_files\nEOF"
echo "[run_checks-debug] java_files=<<EOF\n$java_files\nEOF"
echo "[run_checks-debug] py_files=<<EOF\n$py_files\nEOF"
echo "[run_checks-debug] doc_files=<<EOF\n$doc_files\nEOF"

# ESLint config detection/provisioning is handled by the commit skill, not this script.
# This script only runs concrete checks against changed files.

# Helper: find the nearest directory containing package.json for a given file path.
# Returns the directory (relative to repo root) or empty string if none found.
find_pkg_root() {
  local file="$1"
  local dir
  dir=$(dirname "$file")
  while [ "$dir" != "." ] && [ "$dir" != "/" ] && [ "$dir" != "" ]; do
    if [ -f "$dir/package.json" ]; then
      echo "$dir"
      return
    fi
    dir=$(dirname "$dir")
  done
  # fallback: repo root
  if [ -f "package.json" ]; then
    echo "."
  else
    echo ""
  fi
}

# Helper: run ESLint (with optional --fix) from the nearest package.json directory for each file group.
run_eslint_per_pkg() {
  local fix_flag="${1:-}"   # "--fix" or ""
  local extra_flags="${2:-}"
  local files="${3:-}"
  local failed=0

  # Collect unique package roots
  local pkg_roots
  pkg_roots=$(while IFS= read -r f; do
    r=$(find_pkg_root "$f")
    [ -n "$r" ] && echo "$r"
  done <<< "$files" | sort -u)

  if [ -z "$pkg_roots" ]; then
    echo "[run_checks] No package.json root found for JS/TS files; skipping ESLint"
    return 0
  fi

  while IFS= read -r pkg_root; do
    # Collect files under this package root
    local root_files
    root_files=$(while IFS= read -r f; do
      r=$(find_pkg_root "$f")
      [ "$r" = "$pkg_root" ] && echo "$f"
    done <<< "$files")
    [ -z "$root_files" ] && continue

    # Convert absolute repo-relative paths to paths relative to pkg_root
    local rel_files
    rel_files=$(while IFS= read -r f; do
      if [ "$pkg_root" = "." ]; then
        echo "$f"
      else
        # strip the pkg_root/ prefix
        echo "${f#${pkg_root}/}"
      fi
    done <<< "$root_files")

    echo "[run_checks] Running ESLint $fix_flag from '$pkg_root' on:"
    echo "$rel_files"

    if ! (cd "$pkg_root" && npx eslint $fix_flag $extra_flags $(echo "$rel_files" | tr '\n' ' ')); then
      failed=1
    fi
  done <<< "$pkg_roots"

  return $failed
}

# Run JS/TS formatters & auto-fixers
if [ -n "$prettier_files" ]; then
  echo "[run_checks] Running Prettier on JS/TS files"
  if command -v npx >/dev/null 2>&1; then
    # Run Prettier per package root so it picks up the nearest config
    while IFS= read -r f; do
      pkg_root=$(find_pkg_root "$f")
      if [ -n "$pkg_root" ]; then
        rel_f=$([ "$pkg_root" = "." ] && echo "$f" || echo "${f#${pkg_root}/}")
        run_nonfatal "(cd '$pkg_root' && $PRETTIER_CMD $PRETTIER_FLAGS '$rel_f')"
      else
        run_nonfatal $PRETTIER_CMD $PRETTIER_FLAGS "$f"
      fi
    done <<< "$prettier_files"
  else
    echo "[run_checks] Prettier not available; skipping"
  fi

fi

if [ -n "$eslint_files" ]; then
  echo "[run_checks] Running ESLint --fix on JS/TS files"
  if command -v npx >/dev/null 2>&1; then
    run_eslint_per_pkg "--fix" "" "$eslint_files" || true
  else
    echo "[run_checks] ESLint not available; skipping --fix step"
  fi

  # Do not auto-stage formatted JS/TS files; leave staging to the user
fi

# Run Java formatter if Java files present and gradlew exists
if [ -n "$java_files" ]; then
  if [ -f "gradlew" ] || command -v gradle >/dev/null 2>&1; then
    echo "[run_checks] Running Gradle formatting/checks for Java files"
    run_nonfatal $GRADLEW_CMD spotlessApply $GRADLE_FLAGS || true
    # Do not auto-stage formatted Java files; leave staging to the user
  else
    echo "[run_checks] Gradle wrapper not found; skipping Java format step"
  fi
fi

# Python formatters
if [ -n "$py_files" ]; then
  echo "[run_checks] Running Black on Python files"
  if command -v python >/dev/null 2>&1; then
    run_nonfatal $PY_BLACK_CMD $BLACK_FLAGS $(echo "$py_files" | tr '\n' ' ')
    run_nonfatal $PY_ISORT_CMD $ISORT_FLAGS $(echo "$py_files" | tr '\n' ' ')
    # Do not auto-stage formatted Python files; leave staging to the user
  else
    echo "[run_checks] Python not available; skipping Python formatting"
  fi
fi

# Docs formatting
if [ -n "$doc_files" ]; then
  if command -v ${PRETTIER_CMD%% *} >/dev/null 2>&1 || command -v npx >/dev/null 2>&1; then
    echo "[run_checks] Running Prettier on docs"
    run_nonfatal $PRETTIER_CMD $PRETTIER_FLAGS $(echo "$doc_files" | tr '\n' ' ')
    # Do not auto-stage formatted docs; leave staging to the user
  else
    echo "[run_checks] Prettier not available; skipping docs formatting"
  fi
fi

# Run linters (non-fix mode) on changed_set. Use project's canonical tools when configured.
lint_failed=0

if [ -n "$eslint_files" ]; then
  if command -v npx >/dev/null 2>&1; then
    echo "[run_checks] Running ESLint (check-only) on JS/TS files"
    if ! run_eslint_per_pkg "" "" "$eslint_files" 2>&1; then
      lint_failed=1
    fi
  else
    echo "[run_checks] ESLint not available; skipping eslint check"
  fi
fi

if [ -n "$java_files" ]; then
  if [ -f "gradlew" ] || command -v gradle >/dev/null 2>&1; then
    echo "[run_checks] Running Gradle check (Java)"
    if ! $GRADLEW_CMD check $GRADLE_FLAGS; then
      lint_failed=1
    fi
  else
    echo "[run_checks] Gradle wrapper not found; skipping Java check"
  fi
fi

if [ -n "$py_files" ]; then
  if command -v python >/dev/null 2>&1; then
    echo "[run_checks] Running flake8 on Python files"
    if ! $PY_FLAKE_CMD $FLAKE_FLAGS $(echo "$py_files" | tr '\n' ' '); then
      lint_failed=1
    fi
  else
    echo "[run_checks] Python not available; skipping flake8"
  fi
fi

if [ "$lint_failed" -ne 0 ]; then
  echo "{\"status\":\"lint_failed\",\"message\":\"Linters reported non-fixable errors.\"}"
  exit 2
fi

# Secret scan (attempt configured scanner; fail-fast on findings)
if command -v gitleaks >/dev/null 2>&1; then
  echo "[run_checks] Running secret scanner (gitleaks) on diff vs HEAD"
  # Detect supported stdin/pipe option for the installed gitleaks
  if gitleaks detect --help 2>&1 | grep -q -- '--pipe'; then
    echo "[run_checks] Using 'gitleaks detect --pipe' to scan diff"
    if ! git diff HEAD | gitleaks detect --pipe; then
      echo "{\"status\":\"secrets_found\",\"message\":\"Secret scanner detected probable secrets.\"}"
      exit 3
    fi
  elif gitleaks detect --help 2>&1 | grep -q -- '--stdin'; then
    echo "[run_checks] Using 'gitleaks detect --stdin' to scan diff"
    if ! git diff HEAD | gitleaks detect --stdin; then
      echo "{\"status\":\"secrets_found\",\"message\":\"Secret scanner detected probable secrets.\"}"
      exit 3
    fi
  else
    echo "[run_checks] gitleaks installed but no stdin/pipe option detected; running repo scan as fallback"
    if ! gitleaks detect --source .; then
      echo "{\"status\":\"secrets_found\",\"message\":\"Secret scanner detected probable secrets on repo scan.\"}"
      exit 3
    fi
  fi
else
  echo "[run_checks] gitleaks not found; skipping secret scan. To require a scan, set SECRET_SCANNER_CMD or install a scanner."
fi

# Large file check (>5MB)
# Large file check (>5MB) only within changed_set
large_files_list=""
while IFS= read -r f; do
  if [ -f "$f" ]; then
    # use portable stat: macOS uses -f%z, linux uses -c%s
    if stat -f%z "$f" >/dev/null 2>&1; then
      size=$(stat -f%z "$f")
    else
      size=$(stat -c%s "$f" 2>/dev/null || echo 0)
    fi
    if [ "$size" -gt $((5*1024*1024)) ]; then
      large_files_list="$large_files_list\n$f"
    fi
  fi
done <<<"$changed_set"

if [ -n "$(echo "$large_files_list" | tr -d '\n')" ]; then
  echo "{\"status\":\"large_files\",\"files\":[$(echo "$large_files_list" | awk 'NF{printf "\"%s\",", $0}' | sed 's/,$//')] }"
  exit 4
fi

echo "[run_checks] All checks passed. Ready to commit."
exit 0
