# Clone dashboards with data source references across environments
#
# This example shows how to:
# 1. Look up existing custom properties by name (created manually or elsewhere)
# 2. Reference them in dashboard configurations using jsonencode
# 3. Deploy the same dashboard to multiple projects
#
# Usage:
#   terraform init
#   terraform plan
#   terraform apply

terraform {
  required_providers {
    mixpanel = {
      source = "mixpanel/mixpanel"
    }
  }
}

provider "mixpanel" {
  # Credentials from environment variables
}

locals {
  environments = {
    dev     = { project_id = 1234567 }
    staging = { project_id = 2345678 }
    prod    = { project_id = 3456789 }
  }
}

# Look up existing custom properties by name
# These must already exist in each project (created manually or in another stack)
data "mixpanel_custom_property" "user_tier" {
  for_each = local.environments

  project_id = each.value.project_id
  name       = "User Tier"  # Must be named consistently across projects
}

data "mixpanel_custom_property" "lifetime_value" {
  for_each = local.environments

  project_id = each.value.project_id
  name       = "Lifetime Value"
}

# Clone the power users cohort (from clone-cohorts.tf) or reference existing
resource "mixpanel_cohort" "power_users" {
  for_each = local.environments

  project_id  = each.value.project_id
  name        = "Power Users"
  description = "High-value engaged users"

  selector = jsonencode({
    and = [
      {
        # Reference the custom property ID (resolved per environment)
        property_id = data.mixpanel_custom_property.lifetime_value[each.key].id
        operator    = ">"
        value       = 1000
      },
      {
        property_id = data.mixpanel_custom_property.user_tier[each.key].id
        operator    = "=="
        value       = "premium"
      }
    ]
  })
}

# Clone an executive dashboard to all environments
resource "mixpanel_dashboard" "executive_overview" {
  for_each = local.environments

  project_id = each.value.project_id
  name       = "Executive Overview [${upper(each.key)}]"

  # Use jsonencode with Terraform references (not raw JSON strings)
  metadata = jsonencode({
    description = "Key metrics dashboard for ${each.key} environment"

    # Global filters
    filters = {
      cohort_id = mixpanel_cohort.power_users[each.key].id
    }

    # Dashboard charts
    charts = [
      {
        type  = "insights"
        title = "Users by Tier"
        query = {
          event = "Page View"
          breakdown = {
            # ID resolved automatically per environment
            property_id = data.mixpanel_custom_property.user_tier[each.key].id
          }
        }
      },
      {
        type  = "funnel"
        title = "Conversion Funnel"
        query = {
          steps = [
            { event = "Signup" },
            { event = "Activation" },
            { event = "Purchase" }
          ]
          filters = {
            cohort_id = mixpanel_cohort.power_users[each.key].id
          }
        }
      },
      {
        type  = "retention"
        title = "User Retention"
        query = {
          born_event   = "Signup"
          return_event = "Session Start"
          filters = {
            property_id = data.mixpanel_custom_property.user_tier[each.key].id
            operator    = "=="
            value       = "premium"
          }
        }
      }
    ]
  })
}

# Outputs
output "dashboard_ids" {
  description = "Dashboard IDs per environment"
  value = {
    for env, dashboard in mixpanel_dashboard.executive_overview :
    env => dashboard.id
  }
}

output "custom_property_ids" {
  description = "Custom property IDs per environment for debugging"
  value = {
    for env in keys(local.environments) :
    env => {
      user_tier      = data.mixpanel_custom_property.user_tier[env].id
      lifetime_value = data.mixpanel_custom_property.lifetime_value[env].id
    }
  }
}
