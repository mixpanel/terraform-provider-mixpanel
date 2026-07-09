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
