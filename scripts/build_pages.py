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
