# CI Wiki-Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a GitHub Actions workflow that validates the wiki transform on PRs and republishes the wiki from `docs/` on merges to `main`, so the wiki is self-maintaining.

**Architecture:** One workflow (`.github/workflows/wiki-sync.yml`), one job: it runs the wiki transform unit tests + `build_wiki.py --check` on every trigger, and on non-PR events additionally runs the unmodified `publish_wiki.sh` with a PAT injected via git `insteadOf`. No changes to the existing wiki scripts.

**Tech Stack:** GitHub Actions, system `python3` (stdlib-only scripts), Bash, `actionlint` (local validation), a fine-grained PAT secret.

## Global Constraints

- **No changes to the existing wiki scripts** (`scripts/build_wiki.py`, `scripts/publish_wiki.sh`, `scripts/test_build_wiki.py`).
- **Auth:** publish authenticates with the **`WIKI_SYNC_TOKEN`** Actions secret (fine-grained PAT, Contents: write, this repo only) — not `GITHUB_TOKEN`.
- **Trigger paths (identical on `pull_request` and `push`, kept in sync by hand):** `docs/guides/**`, `wiki/**`, `scripts/build_wiki.py`, `scripts/test_build_wiki.py`, `.github/workflows/wiki-sync.yml`. Plus `workflow_dispatch`.
- **Publish only on non-PR events** (`if: github.event_name != 'pull_request'`).
- **`permissions: contents: read`** at workflow level (least privilege; the PAT does the wiki push).
- **`concurrency: wiki-sync-${{ github.ref }}`, `cancel-in-progress: true`.**
- **Pin third-party actions to commit SHAs** (repo convention). Reuse `actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4`.
- A missing `WIKI_SYNC_TOKEN` **fails the publish step with a clear message.**

---

## File Structure

```
.github/workflows/wiki-sync.yml   NEW — the gate+publish workflow (the whole deliverable)
CONTRIBUTING.md                   MODIFY — append a short "Wiki (auto-generated)" note
```

Only third-party action used is `actions/checkout` (pinned). The transform tests + build run on the runner's preinstalled `python3` (scripts are stdlib-only), so no `setup-python` action/SHA is needed.

---

## Task 1: The wiki-sync workflow + contributor note

**Files:**
- Create: `.github/workflows/wiki-sync.yml`
- Modify: `CONTRIBUTING.md` (append a section)

**Interfaces:**
- Consumes: `scripts/test_build_wiki.py` (via `python3 -m unittest discover -s scripts`), `scripts/build_wiki.py --check`, `scripts/publish_wiki.sh` (all already on `main`), and the `WIKI_SYNC_TOKEN` secret (created in Task 2).
- Produces: the workflow that runs the gate on PRs and publishes on merge/dispatch.

- [ ] **Step 1: Write `.github/workflows/wiki-sync.yml`** with exactly this content:

```yaml
name: Wiki Sync

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

permissions:
  contents: read

concurrency:
  group: wiki-sync-${{ github.ref }}
  cancel-in-progress: true

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4

      - name: Wiki transform tests
        run: python3 -m unittest discover -s scripts -p 'test_*.py'

      - name: Build and verify wiki tree
        run: python3 scripts/build_wiki.py --check

      - name: Publish to wiki
        if: ${{ github.event_name != 'pull_request' }}
        env:
          WIKI_SYNC_TOKEN: ${{ secrets.WIKI_SYNC_TOKEN }}
        run: |
          if [ -z "${WIKI_SYNC_TOKEN:-}" ]; then
            echo "::error::WIKI_SYNC_TOKEN secret is not set — cannot publish the wiki." >&2
            echo "Create a fine-grained PAT (Contents: write on this repo) and add it as the WIKI_SYNC_TOKEN Actions secret." >&2
            exit 1
          fi
          git config --global url."https://x-access-token:${WIKI_SYNC_TOKEN}@github.com/".insteadOf "https://github.com/"
          ./scripts/publish_wiki.sh
```

- [ ] **Step 2: Validate the workflow with actionlint**

Run: `actionlint .github/workflows/wiki-sync.yml`
Expected: no output, exit 0 (actionlint 1.7.x is installed).

- [ ] **Step 3: Structural self-check** (read the file back and confirm):
  - `pull_request` and `push` carry the **identical** 5-path list; `workflow_dispatch` present.
  - The `Publish to wiki` step is the only one gated by `if: ${{ github.event_name != 'pull_request' }}`; the tests + `--check` steps run unconditionally.
  - `permissions: contents: read`; `concurrency.group` is `wiki-sync-${{ github.ref }}` with `cancel-in-progress: true`.
  - The publish step guards an empty `WIKI_SYNC_TOKEN`, sets the `insteadOf` credential, then calls `./scripts/publish_wiki.sh`.
  - `actions/checkout` is SHA-pinned.

- [ ] **Step 4: Append a "Wiki" note to `CONTRIBUTING.md`** (add at end of file):

