#!/bin/sh
# Batches the unreleased changelog fragments into a new version and commits
# the result as "Release vX.Y.Z". It never tags or pushes: it prints the
# commands for that instead. Run it through make, which pins Changie.
#
#   make prepare-release                  # version from the fragment kinds
#   make prepare-release VERSION=v0.3.0   # explicit version
set -eu
cd "$(dirname "$0")/.."
: "${CHANGIE:?run this through make prepare-release}"

die() {
  echo "prepare-release: $*" >&2
  exit 1
}

[ "$(git branch --show-current)" = main ] || die "releases are prepared on main"
[ -z "$(git status --porcelain)" ] || die "the working tree must be clean"
git fetch --quiet origin main
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || die "main is not up to date with origin/main"
# Changie would batch an empty release for an explicit version.
ls .changes/unreleased/*.yaml >/dev/null 2>&1 || die "no unreleased changes; add one with make change"

version=${1:-$($CHANGIE next auto)}
# Changie accepts versions such as v1.2 that are not valid Go module tags.
echo "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$' ||
  die "expected a version like v1.2.3, got '$version'"

$CHANGIE batch "$version"
$CHANGIE merge
git add .changes CHANGELOG.md
git commit --quiet -m "Release $version"

cat <<EOF
Committed "Release $version". Review it with git show, then tag and push:

  git tag -a $version -m $version
  git push --atomic origin main $version

Optionally, publish the release notes on GitHub (skipping the version heading):

  tail -n +3 .changes/$version.md | gh release create $version --verify-tag --title $version --notes-file -
EOF
