# GitHub Wiki (Phase 1 publishing surface) — Design

**Date:** 2026-07-09
**Status:** Approved (design presented and accepted in session)
**Depends on:** Phase 0 docs corpus (merged to `main`, commit `1bee58b`).

## 1. Purpose

Stand up the repository's GitHub Wiki as the **org-facing front door** for the provider: a curated narrative hub that communicates the value of infrastructure-as-code for product analytics to the broader Mixpanel org (PMs, leadership, the analytics-curious), and orients newcomers — while linking *into* the Terraform Registry for the exhaustive per-resource reference. The committed `docs/` corpus remains the single source of truth; the wiki is a **derived** surface.

## 2. Scope decisions (user-confirmed)

- **Curated narrative hub**, not a mirror. The 89 reference pages are NOT duplicated onto the wiki — they live (auto-synced per release) on the Terraform Registry, which the wiki links to.
- **Repeatable build script** sync model: a committed script transforms `docs/` + wiki-native templates into the wiki page tree and pushes to `.wiki.git`. Nothing is hand-edited directly in the wiki. CI automation is deferred.
- **Cut `v0.1.0-alpha7`** is a prerequisite so the Registry reflects Phase 0 (the wiki deep-links into it).

## 3. Context / ground truths

- The Registry hosts `mixpanel/mixpanel` (source = this repo). **Latest published version is `alpha6` (2026-07-02), which predates Phase 0** — so the live Registry shows the pre-Phase-0 docs (flat, ungrouped; only the `import` guide; old overview). Cutting a new release republishes the current corpus (7-family `subcategory` grouping, 6 guides, 15 new pages, import-ID fixes).
- Registry docs UX: left sidebar (**Overview → Guides → Resources → Data Sources**; Resources/Data Sources grouped by `subcategory`), title-only sidebar filter, main render pane, version dropdown showing **released** versions only. Deep-link shape: `https://registry.terraform.io/providers/mixpanel/mixpanel/latest/docs/<category>/<name>` where `<category>` ∈ `resources`, `data-sources`, `guides`. `latest` resolves to the newest published version (currently all prereleases; `latest` still resolves).
- The wiki feature is enabled (`has_wiki: true`) but **`.wiki.git` is not initialized** — cloning returns "Repository not found". GitHub creates the wiki repo only after the first page is created via the web UI; there is no API/`gh` command to create it. → one-time manual bootstrap (§7).
- Release mechanics: `main.version` is injected via ldflags from the git tag (`-X main.version={{.Version}}`); no version file or CHANGELOG to bump. `.goreleaser.yml` has `draft: false`, `prerelease: auto`. `.github/workflows/release.yml` triggers on `v*` tags and runs goreleaser with GPG signing (secrets already configured).
- The corpus's guides use relative links (`../guides/*.md`, `../resources/*.md`, `../data-sources/*.md`, `../index.md`, `./*.md`) and YAML frontmatter (`page_title`, `subcategory`, `description`) — all of which must be transformed for the wiki (§6).

## 4. Repository layout (new files)

```
wiki/                          # wiki-native SOURCE (authored, versioned in main repo)
  Home.md                      # landing/pitch
  _Sidebar.md                  # nav
  _Footer.md                   # provenance + registry pointer
  Reference.md                 # 7-family capability map → Registry
scripts/
  build_wiki.py                # pure transform: docs/guides + wiki/ -> build/wiki/
  publish_wiki.sh              # side effect: clone .wiki.git, sync build/wiki/, commit, push
build/wiki/                    # generated output (gitignored)
```

`.gitignore`: add `build/`.

## 5. Wiki page set (10 files, curated hub)

| Wiki page (file) | Title rendered | Source | Kind |
| --- | --- | --- | --- |
| `Home.md` | Home | `wiki/Home.md` | authored |
| `_Sidebar.md` | (nav) | `wiki/_Sidebar.md` | authored |
| `_Footer.md` | (footer) | `wiki/_Footer.md` | authored |
| `Reference.md` | Reference | `wiki/Reference.md` | authored |
| `Getting-Started.md` | Getting Started | `docs/guides/getting-started.md` | transformed |
| `Analytics-as-Code.md` | Analytics as Code | `docs/guides/analytics-as-code.md` | transformed |
| `Import.md` | Import | `docs/guides/import.md` | transformed |
| `Cross-Project-Portability.md` | Cross Project Portability | `docs/guides/cross-project-portability.md` | transformed |
| `Drift-Detection.md` | Drift Detection | `docs/guides/drift-detection.md` | transformed |
| `Sharing.md` | Sharing | `docs/guides/sharing.md` | transformed |

GitHub Wiki renders a file's page title from its filename (dashes → spaces). Links between pages use `[text](Page-Name)` (no `.md`, dashes for spaces).

### 5a. Authored page contents

- **Home.md** — one-line "what it is"; a 3–5 sentence "why" distilled from `analytics-as-code.md` (no hype; no first/only — same claim discipline as Phase 0); a prominent **New here → [Getting Started](Getting-Started)** and **Why → [Analytics as Code](Analytics-as-Code)**; a Guides list (the 4 how-tos); a **Full per-resource reference → Terraform Registry** CTA (link to `…/latest/docs`); a compact quickstart (the `mixpanel_annotation` provider-block + env-var-auth example, never-commit-secrets note) **and** a "full walkthrough → [Getting Started](Getting-Started)" link; an **alpha status** note linking the repo/Registry. HCL reuses the schema-verified `mixpanel_annotation` quickstart.
- **_Sidebar.md** — Home · Getting Started · Analytics as Code · **Guides** (Import, Cross Project Portability, Drift Detection, Sharing) · Reference · **Contributing** (→ `https://github.com/mixpanel/terraform-provider-mixpanel/blob/main/CONTRIBUTING.md`).
- **_Footer.md** — "This wiki is generated from `docs/` in the main repo — do not edit pages here; edit `docs/` and re-run `scripts/build_wiki.py`. Per-resource reference: [Terraform Registry](…/latest/docs)."
- **Reference.md** — intro that the exhaustive, versioned reference lives on the Registry, with a top-level **Browse all → [Terraform Registry docs](…/latest/docs)** link (Registry has no per-subcategory URL). Then the **7 families** (Analytics & Reporting; Governance & Lexicon; Experimentation & Delivery; Data Pipeline; Session Replay & Heatmaps; AI & Automation; Administration & Access), each as: a one-line description **plus 1–2 representative resources deep-linked** to `…/latest/docs/resources/<name>` (resource names verified to exist in `docs/resources/`). This gives every family at least one working deep-link while directing exhaustive lookups to the Registry root.

