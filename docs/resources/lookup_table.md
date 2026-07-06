---
page_title: "mixpanel_lookup_table Resource - mixpanel"
subcategory: ""
description: |-
  Manages a Mixpanel lookup table: uploads CSV content through the signed-URL handshake and keeps the table's name/description in sync.
---

# mixpanel_lookup_table (Resource)

Lookup tables enrich Mixpanel data by mapping a join key (the first CSV
column) to supplementary attributes — for example user IDs to CRM fields, or
product SKUs to catalog metadata. Each lookup table is backed by a dimension
data group.

## Upload Handshake

Creating (and re-uploading) a lookup table is a stateful handshake, which the
provider drives automatically:

1. `GET .../data-definitions/lookup-tables/upload-url?content-type=text/csv`
   mints a signed storage URL (`{url, path, key}`, 60-minute expiry).
2. The CSV content is `PUT` directly to the signed URL (external storage, with
   `Content-Type: text/csv`).
3. `POST .../data-definitions/lookup-tables` (form-encoded `name`, `path`,
   `key`, plus `data-group-id` when replacing an existing table's rows)
   registers the upload. Small files (< 5 MB) are ingested synchronously and
   the response carries the table id; larger files are ingested by a
   background task and the response carries an `uploadId`.
4. For background ingestion, the provider polls
   `GET .../lookup-tables/upload-status?upload-id=...` with backoff (bounded
   at 10 minutes) until the task succeeds or fails.

~> **Environment support.** Step 3 creates the dimension data group in the
ingestion backend. Environments without a working lookup-table ingestion
pipeline reject it even though the CSV upload itself succeeded; the provider
surfaces this as an explicit error on the create/update. Deletion is
separately gated by the server-side `can-delete-data-groups` setting — where
it is disabled, `terraform destroy` fails with the server's message and the
resource can be dropped from state with `terraform state rm` instead.

~> **Workspace routing.** Lookup-table endpoints live in the dual-mounted
data-definitions API. As with `mixpanel_lexicon_tag`, the provider prefers the
workspace-scoped mount (the project's canonical workspace, the route the
Mixpanel UI uses) and transparently falls back to the project-scoped mount
when the credential is not a member of that workspace.

## Example Usage

```terraform
resource "mixpanel_lookup_table" "accounts" {
  project_id  = 1234567
  name        = "Account Attributes"
  description = "CRM attributes keyed by account id"

  # First column is the join key; header row required.
  csv_content = file("${path.module}/data/accounts.csv")
}
```

Inline content works too:

```terraform
resource "mixpanel_lookup_table" "plans" {
  project_id = 1234567
  name       = "Plan Metadata"

  csv_content = <<-CSV
    plan_id,tier,monthly_price
    p1,free,0
    p2,pro,49
    p3,enterprise,499
  CSV
}
```

## Update Behavior

- Changing `csv_content` **replaces the table's rows in place** (same table
  id) via the full upload handshake, keyed by the existing data-group id.
- Changing `name` or `description` is a lightweight metadata `PATCH` — no
  re-upload.
- Changing `project_id` forces a new resource.

## Schema

### Required

- `name` (String) The lookup table name (renamed in place).
- `csv_content` (String) The CSV content of the table (header row first; first column is the join key). Changing it re-uploads the table in place. Use `file("...")` to source it from a file.

### Optional

- `description` (String) Human-readable description.
- `project_id` (Number) The project ID (defaults to the provider project). Changing it forces a new resource.

### Read-Only

- `id` (String) The lookup table id — the dimension data-group id, a full-range (possibly negative) 64-bit integer rendered as a string.

## Import

```bash
terraform import mixpanel_lookup_table.accounts '1234567:-9199707515373727904'
```

Format: `PROJECT_ID:DATA_GROUP_ID` (the id may be negative; everything after
the first `:` is the id).

-> `csv_content` cannot be read back from the API (the download endpoint
reconstructs rows from live queries, not the uploaded file), so after import
the first plan shows `csv_content` being set; the first apply re-uploads your
configured CSV to bring the table under management.

## Drift Detection

`csv_content` drift is detected against the configuration only: if someone
re-uploads the table outside Terraform, the provider cannot see it (the API
does not expose the stored CSV). `name` and `description` are refreshed from
the API on every plan.
