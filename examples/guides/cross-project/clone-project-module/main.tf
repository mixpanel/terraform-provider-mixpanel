# Complete project configuration module
#
# This module encapsulates a full Mixpanel project setup including:
# - Custom properties
# - Cohorts with cross-references
# - Dashboards
# - Alerts
#
# Use this module to deploy identical configurations across environments.

terraform {
  required_providers {
    mixpanel = {
      source = "mixpanel/mixpanel"
    }
  }
}

variable "project_id" {
  description = "Mixpanel project ID"
  type        = number
}

variable "environment" {
  description = "Environment name (dev, staging, prod)"
  type        = string

  validation {
    condition     = contains(["dev", "staging", "prod"], var.environment)
    error_message = "Environment must be dev, staging, or prod"
  }
}

variable "alert_threshold" {
  description = "Alert threshold for error rate"
  type        = number
  default     = 5
}

# Custom properties
resource "mixpanel_custom_property" "properties" {
  for_each = {
    user_tier = {
      display_name = "User Tier"
      description  = "Customer tier level"
    }
    lifetime_value = {
      display_name = "Lifetime Value"
      description  = "Total customer lifetime value"
    }
    last_active = {
      display_name = "Last Active"
      description  = "Last activity timestamp"
    }
    feature_flags = {
      display_name = "Feature Flags"
      description  = "Active feature flags for user"
    }
  }

  project_id   = var.project_id
  name         = each.key
  display_name = each.value.display_name
  description  = each.value.description

  # Make computed properties visible in UI
  is_visible = true
}

# Data group for lookup tables
resource "mixpanel_data_group" "user_attributes" {
  project_id    = var.project_id
  display_name  = "User Attributes"
  property_name = "user_attribute_id"

  # Optional metadata
  metadata = {
    description = "Additional user attributes from CRM"
  }
}

# Cohorts with dependencies
resource "mixpanel_cohort" "high_value_users" {
  project_id  = var.project_id
  name        = "High-Value Users"
  description = "Users with LTV > 1000"
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
          # Reference the custom property ID (auto-resolved)
          property_id = mixpanel_custom_property.properties["lifetime_value"].id
          operator    = ">"
          value       = 1000
        }
      ]
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])

  # Explicit dependency (usually inferred, but makes intent clear)
  depends_on = [mixpanel_custom_property.properties]
}

resource "mixpanel_cohort" "premium_users" {
  project_id  = var.project_id
  name        = "Premium Users"
  description = "Users on premium tier"
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
          property_id = mixpanel_custom_property.properties["user_tier"].id
          operator    = "=="
          value       = "premium"
        }
      ]
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])
}

resource "mixpanel_cohort" "at_risk_users" {
  project_id  = var.project_id
  name        = "At-Risk Users"
  description = "High-value users who haven't been active recently"
  is_visible  = true

  groups = jsonencode([
    {
      event = {
        resourceType = "cohort"
        value        = mixpanel_cohort.high_value_users.id
        label        = "High-Value Users"
      }
      filters = [
        {
          property_id = mixpanel_custom_property.properties["last_active"].id
          operator    = "not_within"
          value       = "30d"
        }
      ]
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])

  # Explicit dependencies
  depends_on = [
    mixpanel_cohort.high_value_users,
    mixpanel_custom_property.properties
  ]
}

# Dashboards
# Note: Dashboard content (report cells) is managed via mixpanel_bookmark
# resources rather than inline metadata
resource "mixpanel_dashboard" "overview" {
  project_id  = var.project_id
  title       = "Overview Dashboard [${upper(var.environment)}]"
  description = "Main analytics dashboard for ${var.environment}"
}

# Custom alert
resource "mixpanel_custom_alert" "error_rate" {
  project_id = var.project_id
  name       = "High Error Rate [${var.environment}]"

  condition = jsonencode({
    type      = "threshold"
    metric    = "error_count"
    operator  = ">"
    threshold = var.alert_threshold
    window    = "1h"
  })

  subscriptions = jsonencode([
    {
      type    = "email"
      target  = "oncall@example.com"
      enabled = var.environment == "prod"  # Only alert in prod
    }
  ])
}

# Outputs
output "project_id" {
  description = "Project ID"
  value       = var.project_id
}

output "cohort_ids" {
  description = "Created cohort IDs"
  value = {
    high_value = mixpanel_cohort.high_value_users.id
    premium    = mixpanel_cohort.premium_users.id
    at_risk    = mixpanel_cohort.at_risk_users.id
  }
}

output "custom_property_ids" {
  description = "Created custom property IDs"
  value = {
    for name, prop in mixpanel_custom_property.properties :
    name => prop.id
  }
}

output "dashboard_id" {
  description = "Overview dashboard ID"
  value       = mixpanel_dashboard.overview.id
}