## 6. Transform rules (`docs/guides/*.md` → wiki)

Applied by `build_wiki.py` to each transformed guide:

1. **Strip YAML frontmatter** (the leading `---` … `---` block). Title derives from the wiki filename; the guide's H1 stays in the body.
2. **Rewrite internal links** (markdown `](target)`):
   - Guide → guide: `../guides/<slug>.md` or `./<slug>.md` → the wiki page name, via this exact map: `getting-started`→`Getting-Started`, `analytics-as-code`→`Analytics-as-Code`, `import`→`Import`, `cross-project-portability`→`Cross-Project-Portability`, `drift-detection`→`Drift-Detection`, `sharing`→`Sharing`. Preserve any `#anchor` suffix.
   - Resource/data-source: `../resources/<name>.md` → `https://registry.terraform.io/providers/mixpanel/mixpanel/latest/docs/resources/<name>`; `../data-sources/<name>.md` → `…/latest/docs/data-sources/<name>`.
   - Index: `../index.md` or `./index.md` → `Home`.
   - Anchors-only (`#…`), and absolute (`http(s)://…`) links: unchanged.
3. **Callouts**: convert leading Hashicorp callout markers `->`, `~>`, `!>` on a line to a `>` blockquote prefixed `**Note:** ` / `**Warning:** ` / `**Important:** ` respectively.
4. Leave code fences, tables, and inline formatting untouched.
5. Any relative `.md` link that does not match a known mapping is a **build error** (fail loudly rather than emit a broken wiki link).

## 7. Prerequisites (both one-time)

1. **Cut `v0.1.0-alpha7`** (refresh the Registry): `git tag -a v0.1.0-alpha7 -m "…"` on `main` HEAD, `git push origin v0.1.0-alpha7`. `release.yml` builds/signs/publishes a (pre)release; the Registry ingests it (minutes–hours). No version file to bump. **Outward-facing** — confirm with the user before pushing the tag. Sequenced first so the wiki's Registry deep-links resolve to Phase 0 content.
2. **Bootstrap the wiki repo** (user action, browser): repo → **Wiki** tab → *Create the first page* → Save any content. This initializes `.wiki.git` so the script can clone/push. No API exists for this; it cannot be scripted.

## 8. Build & publish flow

- `scripts/build_wiki.py`: reads `wiki/` (authored) + `docs/guides/` (transformed per §6), writes the 10-file tree to `build/wiki/`. Pure (no network, no push). Supports `--check` (build + run §9 verification, non-zero exit on failure).
- `scripts/publish_wiki.sh`: requires `build/wiki/` to exist; clones `https://github.com/mixpanel/terraform-provider-mixpanel.wiki.git` to a temp dir, copies the generated pages over its contents, `git add -A`, commits `wiki: sync from docs @ <main-sha>`, pushes. Idempotent (no-op commit when unchanged). Fails with a clear message if the wiki repo is not yet initialized (§7.2).

## 9. Verification

`build_wiki.py --check` asserts, over `build/wiki/`:
- No residual YAML frontmatter (`^---$` at file start) in any page.
- No residual relative `.md` links (`](…​.md)` / `](../…)` / `](./…)`) remain in any page.
- Every guide→guide wiki link points to a page that exists in the generated set.
- Every `_Sidebar.md` link resolves to a generated page or an absolute URL.
- All 10 files are present.
Plus a sampled network check (manual or in `--check` when online): a handful of Registry deep-links (e.g. `resources/cohort`, `data-sources/experiment`) return HTTP 200. HCL in `Home.md` is schema-verified per the Phase 0 protocol.

## 10. Out of scope (follow-ups)

- CI automation of the wiki sync (GitHub Action on `docs/` changes to `main`; needs a wiki-write token).
- Other publishing surfaces (GitHub Pages, Notion).
- A stable (non-alpha) release.
- Registry per-subcategory deep-linking (not supported by the Registry today).

## 11. Acceptance criteria

- `wiki/` (4 authored pages), `scripts/build_wiki.py`, `scripts/publish_wiki.sh` exist and are committed; `build/` gitignored.
- `build_wiki.py --check` passes: 10 pages generated, no frontmatter, no residual relative `.md` links, all intra-wiki + sidebar links resolve.
- `v0.1.0-alpha7` tagged & pushed; the GitHub release publishes; the Registry shows the Phase 0 corpus (7-family grouping, 6 guides) at `…/latest/docs` (confirm after ingest).
- After the user bootstraps the wiki repo, `publish_wiki.sh` pushes the 10 pages; the wiki Home renders with working nav (Sidebar/Footer) and the guides render with rewritten links; Reference and guide→Registry links resolve.
- No hand-edited pages in the wiki; re-running the build reproduces the tree.
- Claim discipline & no internal-codename leakage carried over from Phase 0 (authored pages included).
