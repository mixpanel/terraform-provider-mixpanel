# A project within an organization. Requires an organization-admin service
# account. `name` forces replacement.
resource "mixpanel_project" "new" {
  organization_id = 7654321
  name            = "my-new-project"
}
