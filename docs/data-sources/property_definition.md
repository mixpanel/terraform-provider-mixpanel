---
page_title: "property_definition Data Source - mixpanel"
subcategory: "Governance & Lexicon"
description: |-
  Fetches a Mixpanel Lexicon property definition by name.
---

# property_definition (Data Source)

Fetches a Mixpanel **Lexicon property definition** by name — the metadata,
type, visibility, and documentation for one event or profile property.

-> The API never returns 404 for an unknown property name — it synthesizes an
empty definition with `id = 0`. Check the `exists` attribute to distinguish "a
definition row exists" from "nothing defined yet".

## Example Usage

```terraform
data "mixpanel_property_definition" "plan_type" {
  project_id = 1234567
  name       = "plan_type"
}

output "plan_type_description" {
  value = data.mixpanel_property_definition.plan_type.description
}
```

## Schema

### Required

- `name` (String) The property key to look up.

### Optional

- `project_id` (Number) The project ID (defaults to the provider project).
- `resource_type` (String) `Event` (default) or `User`.

### Read-Only

- `id` (Number) The Lexicon definition row id (`0` when no definition row exists yet).
- `exists` (Boolean) Whether a Lexicon definition row exists for this property.
- `display_name` (String)
- `description` (String)
- `example_value` (String)
- `type` (String)
- `hidden` (Boolean)
- `dropped` (Boolean)
- `sensitive` (Boolean)
