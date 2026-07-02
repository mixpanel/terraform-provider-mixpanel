---
page_title: "mixpanel_lookup_table Resource - mixpanel"
subcategory: ""
description: |-
  Manages a Mixpanel lookup table for enriching event data with additional attributes.
---

# mixpanel_lookup_table (Resource)

Lookup tables allow you to enrich your Mixpanel event data with additional
attributes by mapping a key column to supplementary data. For example, you can
map user IDs to account attributes from your CRM, or product SKUs to product
metadata.

Lookup tables are uploaded as CSV files and stored in a data group.

## Upload Workflow

The lookup table resource implements an **asynchronous upload workflow**:

1. **Create**: POST to `/lookup-tables` creates the table metadata and returns an `uploadId`
2. **Get upload URL**: Call `/upload-url?uploadId=X` to obtain a signed upload URL
3. **Upload CSV**: Upload your CSV file to the signed URL (S3/GCS)
4. **Poll status**: Poll `/upload-status?uploadId=X` until the upload completes
5. **Mark ready**: Optionally set `mark-ready: true` to make the table visible

This workflow is handled automatically by the provider. You provide the CSV
file path, and the provider manages the upload handshake.

## Example Usage

### Basic lookup table

```hcl
resource "mixpanel_lookup_table" "user_attributes" {
  project_id  = 1234567
  name        = "User Attributes"
  description = "Additional user attributes from CRM"

  # Path to CSV file (local or remote)
  csv_file = "${path.module}/data/user_attributes.csv"

  # Mark as ready to use in queries
  mark_ready = true
}
```

### CSV file format

The CSV file should have a header row with column names. The first column is
typically the join key (e.g., `user_id`).

**Example `user_attributes.csv`:**
```csv
user_id,account_tier,lifetime_value,signup_date
user_123,premium,5000,2024-01-15
user_456,free,0,2024-02-01
user_789,enterprise,25000,2023-11-20
```

### With data group reference

Lookup tables are stored in data groups. You can create the data group
explicitly and reference it:

```hcl
resource "mixpanel_data_group" "crm_data" {
  project_id = var.project_id
  name       = "CRM Data"
}

resource "mixpanel_lookup_table" "accounts" {
  project_id = var.project_id
  name       = "Account Details"

  # Reference the data group
  data_group_id = mixpanel_data_group.crm_data.data_group_id

  csv_file   = "${path.module}/data/accounts.csv"
  mark_ready = true
}
```

### Updating a lookup table

To update the data in a lookup table, modify the CSV file and re-apply:

```hcl
resource "mixpanel_lookup_table" "products" {
  project_id = var.project_id
  name       = "Product Catalog"

  # Terraform detects file changes via checksum
  csv_file = "${path.module}/data/products_v2.csv"

  mark_ready = true
}
```

When you change `csv_file`, Terraform triggers a new upload via the update
operation.

### Environment-specific lookup tables

Deploy different lookup tables to dev/staging/prod:

```hcl
locals {
  environments = {
    dev  = { project_id = 1111111, csv = "dev_users.csv" }
    prod = { project_id = 3333333, csv = "prod_users.csv" }
  }
}

resource "mixpanel_lookup_table" "users" {
  for_each = local.environments

  project_id = each.value.project_id
  name       = "User Attributes [${upper(each.key)}]"

  csv_file   = "${path.module}/data/${each.value.csv}"
  mark_ready = true
}
```

## Schema

### Required

- `project_id` (Number) - Mixpanel project ID
- `name` (String) - Lookup table name

### Optional

- `data_group_id` (String/Number) - Data group ID to store the table in. If
  omitted, a new data group is created automatically.
- `description` (String) - Human-readable description
- `csv_file` (String) - Path to CSV file to upload. Can be a local file path
  or a remote URL. Changes to this file trigger a re-upload.
- `mark_ready` (Boolean) - Whether to mark the table as ready for use in
  queries after upload completes. Defaults to `true`.

### Read-Only

- `id` (String) - The lookup table ID (same as data group ID)
- `entity_id` (Number) - Internal entity ID
- `upload_status` (String) - Current upload status (`pending`, `processing`, `complete`, `failed`)

## Import

Lookup tables can be imported using the project-scoped import ID format:

```bash
terraform import mixpanel_lookup_table.example 1234567:table_abc123
```

Format: `PROJECT_ID:LOOKUP_TABLE_ID`

## Implementation Notes

### Async Upload Handshake

The provider implements the Mixpanel lookup table upload workflow:

1. **Create** (`POST /lookup-tables`):
   ```json
   {
     "name": "User Attributes",
     "mark-ready": false
   }
   ```
   Response: `{"uploadId": "abc123", "id": "42"}`

2. **Get Upload URL** (`GET /upload-url?uploadId=abc123`):
   Response: `{"url": "https://storage.googleapis.com/...", "method": "PUT"}`

3. **Upload CSV** (`PUT https://storage.googleapis.com/...`):
   Upload the CSV file to the signed URL

4. **Poll Status** (`GET /upload-status?uploadId=abc123`):
   ```json
   {
     "uploadStatus": "processing",
     "result": null
   }
   ```
   Poll until `uploadStatus == "complete"`

5. **Mark Ready** (optional, `PATCH /lookup-tables`):
   ```json
   {
     "data-group-id": "42",
     "mark-ready": true
   }
   ```

The provider handles this automatically. From the user's perspective, you just
specify `csv_file` and the upload happens transparently.

### File Change Detection

The provider uses file checksums to detect changes to the CSV file. When the
checksum changes, Terraform triggers an update operation that re-uploads the
file via the same async workflow.

### Retry Logic

Upload operations include retry logic with exponential backoff for:
- Transient network errors during upload
- Upload status polling (max 5 minutes)
- Signed URL expiration (request a fresh URL)

## Limitations

- **CSV only**: Lookup tables only accept CSV format (not JSON, Parquet, etc.)
- **File size**: Maximum file size depends on your Mixpanel plan (typically 100MB-1GB)
- **Upload time**: Large files may take several minutes to process
- **Schema changes**: Changing column names requires creating a new lookup table
  (updates only replace data, not schema)

## Use Cases

### User attribute enrichment

Map user IDs to CRM attributes:

```csv
user_id,account_tier,company,industry
u001,enterprise,Acme Corp,Manufacturing
u002,pro,Widget Inc,Retail
```

Query in Mixpanel:
```
Event: Page View
Breakdown by: User Attributes.account_tier
```

### Product metadata

Map product SKUs to product details:

```csv
sku,category,price,brand
SKU001,Electronics,299.99,BrandA
SKU002,Clothing,49.99,BrandB
```

### Geographic data

Map postal codes to regions:

```csv
postal_code,city,state,region
10001,New York,NY,Northeast
90001,Los Angeles,CA,West
```

## Best Practices

1. **Use consistent join keys**: Ensure the key column in your CSV matches the
   property name in your Mixpanel events

2. **Include a header row**: The CSV must have column names in the first row

3. **Keep data current**: Set up automated uploads via CI/CD to keep lookup
   tables in sync with your source systems

4. **Version your CSV files**: Store CSV files in git alongside your Terraform
   configs for audit trails

5. **Use descriptive names**: Name columns clearly (e.g., `account_tier` not
   `tier`)

6. **Test in dev first**: Upload to a dev project before deploying to production

## See Also

- [mixpanel_data_group](./data_group.md) - Create data groups explicitly
- [Cross-Project Portability Guide](../guides/cross-project-portability.md) - Deploy lookup tables across environments
