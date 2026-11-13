#!/usr/bin/env bash
# The release rule, in one place so the Release workflow and its test run the same code.
#
#   scripts/release-version.sh compute               prints the next version (vX.Y.Z), or "none"
#   scripts/release-version.sh require-changelog vX.Y.Z   fails unless CHANGELOG.md has "## [X.Y.Z]"
#
# The version comes from conventional commits since the last tag: a subject with `!` before the
# colon or a BREAKING CHANGE footer is a breaking change, `feat:` a feature, `fix:` a fix; anything
# else (docs, ci, chore, test, refactor) accumulates into the next real release.
#
#   before 1.0.0:  breaking or feature -> minor (v0.2.1 -> v0.3.0),  fix -> patch (v0.2.0 -> v0.2.1)
#   from 1.0.0 on: breaking -> major,  feature -> minor,  fix -> patch
#
# With no tag yet, the first version is the newest `## [x.y.z]` section of CHANGELOG.md, and
# "none" when there is no section. Run from the repository root.
set -euo pipefail

compute() {
  local last subjects bodies level maj min pat first
  last=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
  if [ -z "$last" ]; then
    first=$(grep -m1 -oE '^## \[[0-9]+\.[0-9]+\.[0-9]+\]' CHANGELOG.md 2>/dev/null | tr -d '#[] ' || true)
    if [ -z "$first" ]; then
      echo none
    else
      echo "v$first"
    fi
    return 0
  fi

  subjects=$(git log --format=%s "$last"..HEAD)
  bodies=$(git log --format=%b "$last"..HEAD)
  level=none
  if echo "$subjects" | grep -qE '^[a-z]+(\(.*\))?!:' || echo "$bodies" | grep -q 'BREAKING CHANGE'; then
    level=breaking
  elif echo "$subjects" | grep -qE '^feat(\(.*\))?:'; then
    level=feature
  elif echo "$subjects" | grep -qE '^fix(\(.*\))?:'; then
    level=fix
  fi
  if [ "$level" = none ]; then
    echo none
    return 0
  fi

  IFS=. read -r maj min pat <<< "${last#v}"
  if [ "$maj" = 0 ]; then
    case "$level" in
      breaking | feature) echo "v0.$((min + 1)).0" ;;
      fix) echo "v0.$min.$((pat + 1))" ;;
    esac
  else
    case "$level" in
      breaking) echo "v$((maj + 1)).0.0" ;;
      feature) echo "v$maj.$((min + 1)).0" ;;
      fix) echo "v$maj.$min.$((pat + 1))" ;;
    esac
  fi
}

require_changelog() {
  local next="${1:?usage: release-version.sh require-changelog vX.Y.Z}"
  local version="${next#v}"
  if ! grep -qE "^## \[${version//./\\.}\]" CHANGELOG.md; then
    echo "::error::The commits call for $next, but CHANGELOG.md has no ## [$version]" \
      "section. Write the entry (that IS the release decision) and push it; this" \
      "workflow then tags and publishes it." >&2
    return 1
  fi
}

case "${1:-}" in
  compute) compute ;;
  require-changelog) shift; require_changelog "$@" ;;
  *) echo "usage: $0 compute | require-changelog vX.Y.Z" >&2; exit 2 ;;
esac
