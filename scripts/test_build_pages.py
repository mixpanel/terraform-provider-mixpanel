import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from build_pages import (
    strip_frontmatter, read_subcategory, convert_callouts, transform_page,
)


import build_pages


class BuildTests(unittest.TestCase):
    def test_build_and_check(self):
        build_pages.build()
        # landing + guides present
        self.assertTrue((build_pages.OUT / "index.md").exists())
        self.assertEqual(len(list((build_pages.OUT / "guides").glob("*.md"))), 6)
        # reference pages present; count EXCLUDES the generated index.md overview
        for kind in ("resources", "data-sources"):
            got = [p for p in (build_pages.OUT / kind).glob("*.md") if p.name != "index.md"]
            self.assertEqual(len(got), len(build_pages.reference_pages(kind)))
            self.assertTrue((build_pages.OUT / kind / "index.md").exists())
        # assets copied; SUMMARY generated
        self.assertTrue((build_pages.OUT / "stylesheets" / "mixpanel.css").exists())
        self.assertTrue((build_pages.OUT / "javascripts" / "copy-markdown.js").exists())
        summary = (build_pages.OUT / "SUMMARY.md").read_text()
        self.assertIn("resources/cohort.md", summary)
        self.assertIn("Analytics & Reporting", summary)
        self.assertIn("resources/index.md", summary)
        self.assertIn("data-sources/index.md", summary)
        # section-overview links a sibling page with a relative link
        res_index = (build_pages.OUT / "resources" / "index.md").read_text()
        self.assertIn("(cohort.md)", res_index)
        # a transformed reference page has no residual registry frontmatter
        cohort = (build_pages.OUT / "resources" / "cohort.md").read_text()
        self.assertFalse(cohort.startswith("---\n"))
        self.assertEqual(build_pages.check(), 0)


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

    def test_callout_multiline_continuation(self):
        src = (
            "~> **Danger.** line one\n"
            "line two continues\n"
            "line three ends\n"
            "\n"
            "Normal paragraph.\n"
        )
        out = build_pages.convert_callouts(src)
        lines = out.split("\n")
        self.assertEqual(lines[0], "!!! warning")
        self.assertEqual(lines[1], "    **Danger.** line one")
        self.assertEqual(lines[2], "    line two continues")
        self.assertEqual(lines[3], "    line three ends")
        self.assertEqual(lines[4], "")
        self.assertEqual(lines[5], "Normal paragraph.")


if __name__ == "__main__":
    unittest.main()
