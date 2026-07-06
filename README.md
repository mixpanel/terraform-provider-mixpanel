# Mixpanel State

A Terraform provider for managing [Mixpanel](https://mixpanel.com) resources via the
Mixpanel application API. The provider is generated from Mixpanel's OpenAPI specification
using the [HashiCorp Terraform plugin framework](https://developer.hashicorp.com/terraform/plugin/framework)
codegen toolchain, with a deterministic CRUD layer on top.

> ## ⚠️ Alpha
> This provider is in **alpha** (`v0.x` / `-alpha` releases).

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- A Mixpanel [service account](https://developer.mixpanel.com/reference/service-accounts)
  with access to the target project
- [Go](https://go.dev/dl/) >= 1.23 (only to build from source)

## Authentication

The provider authenticates using HTTP Basic auth with a Mixpanel service account
(username = service account name, password = service account secret).

```hcl
provider "mixpanel" {
  service_account        = "my-service-account"   # or MIXPANEL_SERVICE_ACCOUNT
  service_account_secret = var.mixpanel_secret    # or MIXPANEL_SERVICE_ACCOUNT_SECRET
  project_id             = "1234567"              # or MIXPANEL_PROJECT_ID; per-resource override allowed
}
```

Any provider attribute may be supplied via its corresponding environment variable
(`MIXPANEL_SERVICE_ACCOUNT`, `MIXPANEL_SERVICE_ACCOUNT_SECRET`, `MIXPANEL_PROJECT_ID`).
**Never commit credentials** — use environment variables, a Terraform variable, or a secrets
manager.

## Regions

The provider talks to `https://mixpanel.com` by default. For projects hosted in
the EU or India residency regions, set `base_url` (or `MIXPANEL_BASE_URL`) to
the regional API host:

```hcl
provider "mixpanel" {
  base_url = "https://eu.mixpanel.com" # or "https://in.mixpanel.com"
}
```

> **Heads-up:** credentials pointed at the **wrong region** fail with a
> **500-series error, not a 401**. If every request errors with a 500 and your
> service account secret is definitely correct, check that `base_url` matches
> the region your project lives in before debugging anything else.

## Example

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

## Rate limiting and retries

The provider includes automatic retry logic for transient failures:

- **Retries on**: 429 (rate limit), 408 (timeout), 500, 502, 503, 504 (server errors)
- **Retry-After header**: Honored on 429 responses
- **Exponential backoff**: 1s base, doubling each attempt, up to 60s max, with 10% jitter
- **Max retries**: 5 attempts (6 total requests including the initial attempt)
- **No retry on**: 4xx errors (except 408/429) — these are terminal client errors

For large `terraform apply` operations (100+ resources), consider using
`-parallelism=2` to reduce the risk of hitting Mixpanel API rate limits:

```sh
terraform apply -parallelism=2
```

The default parallelism of 10 can overwhelm rate limits when creating many resources
at once. Reducing parallelism trades apply speed for reliability.

## Polymorphic / dynamic fields

Some Mixpanel objects contain polymorphic or free-form JSON (e.g. dashboard layouts,
agent-flow graphs). These are exposed as JSON-encoded **strings**; use Terraform's
[`jsonencode`](https://developer.hashicorp.com/terraform/language/functions/jsonencode)
to set them:

```hcl
resource "mixpanel_dashboard" "example" {
  title    = "My dashboard"
  metadata = jsonencode({ key = "value" })
}
```

These JSON string attributes use semantic JSON equality
(`jsontypes.Normalized`): key order, whitespace, and number-rendering
differences between your configuration and the server's echo are never
reported as diffs, while real changes are.

The analytics-entity blobs (cohort `groups`, behavior/metric/formula
`definition`, bookmark `params`) are additionally validated at `terraform
plan` time: definition shapes that the API accepts with a 200 but that corrupt
the Mixpanel webapp query builder are rejected as plan errors, and server-side
limits (100 funnel steps — the ARB merger cap; 60 retention intervals) are
enforced or warned about before anything is sent. See the per-resource docs
for the exact rules.

## Drift detection

`terraform plan` / `terraform refresh` re-read every resource from the API
with **wire-preferred** semantics: any field the GET response carries wins
over prior state, so edits made in the Mixpanel webapp (a renamed cohort, a
rewritten `groups` definition, a changed feature-flag ruleset) show up as
drift and the next `terraform apply` converges the server back to your
configuration. Fields the API does not echo back on GET keep their prior
state value. See the
[drift detection guide](docs/guides/drift-detection.md) for the exact merge
rules and limitations.

## Resources

`mixpanel_agent_flow`, `mixpanel_annotation`, `mixpanel_bookmark`, `mixpanel_canvas`,
`mixpanel_cohort`, `mixpanel_connector`, `mixpanel_custom_alert`, `mixpanel_custom_event`,
`mixpanel_custom_property`, `mixpanel_custom_role`, `mixpanel_dashboard`,
`mixpanel_data_group`, `mixpanel_email_digest`, `mixpanel_experiment`,
`mixpanel_feature_flag`, `mixpanel_heat_map`, `mixpanel_heat_map_collection`,
`mixpanel_playlist`, `mixpanel_rollup_project`, `mixpanel_service_account`,
`mixpanel_theme`, `mixpanel_webhook`

## Data sources

`mixpanel_agent_flow`, `mixpanel_annotation`, `mixpanel_bookmark`, `mixpanel_canvas`,
`mixpanel_cohort`, `mixpanel_connector`, `mixpanel_custom_alert`, `mixpanel_custom_event`,
`mixpanel_custom_property`, `mixpanel_custom_role`, `mixpanel_dashboard`,
`mixpanel_email_digest`, `mixpanel_experiment`, `mixpanel_feature_flag`,
`mixpanel_heat_map`, `mixpanel_heat_map_collection`, `mixpanel_metric`,
`mixpanel_playlist`, `mixpanel_rollup_project`, `mixpanel_service_account`,
`mixpanel_theme`, `mixpanel_warehouse_source`

## Building from source

```sh
go build ./...
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
