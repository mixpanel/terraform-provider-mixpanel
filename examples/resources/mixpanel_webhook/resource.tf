resource "mixpanel_webhook" "example" {
  project_id = 1234567

  name = "Order events webhook"
  url  = "https://example.com/hooks/mixpanel"

  # Optional basic-auth credentials. auth_type defaults are managed by the API.
  auth_type = "basic"
  username  = "mixpanel"
  password  = "s3cr3t" # tfsec:ignore:general-secrets-no-plaintext-exposure example placeholder
}
