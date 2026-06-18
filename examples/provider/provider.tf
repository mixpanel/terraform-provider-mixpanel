terraform {
  required_providers {
    mixpanel = {
      source = "mixpanel/mixpanel"
    }
  }
}

provider "mixpanel" {
  # Credentials may also be supplied via the MIXPANEL_SERVICE_ACCOUNT,
  # MIXPANEL_SERVICE_ACCOUNT_SECRET, MIXPANEL_PROJECT_ID, and
  # MIXPANEL_ORGANIZATION_ID environment variables.
  service_account        = var.mixpanel_service_account
  service_account_secret = var.mixpanel_service_account_secret

  # Default project for project-scoped resources. Placeholder ID only.
  project_id = "1234567"
}
