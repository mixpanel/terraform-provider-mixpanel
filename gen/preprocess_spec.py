#!/usr/bin/env python3
"""Generalized preprocessor: pruned Ninja OpenAPI 3.1 -> HashiCorp-friendly spec.

Stage 1 of the full (--full) regen pipeline. Driven by gen/refined_manifest.json
(every refined entity) and reads the frozen, lightly-pruned spec at
gen/spec/openapi.pruned.json.

Two transforms, exactly the two HashiCorp constraints we violate:

  1. UNWRAP the BaseOkResponseModel.results envelope on EVERY refined entity's
     create/read/update 2xx response. HashiCorp merges the create-REQUEST root with
     the read-RESPONSE root to build the resource schema; it has no `results`
     unwrap. We rewrite each op's 2xx JSON schema from the wrapper to its inner
     `results` schema (handles both `results: {$ref}` and `results: {inline}`).

  2. COLLAPSE multi-type scalar anyOf/oneOf -> a single scalar (prefer string).
     anyOf[int,str,null] etc. is unsupported (only anyOf[X,null] nullable
     collapses automatically).

Genuinely polymorphic object/oneOf fields are NOT touched here; they are dropped
via `schema.ignores` in generator_config.yml (re-added as jsonencode strings in
the CRUD step).

Run:  python3 gen/preprocess_spec.py   (from the provider repo root)
Out:  gen/build/openapi.hashicorp.json
"""
import json
import os

HERE = os.path.dirname(os.path.abspath(__file__))  # <repo>/gen
SRC = os.path.join(HERE, "spec", "openapi.pruned.json")
MANIFEST = os.path.join(HERE, "refined_manifest.json")
OUT = os.path.join(HERE, "build", "openapi.hashicorp.json")
SCALARS = ("string", "integer", "number", "boolean")

# Collection/instance paths for entities whose manifest entry omits them (the
# manifest stores the resource short-name there for connectors, or leaves them
# blank). Keyed by resource_name. Verified against the merged spec.
PATH_OVERRIDES = {
    "service_account": (
        "/api/app/organizations/{organization_id}/service-accounts",
        "/api/app/organizations/{organization_id}/service-accounts/{serviceaccount_id}",
    ),
    "connector": (
        "/api/app/projects/{project_id}/connectors",
        "/api/app/projects/{project_id}/connectors/{connector_id}",
    ),
    "canvas": (
        "/api/app/projects/{project_id}/canvases",
        "/api/app/projects/{project_id}/canvases/{metric_tree_id}",
    ),
    "heat_map_collection": (
        "/api/app/projects/{project_id}/heat-maps/collection",
        "/api/app/projects/{project_id}/heat-maps/collection/{heat_map_collection_id}",
    ),
    "scim_group": (
        "/api/appscim/v2/Groups",
        "/api/appscim/v2/Groups/{group_scim_id}",
    ),
}


def load_entities():
    """Return [(resource_name, collection, instance, update_verb)] for refined keepers."""
    manifest = json.load(open(MANIFEST))
    out = []
    for e in manifest["entities"]:
        if not e.get("keep"):
            continue
        rn = e["resource_name"]
        coll = e.get("collection")
        inst = e.get("instance")
        if rn in PATH_OVERRIDES:
            coll, inst = PATH_OVERRIDES[rn]
        out.append((rn, coll, inst, e.get("update")))
    return out


def load_read_from_list():
    """Return [(resource_name, list_path, read_schema)] for read-from-list keepers.

    Read-from-list entities have no instance GET: their canonical read is the
    collection GET, whose 2xx response is a LIST wrapper. HashiCorp's generator
    needs the read response to be the single-object item schema, so we point the
    collection GET 2xx response directly at the entity's read_schema (the item
    type). The hand-written CRUD step selects the matching element at runtime."""
    manifest = json.load(open(MANIFEST))
    out = []
    for e in manifest["entities"]:
        if not e.get("keep") or not e.get("read_from_list"):
            continue
        list_path = e.get("list_path") or e.get("collection")
        out.append((e["resource_name"], list_path, e.get("read_schema")))
    return out


