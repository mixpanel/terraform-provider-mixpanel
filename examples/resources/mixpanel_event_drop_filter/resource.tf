resource "mixpanel_event_drop_filter" "drop_debug" {
  project_id = 1234567

  event_name = "debug_ping"
  active     = true
}
