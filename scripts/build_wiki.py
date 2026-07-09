#!/usr/bin/env python3
"""Build the GitHub Wiki page tree from docs/ (source of truth) + wiki/ templates.

Stdlib only (matches gen/). Pure/offline transform; writes build/wiki/.
Run with --check to verify the generated tree.
"""
import argparse
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
DOCS_GUIDES = REPO / "docs" / "guides"
WIKI_SRC = REPO / "wiki"
OUT = REPO / "build" / "wiki"

REGISTRY_BASE = "https://registry.terraform.io/providers/mixpanel/mixpanel/latest/docs"

# guide slug (docs/guides/<slug>.md) -> wiki page name
GUIDE_PAGES = {
    "getting-started": "Getting-Started",
    "analytics-as-code": "Analytics-as-Code",
    "import": "Import",
    "cross-project-portability": "Cross-Project-Portability",
    "drift-detection": "Drift-Detection",
    "sharing": "Sharing",
}
AUTHORED = ["Home", "_Sidebar", "_Footer", "Reference"]
CALLOUTS = {"->": "Note", "~>": "Warning", "!>": "Important"}

FRONTMATTER_RE = re.compile(r"\A---\n.*?\n---\n", re.DOTALL)
LINK_RE = re.compile(r"\]\(([^)]+)\)")


def strip_frontmatter(text: str) -> str:
    return FRONTMATTER_RE.sub("", text, count=1)


def rewrite_link(target: str) -> str:
    path, sep, anchor = target.partition("#")
    if path == "":
        return target  # pure in-page anchor
    if path.startswith(("http://", "https://")):
        return target
    if not path.endswith(".md"):
        return target
    slug = path.rsplit("/", 1)[-1][:-3]
    tail = ("#" + anchor) if sep else ""
    if "/resources/" in path:
        return f"{REGISTRY_BASE}/resources/{slug}{tail}"
    if "/data-sources/" in path:
        return f"{REGISTRY_BASE}/data-sources/{slug}{tail}"
    if slug == "index":
        return "Home" + tail
    if slug in GUIDE_PAGES:
        return GUIDE_PAGES[slug] + tail
    raise ValueError(f"unmapped .md link: {target!r}")


def rewrite_links(text: str) -> str:
    return LINK_RE.sub(lambda m: "](" + rewrite_link(m.group(1)) + ")", text)


def convert_callouts(text: str) -> str:
    out, in_fence = [], False
    for line in text.split("\n"):
        stripped = line.lstrip()
        if stripped.startswith("```"):
            in_fence = not in_fence
            out.append(line)
            continue
        if not in_fence:
            for marker, label in CALLOUTS.items():
                if stripped.startswith(marker + " "):
                    line = f"> **{label}:** " + stripped[len(marker) + 1:]
                    break
        out.append(line)
    return "\n".join(out)


def transform_guide(text: str) -> str:
    return rewrite_links(convert_callouts(strip_frontmatter(text)))


EXPECTED_PAGES = set(AUTHORED) | set(GUIDE_PAGES.values())


def build() -> None:
    OUT.mkdir(parents=True, exist_ok=True)
    for stale in OUT.glob("*.md"):
        stale.unlink()
    for name in AUTHORED:
        (OUT / f"{name}.md").write_text((WIKI_SRC / f"{name}.md").read_text())
    for slug, page in GUIDE_PAGES.items():
        (OUT / f"{page}.md").write_text(transform_guide((DOCS_GUIDES / f"{slug}.md").read_text()))


def check() -> int:
    errors = []
    pages = {p.stem for p in OUT.glob("*.md")}
    missing = EXPECTED_PAGES - pages
    if missing:
        errors.append(f"missing pages: {sorted(missing)}")
    for page in sorted(OUT.glob("*.md")):
        text = page.read_text()
        if text.startswith("---\n"):
            errors.append(f"{page.name}: residual frontmatter")
        for m in LINK_RE.finditer(text):
            target = m.group(1)
            path = target.split("#", 1)[0]
            if not path or "://" in path:
                continue  # in-page anchor or absolute URL — fine
            if path.endswith(".md") or path.startswith(("../", "./")):
                errors.append(f"{page.name}: residual relative link {target!r}")
            elif "/" not in path and path not in EXPECTED_PAGES:
                errors.append(f"{page.name}: dangling wiki link {target!r}")
    if errors:
        print("CHECK FAILED:")
        for e in errors:
            print("  -", e)
        return 1
    print(f"CHECK OK: {len(pages)} pages, all links resolve")
    return 0


def main() -> None:
    parser = argparse.ArgumentParser(description="Build the GitHub Wiki tree from docs/.")
    parser.add_argument("--check", action="store_true", help="verify the generated tree")
    args = parser.parse_args()
    build()
    print(f"built {len(list(OUT.glob('*.md')))} pages -> {OUT.relative_to(REPO)}")
    if args.check:
        sys.exit(check())


if __name__ == "__main__":
    main()
