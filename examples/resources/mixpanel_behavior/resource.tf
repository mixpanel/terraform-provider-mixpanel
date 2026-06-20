# A saved behavior. `definition` is a Mixpanel behavior document passed through
# verbatim as JSON.
resource "mixpanel_behavior" "power_users" {
  project_id  = 1234567
  name        = "Power users"
  description = "Users who performed a key action 5+ times"
  definition  = jsonencode({ behavior = {} })
}
