# GitHub Pages Documentation Site — Design

**Date:** 2026-07-09
**Status:** Approved (design presented and accepted in session)
**Scope:** Phase 2, sub-project B of two (sub-project A, CI wiki-sync, is complete on `main`).
**Style reference:** `mixpanel-headless` MkDocs Material site — https://mixpanel.github.io/mixpanel-headless/ · local `/home/jared/mixpanel-headless` (`mkdocs.yml`, `docs/stylesheets/mixpanel.css`, `docs/javascripts/copy-markdown.js`, `.github/workflows/docs.yml`).

## 1. Purpose

Stand up a full, branded, searchable documentation website on GitHub Pages — the **primary human-facing docs home** for the provider — built from the `docs/` corpus and styled to match the sibling `mixpanel-headless` site. Complements the Terraform Registry (the Terraform-native per-resource reference) and the wiki (a lightweight in-GitHub entry). `docs/` remains the single source of truth; Pages is derived and regenerated, never hand-edited.

## 2. Decisions (user-confirmed)

- **Full corpus** on Pages: a bespoke landing + the 6 guides + all 89 reference pages (43 resources + 46 data sources), grouped by the 7 `subcategory` families, with full-text search.
- **Pages is the primary human docs home** — README repoints to it; Registry stays the Terraform-native reference; the wiki remains a secondary in-GitHub entry (retire-later candidate, out of scope here).
- **SSG:** MkDocs Material, matching `mixpanel-headless` exactly (theme, brand CSS, fonts, feature set, deploy pattern).
- **Served at** `https://mixpanel.github.io/terraform-provider-mixpanel/` (no custom domain).

## 3. Architecture

`docs/` → **transform** (`scripts/build_pages.py`) → `build/pages-docs/` (gitignored) → **MkDocs Material** (`mkdocs.yml` + `requirements-docs.txt`) → **GitHub Actions** (`.github/workflows/docs.yml`) → GitHub Pages.

### 3.1 Transform — `scripts/build_pages.py` (stdlib only)

Generates `build/pages-docs/` **mirroring the `docs/` layout**: `index.md` (the bespoke landing, from `pages/index.md`), `guides/*.md`, `resources/*.md`, `data-sources/*.md` (the latter three transformed from `docs/`). Mirroring the structure means the corpus's existing relative `.md` links resolve natively in MkDocs — minimal link rewriting; `mkdocs build --strict` fails on any broken link.

