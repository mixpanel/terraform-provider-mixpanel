# A behavioral metric. `definition` is a Mixpanel metric definition document
# (BehaviorMetricDefinition) passed through verbatim as JSON.
resource "mixpanel_metric" "signups" {
  project_id = 1234567
  name       = "Signups"
  type       = "metric"
  definition = jsonencode({
    measurement = { /* aggregation over a behavior */ }
    behavior    = { /* the behavior to measure */ }
  })
}
