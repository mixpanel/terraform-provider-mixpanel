# Example usage of the project configuration module
#
# This demonstrates how to use the module to deploy the same Mixpanel
# configuration across dev, staging, and production projects.
#
# Directory structure:
#   clone-project-module/
#     ├── main.tf          (the module)
#     └── usage.tf         (this file - shows how to use it)
#
# Usage:
#   cd clone-project-module
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

# Deploy to dev
module "mixpanel_dev" {
  source = "."  # Use the current directory as module source

  project_id      = 1234567  # Replace with your dev project ID
  environment     = "dev"
  alert_threshold = 20       # Higher threshold in dev (less sensitive)
}

# Deploy to staging
module "mixpanel_staging" {
  source = "."

  project_id      = 2345678  # Replace with your staging project ID
  environment     = "staging"
  alert_threshold = 10
}

# Deploy to production
module "mixpanel_prod" {
  source = "."

  project_id      = 3456789  # Replace with your prod project ID
  environment     = "prod"
  alert_threshold = 5        # Lower threshold in prod (more sensitive)
}

# Outputs aggregated across all environments
output "all_environments" {
  description = "Summary of all deployed environments"
  value = {
    dev = {
      project_id = module.mixpanel_dev.project_id
      cohorts    = module.mixpanel_dev.cohort_ids
      dashboard  = module.mixpanel_dev.dashboard_id
    }
    staging = {
      project_id = module.mixpanel_staging.project_id
      cohorts    = module.mixpanel_staging.cohort_ids
      dashboard  = module.mixpanel_staging.dashboard_id
    }
    prod = {
      project_id = module.mixpanel_prod.project_id
      cohorts    = module.mixpanel_prod.cohort_ids
      dashboard  = module.mixpanel_prod.dashboard_id
    }
  }
}

# Example: Create a cross-environment comparison dashboard in prod
resource "mixpanel_dashboard" "cross_env_comparison" {
  project_id = module.mixpanel_prod.project_id
  name       = "Cross-Environment Comparison"

  metadata = jsonencode({
    description = "Compare metrics across dev, staging, and prod"

    # This dashboard exists in prod but references cohorts from its own project
    charts = [
      {
        type  = "insights"
        title = "High-Value User Count"
        query = {
          event = "Session Start"
          filters = {
            cohort_id = module.mixpanel_prod.cohort_ids.high_value
          }
        }
      }
    ]
  })
}
