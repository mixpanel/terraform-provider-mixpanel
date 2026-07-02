resource "mixpanel_rollup_project" "all_regions" {
  org_id = 1234567
  name   = "All Regions Rollup"

  # IDs of the datasets to include in the rollup project.
  dataset_ids = [
    "dataset_us",
    "dataset_eu",
  ]

  # IMPORTANT: prevent_destroy protects against accidental deletion of rollup projects.
  lifecycle {
    prevent_destroy = true
  }
}