def point_read_from_list(spec, rfl):
    """Rewrite each read-from-list collection GET 2xx response to the item schema."""
    paths = spec["paths"]
    count = 0
    for rn, list_path, read_schema in rfl:
        if not list_path or not read_schema:
            continue
        op = paths.get(list_path, {}).get("get")
        if not op:
            continue
        for code, r in op.get("responses", {}).items():
            if not code.startswith("2"):
                continue
            content = r.get("content", {}).get("application/json", {})
            if "schema" not in content:
                continue
            content["schema"] = {"$ref": "#/components/schemas/%s" % read_schema}
            count += 1
    return count


def load_singletons():
    """Return [(resource_name, instance, read_schema, update_verb)] for singletons.

    A singleton has no collection POST that mints a server id and no id path
    segment: its synthetic id is the project. The GET and the POST share one path
    (the instance). Its GET 2xx response in the spec is a DIFFERENT root shape than
    the create-request body (e.g. the GET returns DataStandards while the POST body
    wraps it under `dataStandards`), which would make the framework merge two
    incompatible roots into the resource schema. We repoint the GET 2xx response to
    the entity's read_schema (== create_req_schema) so the read schema and the
    create schema are byte-identical; the hand-written CRUD step wraps the GET body
    back into that shape at runtime. Sourced from the manifest `singleton` flag."""
    manifest = json.load(open(MANIFEST))
    out = []
    for e in manifest["entities"]:
        if not e.get("keep") or not e.get("singleton"):
            continue
        out.append(
            (
                e["resource_name"],
                e.get("instance"),
                e.get("read_schema"),
                e.get("update"),
            )
        )
    return out


def point_singleton_read(spec, singletons):
    """Repoint a singleton's GET, POST (create) and update 2xx responses to its
    read_schema.

    A singleton's create/update is a POST whose response is a DIFFERENT, much
    richer model than the create request (e.g. DataGovernanceSettingsResponse vs the
    DataStandardsSettingsRequest body). HashiCorp merges the create REQUEST, the
    create RESPONSE and the read RESPONSE into one resource schema; leaving the rich
    POST response in place leaks dozens of response-only fields into the schema and
    drowns out the single request field. We point ALL of the singleton's 2xx
    responses (GET + POST + update) at the read_schema (== create_req_schema) so the
    resource schema is exactly the request shape; the hand-written CRUD reads back
    the canonical body via the GET and wraps it to match."""
    paths = spec["paths"]
    count = 0
    for rn, inst, read_schema, update in singletons:
        if not inst or not read_schema:
            continue
        methods = {"get", "post"}
        if update:
            methods.add(update.lower())
        for m in methods:
            op = paths.get(inst, {}).get(m)
            if not op:
                continue
            for code, r in op.get("responses", {}).items():
                if not code.startswith("2"):
                    continue
                content = r.get("content", {}).get("application/json", {})
                if "schema" not in content:
                    continue
                content["schema"] = {"$ref": "#/components/schemas/%s" % read_schema}
                count += 1
    return count


def results_subschema(schemas, name):
    """If `name` is a BaseOkResponseModel-style envelope, return its `results`
    schema as an inline OpenAPI node ({$ref} or the inline object), else None."""
    s = schemas.get(name, {})
    res = s.get("properties", {}).get("results")
    if not res:
        return None
    if "$ref" in res:
        return {"$ref": res["$ref"]}
    for b in res.get("anyOf", []):
        if "$ref" in b:
            return {"$ref": b["$ref"]}
    # inline results schema (e.g. additionalProperties map for Dict_str__Metric__)
    inner = {k: v for k, v in res.items() if k != "title"}
    if (
        inner.get("type")
        or inner.get("additionalProperties")
        or inner.get("properties")
    ):
        return inner
    return None


