# An organization team. Requires an organization-admin service account.
# `key` is the team name as it appears in the org teams list.
resource "mixpanel_team" "analysts" {
  organization_id = 7654321
  key             = "Analysts"
  payload         = jsonencode({ teams = ["Analysts"] })
}
