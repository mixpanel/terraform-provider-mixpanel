# Board report bookmarks (type insights/retention/funnels/flows) require a
# dashboard: the provider creates them through the dashboards content API so
# the report actually appears on the board. `params` must be a valid report
# definition for the type (insights needs at least one sections.show clause
# and a displayOptions.chartType).
resource "mixpanel_dashboard" "board" {
  title = "Product KPIs"
}

resource "mixpanel_bookmark" "all_events" {
  name         = "All events over time"
  type         = "insights"
  dashboard_id = mixpanel_dashboard.board.id

  params = jsonencode({
    sections = {
      show = [{
        dataset      = "$mixpanel"
        value        = { name = "$all_events", resourceType = "events" }
        resourceType = "events"
        search       = ""
        math         = "total"
      }]
    }
    displayOptions = { chartType = "line" }
  })
}
