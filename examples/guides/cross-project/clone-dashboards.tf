# Clone dashboards across environments
#
# This example shows how to:
# 1. Create cohorts with environment-specific configuration
# 2. Create dashboards that reference those cohorts
# 3. Deploy the same configuration to multiple projects
#
# Note: Dashboard content (report cells) is managed via mixpanel_bookmark
# resources rather than inline metadata.
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

# Create a power users cohort in each environment
resource "mixpanel_cohort" "power_users" {
  for_each = local.environments

  project_id  = each.value.project_id
  name        = "Power Users"
  description = "High-value engaged users"

  groups = jsonencode([
    {
      event = {
        resourceType = "cohort"
        value        = "$all_users"
        label        = "All Users"
      }
      filters = [
        {
          property = "session_count"
          operator = ">"
          value    = 10
        }
      ]
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])
}

# Clone an executive dashboard to all environments
# Dashboard content is managed via mixpanel_bookmark resources, not inline
resource "mixpanel_dashboard" "executive_overview" {
  for_each = local.environments

  project_id  = each.value.project_id
  title       = "Executive Overview [${upper(each.key)}]"
  description = "Key metrics dashboard for ${each.key} environment"
}

# Outputs
output "dashboard_ids" {
  description = "Dashboard IDs per environment"
  value = {
    for env, dashboard in mixpanel_dashboard.executive_overview :
    env => dashboard.id
  }
}

output "cohort_ids" {
  description = "Cohort IDs per environment"
  value = {
    for env, cohort in mixpanel_cohort.power_users :
    env => cohort.id
  }
}
