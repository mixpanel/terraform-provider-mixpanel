# A project within an organization. Requires an organization-admin service
# account.
#
# `name` is updated IN PLACE (the provider calls the same rename endpoint the
# Mixpanel webapp uses), so changing it in config renames the project without
# touching its data.
resource "mixpanel_project" "new" {
  organization_id = 7654321
  name            = "my-new-project"

  # IMPORTANT: destroying this resource DELETES the Mixpanel project and ALL
  # of its analytics data. prevent_destroy protects against accidental
  # deletion (e.g. removing the block, or changing the ForceNew
  # organization_id, which plans a destroy+recreate).
  lifecycle {
    prevent_destroy = true
  }
}