Per-page transform of the 6 guides + 89 reference pages:
1. **Frontmatter:** strip the registry YAML block; re-emit a clean MkDocs frontmatter with `title` (a concise nav title — the `mixpanel_<name>` type, or the guide's human title) and `description` (carried from the registry `description` for SEO/meta). Drop `page_title` and `subcategory` (subcategory drives nav, §3.2).
2. **Callouts:** convert leading Hashicorp callout markers `->`/`~>`/`!>` to Material admonitions `!!! note` / `!!! warning` / `!!! danger`, **fence-aware** (never inside ``` code blocks) — reusing the wiki transform's fence-tracking logic.
3. **Links:** left as-is (MkDocs resolves relative `.md` links; absolute URLs pass through). Any link that `--strict` flags is a build error to fix in the transform or the source.
4. Ships `--check` (see §7).

The landing page (`pages/index.md`) is authored, not transformed — copied verbatim into `build/pages-docs/index.md`.

### 3.2 Navigation — generated, family-grouped

The transform emits `build/pages-docs/SUMMARY.md`, a literate-nav file consumed by the **`mkdocs-literate-nav`** plugin. Structure:

- Home
- **Getting Started**: Getting Started (`guides/getting-started`), Analytics as Code (`guides/analytics-as-code`)
- **Guides**: Import, Cross-Project Portability, Drift Detection, Sharing
- **Resources**: the 7 families (Analytics & Reporting; Governance & Lexicon; Experimentation & Delivery; Data Pipeline; Session Replay & Heatmaps; AI & Automation; Administration & Access), each listing its resource pages (from `subcategory`)
- **Data Sources**: the same 7 families, each listing its data-source pages

Flat `resources/`/`data-sources/` dirs keep links intact; the generated nav supplies the family grouping. Nav is never hand-maintained — it regenerates from `subcategory` frontmatter each build. `mkdocs.yml` omits an explicit `nav:` and sets literate-nav's `nav_file: SUMMARY.md`.

### 3.3 `mkdocs.yml` + theme (committed, human-owned)

Material theme matching `mixpanel-headless`:
- **`docs_dir: build/pages-docs`** (critical — MkDocs must build from the generated tree, NOT the repo's `docs/` registry corpus, whose frontmatter/links/absence-of-nav are registry-shaped). `site_dir` stays the default `site/` (what the workflow uploads).
- `site_name: "Terraform Provider for Mixpanel"`, `site_url: https://mixpanel.github.io/terraform-provider-mixpanel/`, `repo_url`/`repo_name` for `mixpanel/terraform-provider-mixpanel`.
- Palette: deep-purple base with light/dark toggle; `extra_css: [stylesheets/mixpanel.css]` (copied from headless — the exact Mixpanel brand hexes: primary `#7856ff`, light-mode links/primary `#4f44e0`, the syntax palette) and `extra_javascript: [javascripts/copy-markdown.js]` (copied from headless).
- Fonts: Inter (text), JetBrains Mono (code).
- Features: `navigation.tabs`, `navigation.sections`, `navigation.expand`, `navigation.top`, `navigation.instant`, `navigation.tracking`, `search.suggest`, `search.highlight`, `search.share`, `content.code.copy`, `content.code.annotate`, `content.tabs.link`, `header.autohide`.
- Plugins: `search`, `literate-nav` (`nav_file: SUMMARY.md`), `mkdocs-llmstxt` + `mkdocs-llmstxt-md` (kept — the `llms.txt`/`llms-full.txt` output and "Copy markdown" buttons serve the agentic-era pitch). **Not** `mkdocstrings`/`mkdocs-typer` (Python-lib-only; this is a Go provider with markdown reference).
- `markdown_extensions`: `admonition`, `pymdownx.details`, `pymdownx.superfences`, `pymdownx.highlight` (anchor_linenums), `pymdownx.inlinehilite`, `pymdownx.snippets`, `pymdownx.tabbed` (alternate_style), `tables`, `toc` (permalink).
- The `stylesheets/mixpanel.css` and `javascripts/copy-markdown.js` live under the built docs dir; the transform copies them from committed sources (`pages/stylesheets/`, `pages/javascripts/`) into `build/pages-docs/`.

The static bits — `mkdocs.yml`, `requirements-docs.txt`, `pages/index.md`, `pages/stylesheets/mixpanel.css`, `pages/javascripts/copy-markdown.js` — are committed and human-owned; only `build/pages-docs/` is generated/gitignored.

### 3.4 `requirements-docs.txt`

Pinned (`==` or `>=` with upper caution): `mkdocs`, `mkdocs-material`, `mkdocs-literate-nav`, `mkdocs-llmstxt`, `mkdocs-llmstxt-md`. No `uv`/`pyproject` (Go repo).

### 3.5 Landing — `pages/index.md` (authored)

Echoes headless's landing: H1 + one-line tagline; a "Why this exists" narrative distilled from `analytics-as-code.md` (no hype, no "first/only"); the schema-verified `mixpanel_annotation` provider-block quickstart; quicklinks to Getting Started, Analytics as Code, and the Resources/Data Sources sections; an alpha-status note (linking the Registry + repo); an AI-friendly `!!! tip` callout pointing at the generated `llms.txt`. Claim discipline and no-internal-codename rules carry over from Phase 0.

### 3.6 Deploy — `.github/workflows/docs.yml`

Mirrors headless's docs workflow:
- Triggers: `push` → `main`, `pull_request` → `main`, `workflow_dispatch`.
- `permissions: { contents: read, pages: write, id-token: write }`.
- `concurrency: { group: "pages-${{ github.ref }}", cancel-in-progress: true }`.
- **build** job: checkout → `actions/setup-python` → `pip install -r requirements-docs.txt` → `python3 scripts/build_pages.py` (generate the tree) → `mkdocs build --strict` → `actions/upload-pages-artifact` **with `if: github.event_name != 'pull_request'`**.
- **deploy** job (`if: github.event_name != 'pull_request'`, `needs: build`): `actions/deploy-pages`, environment `github-pages`.
- Third-party actions SHA-pinned (repo convention). **PR = strict-build gate (no deploy); merge/dispatch = deploy.**

### 3.7 README repoint

Update README's documentation links so the Pages site is the primary human docs home; keep the Registry (Terraform-native reference) and wiki (in-GitHub entry) as secondary, clearly-labeled links.

## 4. Repository layout (new/changed)

```
pages/
  index.md                     authored landing
  stylesheets/mixpanel.css     brand CSS (copied from headless, adapted names)
  javascripts/copy-markdown.js copied from headless
mkdocs.yml                     committed MkDocs config (no explicit nav; literate-nav)
requirements-docs.txt          pinned docs deps
scripts/build_pages.py         transform + SUMMARY.md generator + --check (stdlib)
scripts/test_build_pages.py    unittest for the transform (stdlib)
.github/workflows/docs.yml     Pages build+deploy workflow
build/pages-docs/              generated (gitignored — build/ already ignored)
README.md                      MODIFY — repoint docs links to Pages
```

## 5. Prerequisite (user/admin, one-time)

Enable GitHub Pages with **source = GitHub Actions** (repo Settings → Pages → Build and deployment → Source: GitHub Actions). Cannot be scripted; the first deploy job fails cleanly until this is set. No `WIKI_SYNC_TOKEN`-style secret is needed — Pages deploy uses the built-in `GITHUB_TOKEN` with `pages: write` + `id-token: write`.

## 6. Testing

- **`scripts/test_build_pages.py`** (stdlib `unittest`): frontmatter strip+re-emit, callout→admonition conversion (incl. fence-skip), and a build/`--check` integration test asserting the generated tree.
- **`mkdocs build --strict`** is the strong end-to-end gate: any broken internal link, missing nav target, or config error fails it. Run locally and in the PR gate.

## 7. Verification (`build_pages.py --check`)

Over `build/pages-docs/`: all 96 corpus pages present (`index.md` + 6 guides + 43 resources + 46 data sources); no residual registry frontmatter (`page_title`/`subcategory`); every reference page assigned to a family in `SUMMARY.md` (no orphans); `stylesheets/mixpanel.css` + `javascripts/copy-markdown.js` copied. Plus, once online, `mkdocs build --strict` passes and `mkdocs serve` renders the landing, nav, and search.

## 8. Out of scope

- Custom domain (served at the `github.io` path).
- Retiring or redirecting the wiki.
- Versioned docs (`mike`) — single "latest" site for now.
- Any change to `docs/` content or the provider.

## 9. Acceptance criteria

- `pages/` (landing + css + js), `mkdocs.yml`, `requirements-docs.txt`, `scripts/build_pages.py` + `scripts/test_build_pages.py`, `.github/workflows/docs.yml` exist and are committed; `build/` gitignored.
- `python3 scripts/build_pages.py --check` passes; `mkdocs build --strict` succeeds locally with no warnings.
- Site matches the `mixpanel-headless` look (Material, Mixpanel purple/blue brand CSS, Inter/JetBrains Mono, same feature set); nav shows the 7 families under Resources and Data Sources; search works.
- The workflow gates on PRs (strict build, no deploy) and deploys on merge/dispatch; SHA-pinned; least-privilege permissions.
- README's primary docs link points to the Pages site.
- After the user enables Pages (source = Actions), the site is live at `https://mixpanel.github.io/terraform-provider-mixpanel/`.
- Claim discipline + no internal-codename leakage on the authored landing.
