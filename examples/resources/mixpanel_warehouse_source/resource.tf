# An inbound warehouse source (Warehouse Connectors). Connection config is passed
# through `params` as JSON; its shape depends on `warehouse_type`
# (bigquery | snowflake | redshift | databricks | postgres).
resource "mixpanel_warehouse_source" "bq" {
  project_id     = 1234567
  source_name    = "prod-bigquery"
  warehouse_type = "bigquery"
  params         = jsonencode({ service_account_key = "{...}", dataset = "mixpanel_export" })
}
