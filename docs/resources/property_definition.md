---
page_title: "mixpanel_property_definition Resource - mixpanel"
subcategory: "Governance & Lexicon"
description: |-
  Manages the Lexicon metadata (display name, description, type, visibility flags) of one event or profile property.
---

# mixpanel_property_definition (Resource)

Manages one property's **Lexicon metadata**: display name, description,
example value, declared type, and the hidden / dropped / sensitive governance
flags. One resource instance corresponds to one property key of one resource
type (`Event` or `User`) in one project — the unit of schema governance at
scale.

The underlying API is an **upsert**: creating this resource issues the same
`PATCH /data-definitions/properties` request as updating it, and the Lexicon
definition row is materialized on first write. Deleting the resource
hard-deletes the definition row (the property's ingested data is untouched;
only its Lexicon metadata is removed). The API refuses to delete a definition
that is still referenced by a metric or cohort (HTTP 409).

~> **Workspace routing.** When the project has workspaces, the provider issues
property-definition requests through the workspace-scoped mount
(`/api/app/workspaces/{workspace_id}/data-definitions/properties`) — the same
route the Mixpanel web UI uses — targeting the project's canonical workspace
(the global "All Project Data" workspace, else the default, else the first).
Both mounts address the same project-keyed definitions; if the credential is
not a member of the canonical workspace (the workspace mount rejects
non-members with a 404 before any change is made), the provider transparently
falls back to the project-scoped mount.

-> **Unset attributes.** Attributes you do not set stay `null` in state and
are left alone server-side (the server represents "unset" as `""` / `false`,
which the provider does not adopt into `null` attributes). Removing a
previously-set string attribute from the configuration clears it server-side
(sets it to `""`); removing a previously-set boolean sets it to `false`.
`type` cannot be un-declared once set — remove it from the configuration and
the last declared type remains on the server.

## Example Usage

```terraform
# Event property metadata
resource "mixpanel_property_definition" "plan_type" {
  project_id = 1234567

  name         = "plan_type"
  display_name = "Plan Type"
  description  = "The subscription plan of the account at event time"
  type         = "string"
}

# Hide a deprecated property from pickers
resource "mixpanel_property_definition" "legacy_id" {
  project_id = 1234567

  name        = "legacy_account_id"
  description = "Deprecated 2025-03; use account_id"
  hidden      = true
}

# Classify a sensitive user-profile property
resource "mixpanel_property_definition" "email" {
  project_id = 1234567

  name          = "$email"
  resource_type = "User"
  sensitive     = true
}
```

## Schema

### Required

- `name` (String) The property key this metadata attaches to (e.g. `plan_type` or `$city`). Changing it forces a new resource (the key is the identity; there is no rename).

### Optional

- `project_id` (Number) The project ID (defaults to the provider project). Changing it forces a new resource.
- `resource_type` (String) Whether this is an event property (`Event`) or a user-profile property (`User`). Defaults to `Event`. Changing it forces a new resource.
- `display_name` (String) Human-friendly display name shown in the Mixpanel UI.
- `description` (String) Description of the property's meaning and usage.
- `example_value` (String) Example value(s) shown in Lexicon.
- `type` (String) Declared data type. One of `string`, `number`, `datetime`, `boolean`, `list`, `object`, `blob`, `null`, `dimension`, `unknown`.
- `hidden` (Boolean) Hide the property from pickers in the Mixpanel UI.
- `dropped` (Boolean) Drop the property at ingestion. Event properties only — the API rejects dropping user properties and Mixpanel-default properties.
- `sensitive` (Boolean) Mark the property as classified/sensitive (data governance).

### Read-Only

- `id` (Number) The Lexicon definition row id.

## Import

Property definitions are imported by project, resource type, and property
name (the name comes last so keys containing `:` import correctly):

```bash
terraform import mixpanel_property_definition.plan_type '1234567:Event:plan_type'
terraform import mixpanel_property_definition.email '1234567:User:$email'
```

-> Boolean flags that are `false` and strings that are empty on the server
import as unset (`null`); set them explicitly in the configuration if you
want to manage them.

## API Notes (verified against the webapp source and live API)

- The property-definitions module exposes `GET`/`PATCH`/`DELETE` on the
  collection path only; the property is addressed by `name` + `resourceType`
  in the query string (GET) or the JSON body (PATCH/DELETE). There is no POST:
  create *is* the PATCH upsert.
- A GET for a property with no definition row returns a synthetic body with
  `id: 0` instead of 404; the provider treats `id == 0` as "deleted
  externally".
- The provider uses the single-property PATCH path exclusively (never the bulk
  `properties`-array path, which supports fewer fields and has per-row
  quirks).
