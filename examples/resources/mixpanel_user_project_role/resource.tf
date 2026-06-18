# Grant a user a role on a project. Requires an organization-admin service
# account. `key` is a stable association key you choose.
resource "mixpanel_user_project_role" "alice_analyst" {
  organization_id = 7654321
  key             = "alice@example.com:1234567:analyst"
  payload         = jsonencode({ users = ["alice@example.com"], projects = [1234567], role = "analyst" })
}
