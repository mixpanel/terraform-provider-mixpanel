# Import a rollup_project by its id (this resource is not project-scoped).
# Config-driven import (Terraform 1.5+ / OpenTofu). Add a matching
# resource "mixpanel_rollup_project" "example" { ... } block, then `terraform plan`
# / `apply` to bring the existing object under management.
import {
  to = mixpanel_rollup_project.example
  id = "7654321"
}
