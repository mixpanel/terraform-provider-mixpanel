# Import a heat_map by "<project_id>:<id>".
# Config-driven import (Terraform 1.5+ / OpenTofu). Add a matching
# resource "mixpanel_heat_map" "example" { ... } block, then `terraform plan`
# / `apply` to bring the existing object under management.
import {
  to = mixpanel_heat_map.example
  id = "1234567:7654321"
}
