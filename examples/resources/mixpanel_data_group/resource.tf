resource "mixpanel_data_group" "accounts" {
  project_id = 1234567

  display_name  = "Accounts"
  property_name = "account_id"

  metadata = {
    is_company_key      = true
    company_key_name    = "account_id"
    company_revenue_key = "mrr"
  }
}
