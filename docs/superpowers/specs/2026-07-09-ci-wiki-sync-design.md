# CI Wiki-Sync Automation — Design

**Date:** 2026-07-09
**Status:** Approved (design presented and accepted in session)
**Depends on:** the wiki build/publish tooling merged on `main` (`scripts/build_wiki.py`, `scripts/publish_wiki.sh`, `wiki/`, PR #20).
**Scope:** Phase 2, sub-project A of two (the other is the GitHub Pages site — separate spec/plan cycle).

## 1. Purpose

Make the GitHub Wiki self-maintaining: a GitHub Actions workflow rebuilds the wiki from `docs/` and publishes it automatically, and validates the transform on PRs before merge. Removes the manual `build_wiki.py` + `publish_wiki.sh` step while keeping `docs/` the single source of truth.

## 2. Decisions (user-confirmed)

- **Gate on PR, publish on merge.** PRs run build + `--check` only (no publish); pushes to `main` build + `--check` + publish; plus manual `workflow_dispatch`.
- **Auth via a fine-grained PAT** stored as the `WIKI_SYNC_TOKEN` Actions secret (Contents: write, this repo only) — reliable for `.wiki.git` pushes, unlike the default `GITHUB_TOKEN`.

## 3. Deliverable

One workflow file: `.github/workflows/wiki-sync.yml`. No changes to the existing wiki scripts (the workflow injects auth around the unmodified `publish_wiki.sh`).

### 3.1 Triggers (path-filtered)

```yaml
on:
  pull_request:
    branches: [main]
    paths:
      - docs/guides/**
      - wiki/**
      - scripts/build_wiki.py
      - scripts/test_build_wiki.py
      - .github/workflows/wiki-sync.yml
  push:
    branches: [main]
    paths:
      - docs/guides/**
      - wiki/**
      - scripts/build_wiki.py
      - scripts/test_build_wiki.py
      - .github/workflows/wiki-sync.yml
  workflow_dispatch:
```

The path list is duplicated on `pull_request` and `push` on purpose — GitHub Actions' support for YAML anchors/aliases in workflow files is inconsistent, so the two lists must be kept in sync by hand (both are also the acceptance-criteria path set).

Rationale for the path set: the wiki output is the 4 authored `wiki/` pages + the 6 transformed `docs/guides/`, produced by `build_wiki.py`. Edits to `docs/resources/**` or `docs/data-sources/**` do **not** change wiki output (the `Reference` page is authored and links to the Registry, not generated from resource pages), so they are intentionally excluded to avoid needless runs. `workflow_dispatch` allows a manual full resync regardless of paths.

### 3.2 Job (`sync`)

Single job on `ubuntu-latest`:

1. `actions/checkout` (main repo).
2. `actions/setup-python` (Python 3.x; the scripts are stdlib-only).
3. **Transform tests:** `python3 -m unittest discover -s scripts -p 'test_*.py'`.
4. **Build + verify:** `python3 scripts/build_wiki.py --check`.
5. **Publish (gated `if: github.event_name != 'pull_request'`):**
   - Guard: if `WIKI_SYNC_TOKEN` is empty, fail with a clear message (misconfigured secret).
   - `git config --global url."https://x-access-token:${WIKI_SYNC_TOKEN}@github.com/".insteadOf "https://github.com/"`
   - `scripts/publish_wiki.sh` (unmodified — its plain-HTTPS clone/push uses the injected credential; its fallback commit identity, added in PR #20, authors the sync commit).

### 3.3 Workflow-level settings

- `permissions: { contents: read }` — the checkout only reads; the wiki push authenticates with the PAT, not `GITHUB_TOKEN`.
- `concurrency: { group: "wiki-sync-${{ github.ref }}", cancel-in-progress: true }` — prevents overlapping publishes; a cancelled run is harmless because the next run republishes the latest (`publish_wiki.sh` is idempotent and no-ops when unchanged).
- Pin third-party actions to commit SHAs (matches the repo's existing workflow convention).

## 4. Prerequisite (user/admin, one-time)

Create a **fine-grained PAT** — Repository access: this repo only; Permissions: **Contents → Read and write** — and add it as the Actions secret **`WIKI_SYNC_TOKEN`** (repo Settings → Secrets and variables → Actions). Cannot be scripted for you (minting a credential is an account action); the plan will state exactly what to create. Without it, the PR gate still works; only the publish step needs it.

## 5. Verification

- Lint the workflow with `actionlint` if available; otherwise `gh workflow view` after push.
- **Dogfood:** the PR that introduces `wiki-sync.yml` matches the trigger paths, so the gate job runs on its own PR — confirm it builds + checks green (no publish on the PR).
- After merge + secret configured: make a trivial `docs/guides/` edit (or run `workflow_dispatch`), confirm the run publishes and the live wiki updates; confirm a no-op run when nothing changed.

## 6. Out of scope

- The GitHub Pages site (sub-project B).
- Any change to the wiki content, page set, or the transform itself.
- Automated PAT rotation.

## 7. Acceptance criteria

- `.github/workflows/wiki-sync.yml` exists, is valid, and is pinned per repo convention.
- On PRs touching the wiki paths: the workflow runs tests + `build_wiki.py --check` and does **not** publish.
- On push to `main` (and `workflow_dispatch`): it runs the gate then publishes via `publish_wiki.sh` using `WIKI_SYNC_TOKEN`.
- The existing wiki scripts are unchanged.
- `permissions` is least-privilege (`contents: read`); concurrency prevents overlapping publishes; a missing secret fails the publish step with a clear message.
- Prerequisite documented (fine-grained PAT → `WIKI_SYNC_TOKEN`).
