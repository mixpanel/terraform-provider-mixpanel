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
