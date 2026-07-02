# Lookup table resource example
#
# Lookup tables enrich event data with additional attributes by mapping
# a join key to supplementary data from CSV files.

terraform {
  required_providers {
    mixpanel = {
      source = "mixpanel/mixpanel"
    }
  }
}

provider "mixpanel" {
  project_id = 1234567
}

# Example 1: Basic lookup table
resource "mixpanel_lookup_table" "user_attributes" {
  project_id  = 1234567
  name        = "User Attributes"
  description = "CRM attributes for user enrichment"

  # CSV file path (committed to repo or generated)
  csv_file = "${path.module}/data/user_attributes.csv"

  # Mark as ready for use in queries
  mark_ready = true
}

# Example 2: Lookup table with explicit data group
resource "mixpanel_data_group" "crm_data" {
  project_id = 1234567
  name       = "CRM Data"
}

resource "mixpanel_lookup_table" "accounts" {
  project_id    = 1234567
  name          = "Account Details"
  description   = "Company account metadata"
  data_group_id = mixpanel_data_group.crm_data.data_group_id

  csv_file   = "${path.module}/data/accounts.csv"
  mark_ready = true
}

# Example 3: Product catalog lookup
resource "mixpanel_lookup_table" "products" {
  project_id  = 1234567
  name        = "Product Catalog"
  description = "Product SKU to metadata mapping"

  csv_file   = "${path.module}/data/products.csv"
  mark_ready = true
}

# Example 4: Environment-specific deployment
locals {
  environments = {
    dev = {
      project_id = 1111111
      csv_file   = "dev_users.csv"
    }
    prod = {
      project_id = 3333333
      csv_file   = "prod_users.csv"
    }
  }
}

resource "mixpanel_lookup_table" "users_by_env" {
  for_each = local.environments

  project_id  = each.value.project_id
  name        = "User Attributes [${upper(each.key)}]"
  description = "User attributes for ${each.key} environment"

  csv_file   = "${path.module}/data/${each.value.csv_file}"
  mark_ready = true
}

# Outputs
output "user_attributes_id" {
  description = "Lookup table ID for user attributes"
  value       = mixpanel_lookup_table.user_attributes.id
}

output "upload_status" {
  description = "Upload status"
  value       = mixpanel_lookup_table.user_attributes.upload_status
}
