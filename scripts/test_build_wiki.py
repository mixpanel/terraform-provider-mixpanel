import unittest
from build_wiki import (
    strip_frontmatter, rewrite_link, rewrite_links, convert_callouts, transform_guide,
)

R = "https://registry.terraform.io/providers/mixpanel/mixpanel/latest/docs"


class TransformTests(unittest.TestCase):
    def test_strip_frontmatter(self):
        src = "---\npage_title: x\nsubcategory: y\n---\n# H\n\nbody\n"
        self.assertEqual(strip_frontmatter(src), "# H\n\nbody\n")

    def test_strip_frontmatter_absent(self):
        self.assertEqual(strip_frontmatter("# H\n"), "# H\n")

    def test_link_guide_forms(self):
        self.assertEqual(rewrite_link("../guides/import.md"), "Import")
        self.assertEqual(rewrite_link("./import.md"), "Import")
        self.assertEqual(rewrite_link("./guides/import.md"), "Import")
        self.assertEqual(rewrite_link("./sharing.md#failure"), "Sharing#failure")

    def test_link_resource_and_datasource(self):
        self.assertEqual(rewrite_link("../resources/cohort.md"), f"{R}/resources/cohort")
        self.assertEqual(rewrite_link("../data-sources/tag.md"), f"{R}/data-sources/tag")

    def test_link_index_to_home(self):
        self.assertEqual(rewrite_link("../index.md"), "Home")

    def test_link_absolute_and_anchor_unchanged(self):
        self.assertEqual(rewrite_link("https://x.io/a"), "https://x.io/a")
        self.assertEqual(rewrite_link("#section"), "#section")

    def test_link_unmapped_raises(self):
        with self.assertRaises(ValueError):
            rewrite_link("../guides/does-not-exist.md")

    def test_rewrite_links_inline(self):
        self.assertEqual(
            rewrite_links("see [import](../guides/import.md) and [c](../resources/cohort.md)"),
            f"see [import](Import) and [c]({R}/resources/cohort)",
        )

    def test_callouts(self):
        self.assertEqual(convert_callouts("-> tip"), "> **Note:** tip")
        self.assertEqual(convert_callouts("~> careful"), "> **Warning:** careful")
        self.assertEqual(convert_callouts("!> danger"), "> **Important:** danger")

    def test_callouts_skip_code_fence(self):
        src = "```hcl\n-> not a callout\n```"
        self.assertEqual(convert_callouts(src), src)

    def test_transform_pipeline(self):
        src = "---\na: b\n---\n-> see [x](../guides/sharing.md)\n"
        out = transform_guide(src)
        self.assertNotIn("---", out)
        self.assertIn("> **Note:** see [x](Sharing)", out)


if __name__ == "__main__":
    unittest.main()