def unwrap_responses(spec, entities):
    """Point every refined entity op's 2xx JSON response at the unwrapped schema."""
    schemas = spec["components"]["schemas"]
    paths = spec["paths"]
    count = 0
    for rn, coll, inst, update in entities:
        ops = []
        if coll:
            ops.append((coll, "post"))
        if inst:
            ops.append((inst, "get"))
            if update:
                ops.append((inst, update))
        for path, m in ops:
            op = paths.get(path, {}).get(m)
            if not op:
                continue
            for code, r in op.get("responses", {}).items():
                if not code.startswith("2"):
                    continue
                content = r.get("content", {}).get("application/json", {})
                sch = content.get("schema")
                if not sch or "$ref" not in sch:
                    continue
                wrapper = sch["$ref"].split("/")[-1]
                inner = results_subschema(schemas, wrapper)
                if inner is not None:
                    content["schema"] = inner
                    count += 1
    return count


def collapse_multitype(node):
    """Recursively collapse anyOf/oneOf with >1 non-null SCALAR branch to one scalar.

    Leaves object / discriminated unions intact (those are `ignores`d in config).
    """
    if isinstance(node, dict):
        for key in ("anyOf", "oneOf"):
            if key in node:
                branches = [
                    b
                    for b in node[key]
                    if not (isinstance(b, dict) and b.get("type") == "null")
                ]
                scalar_types = [
                    b.get("type")
                    for b in branches
                    if isinstance(b, dict) and b.get("type") in SCALARS
                ]
                if (
                    branches
                    and len(scalar_types) == len(branches)
                    and len(branches) > 1
                ):
                    chosen = "string" if "string" in scalar_types else scalar_types[0]
                    nullable = any(
                        isinstance(b, dict) and b.get("type") == "null"
                        for b in node[key]
                    )
                    if nullable:
                        return {
                            "anyOf": [{"type": chosen}, {"type": "null"}],
                            "title": node.get("title", ""),
                        }
                    new = {"type": chosen}
                    if "title" in node:
                        new["title"] = node["title"]
                    return new
        return {k: collapse_multitype(v) for k, v in node.items()}
    if isinstance(node, list):
        return [collapse_multitype(v) for v in node]
    return node


def coerce_empty_schema(node):
    """Replace untyped/empty `{}` schemas (free-form dynamic JSON) with a string.

    tfplugingen-openapi cannot map an empty `{}` (no type/composition) and aborts
    the WHOLE resource on the first one. These fields are genuinely dynamic JSON;
    we surface them as `string` so schema generation always succeeds, and re-add
    them as `jsonencode` string attributes in the hand-written CRUD step (string
    is the compatible TF type). Handles bare `{}` property values and empty
    branches inside anyOf/oneOf (e.g. anyOf[{}, null] -> string|null).
    """
    if isinstance(node, dict):
        for key in ("anyOf", "oneOf"):
            if key in node and any(isinstance(b, dict) and b == {} for b in node[key]):
                nullable = any(
                    isinstance(b, dict) and b.get("type") == "null" for b in node[key]
                )
                if nullable:
                    return {
                        "anyOf": [{"type": "string"}, {"type": "null"}],
                        "title": node.get("title", ""),
                    }
                return {"type": "string", "title": node.get("title", "")}
        return {k: coerce_empty_schema(v) for k, v in node.items()}
    if isinstance(node, list):
        return [coerce_empty_schema(v) for v in node]
    return node


