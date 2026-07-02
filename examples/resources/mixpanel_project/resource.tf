# A project within an organization. Requires an organization-admin service
# account. `name` forces replacement and will destroy all project data.
resource "mixpanel_project" "new" {
  organization_id = 7654321
  name            = "my-new-project"

  # IMPORTANT: prevent_destroy protects against accidental deletion.
  # The Mixpanel API does not support project name updates - any name change
  # would destroy and recreate the project, losing all data.
  lifecycle {
    prevent_destroy = true
  }
}
