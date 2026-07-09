# Terraform Provider for Mixpanel

Manage your Mixpanel analytics estate — cohorts, metrics, taxonomy, and governance — as version-controlled code.

## Why analytics as code?

Your Mixpanel configuration is consequential but largely ungoverned. The cohorts, metrics, dashboards, and event taxonomy your team relies on live as click-state inside a UI, with no history and no review gate. Version-controlling that estate brings the review, blame, and environment parity your infrastructure code already has. Changes become diffs a teammate approves; out-of-band edits are visible as drift; taxonomy accretion becomes a reviewable addition instead of silent decay. The working loop is the one you already use for infrastructure: define the state you want, plan the effect, review the change, apply the approved plan.

**New here? → [Getting Started](Getting-Started)**  
**Why analytics-as-code? → [Analytics as Code](Analytics-as-Code)**

## Guides

- [Import](Import) — bring existing Mixpanel objects under Terraform management
- [Cross Project Portability](Cross-Project-Portability) — deploy one configuration across dev, staging, and production
- [Drift Detection](Drift-Detection) — how the provider detects and reconciles out-of-band changes
- [Sharing](Sharing) — collaborate on Terraform modules for analytics configuration

## Reference

**Full per-resource reference → [Terraform Registry](https://registry.terraform.io/providers/mixpanel/mixpanel/latest/docs)**

## Quick start

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

**Full walkthrough → [Getting Started](Getting-Started)**

## Status

This provider is in alpha. The resource schema is stable across the resources that exist, but the provider surface is still expanding to cover additional Mixpanel capabilities. For the current maturity and testing posture, see the [repository](https://github.com/mixpanel/terraform-provider-mixpanel).
