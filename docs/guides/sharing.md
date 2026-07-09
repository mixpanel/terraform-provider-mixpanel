---
page_title: "Entity sharing"
subcategory: "Guides"
description: |-
  Why Terraform-created entities are invisible in the Mixpanel UI by default, and how share_with_project fixes it.
---

# Entity sharing

## Service-account privacy

Every entity this provider creates (dashboards, cohorts, saved metrics, and so
on) is created by your **service account**, and Mixpanel entities are *private
to their creator* until they are shared. Without sharing, you can apply a
configuration full of dashboards and cohorts, open the Mixpanel webapp, and see
nothing — the entities exist, but they are visible only to the service account
that created them.

## `share_with_project`

To make Terraform-managed entities usable by humans, every shareable resource
carries a `share_with_project` attribute:

```terraform
resource "mixpanel_dashboard" "kpis" {
  title = "Team KPIs"

  # Default; shown for clarity. Set to false to keep the entity
  # private to the service account.
  share_with_project = true
}
```

* `share_with_project = true` (the **default**): immediately after creating the
  entity, the provider shares it with the whole project (with edit permission),
  so every project member can see and use it.
* `share_with_project = false`: the entity stays private to the service
  account. Changing `true` → `false` on an existing resource removes the
  project share; `false` → `true` adds it.

The attribute is available on: `mixpanel_cohort`, `mixpanel_custom_event`,
`mixpanel_custom_property`, `mixpanel_dashboard`, `mixpanel_metric`,
`mixpanel_formula`, `mixpanel_behavior`, `mixpanel_feature_flag`,
`mixpanel_bookmark`, and `mixpanel_experiment`.

## Failure behavior

If the entity is created but the follow-up share call fails (for example, a
transient API error or missing permission), the apply does **not** fail — the
entity exists and is tracked in state. Instead the provider emits a warning and
records `share_with_project = false` in state, so the next `terraform apply`
retries the share. You can also share the entity manually from the Mixpanel UI
(Share dialog on the entity).

## Drift

On refresh, the provider reads the entity's project share back from the API,
so a share added or removed outside Terraform shows up as normal drift. If the
share status cannot be read (for example, insufficient permission), the value
already in state is kept rather than failing the refresh.
