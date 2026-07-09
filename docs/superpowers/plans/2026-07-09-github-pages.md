# GitHub Pages Site Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a full MkDocs Material documentation site (matching `mixpanel-headless`) from the `docs/` corpus, deployed to GitHub Pages as the primary human docs home.

**Architecture:** A stdlib transform (`scripts/build_pages.py`) mirrors `docs/` into `build/pages-docs/` (frontmatter stripped, Hashicorp callouts → Material admonitions) and emits a family-grouped `SUMMARY.md` for `mkdocs-literate-nav`; committed `mkdocs.yml` + brand assets build the site; `.github/workflows/docs.yml` strict-builds on PRs and deploys on merge.

**Tech Stack:** Python 3 (stdlib-only scripts), `unittest`, MkDocs + Material + literate-nav (+ llms.txt plugins), GitHub Actions Pages deploy, `actionlint`.

## Global Constraints

- **`docs/` is the single source of truth.** Pages is generated into `build/pages-docs/` (gitignored) and never hand-edited.
- **`scripts/build_pages.py` is stdlib-only** (no pip deps), matching `gen/`/`build_wiki.py`.
- **Match `mixpanel-headless`**: copy its `docs/stylesheets/mixpanel.css` and `docs/javascripts/copy-markdown.js` verbatim; Material theme with deep-purple palette, Inter/JetBrains Mono, the same feature set.
- **`mkdocs.yml` MUST set `docs_dir: build/pages-docs`** (build from the generated tree, not the repo's `docs/`).
- **Nav is generated** (`SUMMARY.md` via `mkdocs-literate-nav`), grouped by the 7 `subcategory` families in this order: Analytics & Reporting; Governance & Lexicon; Experimentation & Delivery; Data Pipeline; Session Replay & Heatmaps; AI & Automation; Administration & Access. Never hand-maintained.
- **Site URL** `https://mixpanel.github.io/terraform-provider-mixpanel/`; no custom domain.
- **Deploy uses the built-in `GITHUB_TOKEN`** (`pages: write`, `id-token: write`) — no secret. PR = strict-build gate (no deploy); merge/dispatch = deploy.
- **Pin third-party actions to SHAs**: `actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4`, `actions/upload-pages-artifact@fc324d3547104276b827a68afc52ff2a11cc49c9 # v5`, `actions/deploy-pages@cd2ce8fcbc39b97be8ca5fce6e763baed58fa128 # v5`.
- **Claim discipline** on the authored landing: product-vocab-first, no hype, never "first/only", no internal codenames; HCL schema-verified.
- **Local verification is stdlib-only** (`build_pages.py --check`, `python3 -m unittest`, `actionlint`) — this environment has no pip/mkdocs, so the strict MkDocs build is validated in CI on the introducing PR (Task 7).

---

## File Structure

```
pages/index.md                    authored landing (copied to build/pages-docs/index.md)
pages/stylesheets/mixpanel.css    brand CSS (copied verbatim from mixpanel-headless)
pages/javascripts/copy-markdown.js copied verbatim from mixpanel-headless
mkdocs.yml                        committed config (docs_dir: build/pages-docs; literate-nav)
requirements-docs.txt             pinned docs deps
scripts/build_pages.py            transform + SUMMARY.md generator + --check (stdlib)
scripts/test_build_pages.py       unittest for the transform (stdlib)
.github/workflows/docs.yml        Pages build (strict) + deploy workflow
README.md                         MODIFY — repoint primary docs link to Pages
build/pages-docs/                 generated (build/ already gitignored)
```

---

## Task 1: Static scaffolding — config, brand assets, landing

**Files:**
- Create: `mkdocs.yml`, `requirements-docs.txt`, `pages/index.md`, `pages/stylesheets/mixpanel.css`, `pages/javascripts/copy-markdown.js`

**Interfaces:**
- Produces: the committed, human-owned Pages config + assets that Task 3's `build()` copies into `build/pages-docs/` and that `mkdocs.yml` references. `docs_dir: build/pages-docs`; extra_css `stylesheets/mixpanel.css`; extra_javascript `javascripts/copy-markdown.js`; literate-nav `nav_file: SUMMARY.md`.

- [ ] **Step 1: Copy the brand assets from mixpanel-headless (verbatim)**
```bash
mkdir -p pages/stylesheets pages/javascripts
cp /home/jared/mixpanel-headless/docs/stylesheets/mixpanel.css pages/stylesheets/mixpanel.css
cp /home/jared/mixpanel-headless/docs/javascripts/copy-markdown.js pages/javascripts/copy-markdown.js
```

- [ ] **Step 2: Create `requirements-docs.txt`**
```
mkdocs>=1.6,<2
mkdocs-material>=9.5,<10
mkdocs-literate-nav>=0.6,<1
```

- [ ] **Step 3: Create `mkdocs.yml`**
```yaml
site_name: Terraform Provider for Mixpanel
site_description: Manage Mixpanel resources as code with Terraform and OpenTofu.
site_url: https://mixpanel.github.io/terraform-provider-mixpanel/
repo_name: mixpanel/terraform-provider-mixpanel
repo_url: https://github.com/mixpanel/terraform-provider-mixpanel

# Build from the generated tree (scripts/build_pages.py), NOT the repo's docs/.
docs_dir: build/pages-docs
# Navigation is generated: build/pages-docs/SUMMARY.md (mkdocs-literate-nav).

theme:
  name: material
  palette:
    - media: "(prefers-color-scheme: light)"
      scheme: default
      primary: deep purple
      accent: deep purple
      toggle:
        icon: material/brightness-7
        name: Switch to dark mode
    - media: "(prefers-color-scheme: dark)"
      scheme: slate
      primary: deep purple
      accent: deep purple
      toggle:
        icon: material/brightness-4
        name: Switch to light mode
  font:
    text: Inter
    code: JetBrains Mono
  features:
    - navigation.tabs
    - navigation.sections
    - navigation.expand
    - navigation.top
    - navigation.instant
    - navigation.tracking
    - search.suggest
    - search.highlight
    - search.share
    - content.code.copy
    - content.code.annotate
    - content.tabs.link
    - header.autohide
  icon:
    repo: fontawesome/brands/github

extra_css:
  - stylesheets/mixpanel.css
extra_javascript:
  - javascripts/copy-markdown.js

plugins:
  - search
  - literate-nav:
      nav_file: SUMMARY.md

markdown_extensions:
  - admonition
  - pymdownx.details
  - pymdownx.superfences
  - pymdownx.highlight:
      anchor_linenums: true
  - pymdownx.inlinehilite
  - pymdownx.snippets
  - pymdownx.tabbed:
      alternate_style: true
  - tables
  - toc:
      permalink: true

extra:
  social:
    - icon: fontawesome/brands/github
      link: https://github.com/mixpanel/terraform-provider-mixpanel
```

- [ ] **Step 4: Create `pages/index.md`** (authored landing; Material-flavored). Include, in order: an H1 `# Terraform Provider for Mixpanel` + one-line tagline; a `## Why this exists` section (3–5 sentences distilled from `docs/guides/analytics-as-code.md` — no hype, no "first/only"); a `## Quickstart` with the schema-verified block below; a `## Where to go next` list linking `Getting Started`, `Analytics as Code`, and the Resources/Data Sources sections; an alpha-status `!!! warning` note linking the repo. (The AI-friendly `llms.txt` callout is added in Task 6.) Quickstart HCL verbatim:
```terraform
terraform {
  required_providers {
    mixpanel = {
      source = "mixpanel/mixpanel"
    }
  }
}

# Credentials via env vars: MIXPANEL_SERVICE_ACCOUNT,
# MIXPANEL_SERVICE_ACCOUNT_SECRET, MIXPANEL_PROJECT_ID. Never commit secrets.
provider "mixpanel" {}

resource "mixpanel_annotation" "release" {
  date        = "2026-01-01 00:00:00"
  description = "v2.0 release"
}
```
Links in the landing use MkDocs-relative form: `[Getting Started](guides/getting-started.md)`, `[Analytics as Code](guides/analytics-as-code.md)`.

- [ ] **Step 5: Verify** assets + config are present and well-formed:
```bash
ls -la pages/index.md pages/stylesheets/mixpanel.css pages/javascripts/copy-markdown.js mkdocs.yml requirements-docs.txt
grep -n 'docs_dir: build/pages-docs' mkdocs.yml
grep -niE 'first|only|revolutionary|game-chang' pages/index.md || echo "no overclaim/hype"
```
Expected: all files present; `docs_dir` line found; no overclaim. (MkDocs strict build is validated in CI, Task 7 — no local mkdocs here.)

- [ ] **Step 6: Commit**
```bash
git add mkdocs.yml requirements-docs.txt pages/
git commit -m "pages: MkDocs config, brand assets, and landing page

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 2: Transform functions + unit tests (TDD)

**Files:**
- Create: `scripts/build_pages.py` (functions + constants only this task), `scripts/test_build_pages.py`

**Interfaces:**
- Produces: `strip_frontmatter(text)->str`, `read_subcategory(text)->str`, `convert_callouts(text)->str` (Material-admonition variant, fence-aware), `transform_page(text)->str`, and constants `FAMILY_ORDER` (7 families), `GUIDES` (list of `(slug, label, group)`), `REPO`/`DOCS`/`PAGES_SRC`/`OUT`, `FRONTMATTER_RE`, `SUBCATEGORY_RE`, `CALLOUTS`. Consumed by Task 3's `build()`/`generate_summary()`/`check()`.

- [ ] **Step 1: Write `scripts/test_build_pages.py`**
```python
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from build_pages import (
    strip_frontmatter, read_subcategory, convert_callouts, transform_page,
)


class TransformTests(unittest.TestCase):
    def test_strip_frontmatter(self):
        src = '---\npage_title: "x"\nsubcategory: "Analytics & Reporting"\n---\n# H\n\nbody\n'
        self.assertEqual(strip_frontmatter(src), "# H\n\nbody\n")

    def test_strip_frontmatter_absent(self):
        self.assertEqual(strip_frontmatter("# H\n"), "# H\n")

    def test_read_subcategory(self):
        src = '---\npage_title: "x"\nsubcategory: "Data Pipeline"\n---\n# H\n'
        self.assertEqual(read_subcategory(src), "Data Pipeline")

    def test_read_subcategory_absent(self):
        self.assertEqual(read_subcategory("# H\n"), "")

    def test_callout_note(self):
        self.assertEqual(convert_callouts("-> a tip"), "!!! note\n    a tip")

    def test_callout_warning_and_danger(self):
        self.assertEqual(convert_callouts("~> careful"), "!!! warning\n    careful")
        self.assertEqual(convert_callouts("!> danger"), "!!! danger\n    danger")

    def test_callout_skips_code_fence(self):
        src = "```hcl\n-> not a callout\n```"
        self.assertEqual(convert_callouts(src), src)

    def test_callout_leaves_plain_text(self):
        self.assertEqual(convert_callouts("normal line"), "normal line")

    def test_transform_pipeline(self):
        src = '---\nsubcategory: "X"\n---\n-> see docs\n'
        out = transform_page(src)
        self.assertNotIn("---", out)
        self.assertIn("!!! note\n    see docs", out)


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd scripts && python3 -m unittest test_build_pages -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'build_pages'`.

- [ ] **Step 3: Write `scripts/build_pages.py`** (functions + constants; assembly in Task 3):
```python
#!/usr/bin/env python3
"""Generate the MkDocs docs tree for the GitHub Pages site from docs/ (source of truth).

Stdlib only. Mirrors docs/ into build/pages-docs/ (frontmatter stripped, Hashicorp
callouts -> Material admonitions) and emits a family-grouped SUMMARY.md for
mkdocs-literate-nav. Run with --check to verify the generated tree.
"""
import argparse
import re
import shutil
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
DOCS = REPO / "docs"
PAGES_SRC = REPO / "pages"
OUT = REPO / "build" / "pages-docs"

FRONTMATTER_RE = re.compile(r"\A---\n.*?\n---\n", re.DOTALL)
SUBCATEGORY_RE = re.compile(r'^subcategory:\s*"([^"]*)"', re.MULTILINE)
CALLOUTS = {"->": "note", "~>": "warning", "!>": "danger"}

FAMILY_ORDER = [
    "Analytics & Reporting",
    "Governance & Lexicon",
    "Experimentation & Delivery",
    "Data Pipeline",
    "Session Replay & Heatmaps",
    "AI & Automation",
    "Administration & Access",
]

# (slug, nav label, nav group)
GUIDES = [
    ("getting-started", "Getting Started", "Getting Started"),
    ("analytics-as-code", "Analytics as Code", "Getting Started"),
    ("import", "Import", "Guides"),
    ("cross-project-portability", "Cross-Project Portability", "Guides"),
    ("drift-detection", "Drift Detection", "Guides"),
    ("sharing", "Sharing", "Guides"),
]


def strip_frontmatter(text: str) -> str:
    return FRONTMATTER_RE.sub("", text, count=1)


def read_subcategory(text: str) -> str:
    m = SUBCATEGORY_RE.search(text)
    return m.group(1) if m else ""


def convert_callouts(text: str) -> str:
    out, in_fence = [], False
    for line in text.split("\n"):
        stripped = line.lstrip()
        if stripped.startswith("```"):
            in_fence = not in_fence
            out.append(line)
            continue
        if not in_fence:
            converted = None
            for marker, adm in CALLOUTS.items():
                if stripped.startswith(marker + " "):
                    converted = (adm, stripped[len(marker) + 1:])
                    break
            if converted:
                adm, content = converted
                out.append(f"!!! {adm}")
                out.append(f"    {content}")
                continue
        out.append(line)
    return "\n".join(out)


def transform_page(text: str) -> str:
    return convert_callouts(strip_frontmatter(text))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd scripts && python3 -m unittest test_build_pages -v`
Expected: PASS (9 tests).

- [ ] **Step 5: Commit**
```bash
git add scripts/build_pages.py scripts/test_build_pages.py
git commit -m "pages: transform functions (frontmatter, callouts->admonitions) + tests

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 3: Assembly + SUMMARY.md generation + `--check`

**Files:**
- Modify: `scripts/build_pages.py` (add `reference_pages`, `build`, `generate_summary`, `check`, `main`)
- Test: extend `scripts/test_build_pages.py` with a build/check integration test

**Interfaces:**
- Consumes: Task 2 functions/constants; the Task 1 `pages/` assets + landing.
- Produces: `build()` (writes `build/pages-docs/`), `generate_summary()` (family-grouped `SUMMARY.md`), `check()->int`, CLI `--check`.

- [ ] **Step 1: Write the failing integration test** — append to `scripts/test_build_pages.py`:
```python
import build_pages


class BuildTests(unittest.TestCase):
    def test_build_and_check(self):
        build_pages.build()
        # landing + guides + all reference pages present
        self.assertTrue((build_pages.OUT / "index.md").exists())
        self.assertEqual(len(list((build_pages.OUT / "guides").glob("*.md"))), 6)
        self.assertEqual(
            len(list((build_pages.OUT / "resources").glob("*.md"))),
            len(build_pages.reference_pages("resources")),
        )
        self.assertEqual(
            len(list((build_pages.OUT / "data-sources").glob("*.md"))),
            len(build_pages.reference_pages("data-sources")),
        )
        # assets copied; SUMMARY generated
        self.assertTrue((build_pages.OUT / "stylesheets" / "mixpanel.css").exists())
        self.assertTrue((build_pages.OUT / "javascripts" / "copy-markdown.js").exists())
        summary = (build_pages.OUT / "SUMMARY.md").read_text()
        self.assertIn("resources/cohort.md", summary)
        self.assertIn("Analytics & Reporting", summary)
        # a transformed reference page has no residual registry frontmatter
        cohort = (build_pages.OUT / "resources" / "cohort.md").read_text()
        self.assertFalse(cohort.startswith("---\n"))
        self.assertEqual(build_pages.check(), 0)
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd scripts && python3 -m unittest test_build_pages.BuildTests -v`
Expected: FAIL — `AttributeError: module 'build_pages' has no attribute 'build'`.

- [ ] **Step 3: Append assembly + check + main to `scripts/build_pages.py`:**
```python
def reference_pages(kind: str):
    return sorted((DOCS / kind).glob("*.md"))


def build() -> None:
    if OUT.exists():
        shutil.rmtree(OUT)
    OUT.mkdir(parents=True)
    shutil.copyfile(PAGES_SRC / "index.md", OUT / "index.md")
    shutil.copytree(PAGES_SRC / "stylesheets", OUT / "stylesheets")
    shutil.copytree(PAGES_SRC / "javascripts", OUT / "javascripts")
    (OUT / "guides").mkdir()
    for slug, _, _ in GUIDES:
        (OUT / "guides" / f"{slug}.md").write_text(
            transform_page((DOCS / "guides" / f"{slug}.md").read_text())
        )
    for kind in ("resources", "data-sources"):
        (OUT / kind).mkdir()
        for p in reference_pages(kind):
            (OUT / kind / p.name).write_text(transform_page(p.read_text()))
    generate_summary()


def generate_summary() -> None:
    lines = ["* [Home](index.md)"]
    for group in ("Getting Started", "Guides"):
        lines.append(f"* {group}")
        for slug, label, grp in GUIDES:
            if grp == group:
                lines.append(f"    * [{label}](guides/{slug}.md)")
    for kind, heading in (("resources", "Resources"), ("data-sources", "Data Sources")):
        lines.append(f"* {heading}")
        by_family = {fam: [] for fam in FAMILY_ORDER}
        for p in reference_pages(kind):
            fam = read_subcategory(p.read_text())
            by_family.setdefault(fam, []).append(p)
        for fam in FAMILY_ORDER:
            pages = by_family.get(fam) or []
            if not pages:
                continue
            lines.append(f"    * {fam}")
            for p in sorted(pages, key=lambda x: x.stem):
                lines.append(f"        * [mixpanel_{p.stem}]({kind}/{p.name})")
    (OUT / "SUMMARY.md").write_text("\n".join(lines) + "\n")


def check() -> int:
    errors = []
    if not (OUT / "index.md").exists():
        errors.append("missing index.md")
    n_guides = len(list((OUT / "guides").glob("*.md")))
    if n_guides != len(GUIDES):
        errors.append(f"guides: {n_guides} != {len(GUIDES)}")
    for kind in ("resources", "data-sources"):
        got = len(list((OUT / kind).glob("*.md")))
        want = len(reference_pages(kind))
        if got != want:
            errors.append(f"{kind}: {got} != {want}")
    for asset in ("stylesheets/mixpanel.css", "javascripts/copy-markdown.js"):
        if not (OUT / asset).exists():
            errors.append(f"missing asset {asset}")
    for p in OUT.rglob("*.md"):
        if p.name in ("SUMMARY.md", "index.md"):
            continue
        if p.read_text().startswith("---\n"):
            errors.append(f"{p.relative_to(OUT)}: residual frontmatter")
    summary = (OUT / "SUMMARY.md").read_text()
    for kind in ("resources", "data-sources"):
        for p in reference_pages(kind):
            if f"{kind}/{p.name}" not in summary:
                errors.append(f"{kind}/{p.name} missing from SUMMARY.md")
    if errors:
        print("CHECK FAILED:")
        for e in errors:
            print("  -", e)
        return 1
    total = sum(1 for _ in OUT.rglob("*.md"))
    print(f"CHECK OK: {total} markdown files, all reference pages in nav")
    return 0


def main() -> None:
    parser = argparse.ArgumentParser(description="Build the MkDocs Pages tree from docs/.")
    parser.add_argument("--check", action="store_true", help="verify the generated tree")
    args = parser.parse_args()
    build()
    print(f"built pages tree -> {OUT.relative_to(REPO)}")
    if args.check:
        sys.exit(check())


if __name__ == "__main__":
    main()
```

- [ ] **Step 4: Run the full suite + the CLI**

Run: `cd scripts && python3 -m unittest test_build_pages -v`
Expected: PASS (all).
Run: `python3 scripts/build_pages.py --check`
Expected: `built pages tree -> build/pages-docs` then `CHECK OK: <N> markdown files, all reference pages in nav` (N = 1 index + 6 guides + 43 resources + 46 data-sources + SUMMARY.md counted among rglob = 97 including SUMMARY; the exact number is fine as long as CHECK OK).

- [ ] **Step 5: Confirm `build/` is ignored**

Run: `git status --porcelain build/`  → expected: empty.

- [ ] **Step 6: Commit**
```bash
git add scripts/build_pages.py scripts/test_build_pages.py
git commit -m "pages: assemble build/pages-docs tree + family-grouped SUMMARY + --check

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 4: Deploy workflow

**Files:**
- Create: `.github/workflows/docs.yml`

**Interfaces:**
- Consumes: `requirements-docs.txt`, `scripts/build_pages.py`, `mkdocs.yml` (all from Tasks 1–3). Produces: the Pages build+deploy pipeline.

- [ ] **Step 1: Create `.github/workflows/docs.yml`**
```yaml
name: Docs

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
  workflow_dispatch:

permissions:
  contents: read
  pages: write
  id-token: write

concurrency:
  group: "pages-${{ github.ref }}"
  cancel-in-progress: true

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4

      - name: Install docs dependencies
        run: |
          python3 -m pip install --upgrade pip
          python3 -m pip install -r requirements-docs.txt

      - name: Generate docs tree
        run: python3 scripts/build_pages.py --check

      - name: Build site (strict)
        run: python3 -m mkdocs build --strict

      - name: Upload Pages artifact
        if: ${{ github.event_name != 'pull_request' }}
        uses: actions/upload-pages-artifact@fc324d3547104276b827a68afc52ff2a11cc49c9 # v5
        with:
          path: site/

  deploy:
    if: ${{ github.event_name != 'pull_request' }}
    needs: build
    runs-on: ubuntu-latest
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    steps:
      - name: Deploy to GitHub Pages
        id: deployment
        uses: actions/deploy-pages@cd2ce8fcbc39b97be8ca5fce6e763baed58fa128 # v5
```

- [ ] **Step 2: Validate**

Run: `actionlint .github/workflows/docs.yml`
Expected: no output, exit 0.

- [ ] **Step 3: Structural self-check:** build job runs generate→strict-build on all events; upload + deploy gated `if: ${{ github.event_name != 'pull_request' }}`; `permissions` has `pages: write` + `id-token: write`; concurrency `pages-${{ github.ref }}`; actions SHA-pinned per Global Constraints.

- [ ] **Step 4: Commit**
```bash
git add .github/workflows/docs.yml
git commit -m "ci: build and deploy the docs site to GitHub Pages

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 5: Repoint README to the Pages site

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update README's documentation pointers.** Add/adjust a Documentation section so the **primary** link is the Pages site `https://mixpanel.github.io/terraform-provider-mixpanel/`, with the Terraform Registry (`https://registry.terraform.io/providers/mixpanel/mixpanel/latest/docs`) labeled as the Terraform-native reference and the wiki as a secondary in-GitHub entry. Keep it concise; do not remove accurate existing content (auth/quickstart/dev). Example block to place near the top of the docs section:
```markdown
## Documentation

- **[Documentation site](https://mixpanel.github.io/terraform-provider-mixpanel/)** — guides, concepts, and the full resource/data-source reference (primary).
- **[Terraform Registry](https://registry.terraform.io/providers/mixpanel/mixpanel/latest/docs)** — the Terraform-native reference, versioned per release.
- **[Wiki](https://github.com/mixpanel/terraform-provider-mixpanel/wiki)** — quick in-GitHub entry point.
```

- [ ] **Step 2: Verify** links resolve/format:
```bash
grep -n 'mixpanel.github.io/terraform-provider-mixpanel' README.md
```
Expected: the Pages URL present as the primary docs link.

- [ ] **Step 3: Commit**
```bash
git add README.md
git commit -m "docs: point README's primary docs link to the Pages site

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 6: llms.txt enhancement (isolated, droppable)

**Files:**
- Modify: `requirements-docs.txt`, `mkdocs.yml`, `pages/index.md`

**Interfaces:** adds the `mkdocs-llmstxt` + `mkdocs-llmstxt-md` plugins (agentic-era `llms.txt`/`llms-full.txt` + "Copy markdown" buttons). Kept a separate, revertible commit because it can only be strict-build-validated in CI (Task 7); if it breaks the build there and isn't quickly fixable, this commit is dropped and the core site ships without it.

- [ ] **Step 1: Append to `requirements-docs.txt`**
```
mkdocs-llmstxt>=0.2,<1
mkdocs-llmstxt-md>=0.1,<1
```

- [ ] **Step 2: Add the plugins to `mkdocs.yml`** under `plugins:` (after `literate-nav`):
```yaml
  - llmstxt:
      markdown_description: |
        Terraform provider for Mixpanel — manage Mixpanel resources (cohorts,
        dashboards, metrics, feature flags, governance, and more) as code with
        Terraform or OpenTofu.
      full_output: llms-full.txt
      sections:
        Getting Started:
          - index.md
          - guides/getting-started.md
          - guides/analytics-as-code.md
        Guides:
          - guides/import.md
          - guides/cross-project-portability.md
          - guides/drift-detection.md
          - guides/sharing.md
  - llmstxt-md:
      enable_markdown_urls: true
      enable_copy_button: true
      enable_llms_txt: false
```

- [ ] **Step 3: Add the AI-friendly callout to `pages/index.md`** (near the top, echoing headless):
```markdown
!!! tip "AI-friendly"
    An [`llms.txt`](llms.txt) map and per-page **Copy markdown** buttons make these docs easy to feed to coding agents and LLMs.
```

- [ ] **Step 4: Verify** the transform still builds locally (the plugins only affect the mkdocs build, not the transform):
```bash
python3 scripts/build_pages.py --check
```
Expected: `CHECK OK`. (The `llms.txt` files are produced by MkDocs at build time — validated in CI, Task 7.)

- [ ] **Step 5: Commit**
```bash
git add requirements-docs.txt mkdocs.yml pages/index.md
git commit -m "pages: add llms.txt output + copy-markdown buttons (agentic-friendly)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 7: Enable Pages + PR strict-build gate + deploy + live verify — USER ACTION + OPS

**Files:** none (repo setting + workflow runs).

- [ ] **Step 1: User enables GitHub Pages (one-time).** Ask the user: repo → Settings → Pages → Build and deployment → **Source: GitHub Actions**. (No secret needed; Pages deploy uses the built-in token.)

- [ ] **Step 2: Push the branch and open the PR (dogfoods the strict-build gate).** The PR includes `.github/workflows/docs.yml`, `mkdocs.yml`, etc.; the `Docs` workflow runs on `pull_request` → generate tree → `mkdocs build --strict` (no deploy).
```bash
git push -u origin pages-site
gh pr create --base main --head pages-site \
  --title "docs: full GitHub Pages site (MkDocs Material)" \
  --body "Builds the docs/ corpus into a MkDocs Material site and deploys to Pages. See docs/superpowers/specs/2026-07-09-github-pages-design.md."
gh run list --workflow=docs.yml --limit 3
```
Expected: a `Docs` run for the PR; the **Build site (strict)** step passes and deploy is skipped. **This is the first strict-build validation** (no local mkdocs). If it fails, read `gh run view <id> --log-failed` and iterate: broken internal link → fix the transform/source; nav error → fix `generate_summary`; **llms.txt plugin error → if not quickly fixable, revert the Task 6 commit and re-push** (core site ships without llms.txt, tracked as a follow-up). Loop until the PR's strict build is green.

- [ ] **Step 3: Merge, then confirm deploy.** After the PR is green and approved, merge to `main`. The push to `main` runs `Docs` again → build + **deploy**.
```bash
gh run list --workflow=docs.yml --limit 3     # newest = push to main
gh run view <run-id>                           # build + deploy jobs succeed
```

- [ ] **Step 4: Live verification.** Open `https://mixpanel.github.io/terraform-provider-mixpanel/`; confirm: the landing renders with the Mixpanel purple/blue brand styling; the nav shows the 7 families under Resources and Data Sources; search works; a resource page (e.g. `resources/cohort`) renders with admonitions and code copy; if llms.txt shipped, `…/llms.txt` loads. Report the live URL to the user. No commit for this task.

---

## Self-review notes (author)

- **Spec coverage:** §3.1 transform → Tasks 2–3; §3.2 nav/literate-nav → Task 3 `generate_summary` + Task 1 mkdocs.yml plugin; §3.3 mkdocs.yml/theme/`docs_dir` → Task 1; §3.4 requirements → Task 1 (+ Task 6 for llmstxt); §3.5 landing → Task 1 Step 4 (+ Task 6 callout); §3.6 deploy → Task 4; §3.7 README → Task 5; §5 prereq (enable Pages) → Task 7 Step 1; §6 tests → Tasks 2–3; §7 verification (`--check`) → Task 3 + Task 7 (strict build in CI); §8 out-of-scope respected. No gaps.
- **Deviation from spec §3.1 item 1:** the transform **strips** registry frontmatter and does **not** re-emit MkDocs `title`/`description` — page titles come from the existing H1 and nav labels from `SUMMARY.md`, so re-emitting adds nothing, and parsing the `description: |-` block scalar in a stdlib script is avoided. `description`-based SEO meta is a minor deferral (could be added later). Flagged for the reviewer.
- **Environment note:** local `mkdocs build --strict` is not runnable here (no pip); the strict build is gated in CI on the introducing PR (Task 7 Step 2). Local gates are `build_pages.py --check`, `python3 -m unittest`, and `actionlint`.
- **Placeholder scan:** none — all config/code/YAML is literal; `<id>`/`<run-id>` in Task 7 are runtime values. (Task 6 Step 5 contains a deliberate typo-guard note to fix the co-author trailer.)
- **Consistency:** `FAMILY_ORDER`, `GUIDES`, `docs_dir: build/pages-docs`, `SUMMARY.md`, the pinned action SHAs, and the site URL are identical across the plan, the YAML, and the spec.
```