```markdown

## Wiki (auto-generated)

The GitHub Wiki is generated from `docs/` — **do not edit wiki pages by hand**
(they are overwritten). `.github/workflows/wiki-sync.yml` validates the transform
on PRs (runs the transform tests and `build_wiki.py --check`) and republishes the
wiki on merges to `main` that touch `docs/guides/`, `wiki/`, or the wiki scripts,
authenticating with the `WIKI_SYNC_TOKEN` secret. To publish manually:
`python3 scripts/build_wiki.py && scripts/publish_wiki.sh`.
```

- [ ] **Step 5: Confirm no wiki scripts were modified**

Run: `git status --porcelain scripts/`
Expected: empty (Task 1 only adds the workflow and edits CONTRIBUTING.md).

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/wiki-sync.yml CONTRIBUTING.md
git commit -m "ci: auto-sync the wiki from docs (gate on PR, publish on merge)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 2: Secret provisioning + end-to-end verification — USER ACTION + OPS

**Files:** none (repo secret + workflow runs).

- [ ] **Step 1: User creates the PAT + secret (one-time, cannot be scripted).** Ask the user to:
  1. GitHub → Settings → Developer settings → **Fine-grained personal access tokens** → Generate: Resource owner `mixpanel`, Repository access → *Only select repositories* → `terraform-provider-mixpanel`, Permissions → Repository → **Contents: Read and write**. Generate and copy the token.
  2. Repo → Settings → Secrets and variables → Actions → **New repository secret** → name **`WIKI_SYNC_TOKEN`**, value = the token.

  Confirm it exists: `gh secret list --repo mixpanel/terraform-provider-mixpanel` should list `WIKI_SYNC_TOKEN`.

- [ ] **Step 2: Dogfood the gate on the introducing PR.** Push the `ci-wiki-sync` branch and open a PR to `main` (the diff includes `.github/workflows/wiki-sync.yml`, which matches the trigger paths, so the `Wiki Sync` gate runs on the PR itself):

```bash
git push -u origin ci-wiki-sync
gh pr create --base main --head ci-wiki-sync \
  --title "ci: auto-sync the wiki from docs" \
  --body "Adds .github/workflows/wiki-sync.yml — validates the wiki transform on PRs and publishes on merge. See docs/superpowers/specs/2026-07-09-ci-wiki-sync-design.md."
gh run list --workflow=wiki-sync.yml --limit 3
```
Expected: a `Wiki Sync` run for the PR that runs the tests + `--check` and **does not** publish (the publish step is skipped on `pull_request`). Confirm the run concluded success and the "Publish to wiki" step shows as skipped (`gh run view <id>`).

- [ ] **Step 3: Merge, then confirm the publish path.** After the secret exists and the PR is approved, merge to `main`. The merge is a `push` to `main` touching the paths → triggers publish. Verify:

```bash
gh run list --workflow=wiki-sync.yml --limit 3        # newest run = the push to main
gh run view <run-id>                                   # "Publish to wiki" step ran (not skipped), success
```
Then confirm the live wiki reflects the current `docs/` (e.g. clone `…​.wiki.git` and diff against `python3 scripts/build_wiki.py` output, or spot-check a page). If nothing changed since the last manual publish, the run logs `wiki already up to date; nothing to push` — also a valid success.

- [ ] **Step 4: (Optional) exercise `workflow_dispatch`.** `gh workflow run wiki-sync.yml --ref main`; confirm it builds, checks, and publishes/no-ops. No commit for this task.

---

## Self-review notes (author)

- **Spec coverage:** §3.1 triggers → Task 1 Step 1 (dup path lists) ✓; §3.2 job steps → Step 1 (tests, `--check`, gated publish with token guard + `insteadOf`) ✓; §3.3 permissions/concurrency/SHA-pin → Step 1 + Global Constraints ✓; §4 prereq PAT/secret → Task 2 Step 1 ✓; §5 verification (actionlint, dogfood, post-merge publish) → Task 1 Step 2 + Task 2 Steps 2–3 ✓; §6 out-of-scope respected (no script/content changes; Step 5 asserts scripts untouched) ✓; §7 acceptance → covered across both tasks. No gaps.
- **Deviation from spec §3.2:** dropped the `actions/setup-python` step — the runner's preinstalled `python3` runs the stdlib-only scripts, removing a pinned-action dependency. Same intent (tests + `--check` run under Python 3). Noted here for the reviewer.
- **Placeholder scan:** none — the full workflow YAML and CONTRIBUTING snippet are literal; `<id>`/`<run-id>` in Task 2 are runtime run IDs the operator reads from `gh run list`, not code placeholders.
- **Consistency:** `WIKI_SYNC_TOKEN` name, the 5-path list, `permissions: contents: read`, and `concurrency: wiki-sync-${{ github.ref }}` are identical between the plan, the YAML, and the spec.
