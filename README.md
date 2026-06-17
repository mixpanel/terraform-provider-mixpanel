# Terraform Provider for Mixpanel

A Terraform provider for managing [Mixpanel](https://mixpanel.com) resources via the
Mixpanel application API. The provider is generated from Mixpanel's OpenAPI specification
using the [HashiCorp Terraform plugin framework](https://developer.hashicorp.com/terraform/plugin/framework)
codegen toolchain, with a deterministic CRUD layer on top.

> ## ⚠️ Alpha — use at your own risk
>
> This provider is in **alpha** (`v0.x` / `-alpha` releases). It is **largely
> untested**: only a subset of resources has been verified end to end, and many
> have not been exercised against a live project at all. **Breaking changes may
> land in any release**, including to resource schemas and state.
>
> Do **not** use it against production Mixpanel projects you cannot afford to
> disrupt. Always run `terraform plan` and review the diff before applying, and
> pin an exact version (`version = "0.1.0-alpha1"`).

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
