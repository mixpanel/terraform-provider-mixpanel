---
page_title: "Drift detection and refresh semantics"
subcategory: "Guides"
---

# Drift detection and refresh semantics

The provider refreshes resource state from the Mixpanel API on every
`terraform plan` / `terraform refresh`, so changes made outside Terraform
(in the Mixpanel webapp, via the API, or by another tool) show up as drift
and are converged back to the configuration by the next `terraform apply`.

## How Read refreshes state

On Read, the API response is **wire-preferred**: every field the GET response
carries wins over the prior state, for typed attributes (a cohort's `name`, a
dashboard's `title`, a feature flag's `ruleset`) and for JSON blob attributes
(a cohort's `groups`, a metric's `definition`) alike. The prior state value is
kept only when the response **omits** the field (or returns JSON `null`) —
this covers the documented echo gaps:

* fields the API accepts on write but never returns on GET (form-encoded
  endpoints, write-only secrets such as connector credentials),
* `params`-style spread attributes (e.g. `mixpanel_warehouse_source.params`),
  whose flattened contents are never echoed back as one field,
* partial reads (e.g. `mixpanel_custom_alert`, whose GET omits some of the
  writable fields).

On Create/Update the merge is **plan-preferred** instead: values you set in
configuration are preserved verbatim in state (Terraform's "planned value
must equal applied value" contract), except JSON blob attributes, which take
the server's echo when present — see below for why that is safe.

The exact rules live in `internal/client/tfjson.go` (`MergeApply` /
`MergeRead`).

## JSON blob attributes use semantic JSON equality

Attributes that hold `jsonencode(...)` payloads (`groups`, `definition`,
`filters`, `settings`, …) use the
[`jsontypes.Normalized`](https://github.com/hashicorp/terraform-plugin-framework-jsontypes)
custom type. Two JSON strings compare **semantically**: differences in object
key order, whitespace, or number rendering are not diffs, so a server that
echoes your JSON back re-rendered (which Mixpanel endpoints routinely do)
cannot cause `Provider produced inconsistent result after apply` errors or
perpetual plan diffs. Genuinely different JSON — changed values, added or
removed fields, and reordered **arrays** (array order is semantic) — still
shows as drift.

The type is wire-compatible with plain strings, so states written by earlier
provider versions load unchanged. One consequence: the value must be valid
JSON; use `jsonencode()` rather than hand-written strings where possible.

## What this looks like

```console
$ # someone edits the cohort in the webapp ...
$ terraform plan
  # mixpanel_cohort.drift will be updated in-place
  ~ resource "mixpanel_cohort" "drift" {
      ~ description = "EDITED IN WEBAPP" -> "managed by terraform"
      ~ groups      = jsonencode(
          ~ [
              ~ {
                  ~ filtersOperator = "or" -> "and"
                    # (4 unchanged attributes hidden)
                },
            ]
        )
    }
$ terraform apply   # converges the server back to the configuration
```

## Limitations

* Drift on an attribute the GET response omits is invisible (there is nothing
  to compare against); the prior state value is preserved instead.
* Drift on Optional+Computed attributes you did not set in configuration is
  refreshed into state but does not produce a plan diff — that is standard
  Terraform behavior for computed attributes.
* If a GET echoes a field in a shape the schema cannot hold, the refresh
  keeps the prior value for that one attribute rather than failing the whole
  Read.
