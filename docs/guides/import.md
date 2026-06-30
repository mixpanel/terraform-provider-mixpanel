# Importing existing Mixpanel objects into Terraform

This guide shows how to bring objects that already exist in your Mixpanel
project (or organization) under management by the `mixpanel` Terraform
provider — one object at a time, or in bulk with a single `for_each` loop.

It uses the modern, config-driven [`import` block][import-block] flow
introduced in Terraform 1.5 and supported by OpenTofu. All examples are
written for **Terraform / OpenTofu 1.12**.

> The `terraform import` *CLI command* still works, but the `import {}` block
> is the recommended approach: it lives in your configuration, is reviewable in
> a PR, and pairs with `-generate-config-out` to scaffold the resource bodies
> for you.

[import-block]: https://developer.hashicorp.com/terraform/language/import

---

## 1. The import ID format

Every resource is imported by an **import ID**. Because Mixpanel objects live
inside a project (or an organization), the import ID is a **composite** string
of the scope id and the object id, joined with a colon. The exact shape depends
on the resource's scope:

| Scope class      | Import ID format        | Example                 | Resources (examples)                                   |
| ---------------- | ----------------------- | ----------------------- | ------------------------------------------------------ |
| Project-scoped   | `PROJECT_ID:ID`         | `2195193:417`           | `annotation`, `custom_alert`, `experiment`, `heat_map`, `playlist`, `custom_property`, `custom_role`, `email_digest`, `agent_flow`, `dashboard`, … |
| Workspace-scoped | `PROJECT_ID:ID`         | `2195193:flag_abc`      | `feature_flag` (the workspace is resolved on read)     |
| Org-scoped       | `ORGANIZATION_ID:ID`    | `1042:8821`             | `service_account`                                      |
| Singleton        | `PROJECT_ID`            | `2195193`               | `data_governance_settings` (one per project, no separate id) |
| Unscoped         | `ID`                    | `33914`                 | `rollup_project`                                       |

Notes:

- The scope segment (`PROJECT_ID` / `ORGANIZATION_ID`) is **part of the import
  ID**. It is parsed during import and written into the resource's `project_id`
  / `organization_id` attribute, so you can import objects from a project other
  than the provider's default `project_id` without changing provider config.
- For project-scoped resources whose object id is itself an integer (e.g.
  `annotation`), both segments are integers: `2195193:417`. For resources with
  string ids (e.g. `feature_flag`), the id segment is a string: `2195193:flag_abc`.
- An empty segment is rejected — `:417` or `2195193:` will error with
  `expected import ID in the form "project_id:<id>"`.

