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


def reference_pages(kind: str):
    return sorted((DOCS / kind).glob("*.md"))


SECTION_INTRO = {
    "resources": (
        "Every Mixpanel object this provider can manage, grouped by area. "
        "Each page documents the resource's schema, an example configuration, "
        "and its import syntax."
    ),
    "data-sources": (
        "Read-only Mixpanel objects you can reference in configurations, grouped "
        "by area. Each page documents the data source's arguments and attributes."
    ),
}


def section_index(kind: str, heading: str) -> str:
    lines = [f"# {heading}", "", SECTION_INTRO[kind], ""]
    by_family = {fam: [] for fam in FAMILY_ORDER}
    for p in reference_pages(kind):
        by_family.setdefault(read_subcategory(p.read_text()), []).append(p)
    for fam in FAMILY_ORDER:
        pages = by_family.get(fam) or []
        if not pages:
            continue
        lines.append(f"## {fam}")
        lines.append("")
        for p in sorted(pages, key=lambda x: x.stem):
            lines.append(f"- [mixpanel_{p.stem}]({p.name})")
        lines.append("")
    return "\n".join(lines).rstrip() + "\n"


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
    for kind, heading in (("resources", "Resources"), ("data-sources", "Data Sources")):
        (OUT / kind).mkdir()
        for p in reference_pages(kind):
            (OUT / kind / p.name).write_text(transform_page(p.read_text()))
        (OUT / kind / "index.md").write_text(section_index(kind, heading))
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
        lines.append(f"    * [Overview]({kind}/index.md)")
        by_family = {fam: [] for fam in FAMILY_ORDER}
        for p in reference_pages(kind):
            by_family.setdefault(read_subcategory(p.read_text()), []).append(p)
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
        got = len([p for p in (OUT / kind).glob("*.md") if p.name != "index.md"])
        want = len(reference_pages(kind))
        if got != want:
            errors.append(f"{kind}: {got} != {want}")
        if not (OUT / kind / "index.md").exists():
            errors.append(f"missing {kind}/index.md")
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
        if f"{kind}/index.md" not in summary:
            errors.append(f"{kind}/index.md missing from SUMMARY.md")
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
