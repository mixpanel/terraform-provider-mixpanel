resource "mixpanel_dashboard" "board" {
  title       = "Product KPIs"
  description = "Managed by Terraform"

  # Optional: arrange/resize the board's cells in the dashboards PATCH write
  # format. Use the real row/cell ids from a prior refresh; report cells are
  # created automatically when a mixpanel_bookmark board report is attached
  # to this board.
  #
  # layout = jsonencode({
  #   rows = [{
  #     id     = "CVs5n7mF"
  #     height = 300
  #     cells  = [{ id = "RNTLz4Z4", width = 6 }]
  #   }]
  #   rows_order = ["CVs5n7mF"]
  # })
}
