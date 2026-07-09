# Terraform Provider for Mixpanel

Manage Mixpanel resources as code with Terraform and OpenTofu.

!!! tip "AI-friendly"
    An [`llms.txt`](llms.txt) map and per-page **Copy markdown** buttons make these docs easy to feed to coding agents and LLMs.

## Why this exists

Analytics configuration that lives as click-state in a web UI has no version history, no review gate, and no diff to show who changed what or why. When cohorts, metrics, and taxonomy are managed this way, dev and prod drift apart, duplicate event names accrete silently, and the audit trail is whatever people remember. This provider moves durable analytics assets into version-controlled HCL files so changes are reviewed in pull requests, applied by Terraform, and recorded in git — giving you review and blame for analytics, environment parity, drift detection, and governance as code.

## Quickstart

```terraform
terraform {
  required_providers {
    mixpanel = {
      source = "mixpanel/mixpanel"
    }
  }
}

# Credentials via env vars: MIXPANEL_SERVICE_ACCOUNT,
# MIXPANEL_SERVICE_ACCOUNT_SECRET, MIXPANEL_PROJECT_ID. Never commit secrets.
provider "mixpanel" {}

resource "mixpanel_annotation" "release" {
  date        = "2026-01-01 00:00:00"
  description = "v2.0 release"
}
```

## Where to go next

- [Getting Started](guides/getting-started.md) — install Terraform or OpenTofu, write your initial configuration, and run the plan/apply loop end to end.
- [Analytics as Code](guides/analytics-as-code.md) — the case for managing analytics configuration as version-controlled code, and what you get from doing so.
- [Resources](resources/index.md) — cohorts, metrics, dashboards, Lexicon governance, experiments, and more.
- [Data Sources](data-sources/index.md) — read-only Mixpanel objects for use in configurations.

!!! warning "Alpha status"
    This provider is under active development. The resource schemas are stabilizing but may change. See the [repository](https://github.com/mixpanel/terraform-provider-mixpanel) for current status and to report issues.
