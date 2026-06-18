resource "mixpanel_theme" "brand" {
  project_id = 1234567

  name = "Brand Theme"
  type = "dashboard"

  # One of: "off", "viewer", "editor".
  global_access_type = "viewer"
}
