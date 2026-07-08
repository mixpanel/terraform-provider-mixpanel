# Terraform Provider for Mixpanel

Terraform provider for managing your [Mixpanel](https://mixpanel.com) estate — cohorts,
metrics, formulas, dashboards, event taxonomy, experiments, data pipelines, and org
administration — as version-controlled code instead of unversioned click-state in a web
app. It is generated from Mixpanel's OpenAPI specification using the
[HashiCorp Terraform plugin framework](https://developer.hashicorp.com/terraform/plugin/framework)
codegen toolchain, with a deterministic CRUD layer on top.

## Why manage Mixpanel as code

- **Review and history for analytics.** Every change to a cohort, metric, or dashboard is
  a diff a teammate approves in a pull request, and `git blame` gives you the who, when,
  and why the UI never could.
- **Environment parity.** One configuration deploys to your dev, staging, and production
  projects, so they stop drifting apart instead of each being configured by hand.
- **Drift detection and governance.** Out-of-band edits made in the UI surface in the next
  `terraform plan`, and your Lexicon taxonomy lives in the same reviewed repo as the metrics
  that depend on it.

For the full argument — including how `terraform plan` plus pull-request review becomes a
review harness for AI-proposed changes — see
[Analytics as code](./docs/guides/analytics-as-code.md).

## Status

This provider is in **alpha** (`v0.x` / `-alpha` releases); breaking changes — including to
resource schemas and state — may land in any release. In CI, every resource runs a full
plan/apply/refresh/destroy lifecycle against an in-process mock of the Mixpanel API, and a
live acceptance suite exercises core resources against real Mixpanel projects. Pin an exact
version, run `terraform plan` and review the diff before every apply, and don't point it at a
production project you can't afford to disrupt. See the
[provider index](./docs/index.md) for the full trust posture and the per-resource maturity
table.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0 (or OpenTofu)
- A Mixpanel [service account](https://developer.mixpanel.com/reference/service-accounts)
  with access to the target project
- [Go](https://go.dev/dl/) >= 1.23 (only to build from source)

## Quickstart

### Authentication

The provider authenticates with HTTP Basic auth using a Mixpanel service account
(username = service account name, password = service account secret):

```hcl
provider "mixpanel" {
  service_account        = "my-service-account"   # or MIXPANEL_SERVICE_ACCOUNT
  service_account_secret = var.mixpanel_secret    # or MIXPANEL_SERVICE_ACCOUNT_SECRET
  project_id             = "1234567"              # or MIXPANEL_PROJECT_ID; per-resource override allowed
}
```

Any provider attribute may be supplied via its matching environment variable
(`MIXPANEL_SERVICE_ACCOUNT`, `MIXPANEL_SERVICE_ACCOUNT_SECRET`, `MIXPANEL_PROJECT_ID`).
**Never commit credentials** — use environment variables, a Terraform variable, or a
secrets manager.

### Regions

The provider talks to `https://mixpanel.com` by default. For projects in the EU or India
residency regions, set `base_url` (or `MIXPANEL_BASE_URL`) to the regional API host:

```hcl
provider "mixpanel" {
  base_url = "https://eu.mixpanel.com" # or "https://in.mixpanel.com"
}
```

> **Heads-up:** credentials pointed at the **wrong region** fail with a **500-series error,
> not a 401**. If every request errors with a 500 and your service account secret is
> definitely correct, check that `base_url` matches your project's region before debugging
> anything else.

### Your first resource

```hcl
terraform {
  required_providers {
    mixpanel = {
      source = "mixpanel/mixpanel"
    }
  }
}

provider "mixpanel" {}

resource "mixpanel_annotation" "release" {
  date        = "2026-01-01 00:00:00"
  description = "v2.0 release"
}
```

Run `terraform plan` to preview the change and `terraform apply` to create it. For a full
walkthrough aimed at people new to Terraform, see
[Getting started](./docs/guides/getting-started.md).

## Documentation

The complete reference for every resource and data source lives on the
[Terraform Registry](https://registry.terraform.io/providers/mixpanel/mixpanel/latest/docs)
and under [`docs/`](./docs/index.md). Guides:

- [Analytics as code](./docs/guides/analytics-as-code.md) — why manage your Mixpanel estate
  as version-controlled code.
- [Getting started](./docs/guides/getting-started.md) — install Terraform or OpenTofu and
  run the plan/apply loop end to end.
- [Importing existing Mixpanel objects](./docs/guides/import.md) — bring objects you built
  in the UI under Terraform management.
- [Cross-project portability](./docs/guides/cross-project-portability.md) — deploy one
  configuration across dev, staging, and production projects.
- [Drift detection](./docs/guides/drift-detection.md) — how the provider detects and
  reconciles changes made outside Terraform.
- [Entity sharing](./docs/guides/sharing.md) — why Terraform-created objects are invisible
  in the Mixpanel UI by default, and how to fix it.

## Operational notes

- **Rate limits and retries.** The provider automatically retries transient failures (429,
  408, and 500/502/503/504) with exponential backoff (1s base, up to 60s, 10% jitter, 5
  retries) and honors the `Retry-After` header on 429s. For large applies (100+ resources),
  `terraform apply -parallelism=2` trades speed for a lower chance of hitting API rate
  limits.
- **JSON string fields.** Polymorphic or free-form fields (e.g. dashboard layouts, cohort
  `groups`, metric `definition`) are exposed as JSON-encoded **strings**; set them with
  [`jsonencode`](https://developer.hashicorp.com/terraform/language/functions/jsonencode).
  They use semantic JSON equality, so key order, whitespace, and number formatting never
  show as spurious diffs. Some analytics-entity blobs are additionally validated at
  `terraform plan` time; see the per-resource docs for the exact rules.
- **Drift detection.** `terraform plan` / `terraform refresh` re-read every resource from
  the API, so edits made in the Mixpanel UI show up as drift and the next apply converges
  the server back to your configuration. See the
  [drift detection guide](./docs/guides/drift-detection.md) for the merge rules and
  limitations.

## Development

Build the provider:

```sh
go build ./...
```

Run the tests:

```sh
make test              # unit + in-process mock lifecycle tests
make test-acceptance   # live suite; requires TF_ACC=1 and service-account credentials
```

### Local development with `dev_overrides`

Build the binary and point Terraform at it directly (no `terraform init` required):

```sh
go build -o terraform-provider-mixpanel .
```

```hcl
# ~/.terraformrc
provider_installation {
  dev_overrides {
    "mixpanel/mixpanel" = "/path/to/dir/containing/the/binary"
  }
  direct {}
}
```

## License

Apache License 2.0 — see [LICENSE](./LICENSE).
