# Import the singleton org_request_access_settings for a project by the project id.
# Config-driven import (Terraform 1.5+ / OpenTofu). Add a matching
# resource "mixpanel_org_request_access_settings" "example" { ... } block, then `terraform plan`
# / `apply` to bring the existing object under management.
import {
  to = mixpanel_org_request_access_settings.example
  id = "1234567"
}
