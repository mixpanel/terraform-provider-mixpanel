# Import the singleton business_context for a project by the project id.
# Config-driven import (Terraform 1.5+ / OpenTofu). Add a matching
# resource "mixpanel_business_context" "example" { ... } block, then `terraform plan`
# / `apply` to bring the existing object under management.
import {
  to = mixpanel_business_context.example
  id = "1234567"
}