def load_concrete_roots():
    """Return [(resource_name, coll, inst, update, create_schema, read_schema)].

    For entities whose create-REQUEST root and/or read-RESPONSE root is a
    discriminated/polymorphic oneOf (e.g. metric: MetricsRequest oneOf
    BehaviorMetricRequest/FormulaMetricRequest, and Metric oneOf
    BehaviorMetric/FormulaMetric), tfplugingen-openapi cannot build the resource
    schema at all ("unsupported multi-type, attribute cannot be created") and the
    whole resource is skipped. When the manifest provides concrete_create_schema
    and/or concrete_read_schema, we repoint that entity's request body and 2xx
    responses to the single concrete variant. The variants must be field-identical
    apart from fields that are `ignores`d/jsonencode'd in the CRUD step (e.g. metric
    `definition`), so the chosen variant is a lossless stand-in for the union."""
    manifest = json.load(open(MANIFEST))
    out = []
    for e in manifest["entities"]:
        if not e.get("keep"):
            continue
        cc = e.get("concrete_create_schema")
        cr = e.get("concrete_read_schema")
        if not cc and not cr:
            continue
        rn = e["resource_name"]
        coll = e.get("collection")
        inst = e.get("instance")
        if rn in PATH_OVERRIDES:
            coll, inst = PATH_OVERRIDES[rn]
        out.append((rn, coll, inst, e.get("update"), cc, cr))
    return out


def point_concrete_roots(spec, concretes):
    """Repoint polymorphic-root entities to their concrete create/read schemas.

    Runs BEFORE unwrap_responses so the concrete read schema replaces the
    envelope/oneOf/dict-map response entirely (unwrap then finds a plain $ref and
    leaves it). Repoints: the collection POST request body (concrete_create), and
    every 2xx JSON response on the create/read/update ops (concrete_read)."""
    paths = spec["paths"]
    count = 0
    for rn, coll, inst, update, cc, cr in concretes:
        ops = []
        if coll:
            ops.append((coll, "post"))
        if inst:
            ops.append((inst, "get"))
            if update:
                ops.append((inst, update))
        for path, m in ops:
            op = paths.get(path, {}).get(m)
            if not op:
                continue
            if cc and m == "post":
                body = (
                    op.get("requestBody", {})
                    .get("content", {})
                    .get("application/json", {})
                )
                if "schema" in body:
                    body["schema"] = {"$ref": "#/components/schemas/%s" % cc}
                    count += 1
            if cr:
                for code, r in op.get("responses", {}).items():
                    if not code.startswith("2"):
                        continue
                    content = r.get("content", {}).get("application/json", {})
                    if "schema" not in content:
                        continue
                    content["schema"] = {"$ref": "#/components/schemas/%s" % cr}
                    count += 1
    return count


def load_read_repoints():
    """Return [(resource_name, collection, instance, read_schema, update)] for
    entities whose responses must be repointed to a NAMED schema.

    Some entities (e.g. behaviors) have a POLYMORPHIC create body and a fully
    UNTYPED response envelope (BaseOkResponseModel[dict] whose `results` is an
    open object) -- there is no typed read entity in the spec. The framework
    cannot build a useful resource schema from an open object. We repoint the
    collection POST, the instance GET and the instance update 2xx responses to
    the entity's read_repoint_schema (== create_req_schema) so the resource
    schema is exactly the request shape; the hand-written CRUD reads the id back
    from the (untyped) create response (read_after_create) and GETs the instance.
    Sourced from the manifest `read_repoint_schema` field. This is the non-
    singleton analogue of point_singleton_read (the entity HAS a server-minted id
    and an instance path)."""
    manifest = json.load(open(MANIFEST))
    out = []
    for e in manifest["entities"]:
        if not e.get("keep") or not e.get("read_repoint_schema"):
            continue
        coll = e.get("collection")
        inst = e.get("instance")
        if e["resource_name"] in PATH_OVERRIDES:
            coll, inst = PATH_OVERRIDES[e["resource_name"]]
        out.append(
            (
                e["resource_name"],
                coll,
                inst,
                e.get("read_repoint_schema"),
                e.get("update"),
            )
        )
    return out


def point_read_repoint(spec, repoints):
    """Repoint a read_repoint entity's collection POST + instance GET + update 2xx
    responses to its named read_repoint_schema."""
    paths = spec["paths"]
    count = 0
    for rn, coll, inst, read_schema, update in repoints:
        if not read_schema:
            continue
        ops = []
        if coll:
            ops.append((coll, "post"))
        if inst:
            ops.append((inst, "get"))
            if update:
                ops.append((inst, update.lower()))
        for path, m in ops:
            op = paths.get(path, {}).get(m)
            if not op:
                continue
            for code, r in op.get("responses", {}).items():
                if not code.startswith("2"):
                    continue
                content = r.get("content", {}).get("application/json", {})
                if "schema" not in content:
                    continue
                content["schema"] = {"$ref": "#/components/schemas/%s" % read_schema}
                count += 1
    return count


