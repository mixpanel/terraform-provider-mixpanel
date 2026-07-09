# Clone cohorts across multiple Mixpanel projects
#
# This example demonstrates the for_each pattern for deploying the same cohort
# definitions to dev, staging, and production projects.
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
  # Credentials from environment variables:
  # MIXPANEL_SERVICE_ACCOUNT, MIXPANEL_SERVICE_ACCOUNT_SECRET
}

# Define your environments
locals {
  environments = {
    dev = {
      project_id              = 1234567  # Replace with your dev project ID
      cohort_size_threshold   = 100
      engagement_days         = 7
    }
    staging = {
      project_id              = 2345678  # Replace with your staging project ID
      cohort_size_threshold   = 500
      engagement_days         = 14
    }
    prod = {
      project_id              = 3456789  # Replace with your prod project ID
      cohort_size_threshold   = 1000
      engagement_days         = 30
    }
  }
}

# Create a "Power Users" cohort in each environment
resource "mixpanel_cohort" "power_users" {
  for_each = local.environments

  project_id  = each.value.project_id
  name        = "Power Users [${upper(each.key)}]"
  description = "Users who meet the power user criteria for ${each.key}"
  is_visible  = true

  # Environment-specific thresholds
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
          operator = ">="
          value    = each.value.cohort_size_threshold
        },
        {
          property = "last_seen"
          operator = "within"
          value    = "${each.value.engagement_days}d"
        }
      ]
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])
}

# Create a "Recent Signups" cohort in each environment
resource "mixpanel_cohort" "recent_signups" {
  for_each = local.environments

  project_id  = each.value.project_id
  name        = "Recent Signups [${upper(each.key)}]"
  description = "Users who signed up in the last ${each.value.engagement_days} days"
  is_visible  = true

  groups = jsonencode([
    {
      event = {
        resourceType = "cohort"
        value        = "$all_users"
        label        = "All Users"
      }
      filters = [
        {
          property = "signup_date"
          operator = "within"
          value    = "${each.value.engagement_days}d"
        }
      ]
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])
}

# Output cohort IDs for verification
output "power_users_cohort_ids" {
  description = "Map of environment to cohort ID"
  value = {
    for env, cohort in mixpanel_cohort.power_users :
    env => cohort.id
  }
}

output "recent_signups_cohort_ids" {
  description = "Map of environment to cohort ID"
  value = {
    for env, cohort in mixpanel_cohort.recent_signups :
    env => cohort.id
  }
}
