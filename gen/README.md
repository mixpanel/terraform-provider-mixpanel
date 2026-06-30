# `gen/` — Terraform provider code generator

This directory deterministically generates the Go source for the
`mixpanel/mixpanel` Terraform provider under [`../internal/provider/`](../internal/provider/).
Given a frozen OpenAPI spec and a hand-curated entity manifest, it emits a full
CRUD resource, a singular data source, an `AttrSpec` wire-mapping table, shared
runtime helpers, an acceptance test, and registry-convention examples for every
managed entity.

If you are adding a Mixpanel resource to the provider, you are almost certainly
here to edit [`refined_manifest.json`](refined_manifest.json) and re-run
[`regen.sh`](regen.sh). Jump to [How to add a new entity](#how-to-add-a-new-entity).

> Audience: a Mixpanel engineer who has never seen this pipeline. Everything the
> generator does is driven by data files in this directory — there is no hidden
> state.

## Overview

The provider's source of truth is the **intersection** of two committed files:

- [`refined_manifest.json`](refined_manifest.json) — the *roster and knob*. One
  record per entity declaring its paths, schemas, identity, and CRUD quirks. This
  is what a maintainer edits.
- [`provider_code_spec.json`](provider_code_spec.json) — the HashiCorp
  *intermediate representation* (IR): the typed attribute schema for each entity,
  derived from the OpenAPI spec.

`crudgen.py` generates code only for entities that appear in **both** the manifest
(`keep: true`) **and** the IR. The manifest says *what to manage and how*; the IR
says *what the attributes are*. Generated files carry a
`// Code generated ... DO NOT EDIT.` header.

## Architecture

The full pipeline is four stages. Most of the time you only run **stage 4**
(`crudgen.py`); the earlier stages only re-run when a schema *attribute* changes.

```
                committed inputs                              generated output
   ┌───────────────────────────────────┐
   │ gen/spec/openapi.pruned.json       │  (frozen, 504 paths)
   │ gen/refined_manifest.json          │  (roster/knob)
   └───────────────┬───────────────────┘
                   │
        [1] preprocess_spec.py            ← unwrap results-envelope,
                   │                          collapse multi-type scalar anyOf,
                   ▼                          repoint singletons / oneOf roots
   gen/build/openapi.hashicorp.json
                   │
        [2] tfplugingen-openapi           ← HashiCorp; uses generator_config.yml
                   │                          (schema.ignores drops jsonencode fields)
                   ▼
   gen/provider_code_spec.json  (the IR, COMMITTED)
                   │
        [2.5] postprocess_ir.py           ← dedupe path/body name clashes,
                   │ (in place)              strip server-populated defaults
                   ▼
        [3] tfplugingen-framework         ← HashiCorp; emits typed schema pkgs
                   │
                   ▼
   ../internal/provider/{resource,datasource}_*/*_gen.go
                   │
        [4] crudgen.py  ◄──────────────── reads the IR + manifest
                   │                       (+ openapi.pruned.json components only,
                   ▼                        to recover camelCase wire keys)
   ../internal/provider/*_resource.go, *_data_source.go,
   *_spec.go, *_resource_test.go, crud_helpers.go, examples/
```

### Why each stage exists

**[1] `preprocess_spec.py`** rewrites the OpenAPI spec to satisfy the two
constraints HashiCorp's generator cannot handle:

1. **Results-envelope unwrap.** Every Mixpanel 2xx response is a
   `BaseOkResponseModel` envelope (`{"status":"ok","results":{...}}`). HashiCorp
   merges the create-*request* root with the read-*response* root to build the
   resource schema and has no `results` unwrap, so each refined op's 2xx schema is
   repointed to its inner `results` schema.
2. **Multi-type scalar collapse.** `anyOf[int,str,null]` and similar are collapsed
   to a single scalar (prefer `string`). Only `anyOf[X,null]` (plain nullable)
   collapses automatically in HashiCorp.

   It also handles a few harder shapes driven by manifest hints: repointing
   discriminated-`oneOf` create/read roots to a concrete variant
   (`concrete_*_schema`), pointing read-from-list / singleton reads at the item
   schema, injecting a synthetic flat body for spread-create entities
   (`synthetic_create_body`), a `settings`-passthrough schema for settings
   singletons, and a synthetic model for RPC-lifecycle entities. Genuinely
   polymorphic object fields are *not* touched here; they are dropped via
   `schema.ignores` in [`generator_config.yml`](generator_config.yml) and re-added
   as `jsonencode` string attributes in stage 4.

**[2] `tfplugingen-openapi`** (HashiCorp, first-party) turns the preprocessed spec
into the IR using `generator_config.yml`, which is itself generated from the
manifest and lists each entity's CRUD paths plus the `schema.ignores` for its
polymorphic fields.

**[2.5] `postprocess_ir.py`** edits the IR in place:

- **Dedupe path/body name clashes.** A snake-cased body field (e.g.
  `customPropertyId` → `custom_property_id`) can collide with a same-named path
  param. The framework rejects duplicate attribute names, so the settable
  path-param variant is kept and the pure-`computed` body duplicate dropped.
- **Strip server-populated defaults.** A `default` copied onto a pure-`computed`
  attribute becomes a static `Default:` that fights the real API value
  ("inconsistent result after apply"); it is removed.

**[3] `tfplugingen-framework`** (HashiCorp) emits the typed schema packages
(`*_gen.go`) under `internal/provider/{resource,datasource}_*/`.

**[4] `crudgen.py`** is the engine. For every entity in *both* the manifest and the
IR it emits the editable CRUD plumbing:

- **CRUD + import.** POST collection (create) / GET instance (read) / PUT|PATCH
  instance (update, or `ForceNew` when there is no update verb) / DELETE instance,
  plus `ImportState` with a scope-aware `<scope>:<id>` composite parser. Every
  request body is the *unenveloped* object; every response runs through
  `UnwrapEnvelope`.
- **The tftypes bridge.** State is handled at the `tftypes.Value` level
  (`req.Plan.Raw` / `req.State.Raw`), not via the typed `<Entity>Model`, so the
  injected `jsonencode` attributes (which aren't on the generated struct) work
  through one generic (un)marshalling path. Each entity gets an `AttrSpec`
  (`*_spec.go`) describing identity/scope/wire-key/output-only/jsonencode/spread
  metadata.
- **Wire-key recovery.** Terraform forces snake_case attribute names, but many
  Mixpanel wire keys are camelCase (`resourceType`, `displayFormula`). crudgen
  reads `components/schemas` from `openapi.pruned.json` (**never `paths`**) to pin
  each attribute's true wire key, so the API doesn't silently drop snake_cased
  fields. Because only components are read, path pruning can never change the
  generated output.
- **`jsonencode` passthroughs for polymorphic fields.** Fields that are
  `oneOf`/dynamic-object are surfaced as `Optional+Computed` `types.String`
  attributes carrying a JSON string (decoded on the way out, re-encoded on the way
  in). `format: json-object` fields are passed through verbatim as JSON strings.

A large `OVERRIDES` dict in `crudgen.py` carries the per-entity exceptions
(non-default paths, form-encoded bodies, `results`-as-map peeling, client-id
upserts, body-id CRUD, singletons, RPC lifecycle, …). Each override is documented
inline with the empirical reason it exists — read those comments before changing
an entity's contract.

## Files in this directory

| File | Purpose | Committed? |
|---|---|---|
| `refined_manifest.json` | Entity roster + per-entity knobs (the thing you edit) | ✅ committed |
| `provider_code_spec.json` | The HashiCorp IR (typed schemas) — committed so the default regen runs standalone | ✅ committed |
| `generator_config.yml` | tfplugingen-openapi config (paths + `schema.ignores`); generated from the manifest | ✅ committed |
| `spec/openapi.pruned.json` | Frozen, lightly-pruned OpenAPI input (504 paths) | ✅ committed |
| `crudgen.py` | Stage 4 engine — emits the CRUD Go, specs, tests, examples | ✅ committed |
| `preprocess_spec.py` | Stage 1 — spec transforms | ✅ committed |
| `postprocess_ir.py` | Stage 2.5 — IR cleanup | ✅ committed |
| `prune_spec.py` | Deny-list prune that produces `openapi.pruned.json` | ✅ committed |
| `regen.sh` | Orchestrator (default = stage 4; `--full` = stages 1–4) | ✅ committed |
| `spec/openapi.merged.json` | Transient full 780-path spec; only needed to re-run the prune | ❌ gitignored |
| `build/` | Stage-1 scratch (`openapi.hashicorp.json`) | ❌ gitignored |

## Generated vs static boundary

Almost everything in `internal/provider/` is **generated** and carries a
`DO NOT EDIT` header. Re-running `regen.sh` overwrites it. **Never hand-edit a
generated file** — make the change in the manifest / `crudgen.py` / `OVERRIDES`
and regenerate.

The **static, hand-written** files (never touched by regen) are:

- `internal/client/*.go` — the tfjson bridge (`UnwrapEnvelope`, camelCasing, the
  HTTP client) and its tests.
- `internal/provider/mock_test.go` — the in-process echo mock server used by the
  generated acceptance tests.
- `internal/provider/registry.go` — the provider's resource/data-source
  constructor lists (`providerResources()` / `providerDataSources()`). It was
  originally bootstrapped by a `gen_stubs.py` script that is no longer part of
  this pipeline; today it is **maintained by hand** — add your constructor here
  when you add an entity (see the runbook below). `crudgen.py` does not touch it.
- `main.go` — the provider entrypoint.

Everything else under `internal/provider/` (`*_resource.go`, `*_data_source.go`,
`*_spec.go`, `*_resource_test.go`, `crud_helpers.go`, the `*_gen.go` schema
packages) is generated.

## Regenerating

Both commands are run **from the provider repo root**. Build/test with
`GOWORK=off`.

### Default — stage 4 only (the common case)

```bash
./gen/regen.sh
```

Re-runs only `crudgen.py` against the **committed** `gen/provider_code_spec.json`.
Use this for a new entity, a CRUD/manifest tweak, or an `OVERRIDES` change.
Requires only `python3` + `gofmt` — **no external binaries**. Touches
`internal/provider/*.go` and `examples/`.

### Full — stages 1–4 (after a schema change)

```bash
./gen/regen.sh --full
```

Rebuilds the IR from the frozen spec too:
`preprocess → tfplugingen-openapi → postprocess → tfplugingen-framework → crudgen`.
Needed **only** when a schema *attribute* changes (i.e. after refreshing the
frozen spec). Requires HashiCorp's
[`tfplugingen-openapi`](https://github.com/hashicorp/terraform-plugin-codegen-openapi)
and
[`tfplugingen-framework`](https://github.com/hashicorp/terraform-plugin-codegen-framework)
on `PATH`.

### Toolchain

- Python 3 (standard library only — no pip deps).
- Go 1.25+.
- HashiCorp `tfplugingen-openapi` + `tfplugingen-framework` (free first-party;
  only for `--full`).

## How to add a new entity

1. **Add a record to `refined_manifest.json`** (in the `entities` array). The
   core fields, inferred from the existing records and `crudgen.resolve_entity`:

   | Field | Meaning |
   |---|---|
   | `short` | Original spec short-name / scope hint (bookkeeping). |
   | `keep` | `true` to generate; `false` documents why it's excluded (`drop_reason`). |
   | `resource_name` | The TF type name → `mixpanel_<resource_name>`. |
   | `id_attr` | The body field holding the server-assigned identity (usually `"id"`). |
   | `read_schema` | The component schema of the read response item. |
   | `create_req_schema` | The component schema of the create request body. |
   | `update_req_schema` | (Optional) update body schema, if different. |
   | `update` | Update verb: `"put"` / `"patch"` / `"post"`, or `null` (→ `ForceNew`). |
   | `delete` | `"delete"`, `"post"`, or `null` (no-op delete). |
   | `collection` | Templated create/list path (`/api/app/projects/{project_id}/...`). |
   | `instance` | Templated instance path with the id param (`.../{thing_id}`). |
   | `jsonencode_fields` | Dotted field paths that are polymorphic/dynamic → surfaced as JSON-string attrs. |
   | `collapse_fields` | Multi-type scalar fields to collapse to one scalar. |

   Scope / settings-singleton fields: `singleton`, `settings_singleton`, `scope`
   (`"project"` | `"org"`), `read_path`, `update_path`. A settings singleton has a
   base-path GET, a distinct `/update` POST with no request body, and is surfaced
   as one `settings` jsonencode passthrough (see `org_session_settings`).

   RPC-lifecycle fields (entities with no typed REST CRUD, e.g. `project`):
   `rpc_lifecycle`, `read_from_list`, `list_path`, `create_path`, `delete_path`,
   `create_name_key`, `id_list_key` (the request-body wrapper keys), and
   `create_match_attr`. See the `project` record for the canonical example.

2. **Classify tricky fields.**
   - Polymorphic `oneOf` / dynamic object → list it in `jsonencode_fields` (it
     becomes a JSON-string attribute, and is also dropped via `schema.ignores` in
     `generator_config.yml`).
   - Multi-type scalar (e.g. `anyOf[int,str]`) → list it in `collapse_fields`.
   - Discriminated-union *body root* → set `concrete_create_schema` /
     `concrete_read_schema`, or `synthetic_create_body` for a flat spread body.

3. **Register the constructor(s) in `internal/provider/registry.go`** (a
   hand-maintained file — see [Generated vs static](#generated-vs-static-boundary)).
   Add `New<Entity>Resource` to `providerResources()` and/or
   `New<Entity>DataSource` to `providerDataSources()`, grouped with the existing
   entries.

4. **Refresh the spec only if the endpoint is brand-new** (not already in
   `gen/spec/openapi.pruned.json`). See
   [Refreshing the frozen spec](#refreshing-the-frozen-spec). The prune asserts
   that every manifest path survives, so a missing endpoint fails loudly.

5. **Regenerate, build, test, live-verify.**

   ```bash
   ./gen/regen.sh            # or --full if a schema attribute changed
   GOWORK=off go build ./...
   GOWORK=off go test ./internal/...
   ```

   Then live-verify against a real project: `apply` → read-back → `destroy`,
   using a service account over Basic auth.

6. **Hand-curate the docs page.** The pages under `../docs/` are
   **hand-curated**, *not* raw `tfplugindocs` output. Write/update the page by
   hand; never blindly regenerate it.

## Refreshing the frozen spec

The frozen spec is intentionally **decoupled** from the live API — refreshing it
is a manual step:

1. Obtain the full merged OpenAPI spec from the webapp
   (`webapp/ninja/generated/openapi.merged.json`).
2. Drop it in as `gen/spec/openapi.merged.json` (this file is **gitignored**;
   only the pruned spec is committed).
3. Re-run the prune:

   ```bash
   python3 gen/prune_spec.py
   ```

   This regenerates `gen/spec/openapi.pruned.json`. `prune_spec.py` is a
   **deny-list** prune: it drops admin/internal, SCIM, auth plumbing/session,
   billing, destructive/GDPR (data-deletions/retrievals, project
   reset/delete/transfer, account-deletion), and per-user UI/assistant endpoints,
   while keeping the full config surface plus read-only audit-logs/usage/metadata.
   It enforces a **hard invariant**: every path the manifest references *must*
   survive the prune (the build fails otherwise). `components/schemas` are never
   pruned.
4. Rebuild everything:

   ```bash
   ./gen/regen.sh --full
   ```

## Known limitations / backlog

- **Discriminated-union bodies** and **map-keyed read responses** need
  preprocessor help (`concrete_*_schema`, `synthetic_create_body`, `results`-map
  peeling); a new endpoint of either shape will need a manifest hint before it
  generates cleanly.
- Some governance endpoints advertise a `$ref` but back it with an empty
  (`extra=allow`) model; these are surfaced via the settings-passthrough path.
- **Genuinely dynamic fields** are surfaced as `jsonencode` JSON-string
  attributes rather than typed nested schemas — the user supplies/reads JSON.