def load_synthetic_create_bodies():
    """Return [(resource_name, collection, instance, update_verb, synth)] for
    entities with a `synthetic_create_body` manifest hint.

    Some create/update request bodies are a DISCRIMINATED `oneOf` of N variant
    schemas (e.g. a warehouse source's bigquery/snowflake/redshift/databricks/
    postgres connection config keyed by `warehouse_type`). The HashiCorp generator
    cannot model schema composition and SKIPS the whole resource. We replace the
    polymorphic body root with a single flat synthetic schema: a few typed scalars
    that are common to every variant (and to the read response) plus one
    `format: json-object` string field that carries all the variant-specific config
    verbatim. The hand-written CRUD step spreads that json-string back into the body
    root on the wire (see AttrSpec.SpreadAttrs). `synth` is the manifest dict
    {"scalars": {name: required_bool}, "spread": "<field>"}."""
    manifest = json.load(open(MANIFEST))
    out = []
    for e in manifest["entities"]:
        if not e.get("keep") or not e.get("synthetic_create_body"):
            continue
        rn = e["resource_name"]
        coll, inst = e.get("collection"), e.get("instance")
        if rn in PATH_OVERRIDES:
            coll, inst = PATH_OVERRIDES[rn]
        out.append((rn, coll, inst, e.get("update"), e["synthetic_create_body"]))
    return out


def synth_body_schema(synth):
    """Build a flat OpenAPI object schema from a synthetic_create_body hint."""
    props = {}
    required = []
    for name, req in (synth.get("scalars") or {}).items():
        props[name] = {"type": "string", "title": ""}
        if req:
            required.append(name)
    spread = synth.get("spread")
    if spread:
        # format:json-object => crudgen treats it as a verbatim JSON-string
        # passthrough; SpreadAttrs (set in crudgen OVERRIDES) then spreads it.
        # The spread field is deliberately NOT required: the GET never echoes it back
        # (connection config / credentials are write-only), so Read merges it from
        # prior state. A Required attr cannot round-trip `terraform import` (no prior
        # state -> null -> the framework rejects the plan). Optional lets import
        # succeed; the user supplies the connection config on the next apply.
        props[spread] = {"type": "string", "format": "json-object", "title": ""}
    schema = {
        "type": "object",
        "properties": props,
        "additionalProperties": False,
        "title": "SyntheticBody",
    }
    if required:
        schema["required"] = required
    return schema


def replace_synthetic_create_bodies(spec, synth_entities):
    """Repoint each entity's polymorphic create POST (and update PATCH) request body
    to an inline flat synthetic schema, so the framework can build the resource."""
    paths = spec["paths"]
    count = 0
    for rn, coll, inst, update, synth in synth_entities:
        inline = synth_body_schema(synth)
        ops = []
        if coll:
            ops.append((coll, "post"))
        if inst and update:
            ops.append((inst, update.lower()))
        for path, m in ops:
            op = paths.get(path, {}).get(m)
            if not op:
                continue
            content = (
                op.get("requestBody", {}).get("content", {}).get("application/json", {})
            )
            if "schema" not in content:
                continue
            content["schema"] = json.loads(json.dumps(inline))
            count += 1
    return count


# Synthetic component injected for settings singletons (see inject_settings_schema).
# A uniform single-`settings` string property: the whole loose/untyped settings
# object is surfaced to Terraform as one jsonencode passthrough string attribute.
SETTINGS_PASSTHROUGH_SCHEMA = "MxpSettingsPassthrough"


