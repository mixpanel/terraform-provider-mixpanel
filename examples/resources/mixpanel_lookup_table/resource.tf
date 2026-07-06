# Lookup table resource examples.
#
# Lookup tables enrich Mixpanel data by mapping a join key (the first CSV
# column) to supplementary attributes. The provider drives the full
# signed-URL upload handshake (upload-url -> storage PUT -> register ->
# status polling) automatically from `csv_content`.

# Example 1: CSV sourced from a file. Changing the file re-uploads the
# table's rows in place (same table id).
resource "mixpanel_lookup_table" "accounts" {
  project_id  = var.project_id
  name        = "Account Attributes"
  description = "CRM attributes keyed by account id"

  csv_content = file("${path.module}/data/accounts.csv")
}

# Example 2: inline CSV content.
resource "mixpanel_lookup_table" "plans" {
  project_id = var.project_id
  name       = "Plan Metadata"

  csv_content = <<-CSV
    plan_id,tier,monthly_price
    p1,free,0
    p2,pro,49
    p3,enterprise,499
  CSV
}

output "accounts_table_id" {
  description = "The dimension data-group id backing the lookup table (int64 as string)"
  value       = mixpanel_lookup_table.accounts.id
}
