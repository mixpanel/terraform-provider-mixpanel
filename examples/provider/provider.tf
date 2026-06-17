terraform {
  required_providers {
    mixpanel = {
      source = "mixpanel/mixpanel"
    }
  }
}

# Credentials are read from MIXPANEL_SERVICE_ACCOUNT, MIXPANEL_SERVICE_ACCOUNT_SECRET,
# and MIXPANEL_PROJECT_ID when not set explicitly here. Never commit secrets.
provider "mixpanel" {
  # service_account        = "my-service-account"
  # service_account_secret = var.mixpanel_secret
  # project_id             = "1234567"
}

resource "mixpanel_annotation" "release" {
  date        = "2026-01-01 00:00:00"
  description = "v2.0 release"
}