def inject_settings_schema(spec):
    """Add the MxpSettingsPassthrough component and repoint each settings-singleton
    READ GET 2xx response to it.

    The settings singletons (twofactor, *-session-settings, *-settings) have loose /
    untyped result schemas (additionalProperties:true, mixed empty-branch anyOf) and a
    WRITE endpoint with NO request body. Rather than expand their unmodelable result
    fields, every one is surfaced as a single `settings` jsonencode string attribute
    holding the whole settings object. We point each READ GET response at a synthetic
    schema with exactly one `settings` string property; the field is ignored in
    generator_config and re-injected as a jsonencode passthrough in the CRUD step.
    The scope attribute (project_id / organization_id) still comes from the path
    parameter. Returns the number of READ responses repointed."""
    schemas = spec["components"]["schemas"]
    schemas[SETTINGS_PASSTHROUGH_SCHEMA] = {
        "type": "object",
        "additionalProperties": False,
        "properties": {"settings": {"type": "string", "title": ""}},
        "title": SETTINGS_PASSTHROUGH_SCHEMA,
    }
    manifest = json.load(open(MANIFEST))
    paths = spec["paths"]
    ref = {"$ref": "#/components/schemas/%s" % SETTINGS_PASSTHROUGH_SCHEMA}
    count = 0
    for e in manifest["entities"]:
        if not e.get("keep") or not e.get("settings_singleton"):
            continue
        rpath = e.get("read_path") or e.get("collection")
        op = paths.get(rpath, {}).get("get")
        if op:
            for code, r in op.get("responses", {}).items():
                if not code.startswith("2"):
                    continue
                content = r.get("content", {}).get("application/json", {})
                if "schema" not in content:
                    continue
                content["schema"] = dict(ref)
                count += 1
        # The WRITE (update) endpoint has NO request body in the spec, so the
        # generator would report "no compatible schema found" (no settable attrs).
        # Give it a request body of the synthetic single-`settings` schema so the
        # resource is built with a writable `settings` attribute. The actual payload
        # is the decoded settings object, sent by the hand-written CRUD step.
        upath = e.get("update_path")
        wop = paths.get(upath, {}).get("post") if upath else None
        if wop is not None:
            wop["requestBody"] = {
                "required": True,
                "content": {"application/json": {"schema": dict(ref)}},
            }
            count += 1
            # Also repoint the write RESPONSE: the org update endpoints return the rich
            # BaseOkResponseModel_OrgSettingsResponse_ (results/status), which the
            # generator would merge into the resource schema and leak response-only
            # attributes. Point every 2xx write response at MxpSettingsPassthrough so
            # the merged resource schema is exactly {settings, scope}.
            for code, r in wop.get("responses", {}).items():
                if not code.startswith("2"):
                    continue
                content = r.get("content", {}).get("application/json", {})
                if "schema" not in content:
                    continue
                content["schema"] = dict(ref)
                count += 1
    return count