You do **not** have to assemble these strings by hand. The plural data sources
(see [section 3](#3-bulk-import-with-a-plural-data-source--for_each)) expose an
`import_ids` attribute that emits the correct composite for every object.

---

## 2. Importing a single object

### Step 1 — declare an `import` block

```hcl
import {
  to = mixpanel_annotation.release_marker
  id = "2195193:417" # PROJECT_ID:ID
}
```

### Step 2 — generate the resource configuration

Run `plan` with `-generate-config-out` to scaffold the resource body for every
`import` block that does not yet have a matching `resource` block:

```sh
terraform plan -generate-config-out=generated.tf
# or, with OpenTofu:
tofu plan -generate-config-out=generated.tf
```

This writes a `mixpanel_annotation "release_marker"` resource block into
`generated.tf`, populated from the live object.

> **Config generation is experimental.** It:
> - omits read-only / computed-only attributes (e.g. `id`, computed
>   timestamps),
> - cannot guess values for **write-only / secret** fields (see
>   [section 4](#4-gotchas)), and
> - may emit attributes that conflict with each other or need hand-editing.
>
> Always **review and edit** `generated.tf` before applying. Move the generated
> block out of `generated.tf` into a real `.tf` file once you're happy with it.

### Step 3 — apply, then plan until clean

```sh
terraform apply          # performs the import and records state
terraform plan           # should report: No changes.
```

If the second `plan` is not clean, see [Gotchas](#4-gotchas) — the usual causes
are normalization drift or write-only fields.

### Step 4 — remove the `import` block

Once imported, the `import` block has done its job. You may delete it (leaving
it is harmless; Terraform treats already-imported objects as a no-op).

---

## 3. Bulk import with a plural data source + `for_each`

To import **every** object of a kind in a project, pair a plural data source
with a `for_each` import block. The provider ships a plural list data source for
each of the following (the "GREEN-10"):

| Data source                  | Imports resource         |
| ---------------------------- | ------------------------ |
| `mixpanel_agent_flows`       | `mixpanel_agent_flow`    |
| `mixpanel_annotations`       | `mixpanel_annotation`    |
| `mixpanel_cohorts`           | `mixpanel_cohort`        |
| `mixpanel_custom_alerts`     | `mixpanel_custom_alert`  |
| `mixpanel_custom_propertys`  | `mixpanel_custom_property`|
| `mixpanel_custom_roles`      | `mixpanel_custom_role`   |
| `mixpanel_email_digests`     | `mixpanel_email_digest`  |
| `mixpanel_experiments`       | `mixpanel_experiment`    |
| `mixpanel_feature_flags`     | `mixpanel_feature_flag`  |
| `mixpanel_heat_maps`         | `mixpanel_heat_map`      |
| `mixpanel_playlists`         | `mixpanel_playlist`      |

> `mixpanel_custom_propertys` is spelled with a naive plural `s` on purpose —
> the type name is derived mechanically from the resource name.

Each plural data source exposes:

| Attribute    | Type           | Meaning                                                            |
| ------------ | -------------- | ----------------------------------------------------------------- |
| `project_id` | Optional String | Project to list. Defaults to the provider's `project_id`.        |
| `ids`        | Computed `list(string)` | Raw server ids of every object.                          |
| `import_ids` | Computed `list(string)` | `PROJECT_ID:ID` composites — feed these straight to `for_each`. |

### The pattern

```hcl
# 1. List every object in the project.
data "mixpanel_annotations" "all" {}

# 2. Import each one into an instance of the keyed resource.
import {
  for_each = toset(data.mixpanel_annotations.all.import_ids)
  to       = mixpanel_annotation.this[each.key]
  id       = each.value
}

# 3. Declare the keyed resource. After -generate-config-out fills the body,
#    each instance is addressed by its import id.
resource "mixpanel_annotation" "this" {
  for_each = toset(data.mixpanel_annotations.all.import_ids)
  # body generated by -generate-config-out, then reviewed & edited
}
```

`each.key` and `each.value` are both the composite import id (because the set is
keyed by its own elements), so `to = mixpanel_annotation.this[each.key]` and
`id = each.value` refer to the same object — one import block instance per
object, one resource instance per object.

### Run it

```sh
# Scaffold config for every instance the import blocks reference.
terraform plan -generate-config-out=generated.tf

# Review generated.tf carefully (see Gotchas), then:
terraform apply

# Confirm convergence — iterate on config until this is clean.
terraform plan   # goal: No changes.
```

To import from a **non-default** project, set `project_id` on the data source;
the emitted `import_ids` carry that project automatically:

```hcl
data "mixpanel_annotations" "all" {
  project_id = "2195193"
}
```

---

## 4. Gotchas

**Write-only / secret fields can't be read back.** Some attributes are never
returned by the API after creation:

- `service_account` — the account's **token/secret** is write-only.
- `connector` — warehouse **credentials** are write-only.

A `terraform import` sets the object's identity and scope, but these secret
attributes import as `null`. Expect a one-time diff on the secret attribute on
the first post-import `plan`; supply the secret in config to resolve it. Objects
whose *only* meaningful config is a secret are effectively not cleanly
importable.

**Partial reads leave adds in the plan.** A few endpoints return only a subset
of fields on a single GET (e.g. `custom_alert`'s GET returns roughly
`{id, name}`). After import, the un-returned attributes will plan as additions
until you fill them in config. Review the first `plan` and reconcile.

**Normalization / format drift.** Server-canonicalized values can differ from
what `-generate-config-out` writes or what you hand-author:

- Timestamp strings (`annotation.date`, `email_digest.start_date`) may echo back
  in a different format than your config — match the server's canonical form.
- List ordering (e.g. `agent_flow.tags`, `experiment.tags` / `metrics` /
  `variants`, `custom_role.permissions`, `email_digest.recipients`,
  `playlist.bookmarked_replays`) may be reordered by the server.
- JSON-blob attributes (`heat_map`, `playlist`, `experiment`, `feature_flag`,
  `agent_flow`) may have server-injected sub-keys that appear on the first plan.

If a post-import plan shows a drift like this, edit your config to match the
server's returned value, then re-plan until clean.

**Always review generated config before applying.** `-generate-config-out` is
experimental: it omits computed attributes, can't fill secrets, and may produce
conflicting attributes. Treat `generated.tf` as a draft.

**One object → one resource address.** Each `import` block (or `for_each`
instance) maps exactly one live object to exactly one resource address. Don't
point two import blocks at the same address, and don't reuse one address for two
objects.

**Composite id, not bare id.** Project- and org-scoped resources need the scope
segment. Importing an `annotation` with just `417` (instead of `2195193:417`)
fails. The `import_ids` attribute always gives you the correct composite.

---

## 5. Worked end-to-end example: bulk-import every annotation

This imports **all** annotations in project `2195193` in one pass.

### `main.tf`

```hcl
terraform {
  required_providers {
    mixpanel = {
      source = "mixpanel/mixpanel"
    }
  }
}

provider "mixpanel" {
  service_account        = var.mixpanel_service_account
  service_account_secret = var.mixpanel_service_account_secret
  project_id             = "2195193"
}

variable "mixpanel_service_account" { type = string }
variable "mixpanel_service_account_secret" {
  type      = string
  sensitive = true
}

# List every annotation in the default project.
data "mixpanel_annotations" "all" {}

# One import block instance per annotation.
import {
  for_each = toset(data.mixpanel_annotations.all.import_ids)
  to       = mixpanel_annotation.this[each.key]
  id       = each.value
}

# One resource instance per annotation. Body filled by -generate-config-out.
resource "mixpanel_annotation" "this" {
  for_each = toset(data.mixpanel_annotations.all.import_ids)
}
```

### Run

```sh
export MIXPANEL_SERVICE_ACCOUNT="…"
export MIXPANEL_SERVICE_ACCOUNT_SECRET="…"
export TF_VAR_mixpanel_service_account="$MIXPANEL_SERVICE_ACCOUNT"
export TF_VAR_mixpanel_service_account_secret="$MIXPANEL_SERVICE_ACCOUNT_SECRET"

terraform init
terraform plan -generate-config-out=generated.tf   # scaffolds each annotation
# Review generated.tf — annotations expose: date, description, user, user_id.
terraform apply                                     # imports all annotations
terraform plan                                      # expect: No changes.
```

After `apply`, each annotation is addressable as
`mixpanel_annotation.this["2195193:417"]`, and its `id`, `project_id`, and
`annotation_id` are populated as computed values. If the post-import plan shows
a `date` format diff, edit the generated `date` value to match what the API
echoed, then re-plan until clean.

A ready-to-run copy of this configuration lives in
[`examples/import/`](../../examples/import/).
