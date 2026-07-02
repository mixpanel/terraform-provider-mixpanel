# Property definition (Lexicon metadata) examples.
#
# One resource instance = one property key's Lexicon metadata in one project.
# The API is an upsert: creating and updating are the same PATCH; deleting
# removes only the metadata row, never the ingested data.

# Example 1: document an event property.
resource "mixpanel_property_definition" "plan_type" {
  project_id = var.project_id

  name         = "plan_type"
  display_name = "Plan Type"
  description  = "The subscription plan of the account at event time"
  type         = "string"
}

# Example 2: hide a deprecated property from pickers.
resource "mixpanel_property_definition" "legacy_account_id" {
  project_id = var.project_id

  name        = "legacy_account_id"
  description = "Deprecated 2025-03; use account_id instead"
  hidden      = true
}

# Example 3: classify a sensitive user-profile property.
resource "mixpanel_property_definition" "email" {
  project_id = var.project_id

  name          = "$email"
  resource_type = "User"
  sensitive     = true
}

# Example 4: govern a whole schema from one map.
locals {
  event_properties = {
    "account_id" = { display_name = "Account ID", type = "string", description = "Stable account identifier" }
    "mrr_cents"  = { display_name = "MRR (cents)", type = "number", description = "Monthly recurring revenue in cents" }
    "is_trial"   = { display_name = "Is Trial", type = "boolean", description = "Whether the account is in trial" }
  }
}

resource "mixpanel_property_definition" "governed" {
  for_each = local.event_properties

  project_id   = var.project_id
  name         = each.key
  display_name = each.value.display_name
  description  = each.value.description
  type         = each.value.type
}

# Reading back a definition (e.g. one owned by another team):
data "mixpanel_property_definition" "city" {
  project_id = var.project_id
  name       = "$city"
}

output "city_is_defined" {
  value = data.mixpanel_property_definition.city.exists
}
