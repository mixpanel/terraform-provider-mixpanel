#!/usr/bin/env bash
# Publish build/wiki/ to the repo's GitHub Wiki (.wiki.git).
# Prereqs: run scripts/build_wiki.py first (creates build/wiki/), and the wiki
# repo must be initialized (create the first page once in the GitHub UI).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD="$REPO_ROOT/build/wiki"
WIKI_URL="https://github.com/mixpanel/terraform-provider-mixpanel.wiki.git"

if ! ls "$BUILD"/*.md >/dev/null 2>&1; then
  echo "error: no pages in $BUILD — run 'python3 scripts/build_wiki.py' first" >&2
  exit 1
fi

SHA="$(git -C "$REPO_ROOT" rev-parse --short HEAD)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

if ! git clone --depth 1 "$WIKI_URL" "$TMP" 2>/dev/null; then
  echo "error: could not clone $WIKI_URL" >&2
  echo "the wiki repo is likely not initialized — create the first page once in" >&2
  echo "the GitHub UI (repo -> Wiki -> Create the first page), then re-run." >&2
  exit 1
fi

find "$TMP" -maxdepth 1 -name '*.md' -delete
cp "$BUILD"/*.md "$TMP"/
git -C "$TMP" add -A
if git -C "$TMP" diff --cached --quiet; then
  echo "wiki already up to date; nothing to push"
  exit 0
fi
git -C "$TMP" commit -q -m "wiki: sync from docs @ $SHA"
git -C "$TMP" push -q origin HEAD
echo "pushed wiki update (docs @ $SHA)"
