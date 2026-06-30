# Create a lexicon tag for categorizing events and properties
resource "mixpanel_lexicon_tag" "acquisition" {
  project_id  = var.project_id
  name        = "acquisition"
  description = "User acquisition and onboarding events"
  color       = "#4CAF50"
}

# Create another tag for revenue tracking
resource "mixpanel_lexicon_tag" "revenue" {
  project_id  = var.project_id
  name        = "revenue"
  description = "Revenue and monetization events"
  color       = "#2196F3"
}

# Reference the tag in an event definition
resource "mixpanel_event_definition" "signup" {
  project_id  = var.project_id
  definitions = [{
    name        = "Sign Up"
    description = "User completes account creation"
    verified    = true
  }]
  # Note: To use tags with event definitions, you would reference the tag ID
  # tags = [mixpanel_lexicon_tag.acquisition.id]
}