def inject_rpc_lifecycle(spec):
    """Synthesize a typed schema + create requestBody for RPC-lifecycle entities.

    Project lifecycle is exposed only through org-scoped RPC verbs whose request
    and response bodies are UNTYPED in the merged spec (OrgProjectsResponse is an
    open `additionalProperties:true` object, and create/delete carry no
    requestBody). HashiCorp's generator needs (a) a create request body to derive
    the settable attributes and (b) a read response item schema to derive the
    computed outputs. We inject a single synthetic ProjectLifecycleModel schema
    (name = the user-settable create input; id/token/api_key/api_secret/url =
    server outputs) and attach it as the create POST's requestBody. The list GET
    2xx response is repointed to the same schema by the read-from-list pass (its
    read_schema == ProjectLifecycleModel), so the merged resource schema is
    exactly this shape. Driven by the manifest `rpc_lifecycle` flag."""
    manifest = json.load(open(MANIFEST))
    schemas = spec["components"]["schemas"]
    paths = spec["paths"]
    count = 0
    for e in manifest["entities"]:
        if not e.get("keep") or not e.get("rpc_lifecycle"):
            continue
        model = e.get("read_schema")
        coll = e.get("collection")
        if not model or not coll:
            continue
        # Synthetic READ item schema (read_schema): `name` is the only user-settable
        # field; the rest are server-assigned outputs surfaced by the list serializer.
        # The generator merges the create REQUEST root with the read RESPONSE root, so
        # keeping the outputs OUT of the request schema makes them pure-computed
        # (not computed_optional) and excludes them from the ForceNew set.
        schemas[model] = {
            "type": "object",
            "title": model,
            "additionalProperties": False,
            "required": ["name"],
            "properties": {
                "name": {"type": "string", "title": "Name"},
                "id": {"type": "integer", "title": "Id"},
                "token": {"type": "string", "title": "Token"},
                "api_key": {"type": "string", "title": "Api Key"},
                "api_secret": {"type": "string", "title": "Api Secret"},
                "url": {"type": "string", "title": "Url"},
                "timezone_name": {"type": "string", "title": "Timezone Name"},
            },
        }
        # Synthetic CREATE request schema: only the settable `name`. Attaching this
        # (not the full model) as the create POST requestBody keeps id/token/api_*/url
        # response-only -> pure-computed in the merged resource schema.
        req_model = "%sCreateRequest" % model
        schemas[req_model] = {
            "type": "object",
            "title": req_model,
            "additionalProperties": False,
            "required": ["name"],
            "properties": {"name": {"type": "string", "title": "Name"}},
        }
        op = paths.get(coll, {}).get("post")
        if op is not None:
            op["requestBody"] = {
                "required": True,
                "content": {
                    "application/json": {
                        "schema": {"$ref": "#/components/schemas/%s" % req_model}
                    }
                },
            }
            count += 1
    return count


def main():
    spec = json.load(open(SRC))
    # settings singletons: inject synthetic passthrough schema before anything else
    n_settings_schema = inject_settings_schema(spec)
    entities = load_entities()
    # project lifecycle: synthesize RPC create/read schema + attach to spec
    n_rpc = inject_rpc_lifecycle(spec)
    # metric (+ any concrete-root entity): repoint oneOf create/read roots to a
    # concrete variant the IR generator can model
    concretes = load_concrete_roots()
    n_concrete = point_concrete_roots(spec, concretes)
    n_unwrap = unwrap_responses(spec, entities)
    rfl = load_read_from_list()
    n_rfl = point_read_from_list(spec, rfl)
    singletons = load_singletons()
    n_singleton = point_singleton_read(spec, singletons)
    # behaviors (+ any non-singleton polymorphic entity): repoint untyped reads
    repoints = load_read_repoints()
    n_repoint = point_read_repoint(spec, repoints)
    # warehouse_source (+ any spread-create entity): replace collapsed polymorphic
    # create bodies with a synthetic spread schema
    synth_entities = load_synthetic_create_bodies()
    n_synth = replace_synthetic_create_bodies(spec, synth_entities)
    spec["components"]["schemas"] = coerce_empty_schema(spec["components"]["schemas"])
    spec["paths"] = coerce_empty_schema(spec["paths"])
    spec["components"]["schemas"] = collapse_multitype(spec["components"]["schemas"])
    spec["paths"] = collapse_multitype(spec["paths"])
    json.dump(spec, open(OUT, "w"), indent=2)
    print(
        "entities=%d; settings-schema=%d; rpc-lifecycle=%d; concrete-roots=%d (%d repointed); "
        "unwrapped %d entity responses; read-from-list=%d (%d responses); "
        "singletons=%d (%d responses); read-repoints=%d (%d responses); "
        "synthetic-create-bodies=%d (%d ops); collapsed multi-type scalars"
        % (
            len(entities),
            n_settings_schema,
            n_rpc,
            len(concretes),
            n_concrete,
            n_unwrap,
            len(rfl),
            n_rfl,
            len(singletons),
            n_singleton,
            len(repoints),
            n_repoint,
            len(synth_entities),
            n_synth,
        )
    )
    print("wrote", OUT)


if __name__ == "__main__":
    main()
