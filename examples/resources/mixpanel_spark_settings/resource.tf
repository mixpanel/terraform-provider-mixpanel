# Organization-scoped settings. Requires an organization-admin service account.
resource "mixpanel_spark_settings" "org" {
  organization_id = 7654321
  settings        = jsonencode({})
}
