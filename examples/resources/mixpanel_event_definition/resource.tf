resource "mixpanel_event_definition" "purchase" {
  project_id = 1234567

  # One or more underlying event definitions to group under this entry.
  definitions = [
    {
      name        = "Purchase"
      description = "Fires when a customer completes a checkout"
      verified    = true
    },
  ]
}
