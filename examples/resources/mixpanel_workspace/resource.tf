resource "mixpanel_workspace" "marketing" {
  # project_id is computed/assigned from the provider default for this resource.
  name        = "Marketing"
  description = "Workspace for the marketing analytics team"

  # Whether membership is restricted and whether the workspace is visible.
  is_restricted = true
  is_visible    = true
}
