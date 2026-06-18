# Import a playlist by "<project_id>:<id>".
# Config-driven import (Terraform 1.5+ / OpenTofu). Add a matching
# resource "mixpanel_playlist" "example" { ... } block, then `terraform plan`
# / `apply` to bring the existing object under management.
import {
  to = mixpanel_playlist.example
  id = "1234567:7654321"
}
