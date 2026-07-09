# Contributing to terraform-provider-mixpanel

Thank you for contributing! This provider is built with codegen tooling and hand-curated documentation.

## Docs are hand-curated

**The most important rule:** Documentation pages under `docs/` are written and maintained **by hand**. Never run `tfplugindocs generate`.

- Only `templates/index.md.tmpl` exists as a template. Regenerating docs would overwrite every hand-written page.
- `make docs` is intentionally a no-op guard that prints a reminder instead of running tfplugindocs.
- When you edit `docs/index.md`, keep `templates/index.md.tmpl` in sync. Their non-directive prose must be identical (the template preserves the `{{ .SchemaMarkdown }}` and `{{ tffile ... }}` directives).

For context on the codegen pipeline, see [gen/README.md](gen/README.md) step 6.

## The canonical page pattern

Every resource and data source page follows this structure:

```markdown
---
page_title: "<type> Resource - mixpanel"  # or "Data Source"
subcategory: "<family>"
description: |-
  <one plain-English sentence>
---

# <type> (Resource)

<Plain-English intro (1–3 sentences): what the Mixpanel object IS, product term first>

## Example Usage

```terraform
<minimal working example, schema-verified>
```

<Guidance sections with ## headings>

## Schema

<Generated schema block — retained and fixed in place>

## Import

<Composite ID format + import {} block example, OR explicit note if not cleanly importable>
```

Data source pages omit the `## Import` section. For plural/list data sources, the intro explains it lists every object of a kind, and the Schema documents `ids` and `import_ids` with a link to the bulk-import pattern in `guides/import.md`.

### Subcategories

Assign exactly one subcategory from this list:

- **Analytics & Reporting** — annotation, behavior, bookmark, canvas, cohort, custom_alert, custom_event, custom_property, dashboard, email_digest, formula, metric, theme
- **Governance & Lexicon** — data_governance_settings, data_group, event_definition, event_drop_filter, lexicon_tag, lookup_table, property_definition, tag, schema_graph
- **Experimentation & Delivery** — experiment, feature_flag
- **Data Pipeline** — connector, dataset, rollup_project, warehouse_source, webhook, project_outgoing_integration
- **Session Replay & Heatmaps** — heat_map, heat_map_collection, playlist
- **AI & Automation** — agent_flow, business_context, spark_settings
- **Administration & Access** — custom_role, org_request_access_settings, org_session_settings, project, service_account, service_account_project, team, twofactor_settings, user_project_role, workspace

## Style rules

- **Product vocabulary leads, Terraform type follows.** Example: "Manages a Mixpanel **Board** (`mixpanel_dashboard`)".
- **Gloss Terraform jargon on first use** per page (taint, ForceNew, Optional+Computed).
- **Trust/alpha language lives only in `docs/index.md`.** No per-page alpha warnings.
- **Cross-links are real relative links** within `docs/` (e.g. `docs/guides/sharing.md`) or absolute GitHub URLs (`https://github.com/mixpanel/terraform-provider-mixpanel/...`). Links must not escape `docs/` except as absolute URLs.
- **No internal codenames in docs.** Never use GREEN-*, ARB (standalone), api-spec.yml, pydantic docstrings, "frozen spec", or spec numbers (045/047) in user-facing documentation.
- **Claim discipline.** Never claim "first" or "only" analytics provider. State the breadth and depth of the analytical surface with a dated comparison when relevant (see `docs/index.md` for the approved framing).
- **No hype.** Plain language, no marketing vocabulary.

## Example-verification protocol

Every HCL example's attributes must be verified against the Go schema before merging:

1. **Check the generated schema files.** For each attribute, confirm it exists in:
   - `internal/provider/resource_<entity>/*_gen.go` (generated attributes)
   - `internal/provider/<entity>_resource.go` (hand-injected attributes)
   - `internal/provider/<entity>_spec.go` (wire mappings)

2. **JSON-blob payloads** (e.g. cohort `groups`, dashboard `layout`) must satisfy the plan-time validation rules documented on their page.

3. **Prefer copying from live-verified sources** over authoring fresh examples:
   - `examples/resources/<type>/resource.tf`
   - Acceptance tests in `internal/provider/*_acc_test.go`

4. **Key gotcha:** Do NOT re-copy attribute descriptions from the Go `*_gen.go` files into docs. The generated schema files still contain internal phrasing unsuitable for user-facing documentation. Write descriptions in plain English.

## Building and testing

See the [README](README.md) development section and [internal/provider/ACCEPTANCE_TESTING.md](internal/provider/ACCEPTANCE_TESTING.md) for instructions on:

- Building the provider
- Running unit and acceptance tests
- Setting up `dev_overrides` for local testing with `terraform validate`
