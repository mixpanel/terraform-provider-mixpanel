#!/usr/bin/env python3
"""Deterministic CRUD generator for the Mixpanel Terraform provider.

For every entity in the refined manifest that is also present in the framework
provider-code-spec (provider_code_spec.json), this emits:

  * an editable resource file <entity>_resource.go implementing
    Create/Read/Update/Delete/Metadata/Schema/Configure/ImportState, and
  * a singular data source <entity>_data_source.go implementing
    Metadata/Schema/Configure/Read.

Design (see internal/client/tfjson.go for the bridge):

  * Schema  = the generated <Entity>ResourceSchema(ctx), with top-level
    jsonencode_fields injected as Optional+Computed types.String attributes
    (they were dropped during framework generation because they are dynamic /
    oneOf). The id identity attribute is forced Computed.
  * Model   = handled at the tftypes.Value level (req.Plan.Raw / req.State.Raw)
    rather than via the typed <Entity>Model, because the injected jsonencode
    attributes are not present on the generated struct. This keeps a single,
    generic (un)marshalling path that works for every entity.
  * CRUD    = POST collection (create) / GET instance (read) /
    PUT|PATCH instance (update, or ForceNew when no update verb) /
    DELETE instance (delete, tolerating a JSON body). Every request body is the
    UNENVELOPED object; every response is passed through client.UnwrapEnvelope
    before being turned back into state.
  * Identity= after create the id is read from the response (id_attr, possibly a
    nested json path) and stored on the path-param attribute, which is the
    instance-path key and the import id. It is exposed as a computed attribute.

Run:  python3 gen/crudgen.py   (from the provider repo root; see gen/README.md)
"""
import json
import os
import re

# Layout: this file lives at <repo>/gen/crudgen.py. Generated Go lands in
# <repo>/internal/provider; the IR + manifest + frozen spec live alongside in gen/.
HERE = os.path.dirname(os.path.abspath(__file__))  # <repo>/gen
REPO_ROOT = os.path.dirname(HERE)  # provider repo root
IR = os.path.join(HERE, "provider_code_spec.json")
MANIFEST = os.path.join(HERE, "refined_manifest.json")
OUTDIR = os.path.join(REPO_ROOT, "internal", "provider")
EXAMPLES_DIR = os.path.join(REPO_ROOT, "examples")
MODULE = "github.com/mixpanel/terraform-provider-mixpanel"

# Frozen, lightly-pruned OpenAPI spec (see gen/prune_spec.py). tfplugingen forces
# Terraform attribute names to snake_case, but the Mixpanel API wire keys are
# camelCase for many fields (e.g. resourceType, displayFormula). We read the
# ORIGINAL property names from this spec so the bridge can emit/read the correct
# camelCase wire keys for EVERY attribute, not just the jsonencode passthroughs.
# Without this, scalar attrs (resource_type, display_formula, ...) are sent
# verbatim in snake_case and the API drops them (e.g. 400 "Missing required
# parameter: resourceType"). NOTE: only components/schemas are read here, never
# paths -- so the path-pruning in prune_spec.py cannot affect this step's output.
# This file is gitignored (the spec is never committed); fetch + prune it locally
# before regenerating -- see gen/README.md "Refreshing the frozen spec".
MERGED_SPEC = os.path.join(HERE, "spec", "openapi.pruned.json")


def pascal(n):
    return "".join(p.capitalize() for p in n.split("_"))


def snake(n):
    """Convert a (possibly camelCase) API field name into a Terraform-legal
    snake_case attribute name. Terraform requires attribute names to match
    ^[a-z][a-z0-9_]*$, so jsonencode fields whose API key is camelCase
    (e.g. composedProperties) must be exposed under a snake_case alias
    (composed_properties) while the original key is preserved on the wire."""
    s = re.sub(r"[-\s]+", "_", n)
    s = re.sub(r"(.)([A-Z][a-z]+)", r"\1_\2", s)
    s = re.sub(r"([a-z0-9])([A-Z])", r"\1_\2", s)
    s = re.sub(r"__+", "_", s)
    return s.lower()


# ---------------------------------------------------------------------------
# Path / identity overrides for entities whose manifest collection/instance is
# null or non-templated. Derived from the manifest notes. {project_id} and the
# id path-param are substituted at runtime by the generated Go.
# ---------------------------------------------------------------------------
OVERRIDES = {
    # feature_flag's project-only create/CRUD route is gated by a
    # require_workspace_is_set decorator on the backend: on that route
    # views._create_feature_flag_impl is invoked with workspace=None and the
    # request 400s with {"error":"Workspace is required"}. The functioning CRUD
    # route is the workspace-bearing variant. workspace_scoped=True makes the
    # generator template {workspace_id} into collection/instance and resolve it
    # at runtime (explicit workspace_id attr if the user set one, else the
    # project's global "All Project Data" workspace via the workspaces list).
    "feature_flag": {
        "collection": "/api/app/projects/{project_id}/workspaces/{workspace_id}/feature-flags",
        "instance": "/api/app/projects/{project_id}/workspaces/{workspace_id}/feature-flags/{flag_id}",
        "id_param": "flag_id",
        "update": "put",
        "delete": "delete",
        "project_scoped": True,
        "workspace_scoped": True,
        "enveloped": True,
    },
    # The custom_events POST/PUT views read request.POST (form fields), not a
    # JSON body: sending JSON returns 400 "missing required parameters". The
    # slash-less collection/instance paths also 301-redirect to their trailing
    # -slash form (Go would downgrade the redirected POST->GET, hit the list
    # route, get an empty body, and Terraform reports "inconsistent result
    # after apply"), so both paths carry a trailing slash. form_encoded=True
    # routes create/update through client.DoForm (application/x-www-form-
    # urlencoded; non-string fields such as `alternatives` are JSON-encoded,
    # which the view json.loads server-side).
    "custom_event": {
        "collection": "/api/app/custom_events/{project_id}/",
        "instance": "/api/app/custom_events/{project_id}/{customevent_id}/",
        "id_param": "customevent_id",
        "update": "put",
        "delete": "delete",
        "project_scoped": True,
        "enveloped": False,
        "form_encoded": True,
        # custom_event.to_json() returns SNAKE_case keys (custom_event,
        # is_visibility_restricted, ...), but the bridge camelCases wire keys
        # by default. Force the top-level response key verbatim so the nested
        # custom_event object is read back (otherwise it resolves to null and
        # trips "inconsistent result after apply").
        "wire_key_overrides": {"custom_event": "custom_event"},
    },
    "canvas": {
        "collection": "/api/app/projects/{project_id}/canvases",
        "instance": "/api/app/projects/{project_id}/canvases/{canvas_id}",
        "id_param": "canvas_id",
        "update": "patch",
        "delete": "delete",
        "project_scoped": True,
        "enveloped": True,
    },
    # lexicon_tag (data-definitions tags) has NO DELETE endpoint in the API (would 405).
    # Mark it as singleton-style to generate a no-op Delete that removes from state only,
    # orphaning the tag on the server - acceptable for metadata. Still supports create/update/read.
    "lexicon_tag": {
        "collection": "/api/app/projects/{project_id}/data-definitions/tags",
        "instance": "/api/app/projects/{project_id}/data-definitions/tags/{tag_id}",
        "id_param": "tag_id",
        "update": "patch",
        "delete": None,  # No DELETE endpoint - orphan on destroy
        "project_scoped": True,
        "enveloped": True,
    },
    # custom_property's CustomPropertyCreateRequest / CustomPropertyResponse use
    # camelCase wire keys (dataGroupId, displayFormula, ...), but the bridge writes
    # wire keys VERBATIM (AttrSpec.wireKey returns the snake_case TF attr name unless
    # pinned). Without these aliases the writable snake_case attrs are sent under the
    # wrong key (the API silently drops them) and the camelCase response fields are
    # never matched on Read (they land null -> permanent drift after import). Pin the
    # true camelCase wire name for every diverging property. composed_properties /
    # display_options are already aliased by the jsonencode path; the rest are here.
    # See READ-NORMALIZATION audit finding #1.
    "custom_property": {
        "wire_key_overrides": {
            # writable (CustomPropertyCreateRequest)
            "data_group_id": "dataGroupId",
            "display_formula": "displayFormula",
            "example_value": "exampleValue",
            "global_access_type": "globalAccessType",
            "is_locked": "isLocked",
            "is_visible": "isVisible",
            "resource_type": "resourceType",
            # computed-only (CustomPropertyResponse) -- so Read populates them
            "can_update_basic": "canUpdateBasic",
            "is_session_scoped": "isSessionScoped",
            "join_property_type": "joinPropertyType",
            "mapped_data_group_id": "mappedDataGroupId",
            "property_type": "propertyType",
            "referenced_by": "referencedBy",
            "referenced_directly_by": "referencedDirectlyBy",
            "referenced_raw_event_properties": "referencedRawEventProperties",
            "references_borrowed_property": "referencesBorrowedProperty",
        },
    },
    "connector": {
        "collection": "/api/app/projects/{project_id}/connectors",
        "instance": "/api/app/projects/{project_id}/connectors/{connector_id}",
        "id_param": "connector_id",
        "update": None,
        "delete": "delete",
        "project_scoped": True,
        "enveloped": True,
    },
    "heat_map_collection": {
        "collection": "/api/app/projects/{project_id}/heat-maps/collection",
        "instance": "/api/app/projects/{project_id}/heat-maps/collection/{heat_map_collection_id}",
        "id_param": "heat_map_collection_id",
        "update": "put",
        "delete": "delete",
        "project_scoped": True,
        "enveloped": True,
    },
    "scim_group": {
        "collection": "/api/appscim/v2/Groups",
        "instance": "/api/appscim/v2/Groups/{group_scim_id}",
        "id_param": "group_scim_id",
        "update": "put",
        "delete": "delete",
        "project_scoped": False,
        "enveloped": False,
    },
    "service_account": {
        # Service accounts are ORGANIZATION-scoped, not project-scoped. The real
        # CRUD surface is the org router: POST (create) / GET (list) on the
        # collection and GET / DELETE on the instance, all requiring
        # {organization_id} in the URL. The /api/app/projects/{project_id}/
        # service-accounts route is GET-only (list) and cannot create or delete.
        # The instance path param is `serviceaccount_id` (no underscore), matching
        # the Ninja spec.
        "collection": "/api/app/organizations/{organization_id}/service-accounts",
        "instance": "/api/app/organizations/{organization_id}/service-accounts/{serviceaccount_id}",
        "id_param": "serviceaccount_id",
        "update": None,
        "delete": "delete",
        "project_scoped": False,
        "org_scoped": True,
        "enveloped": True,
    },
    # Every themes operation (create / get / patch / list-entry) returns its
    # envelope `results` as a MAP of id -> theme object (themes_to_dict_map:
    # {theme.id: theme.to_json()}), even for a single-theme create/get -- the
    # response is {"status":"ok","results":{"3097":{...theme...}}}. The default
    # unwrap returns that outer {id: object} map as the wire body, so id
    # extraction finds no top-level `id` (theme_id lands null in state, orphaning
    # the resource) and the state merge sees the wrong shape. results_map=True
    # peels the single-entry id->object map: the inner object becomes the wire
    # body and the map key is injected as a synthetic `id` so the existing id
    # path (nestedID(wire,"id")) resolves it.
    "theme": {
        "collection": "/api/app/projects/{project_id}/themes",
        "instance": "/api/app/projects/{project_id}/themes/{theme_id}",
        "id_param": "theme_id",
        "update": "patch",
        "delete": "delete",
        "project_scoped": True,
        "enveloped": True,
        "results_map": True,
    },
    # event_definition: POST /event-definitions takes a batch body that wraps a
    # `definitions` array, but the create RESPONSE is BaseOkResponseModel_List_dict__
    # -- the created event comes back as FLAT fields ({id,name,description,verified,
    # ...}) inside the results LIST, NOT echoed under the nested `definitions` list
    # the request used. The generic create path therefore found no top-level id
    # (id/name/definitions landed null -> "inconsistent result after apply") and a
    # subsequent destroy DELETEd an empty id (405). read_after_create=True makes
    # Create extract the numeric id from the FLAT create response (id_attr at
    # results.id, list-tolerant) and then populate state via the normal instance
    # GET (EventDefinitionResult), so state.id is the server id and the instance
    # path is correct for read/update/delete.
    "event_definition": {
        "collection": "/api/app/projects/{project_id}/event-definitions",
        "instance": "/api/app/projects/{project_id}/event-definitions/{event_definition_id}",
        "id_param": "event_definition_id",
        "update": "patch",
        "delete": "delete",
        "project_scoped": True,
        "enveloped": True,
        "read_after_create": True,
    },
    # dataset is a CLIENT-ID UPSERT: the dataset_id is supplied by the
    # configuration (not server-assigned), and BOTH create and update are a POST
    # to the INSTANCE path /api/app/projects/{project_id}/datasets/{dataset_id}
    # (the collection has GET only). create_to_instance=True makes the generated
    # Create read the client-supplied identity from the plan and POST to the
    # instance path instead of the collection. update=post mirrors the same POST.
    "dataset": {
        "collection": "/api/app/projects/{project_id}/datasets",
        "instance": "/api/app/projects/{project_id}/datasets/{dataset_id}",
        "id_param": "dataset_id",
        "update": "post",
        "delete": "delete",
        "project_scoped": True,
        "enveloped": True,
        "create_to_instance": True,
    },
    # event_drop_filter: COLLECTION-PATH BODY-ID CRUD. Every verb (GET/POST/PATCH/
    # DELETE) lives on the collection path /api/app/projects/{project_id}/
    # data-definitions/events/drop-filters; the id is carried in the JSON body
    # (DELETE also takes a body). There is NO instance path. create POSTs the body
    # (no id) to the collection; the response is an enveloped list and the created
    # filter is selected from it by matching event_name (newest = max id). read GETs
    # the collection and selects the element whose id matches (read_from_list).
    # update PATCHes the collection with the id injected into the body; delete
    # DELETEs the collection with {"id": <id>} as the body. collection_body_id=True
    # selects the dedicated create/update/delete templates that build a body and
    # inject the id rather than templating it into the URL.
    "event_drop_filter": {
        "collection": "/api/app/projects/{project_id}/data-definitions/events/drop-filters",
        "instance": "/api/app/projects/{project_id}/data-definitions/events/drop-filters",
        "id_param": "id",
        "update": "patch",
        "delete": "delete",
        "project_scoped": True,
        "enveloped": True,
        "read_from_list": True,
        "list_path": "/api/app/projects/{project_id}/data-definitions/events/drop-filters",
        "collection_body_id": True,
        # The create response is the full list; select the created item by matching
        # this attr against the planned value (event_name is unique per filter:
        # duplicate event_name+filters are rejected server-side), choosing the
        # numerically-largest id (the just-created row).
        "create_match_attr": "event_name",
    },
    # data_governance_settings: project SINGLETON. No collection POST that mints a
    # server id and no id path segment -- the synthetic id IS the project id. create
    # and update are both a POST to the settings path; read is the GET on the same
    # path; delete is a NO-OP (the settings are project-global and cannot be
    # "deleted", so destroy simply drops the resource from state without mutating
    # the API). singleton=True selects the dedicated create=update POST-to-instance
    # templates, the synthetic-id read, and the no-op delete. The GET returns the
    # DataStandards root; read_wrap_key re-wraps it under the request's top key
    # (dataStandards -> data_standards) so it matches the (request-derived) schema.
    "data_governance_settings": {
        "collection": "/api/app/projects/{project_id}/data-governance/data-standards/settings",
        "instance": "/api/app/projects/{project_id}/data-governance/data-standards/settings",
        "id_param": "id",
        "update": "post",
        "delete": None,
        "project_scoped": True,
        "enveloped": True,
        "singleton": True,
        # The GET body (DataStandards root) is wrapped under this snake_case attr to
        # match the request-derived schema (which has a single `data_standards`).
        "read_wrap_key": "data_standards",
    },
    # metric: PROMOTED from data-source to a full CRUD resource. The Ninja create
    # body (MetricsRequest) and read entity (Metric) are both discriminated oneOf
    # unions (BehaviorMetric*/FormulaMetric*, propertyName=type) that
    # tfplugingen-openapi cannot model; preprocess_spec repoints them to the
    # concrete BehaviorMetricRequest/BehaviorMetric variants (lossless: the variants
    # differ only in `definition`, which is ignored/jsonencode'd). The wire response
    # is the standard envelope whose `results` is a single-entry id->object map
    # ({"status":"ok","results":{"<metric_id>":{...metric...}}}) -- identical to
    # themes -- so results_map=True peels the inner object and injects the map key as
    # the synthetic `id` (id_attr=id, path param metric_id maps to entity id).
    "metric": {
        "collection": "/api/app/projects/{project_id}/metrics",
        "instance": "/api/app/projects/{project_id}/metrics/{metric_id}",
        "id_param": "metric_id",
        "update": "patch",
        "delete": "delete",
        "project_scoped": True,
        "enveloped": True,
        "results_map": True,
    },
    # warehouse_source: inbound WHC connector. The create/update request body is a
    # DISCRIMINATED `oneOf` of 5 warehouse-type variant schemas (bigquery/snowflake/
    # redshift/databricks/postgres connection config keyed by warehouse_type), which
    # the HashiCorp generator cannot model -- it skipped the whole resource. We
    # collapse the polymorphic body to a flat synthetic schema in preprocess_spec.py
    # (typed scalars source_name + warehouse_type, plus a `params` json-object string
    # carrying the variant-specific config). `spread` lists the jsonencode attrs whose
    # decoded object is merged into the body ROOT on the wire (not nested under the
    # attr key), so the flat connection fields land where the API expects them.
    # The create RESPONSE is the FLAT enveloped WarehouseSourceResponse (id-bearing),
    # which does NOT echo the connection config, so read_after_create=True extracts
    # the integer id from the create response then re-reads via the instance GET to
    # build state; the spread `params` is preserved from the plan by the merge-base
    # read path (the GET never returns it). id_param=source_id templates the URL; the
    # identity attr is the response body `id`.
    "warehouse_source": {
        "collection": "/api/app/projects/{project_id}/warehouse-sources/sources",
        "instance": "/api/app/projects/{project_id}/warehouse-sources/sources/{source_id}",
        "id_param": "source_id",
        "update": "patch",
        "delete": "delete",
        "project_scoped": True,
        "enveloped": True,
        "read_after_create": True,
        "id_json_path": "id",
        "spread": ["params"],
    },
    # project: ORG-scoped RPC LIFECYCLE. A Mixpanel project has no typed REST CRUD;
    # it is created/listed/deleted only through org-scoped RPC verbs whose bodies are
    # untyped in the spec. rpc_lifecycle=True selects dedicated create/read/delete
    # templates:
    #   create  POST /organizations/{organization_id}/create-projects/  body
    #           {"projectNames":[<name>]}; the enveloped response is an ARRAY of the
    #           created project(s); the new row is selected by matching `name`
    #           (newest = largest id).
    #   read    GET  /organizations/{organization_id}/projects/  enveloped ARRAY;
    #           select the element whose id == state id (read_from_list).
    #   delete  POST /organizations/{organization_id}/delete-projects/  body
    #           {"projectIds":[<id>]}.
    # There is no update endpoint (name is ForceNew). org_scoped=True templates
    # {organization_id} from the organization_id attr. create_name_key / id_list_key
    # are the request body wrapper keys (projectNames / projectIds). create_match_attr
    # is `name` (the create response carries no input echo other than the row itself).
    "project": {
        # collection == the LIST path (projects): collectionPath() is what the shared
        # READ_FROM_LIST template GETs, so it must be the list, not create-projects.
        # create POSTs to its own create_path; delete POSTs to delete_path.
        "collection": "/organizations/{organization_id}/projects/",
        "instance": "",
        "id_param": "id",
        "update": None,
        "delete": "post",
        "org_scoped": True,
        "enveloped": True,
        "rpc_lifecycle": True,
        "read_from_list": True,
        "list_path": "/organizations/{organization_id}/projects/",
        "create_path": "/organizations/{organization_id}/create-projects/",
        "delete_path": "/organizations/{organization_id}/delete-projects/",
        "create_name_key": "projectNames",
        "id_list_key": "projectIds",
        "create_match_attr": "name",
    },
}


# ---------------------------------------------------------------------------
# settings singletons: org/project SETTINGS objects where the READ is a base-path
# GET and the WRITE is a DISTINCT /update sub-path that takes NO request body.
# Their result schemas are loose/untyped (additionalProperties:true, empty-branch
# anyOf), so each is surfaced as a single `settings` jsonencode passthrough holding
# the whole settings object. The read GET response is repointed to the synthetic
# MxpSettingsPassthrough ({settings:string}) in preprocess; `settings` is ignored in
# generator_config and re-injected as a jsonencode string attr in the CRUD step.
#
# These extend the singleton capability with:
#   - settings_passthrough=True : create/update POST the DECODED `settings` object
#     itself (not a {settings: ...} wrapper) to update_path; read GETs read_path,
#     wraps the body under `settings`, and stores jsonencode(body).
#   - update_path : the DISTINCT write POST target (read path = collection/instance).
#   - org_scoped / project_scoped : selects organization_id vs project_id as the
#     synthetic id and URL scope segment (reuses the existing scope machinery).
# Built from the manifest's settings_singleton entries so the table stays in sync.
def _build_settings_overrides():
    import json as _json

    try:
        man = _json.load(open(MANIFEST))
    except OSError:
        return {}
    out = {}
    for e in man.get("entities", []):
        if not e.get("settings_singleton"):
            continue
        rn = e["resource_name"]
        scope = e.get("scope", "project")
        rpath = e.get("read_path") or e.get("collection")
        upath = e.get("update_path")
        out[rn] = {
            "collection": rpath,
            "instance": rpath,
            "update_path": upath,
            "id_param": "id",
            "update": "post",
            "delete": None,
            "project_scoped": scope == "project",
            "org_scoped": scope == "org",
            "enveloped": True,
            "singleton": True,
            "settings_passthrough": True,
            # GET body is the settings object; wrap it under `settings` so it matches
            # the synthetic single-`settings` schema.
            "read_wrap_key": "settings",
        }
    return out


OVERRIDES.update(_build_settings_overrides())

# id_attr lives at a nested json path in the response for these entities; the
# value at this dotted path (within the unwrapped body) is the instance id.
ID_JSON_PATH = {
    "custom_event": "custom_event.id",
    "custom_property": "customPropertyId",
}

# Entities that get a plural "list" data source (mixpanel_<entity>s). Restricted to
# the GREEN-10: a clean enveloped `results` array whose items each carry the identity
# attr. The list path is ent["collection"] (already workspace-resolved for
# feature_flag via OVERRIDES). import_ids are "<project_id>:<id>" composites that
# the resource ImportState parser consumes directly.
LIST_DATASOURCES = {
    "agent_flow",
    "annotation",
    "custom_role",
    "custom_property",
    "experiment",
    "feature_flag",
    "custom_alert",
    "email_digest",
    "heat_map",
    "playlist",
}

# Raw Go injected into the resource (and data source) Schema() body, AFTER the
# top-level jsonencode passthrough injection. Used to repair NESTED required
# attributes that tfplugingen-openapi dropped from the IR because their schema
# is a polymorphic anyOf with a non-scalar branch (which preprocess_spec's
# scalar-only collapse can't fold to a single type). Such leaves never reach the
# generated *_resource_gen.go schema, so the wire body omits them and the API
# rejects the create.
#
# feature_flag: the API requires ruleset.variants[].value (anyOf[bool,string,
# object]); it was dropped from the variants nested object. We rebuild the whole
# `ruleset` attribute as a plain (no-CustomType) SingleNestedAttribute that adds
# `value` (a free string the API accepts verbatim) to each variant. Dropping the
# generated VariantsValue/RolloutValue CustomTypes is required: the framework
# validates a CustomType's AttributeTypes() against the declared attributes, so
# adding `value` to a CustomType-bearing nested object would error. The generic
# tftypes<->JSON bridge round-trips the plain nested object unchanged.
SCHEMA_OVERRIDES = {
    # dataset is a client-id UPSERT: dataset_id is supplied by the configuration,
    # not server-assigned. tfplugingen marks it Computed (it is a required RESPONSE
    # field), which would make Terraform reject a user-set value and the upsert
    # Create would always fail its "dataset_id must be set" guard. Re-declare it as
    # Optional+Computed so the config can supply it (and a stored value is
    # preserved). It is the instance-path key, so changing it forces replacement,
    # but RequiresReplace is unnecessary here: a different dataset_id is simply a
    # different upsert target the user opts into.
    "dataset": '\ts.Attributes["dataset_id"] = {pkg}.StringAttribute{{Optional: true, Computed: true}}',
    "feature_flag": """	s.Attributes["ruleset"] = {pkg}.SingleNestedAttribute{{
		Required: true,
		Attributes: map[string]{pkg}.Attribute{{
			"rollout": {pkg}.ListNestedAttribute{{
				Required: true,
				NestedObject: {pkg}.NestedAttributeObject{{
					Attributes: map[string]{pkg}.Attribute{{
						"cohort_hash":        {pkg}.StringAttribute{{Optional: true, Computed: true}},
						"name":               {pkg}.StringAttribute{{Optional: true, Computed: true}},
						"rollout_percentage": {pkg}.NumberAttribute{{Required: true}},
						"variant_splits": {pkg}.MapAttribute{{
							ElementType: types.NumberType,
							Required:    true,
						}},
					}},
				}},
			}},
			"variants": {pkg}.ListNestedAttribute{{
				Required: true,
				NestedObject: {pkg}.NestedAttributeObject{{
					Attributes: map[string]{pkg}.Attribute{{
						"description": {pkg}.StringAttribute{{Optional: true, Computed: true}},
						"is_control":  {pkg}.BoolAttribute{{Required: true}},
						"is_sticky":   {pkg}.BoolAttribute{{Optional: true, Computed: true}},
						"key":         {pkg}.StringAttribute{{Required: true}},
						"screenshot":  {pkg}.StringAttribute{{Optional: true, Computed: true}},
						"split":       {pkg}.NumberAttribute{{Required: true}},
						"value":       {pkg}.StringAttribute{{Required: true}},
					}},
				}},
			}},
		}},
	}}""",
    # event_definition: `definitions` is the WRITE-ONLY create input -- the array of
    # events to create. POST /event-definitions accepts it, but the instance GET
    # (EventDefinitionResult) returns a FLAT object and never echoes a `definitions`
    # list. The generated schema makes `definitions` a ListNestedAttribute whose
    # every nested leaf is Optional+Computed; an unset leaf (e.g. `description` when
    # the config only sets `name`) is therefore "(known after apply)" / unknown at
    # plan time. RawFromWireMerged only preserves a planned container verbatim when
    # it is fullyKnown(), so the unknown leaves force a fall-through to the API
    # response, which has no `definitions` -> the attribute lands null and Terraform
    # raises "inconsistent result after apply: .definitions ... now null".
    #
    # Fix: rebuild `definitions` with Optional-only leaves (no Computed) and no
    # CustomType. Unset leaves then resolve to null (a KNOWN value) instead of
    # unknown, so the whole `definitions` plan value is fullyKnown() and is
    # preserved verbatim into post-create state -- the configured value is carried
    # through (fix (b): the planned definitions become the state) and is never read
    # back from the GET. Dropping the CustomType is required (the framework
    # validates a CustomType's AttributeTypes() against the declared attributes);
    # the generic tftypes<->JSON bridge round-trips the plain nested list unchanged.
    "event_definition": """	s.Attributes["definitions"] = {pkg}.ListNestedAttribute{{
		Required: true,
		NestedObject: {pkg}.NestedAttributeObject{{
			Attributes: map[string]{pkg}.Attribute{{
				"collect_everything_event": {pkg}.StringAttribute{{Optional: true}},
				"custom_event":             {pkg}.Int64Attribute{{Optional: true}},
				"description":              {pkg}.StringAttribute{{Optional: true}},
				"name":                     {pkg}.StringAttribute{{Optional: true}},
				"verified":                 {pkg}.BoolAttribute{{Optional: true}},
			}},
		}},
	}}""",
}


# ---------------------------------------------------------------------------
# Acceptance-test generation. Each resource gets a generated mock-server
# lifecycle test (<entity>_resource_test.go) that exercises create -> implicit
# post-apply empty-plan (idempotency) -> change -> plan-action assertion against
# the in-process echo server in mock_test.go. See that file for the harness.
# ---------------------------------------------------------------------------

TEST_SCALAR = {"string", "int64", "number", "float64", "bool"}

# Per-entity HCL bodies for resources the synthesizer can't build automatically
# (a non-scalar required attribute) or whose required attrs are too constrained.
# Mirrors SCHEMA_OVERRIDES. "update" may be None for an idempotency-only test.
TEST_CONFIG_OVERRIDES = {
    "feature_flag": {
        "create": (
            '  name           = "tf_acc_flag"\n'
            '  key            = "tf_acc_flag"\n'
            '  context        = "client"\n'
            '  serving_method = "client"\n'
            "  ruleset = {\n"
            "    rollout = [{\n"
            "      rollout_percentage = 100\n"
            "      variant_splits     = { on = 100 }\n"
            "    }]\n"
            "    variants = [{\n"
            "      is_control = true\n"
            '      key        = "on"\n'
            "      split      = 100\n"
            '      value      = "true"\n'
            "    }]\n"
            "  }"
        ),
        "update": None,  # nested mutate is brittle; idempotency-only.
    },
    "event_definition": {
        "create": '  definitions = [{\n    name = "tf-acc-event"\n  }]',
        "update": None,
    },
    "event_drop_filter": {
        # event_name is the create-match key; it must be set even though optional.
        "create": '  event_name = "tf-acc-event"',
        "update": None,
    },
    "dataset": {
        "create": (
            '  dataset_id  = "tf_acc_ds"\n'
            '  name        = "tf-acc-test"\n'
            '  description = "tf-acc"'
        ),
        "update": (
            '  dataset_id  = "tf_acc_ds"\n'
            '  name        = "tf-acc-renamed"\n'
            '  description = "tf-acc"'
        ),
    },
}

# (entity, attr) -> HCL value for required attrs constrained by a OneOf/regex
# validator. Seeded from triage of validation failures; a required attr listed
# here is held constant (never chosen as the mutate attr).
TEST_VALUE_OVERRIDES = {
    ("agent_flow", "type"): '"metric_monitor"',
}

# Resources skipped (emitted as a t.Skip test) because their CRUD shape is not
# yet modeled by the echo mock. Logged so the gap is explicit, not silent.
TEST_SKIP = {
    "data_governance_settings": "singleton wrapSingleton shape not modeled by the echo mock",
}


def attr_kind_key(a):
    """The type-kind key of a code-spec attribute (e.g. 'string', 'int64')."""
    for k, v in a.items():
        if k != "name" and isinstance(v, dict) and "computed_optional_required" in v:
            return k
    return None


def _test_value(entity, attr, k, mutate=False):
    ov = TEST_VALUE_OVERRIDES.get((entity, attr))
    if ov is not None:
        return ov
    if k == "string":
        return '"tf-acc-renamed"' if mutate else '"tf-acc-test"'
    if k in ("int64", "number", "float64"):
        return "2" if mutate else "1"
    if k == "bool":
        return "true" if mutate else "false"
    return '""'


def synth_test_config(entity, attrs):
    """Return (create_hcl, update_hcl_or_None) for an entity, or None when it
    needs a TEST_CONFIG_OVERRIDES entry (a non-scalar required attribute)."""
    req = []
    for a in attrs:
        if attr_cor(a) == "required":
            k = attr_kind_key(a)
            if k not in TEST_SCALAR:
                return None
            req.append((a["name"], k))
    create = "\n".join("  %s = %s" % (n, _test_value(entity, n, k)) for n, k in req)
    # Pick a mutate attr: a settable required scalar that is not value-overridden.
    mutate = None
    for n, k in req:
        if (entity, n) not in TEST_VALUE_OVERRIDES and k in TEST_SCALAR:
            mutate = (n, k)
            break
    if mutate is None:
        return (create, None)  # idempotency-only (no mutatable required attr)
    update = "\n".join(
        "  %s = %s" % (n, _test_value(entity, n, k, mutate=(n == mutate[0])))
        for n, k in req
    )
    return (create, update)


TEST_TMPL = """// Code generated by gen/crudgen.py. DO NOT EDIT.

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"{plancheck_import}
)

func TestAcc{cls}_lifecycle(t *testing.T) {{
{skip}	srv := newMockServer(t, mockOpts{{enveloped: {enveloped}, idField: "{id_field}", stringID: {string_id}, resultsMap: {results_map}, upsert: {upsert}, listCreate: {list_create}{mock_extra}}})

	resource.Test(t, resource.TestCase{{
		ProtoV6ProviderFactories: testProtoV6,
		Steps: []resource.TestStep{{
			{{
				// Create; the implicit post-apply refresh+plan asserts idempotency.
				Config: providerConfig(srv.URL, `
resource "mixpanel_{entity}" "test" {{
{create}
}}`),
			}},{update_step}{import_step}
		}},
	}})
}}
"""

TEST_IMPORT_STEP = """
			{{
				// Import the resource and assert state round-trips through Read.
				ResourceName:                         "mixpanel_{entity}.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "{identity_attr}",
				ImportStateIdFunc:                    importIDFunc("mixpanel_{entity}.test", "{identity_attr}", "{scope_attr}"),
				ImportStateVerifyIgnore:              []string{{{ignore}}},
			}},"""

TEST_UPDATE_STEP = """
			{{
				// A changed attribute must plan as the expected action.
				Config: providerConfig(srv.URL, `
resource "mixpanel_{entity}" "test" {{
{update}
}}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{{
					PreApply: []plancheck.PlanCheck{{
						plancheck.ExpectResourceAction("mixpanel_{entity}.test", plancheck.ResourceAction{action}),
					}},
				}},
			}},"""


def render_test_file(name, ent, attrs):
    """Render the generated acceptance test for one resource."""
    skip = ""
    if name in TEST_SKIP:
        skip = '\tt.Skip("%s")\n' % TEST_SKIP[name]

    ov = TEST_CONFIG_OVERRIDES.get(name)
    if ov is not None:
        create, update = ov["create"], ov.get("update")
    else:
        synth = synth_test_config(name, attrs)
        if synth is None:
            # Unbuildable config and no override: skip rather than emit junk.
            skip = (
                skip
                or '\tt.Skip("config synthesis unsupported; add TEST_CONFIG_OVERRIDES")\n'
            )
            create, update = "", None
        else:
            create, update = synth

    # No mutate step for skipped entities (they return early anyway).
    if skip:
        update = None

    update_step = ""
    plancheck_import = ""
    if update is not None:
        action = "Replace" if (not ent["singleton"] and not ent["update"]) else "Update"
        update_step = TEST_UPDATE_STEP.format(entity=name, update=update, action=action)
        plancheck_import = (
            '\n\t"github.com/hashicorp/terraform-plugin-testing/plancheck"'
        )

    id_kind = None
    for a in attrs:
        if a["name"] == ent["identity_attr"]:
            id_kind = attr_kind_key(a)
    string_id = ent.get("inject_id") or id_kind == "string"

    # Extra mockOpts the echo server needs to faithfully model non-default CRUD
    # contracts. createIDField: a read_after_create entity whose create response
    # carries the id under a key (id_json_path) different from the read identity
    # field (e.g. behavior: create -> id, read -> behavior_id). rpcLifecycle: the
    # org-scoped RPC project shape (create-/list/delete-<plural>).
    mock_extra = ""
    if ent.get("read_after_create") and ent["id_json_path"] != ent["identity_attr"]:
        mock_extra += ', createIDField: "%s"' % ent["id_json_path"]
    if ent.get("rpc_lifecycle"):
        mock_extra += (
            ', rpcLifecycle: true, createNameKey: "%s", idListKey: "%s", matchAttr: "%s"'
            % (ent["create_name_key"], ent["id_list_key"], ent["create_match_attr"])
        )

    # Import step. Skipped-entity tests still emit a compiling step (it never runs
    # because t.Skip returns first). The composite, scope-aware ImportState parser
    # is live, so the ImportStateIdFunc emits "<scope>:<id>" for scoped resources
    # and a bare id for unscoped/singleton resources. ImportStateVerify re-reads via
    # Read and diffs; ignore the attribute classes the bridge cannot reproduce
    # byte-identically from a bare GET.
    import_step = ""
    if not skip:
        ignore_attrs = []
        ignore_attrs += ent["output_only"]
        ignore_attrs += ent["top_jsonencode"]
        ignore_attrs += ent["top_jsonstring"]
        # scope_attr mirrors the ImportState parser: singleton/unscoped import by a
        # bare id (the singleton id already equals the project id); org-scoped use
        # organization_id; everything else (project/workspace) uses project_id.
        if ent["singleton"] or (not ent["project_scoped"] and not ent["org_scoped"]):
            scope_attr = ""
        elif ent["org_scoped"] and ent["has_org_attr"]:
            scope_attr = "organization_id"
        else:
            scope_attr = "project_id"
        ignore = ", ".join('"%s"' % a for a in sorted(set(ignore_attrs)))
        import_step = TEST_IMPORT_STEP.format(
            entity=name,
            identity_attr=ent["identity_attr"],
            scope_attr=scope_attr,
            ignore=ignore,
        )

    return TEST_TMPL.format(
        cls=ent["cls"],
        entity=name,
        skip=skip,
        enveloped=go_bool(ent["enveloped"]),
        id_field=ent["identity_attr"],
        string_id=go_bool(bool(string_id)),
        results_map=go_bool(ent["results_map"]),
        upsert=go_bool(ent.get("create_to_instance", False)),
        list_create=go_bool(ent.get("collection_body_id", False)),
        create=create,
        update_step=update_step,
        import_step=import_step,
        plancheck_import=plancheck_import,
        mock_extra=mock_extra,
    )


def last_path_param(template):
    params = re.findall(r"{([^}]+)}", template or "")
    return params[-1] if params else None


def is_project_scoped(template):
    return "{project_id}" in (template or "")


def attr_cor(attr):
    """Return the computed_optional_required discriminator of a code-spec
    attribute (the value lives under the single type-kind key, e.g. "string")."""
    for k, v in attr.items():
        if k == "name":
            continue
        if isinstance(v, dict) and "computed_optional_required" in v:
            return v["computed_optional_required"]
    return None


def output_only_attrs(attrs):
    """Attribute names that are read-only (Computed-only). The API never accepts
    these in a create/update body; they are server-populated outputs. Anything
    user-settable (required / optional / computed_optional) is excluded."""
    return [a["name"] for a in attrs if attr_cor(a) == "computed"]


def resolve_entity(name, man, attr_names, attrs=None, merged=None):
    """Produce a normalized descriptor for one entity."""
    ov = OVERRIDES.get(name, {})
    collection = ov.get("collection", man.get("collection"))
    instance = ov.get("instance", man.get("instance"))
    update = ov.get("update", man.get("update"))
    delete = ov.get("delete", man.get("delete"))
    id_attr = man.get("id_attr") or "id"
    id_param = ov.get("id_param") or last_path_param(instance)
    project_scoped = ov.get(
        "project_scoped", is_project_scoped(collection) or is_project_scoped(instance)
    )
    # workspace_scoped entities live under .../workspaces/{workspace_id}/... where
    # {workspace_id} scopes a CHILD resource (e.g. feature_flag) and must be
    # resolved at runtime to the project's canonical workspace. When the override
    # does not say, infer from the templated paths -- BUT the `workspace` entity is
    # itself the workspace: its OWN id_param IS {workspace_id}, so {workspace_id}
    # templates to the resource id, not a separate scope. Treating it as
    # workspace_scoped would (a) attach the default-workspace resolver and (b) emit
    # a duplicate {workspace_id} key in the instance-path Replacer (resolver value
    # first, resource id second), so Read/Update/Delete resolve to the cached
    # default workspace instead of the resource -- destroying the default workspace
    # (HTTP 400). Exclude entities whose id_param is workspace_id from the inference.
    inferred_ws = (
        "{workspace_id}" in (collection or "") or "{workspace_id}" in (instance or "")
    ) and id_param != "workspace_id"
    workspace_scoped = ov.get("workspace_scoped", inferred_ws)
    # org_scoped entities live under .../organizations/{organization_id}/... .
    # The {organization_id} URL segment is filled at runtime from the resource's
    # organization_id attribute (an Optional+Computed int64), falling back to the
    # provider default organization. This mirrors project scoping but uses the
    # organization_id attr/placeholder instead of project_id.
    org_scoped = ov.get(
        "org_scoped",
        "{organization_id}" in (collection or "")
        or "{organization_id}" in (instance or ""),
    )
    enveloped = ov.get("enveloped", True)
    form_encoded = ov.get("form_encoded", False)
    # results_map: the envelope `results` is a single-entry map of id -> object
    # rather than the object directly. When set, the unwrap step peels the inner
    # object and injects the map key as a synthetic `id`. See the theme override.
    results_map = ov.get("results_map", False)
    # create_to_instance: the create is a POST to the INSTANCE path with a
    # client-supplied identity (upsert), not the default POST-to-collection. See
    # the dataset override.
    create_to_instance = ov.get("create_to_instance", False)
    # read_after_create: the create RESPONSE does not faithfully echo the resource
    # body (e.g. POST returns a FLAT id-bearing object/list, not the nested shape
    # the request used). When set, Create POSTs to the collection, extracts the id
    # from the (possibly list-wrapped) flat response, then re-reads the canonical
    # body via the instance GET to build state. See the event_definition override.
    read_after_create = ov.get("read_after_create", man.get("read_after_create", False))
    # read_from_list: the entity has NO instance GET. Read is performed by GETting
    # the collection (which returns a list / enveloped list) and selecting the
    # element whose id matches. create/update/delete still target their own routes
    # (the instance path for delete/update when present). list_path defaults to the
    # collection. Sourced from the manifest (source of truth), override wins. See
    # webhook / data_group.
    read_from_list = ov.get("read_from_list", man.get("read_from_list", False))
    list_path = ov.get("list_path", man.get("list_path") or collection)
    # collection_body_id: every verb lives on the collection path and the id is
    # carried in the JSON body (no instance path). create POSTs the body and selects
    # the created element from the returned list by create_match_attr; update PATCHes
    # the collection with the id injected into the body; delete DELETEs the
    # collection with {"id": <id>} as the body. See the event_drop_filter override.
    collection_body_id = ov.get("collection_body_id", False)
    create_match_attr = ov.get("create_match_attr", "")
    # singleton: a project-global resource with no collection-with-server-id and no
    # id path segment. synthetic id = project id; create=update=POST to the settings
    # path; read=GET the same path; delete is a no-op. read_wrap_key (when set) wraps
    # the unwrapped GET body under that attribute to match the request-derived
    # schema. See the data_governance_settings override.
    singleton = ov.get("singleton", man.get("singleton", False))
    read_wrap_key = ov.get("read_wrap_key", "")
    # settings_passthrough: a settings singleton whose WRITE target is a DISTINCT
    # /update sub-path (update_path) and whose body is the DECODED `settings` object
    # itself. update_path is the POST target; the read path stays collection/instance.
    settings_passthrough = ov.get(
        "settings_passthrough", man.get("settings_singleton", False)
    )
    update_path = ov.get("update_path", man.get("update_path", "")) or ""
    # rpc_lifecycle: an entity (project) whose create/read/delete live on three
    # DISTINCT org-scoped RPC verbs with untyped bodies. create POSTs
    # {create_name_key: [<name>]} to the collection (create-projects) and selects the
    # created row from the enveloped array by create_match_attr; read GETs list_path
    # (projects) and selects by id (read_from_list); delete POSTs
    # {id_list_key: [<id>]} to delete_path (delete-projects). No instance path, no
    # update verb. See the project override.
    rpc_lifecycle = ov.get("rpc_lifecycle", man.get("rpc_lifecycle", False))
    create_path = ov.get("create_path", man.get("create_path", ""))
    delete_path = ov.get("delete_path", man.get("delete_path", ""))
    create_name_key = ov.get("create_name_key", "")
    id_list_key = ov.get("id_list_key", "")

    # The TF identity attribute = the schema attribute that holds the
    # server-assigned identity the API RETURNS. Prefer the body id field
    # (id_attr, usually "id"); the URL path param (id_param, e.g. "dashboard_id")
    # is only used to TEMPLATE the instance URL and is filled from the identity
    # value at runtime. (Bug fix: previously preferred id_param, but the API
    # returns identity under id_attr, so delete/import/id-output read an empty
    # path-param attr -> DELETE with empty id -> 405.)
    if id_attr in attr_names:
        identity_attr = id_attr
    elif id_param and id_param in attr_names:
        identity_attr = id_param
    else:
        identity_attr = id_attr or id_param

    jf = man.get("jsonencode_fields", [])
    top_jsonencode_raw = [f for f in jf if "." not in f]
    # Split the jsonencode fields by their merged-spec wire type. A field typed
    # `string` (format:json-object) is a STRINGIFIED JSON value: it must be passed
    # through verbatim on the wire (a JSON string like "{}"), not decoded into a
    # JSON object. Such fields are emitted as JSONStringAttrs; the remainder are
    # true dynamic-object jsonencode fields (decoded on the way out, re-encoded on
    # the way in) emitted as JSONEncodeAttrs. (Bug fix: bookmark.params /
    # bookmark.metadata are format:json-object; decoding them put `{}` on the wire
    # and the API rejected "{} is not of type 'string'".)
    json_string_raw = json_string_fields_for_entity(merged, man, top_jsonencode_raw)
    jsonencode_obj_raw = [f for f in top_jsonencode_raw if f not in json_string_raw]
    jsonstring_obj_raw = [f for f in top_jsonencode_raw if f in json_string_raw]
    # Expose json attributes under a Terraform-legal snake_case name. The original
    # (possibly camelCase) API key is preserved as the wire key so the
    # request/response body still uses the name the API expects.
    top_jsonencode = [snake(f) for f in jsonencode_obj_raw]
    top_jsonstring = [snake(f) for f in jsonstring_obj_raw]
    # spread: jsonencode-object attrs whose decoded object is merged into the body
    # ROOT on the wire instead of nested under the attr key (collapsed polymorphic
    # oneOf body). Sourced from OVERRIDES (raw API names) intersected with the
    # jsonencode-object set; emitted as snake_case TF attr names. A spread field
    # MUST be a jsonencode-object attr (not a json-string passthrough), since the
    # value has to be decoded to an object before it can be spread.
    spread_raw = ov.get("spread", man.get("spread_fields", []))
    top_spread = [snake(f) for f in spread_raw if f in jsonencode_obj_raw]
    # Base: pin the true wire name for every top-level payload property the
    # bridge's default camelCasing would mangle (snake_case-API entities such as
    # feature_flag). jsonencode aliases and explicit OVERRIDES layer on top.
    wire_key_map = wire_key_overrides_from_spec(merged, man)
    wire_key_map.update({snake(f): f for f in top_jsonencode_raw if snake(f) != f})
    wire_key_map.update(ov.get("wire_key_overrides", {}))
    # Inject (as Optional+Computed string attrs) every json passthrough attr that
    # is NOT already a real schema attribute. Covers both jsonencode-object and
    # json-string fields (compare against the snake_case TF attribute name).
    inject_jsonencode = [
        snake(f) for f in top_jsonencode_raw if snake(f) not in attr_names
    ]

    # Singleton resources are built from a request schema that has no id field
    # (their synthetic id = project id). Inject a Computed string `id` so the
    # resource has a stable identity attribute in state and for import.
    inject_id = singleton and identity_attr not in attr_names

    # Terraform type-kind of the identity attribute, used to emit the correct
    # SetAttribute call in ImportState (string vs int64 vs number). A singleton's
    # injected synthetic id is a Computed StringAttribute, so force "string".
    identity_kind = None
    if attrs:
        for a in attrs:
            if a["name"] == identity_attr:
                identity_kind = attr_kind_key(a)
                break
    if inject_id:
        identity_kind = "string"
    # project_id is Int64 everywhere it appears EXCEPT workspace, where it is
    # Number. organization_id is Int64. Detect the scope attr kind the same way so
    # the project/org segment is set with the right typed value in ImportState.
    scope_kind = None
    if attrs:
        scope_name = (
            "organization_id"
            if (org_scoped and "organization_id" in attr_names)
            else "project_id"
        )
        for a in attrs:
            if a["name"] == scope_name:
                scope_kind = attr_kind_key(a)
                break

    # Read-only (Computed-only) attributes the API never accepts on the wire.
    # The identity / project / path-param attributes are already excluded by the
    # bridge, so leave them out to keep the set minimal.
    out_only = [
        a
        for a in (output_only_attrs(attrs) if attrs else [])
        if a != identity_attr and a != "project_id" and a != id_param
    ]

    return {
        "name": name,
        "cls": pascal(name),
        "collection": collection,
        "instance": instance,
        "update": update,
        "delete": delete,
        "id_attr": id_attr,
        "id_param": id_param,
        "identity_attr": identity_attr,
        "id_json_path": ov.get("id_json_path", ID_JSON_PATH.get(name, id_attr)),
        "project_scoped": project_scoped,
        "workspace_scoped": workspace_scoped,
        "org_scoped": org_scoped,
        "enveloped": enveloped,
        "form_encoded": form_encoded,
        "results_map": results_map,
        "create_to_instance": create_to_instance,
        "read_after_create": read_after_create,
        "read_from_list": read_from_list,
        "list_path": list_path,
        "collection_body_id": collection_body_id,
        "create_match_attr": create_match_attr,
        "singleton": singleton,
        "read_wrap_key": read_wrap_key,
        "settings_passthrough": settings_passthrough,
        "update_path": update_path,
        "rpc_lifecycle": rpc_lifecycle,
        "create_path": create_path,
        "delete_path": delete_path,
        "create_name_key": create_name_key,
        "id_list_key": id_list_key,
        "inject_id": inject_id,
        "identity_kind": identity_kind or "string",
        "scope_kind": scope_kind or "int64",
        "inject_jsonencode": inject_jsonencode,
        "top_jsonencode": top_jsonencode,
        "top_jsonstring": top_jsonstring,
        "top_spread": top_spread,
        "wire_key_map": wire_key_map,
        "has_project_attr": "project_id" in attr_names,
        "has_org_attr": "organization_id" in attr_names,
        "output_only": out_only,
    }


# ---------------------------------------------------------------------------
# Wire-name (camelCase) extraction from the original merged OpenAPI spec.
# ---------------------------------------------------------------------------
def _load_merged_spec():
    try:
        with open(MERGED_SPEC) as fh:
            return json.load(fh)
    except (OSError, ValueError):
        return None


def _schema_props(schemas, schema_name, _seen=None):
    """Collect the union of property names of a schema, resolving $ref / allOf /
    anyOf / oneOf one level deep (enough for the request/response wrappers)."""
    if _seen is None:
        _seen = set()
    if not schema_name or schema_name in _seen:
        return set()
    _seen.add(schema_name)
    sch = schemas.get(schema_name)
    if not sch:
        return set()
    return _props_of(sch, schemas, _seen)


def _props_of(sch, schemas, _seen):
    out = set()
    if not isinstance(sch, dict):
        return out
    for k in sch.get("properties") or {}:
        out.add(k)
    for combiner in ("allOf", "anyOf", "oneOf"):
        for sub in sch.get(combiner, []) or []:
            ref = sub.get("$ref")
            if ref:
                out |= _schema_props(schemas, ref.split("/")[-1], _seen)
            else:
                out |= _props_of(sub, schemas, _seen)
    return out


def wire_names_for_entity(merged, man):
    """Return the set of original (camelCase) wire property names used by an
    entity's create / update / read schemas."""
    if not merged:
        return set()
    schemas = (merged.get("components") or {}).get("schemas") or {}
    names = set()
    for key in ("create_req_schema", "update_req_schema", "read_schema"):
        names |= _schema_props(schemas, man.get(key))
    return names


def _snake_to_camel(name):
    """Python mirror of client.snakeToCamel: the bridge's DEFAULT wire key for a
    snake_case Terraform attribute. "serving_method" -> "servingMethod"."""
    if not name or name[0] == "_":
        return name
    out = []
    up = False
    for c in name:
        if c == "_":
            up = True
            continue
        if up and "a" <= c <= "z":
            c = c.upper()
        up = False
        out.append(c)
    return "".join(out)


def wire_key_overrides_from_spec(merged, man):
    """Return {tf_attr: true_wire_name} for every wire property whose actual name
    differs from what the bridge would synthesize by snakeToCamel(tf_attr).

    The bridge camelCases every wire key by default (most Mixpanel API payloads
    are camelCase: resourceType, displayFormula). But some entities' payloads are
    genuinely snake_case (e.g. feature_flag: serving_method, is_experiment_active,
    data_group_id). For those, snakeToCamel mangles the key (servingMethod) and
    the API drops the field, 400-ing the create. We compare each TOP-LEVEL wire
    property name against snakeToCamel(snake(prop)); when they diverge we pin the
    true name so the bridge sends/reads exactly what the API expects."""
    overrides = {}
    for prop in wire_names_for_entity(merged, man):
        tf_attr = snake(prop)
        if _snake_to_camel(tf_attr) != prop:
            overrides[tf_attr] = prop
    return overrides


def _prop_schema(schemas, schema_name, prop, _seen=None):
    """Resolve the schema dict for property `prop` within `schema_name`, following
    $ref / allOf / anyOf / oneOf one level deep. Returns None if not found."""
    if _seen is None:
        _seen = set()
    if not schema_name or schema_name in _seen:
        return None
    _seen.add(schema_name)
    sch = schemas.get(schema_name)
    if not isinstance(sch, dict):
        return None
    props = sch.get("properties") or {}
    if prop in props:
        return props[prop]
    for combiner in ("allOf", "anyOf", "oneOf"):
        for sub in sch.get(combiner, []) or []:
            ref = sub.get("$ref")
            if ref:
                found = _prop_schema(schemas, ref.split("/")[-1], prop, _seen)
                if found is not None:
                    return found
            elif isinstance(sub, dict):
                sp = (sub.get("properties") or {}).get(prop)
                if sp is not None:
                    return sp
    return None


def _is_json_string_schema(prop_sch):
    """A property is a 'json-string' (stringified JSON) when its OpenAPI type is
    string -- typically `type: string, format: json-object`, possibly wrapped in
    anyOf:[{string},{null}]. Such a field must be passed through verbatim on the
    wire (it is a JSON string like "{}"), NOT decoded into a JSON object."""
    if not isinstance(prop_sch, dict):
        return False
    if prop_sch.get("type") == "string":
        return True
    for combiner in ("anyOf", "oneOf"):
        subs = prop_sch.get(combiner)
        if not subs:
            continue
        types = set()
        ok = True
        for sub in subs:
            if not isinstance(sub, dict):
                ok = False
                break
            t = sub.get("type")
            if t is None:
                ok = False
                break
            types.add(t)
        if ok and types and types <= {"string", "null"} and "string" in types:
            return True
    return False


def json_string_fields_for_entity(merged, man, raw_fields):
    """Of the entity's jsonencode_fields (raw API names, top-level only), return
    the subset whose merged-spec property type is `string` (format:json-object) in
    any of the create / update / read schemas. These are stringified-JSON fields
    that must be passed through verbatim, not decoded into JSON objects."""
    if not merged:
        return set()
    schemas = (merged.get("components") or {}).get("schemas") or {}
    out = set()
    for f in raw_fields:
        for key in ("create_req_schema", "update_req_schema", "read_schema"):
            ps = _prop_schema(schemas, man.get(key), f)
            if ps is not None and _is_json_string_schema(ps):
                out.add(f)
                break
    return out


def go_str_list(items):
    return ", ".join('"%s"' % i for i in items)


def go_bool(b):
    return "true" if b else "false"


# ---------------------------------------------------------------------------
# Go templates
# ---------------------------------------------------------------------------

# ---------------------------------------------------------------------------
# ImportState bodies. Composite, scope-aware import id parsing. Parsing of the
# typed scope/identity segments is encapsulated in setImportID (crud_helpers.go),
# so these bodies only need strings.SplitN + fmt.Sprintf (both already imported by
# RESOURCE_TMPL). The "path" import is owned by the helper, not the template.
# ---------------------------------------------------------------------------

# project / workspace / org common case -> "<scope>:<id>".
IMPORT_COMPOSITE = """	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {{
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("expected import ID in the form \\"{scope_label}:<id>\\", got %q", req.ID),
		)
		return
	}}
	setImportID(ctx, &resp.State, &resp.Diagnostics, "{scope_attr}", parts[0], "{scope_kind}")
	setImportID(ctx, &resp.State, &resp.Diagnostics, "{identity_attr}", parts[1], "{identity_kind}")"""

# singleton (synthetic id == project id): one segment sets BOTH attrs.
IMPORT_SINGLETON = """	if req.ID == "" {{
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"expected import ID to be the project id (the singleton has no separate id)",
		)
		return
	}}
	setImportID(ctx, &resp.State, &resp.Diagnostics, "{scope_attr}", req.ID, "{scope_kind}")
	setImportID(ctx, &resp.State, &resp.Diagnostics, "{identity_attr}", req.ID, "{identity_kind}")"""

# unscoped (e.g. rollup_project): bare id, but still type-correct.
IMPORT_BARE = """	if req.ID == "" {{
		resp.Diagnostics.AddError("Invalid import ID", "import ID must not be empty")
		return
	}}
	setImportID(ctx, &resp.State, &resp.Diagnostics, "{identity_attr}", req.ID, "{identity_kind}")"""


RESOURCE_TMPL = """// Code generated by gen/crudgen.py. DO NOT EDIT.

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"{module}/internal/client"
	rsc "{module}/internal/provider/resource_{entity}"
)

var (
	_ resource.Resource                = (*{cls}Resource)(nil)
	_ resource.ResourceWithConfigure   = (*{cls}Resource)(nil)
	_ resource.ResourceWithImportState = (*{cls}Resource)(nil)
)

// keep the generated schema package and schema builder imported.
var _ = schema.StringAttribute{{}}

// keep the types package imported (used by nested schema overrides).
var _ = types.NumberType

// New{cls}Resource constructs the {entity} resource.
func New{cls}Resource() resource.Resource {{
	return &{cls}Resource{{}}
}}

type {cls}Resource struct {{
	client *client.Client
{ws_struct_field}}}

func (r *{cls}Resource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {{
	resp.TypeName = req.ProviderTypeName + "_{entity}"
}}

func (r *{cls}Resource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {{
	s := rsc.{cls}ResourceSchema(ctx)
{schema_inject}
	resp.Schema = s
}}

func (r *{cls}Resource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {{
	if req.ProviderData == nil {{
		return
	}}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {{
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}}
	r.client = c
}}

// projectID resolves the project from the {entity} project_id attribute (if any)
// falling back to the provider default.
func (r *{cls}Resource) projectID(ctx context.Context, raw tftypes.Value) (string, error) {{
{project_id_body}
}}

{workspace_resolver}func (r *{cls}Resource) collectionPath(projectID string) string {{
{collection_path_body}
}}

func (r *{cls}Resource) instancePath(projectID, id string) string {{
{instance_path_body}
}}
{update_path_method}
func (r *{cls}Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {{
{create_body}
}}

func (r *{cls}Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {{
{read_body}
}}

func (r *{cls}Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {{
{update_body}
}}

func (r *{cls}Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {{
{delete_body}
}}

func (r *{cls}Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {{
{import_state_body}
}}

// write{cls}State turns an unwrapped API body into resource state. base is the
// planned raw value (req.Plan.Raw) on create/update so config-supplied values are
// preserved verbatim, or a null tftypes.Value on read (state is rebuilt from the
// API response alone). See client.RawFromWireMerged for the merge semantics.
func (r *{cls}Resource) write{cls}State(ctx context.Context, state *tfsdk.State, diags *diagAppender, base tftypes.Value, wire map[string]any, projectID, id string) {{
	extras := map[string]any{{
		"{identity_attr}": id,
	}}
{extras_project}
	schemaType := state.Schema.Type().TerraformType(ctx)
	val, err := client.RawFromWireMerged(schemaType, base, wire, extras, {cls}AttrSpec())
	if err != nil {{
		diags.AddError("Building {entity} state", err.Error())
		return
	}}
	state.Raw = val
}}

// idFor{cls} extracts the identity value from an unwrapped response body.
func idFor{cls}(wire map[string]any) string {{
	if v, ok := client.IDFromWire(wire, "{identity_attr}"); ok {{
		return v
	}}
	if v, ok := nestedID(wire, "{id_json_path}"); ok {{
		return v
	}}
	return ""
}}

// unwrap{cls} unwraps the API envelope (when enveloped) and returns the body map.
func unwrap{cls}(respBody []byte) (map[string]any, error) {{
	body, err := unwrapBody(respBody, {enveloped})
	if err != nil {{
		return nil, err
	}}
	return unwrapResultsMap(body, {results_map}), nil
}}
"""


# Default Create: POST the body to the COLLECTION path; the server assigns the
# identity, read back from the response.
CREATE_COLLECTION = """	spec := {cls}AttrSpec()
	projectID, err := r.projectID(ctx, req.Plan.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving project_id", err.Error())
		return
	}}
	body, err := client.WireFromRaw(req.Plan.Raw, spec)
	if err != nil {{
		resp.Diagnostics.AddError("Encoding {entity} request", err.Error())
		return
	}}
	respBody, err := {create_call}
	if err != nil {{
		resp.Diagnostics.AddError("Creating {entity}", err.Error())
		return
	}}
	wire, err := unwrap{cls}(respBody)
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	id := idFor{cls}(wire)
	r.write{cls}State(ctx, &resp.State, &resp.Diagnostics, req.Plan.Raw, wire, projectID, id)"""


# Client-id UPSERT Create: the identity is supplied by the configuration and the
# create is a POST to the INSTANCE path (the collection has no create route). The
# id is read from the plan BEFORE the call and used both to template the instance
# URL and as the stored identity. See the dataset override (create_to_instance).
CREATE_INSTANCE_UPSERT = """	spec := {cls}AttrSpec()
	projectID, err := r.projectID(ctx, req.Plan.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving project_id", err.Error())
		return
	}}
	id, err := stringAttrFromRaw(req.Plan.Raw, "{identity_attr}")
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} id", err.Error())
		return
	}}
	if id == "" {{
		resp.Diagnostics.AddError("Creating {entity}", "{identity_attr} must be set in configuration (client-supplied id)")
		return
	}}
	body, err := client.WireFromRaw(req.Plan.Raw, spec)
	if err != nil {{
		resp.Diagnostics.AddError("Encoding {entity} request", err.Error())
		return
	}}
	respBody, err := r.client.Do(ctx, "POST", r.instancePath(projectID, id), body)
	if err != nil {{
		resp.Diagnostics.AddError("Creating {entity}", err.Error())
		return
	}}
	wire, err := unwrap{cls}(respBody)
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	// The id is client-supplied; prefer the value the API echoes back, falling
	// back to the planned id so the identity is never empty on upsert.
	if rid := idFor{cls}(wire); rid != "" {{
		id = rid
	}}
	r.write{cls}State(ctx, &resp.State, &resp.Diagnostics, req.Plan.Raw, wire, projectID, id)"""


# Read-after-create: POST to the collection, but the create response is a FLAT
# id-bearing object (possibly wrapped in a single-element results list) that does
# NOT echo the resource body. Extract the id from that flat response, then GET the
# instance to fetch the canonical body and build state from it. This is robust to
# create responses whose shape differs from the read schema.
CREATE_READ_AFTER = """	spec := {cls}AttrSpec()
	projectID, err := r.projectID(ctx, req.Plan.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving project_id", err.Error())
		return
	}}
	body, err := client.WireFromRaw(req.Plan.Raw, spec)
	if err != nil {{
		resp.Diagnostics.AddError("Encoding {entity} request", err.Error())
		return
	}}
	respBody, err := {create_call}
	if err != nil {{
		resp.Diagnostics.AddError("Creating {entity}", err.Error())
		return
	}}
	// The create response is a FLAT id-bearing object (possibly inside a single-
	// element results list); extract the server-assigned id from it.
	id, err := flatCreateID(respBody, {enveloped}, "{id_json_path}")
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} create response", err.Error())
		return
	}}
	if id == "" {{
		resp.Diagnostics.AddError("Creating {entity}", "create response did not contain an id at {id_json_path}")
		return
	}}
	// Read back the canonical body via the instance GET so state matches the read
	// schema (the create response shape differs from it).
	getBody, err := r.client.Do(ctx, "GET", r.instancePath(projectID, id), nil)
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} after create", err.Error())
		return
	}}
	wire, err := unwrap{cls}(getBody)
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	r.write{cls}State(ctx, &resp.State, &resp.Diagnostics, req.Plan.Raw, wire, projectID, id)"""


UPDATE_PUT_PATCH = """	spec := {cls}AttrSpec()
	projectID, err := r.projectID(ctx, req.Plan.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving project_id", err.Error())
		return
	}}
	id, err := stringAttrFromRaw(req.State.Raw, "{identity_attr}")
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} id", err.Error())
		return
	}}
	body, err := client.WireFromRaw(req.Plan.Raw, spec)
	if err != nil {{
		resp.Diagnostics.AddError("Encoding {entity} request", err.Error())
		return
	}}
	respBody, err := {update_call}
	if err != nil {{
		resp.Diagnostics.AddError("Updating {entity}", err.Error())
		return
	}}
	wire, err := unwrap{cls}(respBody)
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	r.write{cls}State(ctx, &resp.State, &resp.Diagnostics, req.Plan.Raw, wire, projectID, id)"""


UPDATE_FORCENEW = """	// {entity} has no update operation in the API (create/read/delete only). Every
	// user-settable attribute is marked RequiresReplace in Schema (via the
	// requireReplace helper), so a changed attribute is planned as a replacement
	// and this method is never reached for a real diff. It remains as a defensive
	// backstop: fail loudly rather than silently copy the plan into state (which
	// would report success while leaving the server unchanged).
	resp.Diagnostics.AddError(
		"{entity} does not support in-place update",
		"This resource's API has no update operation; changing an attribute replaces the resource. "+
			"Reaching this error is unexpected (attributes are marked RequiresReplace) — please report it.",
	)"""


# Standard Read: GET the instance path and decode the single-object response.
READ_INSTANCE = """	projectID, err := r.projectID(ctx, req.State.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving project_id", err.Error())
		return
	}}
	id, err := stringAttrFromRaw(req.State.Raw, "{identity_attr}")
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} id", err.Error())
		return
	}}
	respBody, err := r.client.Do(ctx, "GET", r.instancePath(projectID, id), nil)
	if err != nil {{
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {{
			resp.State.RemoveResource(ctx)
			return
		}}
		resp.Diagnostics.AddError("Reading {entity}", err.Error())
		return
	}}
	wire, err := unwrap{cls}(respBody)
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	// Use the prior state as the merge base so attributes the user manages but
	// the API does not faithfully echo back on a GET (fields it never returns, or
	// returns enriched with server-assigned sub-keys such as a subscription id)
	// are preserved instead of being clobbered to null / a server-mangled shape,
	// which would otherwise produce a permanent post-refresh diff. Computed-only
	// values (absent from prior state) are still refreshed from the API response.
	r.write{cls}State(ctx, &resp.State, &resp.Diagnostics, req.State.Raw, wire, projectID, id)"""


# Read-from-list: the entity has no instance GET. GET the collection, unwrap the
# enveloped list, and select the element whose identity attr matches the stored id.
# If no element matches, the resource was deleted out of band -> remove it.
READ_FROM_LIST = """	projectID, err := r.projectID(ctx, req.State.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving project_id", err.Error())
		return
	}}
	id, err := stringAttrFromRaw(req.State.Raw, "{identity_attr}")
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} id", err.Error())
		return
	}}
	respBody, err := r.client.Do(ctx, "GET", r.collectionPath(projectID), nil)
	if err != nil {{
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {{
			resp.State.RemoveResource(ctx)
			return
		}}
		resp.Diagnostics.AddError("Reading {entity}", err.Error())
		return
	}}
	wire, found, err := findInList(respBody, {enveloped}, "{id_json_path}", id)
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	if !found {{
		resp.State.RemoveResource(ctx)
		return
	}}
	// Merge against prior state: the list item may omit user-managed fields the
	// API never echoes back; preserve those instead of clobbering to null.
	r.write{cls}State(ctx, &resp.State, &resp.Diagnostics, req.State.Raw, wire, projectID, id)"""


# Default Delete: DELETE the instance path. DELETE may return a JSON body
# (Mixpanel convention); Do tolerates it. A 404 is treated as already-gone.
DELETE_INSTANCE = """	projectID, err := r.projectID(ctx, req.State.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving project_id", err.Error())
		return
	}}
	id, err := stringAttrFromRaw(req.State.Raw, "{identity_attr}")
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} id", err.Error())
		return
	}}
	// DELETE may return a JSON body (Mixpanel convention); Do tolerates it.
	if _, err := r.client.Do(ctx, "DELETE", r.instancePath(projectID, id), nil); err != nil {{
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {{
			return
		}}
		resp.Diagnostics.AddError("Deleting {entity}", err.Error())
		return
	}}"""


# ---------------------------------------------------------------------------
# Collection-path body-id CRUD (event_drop_filter). Every verb hits the COLLECTION
# path; the id lives in the JSON body. There is no instance path: the path methods
# (collectionPath/instancePath) both resolve to the collection. create POSTs the
# body (no id) and selects the created element from the returned list by matching
# create_match_attr; update PATCHes the collection with the id injected into the
# body; delete DELETEs the collection with {"id": <id>} as the body.
# ---------------------------------------------------------------------------

# Create: POST the body to the collection; the response is the FULL (enveloped)
# list. Select the just-created element by matching match_attr against the planned
# value, choosing the numerically-largest id (the new row), and build state from it.
CREATE_COLLECTION_BODY_ID = """	spec := {cls}AttrSpec()
	projectID, err := r.projectID(ctx, req.Plan.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving project_id", err.Error())
		return
	}}
	body, err := client.WireFromRaw(req.Plan.Raw, spec)
	if err != nil {{
		resp.Diagnostics.AddError("Encoding {entity} request", err.Error())
		return
	}}
	matchVal, err := stringAttrFromRaw(req.Plan.Raw, "{match_attr}")
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} {match_attr}", err.Error())
		return
	}}
	respBody, err := r.client.Do(ctx, "POST", r.collectionPath(projectID), body)
	if err != nil {{
		resp.Diagnostics.AddError("Creating {entity}", err.Error())
		return
	}}
	wire, id, err := selectNewestFromList(respBody, {enveloped}, "{id_json_path}", "{match_attr}", matchVal)
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	if id == "" {{
		resp.Diagnostics.AddError("Creating {entity}", "create response did not contain the new filter (no element matching {match_attr})")
		return
	}}
	r.write{cls}State(ctx, &resp.State, &resp.Diagnostics, req.Plan.Raw, wire, projectID, id)"""


# Update: PATCH the collection with the id injected into the body. The response is
# the full list; re-select the element by id to rebuild state.
UPDATE_COLLECTION_BODY_ID = """	spec := {cls}AttrSpec()
	projectID, err := r.projectID(ctx, req.Plan.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving project_id", err.Error())
		return
	}}
	id, err := stringAttrFromRaw(req.State.Raw, "{identity_attr}")
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} id", err.Error())
		return
	}}
	body, err := client.WireFromRaw(req.Plan.Raw, spec)
	if err != nil {{
		resp.Diagnostics.AddError("Encoding {entity} request", err.Error())
		return
	}}
	// The id is carried in the JSON body, not the URL.
	body["{id_wire_key}"] = jsonNumberOrString(id)
	respBody, err := r.client.Do(ctx, "{update_verb}", r.collectionPath(projectID), body)
	if err != nil {{
		resp.Diagnostics.AddError("Updating {entity}", err.Error())
		return
	}}
	wire, found, err := findInList(respBody, {enveloped}, "{id_json_path}", id)
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	if !found {{
		resp.Diagnostics.AddError("Updating {entity}", "updated filter not found in response")
		return
	}}
	r.write{cls}State(ctx, &resp.State, &resp.Diagnostics, req.Plan.Raw, wire, projectID, id)"""


# Delete: DELETE the collection with {"id": <id>} as the body.
DELETE_COLLECTION_BODY_ID = """	projectID, err := r.projectID(ctx, req.State.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving project_id", err.Error())
		return
	}}
	id, err := stringAttrFromRaw(req.State.Raw, "{identity_attr}")
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} id", err.Error())
		return
	}}
	delBody := map[string]any{{"{id_wire_key}": jsonNumberOrString(id)}}
	if _, err := r.client.Do(ctx, "DELETE", r.collectionPath(projectID), delBody); err != nil {{
		if apiErr, ok := err.(*client.APIError); ok && (apiErr.StatusCode == 404) {{
			return
		}}
		resp.Diagnostics.AddError("Deleting {entity}", err.Error())
		return
	}}"""


# ---------------------------------------------------------------------------
# Singleton CRUD (data_governance_settings). A project-global resource: no
# collection-with-server-id, no id path segment. synthetic id = project id;
# create=update=POST to the settings path; read=GET the same path; delete is a
# no-op. The GET body is wrapped under read_wrap_key (when set) so it matches the
# request-derived schema. The path methods both resolve to the settings path.
# ---------------------------------------------------------------------------

CREATE_SINGLETON = """	spec := {cls}AttrSpec()
	projectID, err := r.projectID(ctx, req.Plan.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving project_id", err.Error())
		return
	}}
	body, err := client.WireFromRaw(req.Plan.Raw, spec)
	if err != nil {{
		resp.Diagnostics.AddError("Encoding {entity} request", err.Error())
		return
	}}
	if _, err := r.client.Do(ctx, "POST", r.collectionPath(projectID), body); err != nil {{
		resp.Diagnostics.AddError("Creating {entity}", err.Error())
		return
	}}
	// Read back the canonical settings so state reflects what the API stored.
	getBody, err := r.client.Do(ctx, "GET", r.collectionPath(projectID), nil)
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} after create", err.Error())
		return
	}}
	wire, err := unwrap{cls}(getBody)
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	wire = wrapSingleton(wire, "{read_wrap_key}")
	// synthetic id = project id (a project singleton has one settings object).
	r.write{cls}State(ctx, &resp.State, &resp.Diagnostics, req.Plan.Raw, wire, projectID, projectID)"""


UPDATE_SINGLETON = """	spec := {cls}AttrSpec()
	projectID, err := r.projectID(ctx, req.Plan.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving project_id", err.Error())
		return
	}}
	body, err := client.WireFromRaw(req.Plan.Raw, spec)
	if err != nil {{
		resp.Diagnostics.AddError("Encoding {entity} request", err.Error())
		return
	}}
	if _, err := r.client.Do(ctx, "POST", r.collectionPath(projectID), body); err != nil {{
		resp.Diagnostics.AddError("Updating {entity}", err.Error())
		return
	}}
	getBody, err := r.client.Do(ctx, "GET", r.collectionPath(projectID), nil)
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} after update", err.Error())
		return
	}}
	wire, err := unwrap{cls}(getBody)
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	wire = wrapSingleton(wire, "{read_wrap_key}")
	r.write{cls}State(ctx, &resp.State, &resp.Diagnostics, req.Plan.Raw, wire, projectID, projectID)"""


READ_SINGLETON = """	projectID, err := r.projectID(ctx, req.State.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving project_id", err.Error())
		return
	}}
	respBody, err := r.client.Do(ctx, "GET", r.collectionPath(projectID), nil)
	if err != nil {{
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {{
			resp.State.RemoveResource(ctx)
			return
		}}
		resp.Diagnostics.AddError("Reading {entity}", err.Error())
		return
	}}
	wire, err := unwrap{cls}(respBody)
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	wire = wrapSingleton(wire, "{read_wrap_key}")
	// synthetic id = project id (a project singleton has one settings object).
	r.write{cls}State(ctx, &resp.State, &resp.Diagnostics, req.State.Raw, wire, projectID, projectID)"""


# Singleton Delete: a project-global settings object cannot be deleted. Destroy
# simply removes the resource from Terraform state without mutating the API.
DELETE_SINGLETON = """	// {entity} is a project singleton: destroy removes it from state only and does
	// NOT mutate the API (the settings are project-global and cannot be deleted).
	_ = ctx
	_ = req
	_ = resp"""


# ---------------------------------------------------------------------------
# Workspace scoping. Entities whose CRUD route is
# /api/app/projects/{project_id}/workspaces/{workspace_id}/... must resolve the
# workspace id at runtime. The project-only variants of these routes are gated by
# a require_workspace_is_set decorator on the backend and 400 with
# "Workspace is required". We resolve the project's canonical workspace (the only
# is_global=true "All Project Data" workspace, else the is_default one) and cache
# it on the resource. The path-method SIGNATURES are unchanged so no CRUD call
# site changes; only the method bodies differ. Non-workspace entities emit bodies
# byte-identical to the pre-workspace generator.
# ---------------------------------------------------------------------------

# RPC lifecycle CRUD (project). create/read/delete live on three DISTINCT
# org-scoped RPC verbs with untyped request bodies. create POSTs
# {create_name_key: [<name>]} to the collection (create-projects); the enveloped
# response is an ARRAY of the created project(s); the new row is selected by
# matching create_match_attr (name), largest id. read GETs list_path (projects)
# and selects by id (the shared READ_FROM_LIST template). delete POSTs
# {id_list_key: [<id>]} to delete_path (delete-projects). No update (ForceNew).
# ---------------------------------------------------------------------------

# Create: read the user-settable `name` (create_match_attr) from the plan, POST
# {create_name_key: [name]} to the collection, then select the created element from
# the enveloped array by name (largest id = the just-created row).
CREATE_RPC_LIFECYCLE = """	projectID, err := r.projectID(ctx, req.Plan.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving organization_id", err.Error())
		return
	}}
	matchVal, err := stringAttrFromRaw(req.Plan.Raw, "{match_attr}")
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} {match_attr}", err.Error())
		return
	}}
	createPath := strings.NewReplacer("{{organization_id}}", projectID).Replace("{create_path}")
	createBody := map[string]any{{"{create_name_key}": []any{{matchVal}}}}
	respBody, err := r.client.Do(ctx, "POST", createPath, createBody)
	if err != nil {{
		resp.Diagnostics.AddError("Creating {entity}", err.Error())
		return
	}}
	wire, id, err := selectNewestFromList(respBody, {enveloped}, "{id_json_path}", "{match_attr}", matchVal)
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	if id == "" {{
		resp.Diagnostics.AddError("Creating {entity}", "create response did not contain the new {entity} (no element matching {match_attr})")
		return
	}}
	r.write{cls}State(ctx, &resp.State, &resp.Diagnostics, req.Plan.Raw, wire, projectID, id)"""


# Delete: POST {id_list_key: [<id>]} to the delete RPC path. A 404 is treated as
# already-gone. delete_path is templated for {organization_id} like the collection.
DELETE_RPC_LIFECYCLE = """	projectID, err := r.projectID(ctx, req.State.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving organization_id", err.Error())
		return
	}}
	id, err := stringAttrFromRaw(req.State.Raw, "{identity_attr}")
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} id", err.Error())
		return
	}}
	delPath := strings.NewReplacer("{{organization_id}}", projectID).Replace("{delete_path}")
	delBody := map[string]any{{"{id_list_key}": []any{{jsonNumberOrString(id)}}}}
	if _, err := r.client.Do(ctx, "POST", delPath, delBody); err != nil {{
		if apiErr, ok := err.(*client.APIError); ok && (apiErr.StatusCode == 404) {{
			return
		}}
		resp.Diagnostics.AddError("Deleting {entity}", err.Error())
		return
	}}"""


# ---------------------------------------------------------------------------

# settings singleton CRUD. Like the plain singleton, but the WRITE target is a
# DISTINCT /update sub-path (updatePath) and the body is the DECODED `settings`
# object itself (not a {settings: ...} wrapper). The single `settings` jsonencode
# attr holds the whole settings object: WireFromRaw yields {"settings": <obj>}; we
# POST the inner <obj> to updatePath. read=GET collectionPath; the GET body is
# wrapped under "settings" so it round-trips to the jsonencode attr. The `scopeID`
# slot carries the project id (project-scoped) or organization id (org-scoped) and
# is templated into the read/update URLs by collectionPath/updatePath.
# ---------------------------------------------------------------------------

CREATE_SETTINGS_SINGLETON = """	spec := {cls}AttrSpec()
	scopeID, err := r.projectID(ctx, req.Plan.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving scope id", err.Error())
		return
	}}
	full, err := client.WireFromRaw(req.Plan.Raw, spec)
	if err != nil {{
		resp.Diagnostics.AddError("Encoding {entity} request", err.Error())
		return
	}}
	// The whole settings object lives under the single "settings" jsonencode attr;
	// POST the inner object (the update endpoint takes the settings fields directly).
	// settings is Optional+Computed: when the user omits it, full["settings"] is nil
	// and there is nothing to write -- skip the POST (POSTing a null/empty body can
	// error or silently clear the scope's settings) and just adopt the live values.
	if body, ok := full["settings"]; ok && body != nil {{
		if _, err := r.client.Do(ctx, "POST", r.updatePath(scopeID), body); err != nil {{
			resp.Diagnostics.AddError("Creating {entity}", err.Error())
			return
		}}
	}}
	getBody, err := r.client.Do(ctx, "GET", r.collectionPath(scopeID), nil)
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} after create", err.Error())
		return
	}}
	wire, err := unwrap{cls}(getBody)
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	wire = wrapSingleton(wire, "{read_wrap_key}")
	r.write{cls}State(ctx, &resp.State, &resp.Diagnostics, req.Plan.Raw, wire, scopeID, scopeID)"""


UPDATE_SETTINGS_SINGLETON = """	spec := {cls}AttrSpec()
	scopeID, err := r.projectID(ctx, req.Plan.Raw)
	if err != nil {{
		resp.Diagnostics.AddError("Resolving scope id", err.Error())
		return
	}}
	full, err := client.WireFromRaw(req.Plan.Raw, spec)
	if err != nil {{
		resp.Diagnostics.AddError("Encoding {entity} request", err.Error())
		return
	}}
	// settings is Optional+Computed: a nil body means "no change" -- skip the POST
	// (a null/empty body can error or silently clear the scope's settings) and adopt
	// the current live values via the read-back below.
	if body, ok := full["settings"]; ok && body != nil {{
		if _, err := r.client.Do(ctx, "POST", r.updatePath(scopeID), body); err != nil {{
			resp.Diagnostics.AddError("Updating {entity}", err.Error())
			return
		}}
	}}
	getBody, err := r.client.Do(ctx, "GET", r.collectionPath(scopeID), nil)
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} after update", err.Error())
		return
	}}
	wire, err := unwrap{cls}(getBody)
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	wire = wrapSingleton(wire, "{read_wrap_key}")
	r.write{cls}State(ctx, &resp.State, &resp.Diagnostics, req.Plan.Raw, wire, scopeID, scopeID)"""


# ---------------------------------------------------------------------------

# updatePath: the DISTINCT write POST target for a settings singleton. The scopeID
# arg carries the project id (project-scoped) or organization id (org-scoped); it
# is templated into the matching URL scope segment. Emitted only when update_path is
# set; empty otherwise so non-settings resources are byte-identical.
UPDATE_PATH_METHOD_PROJECT = """
func (r *{cls}Resource) updatePath(scopeID string) string {{
\treturn strings.NewReplacer("{{project_id}}", scopeID).Replace("{update_path}")
}}
"""
UPDATE_PATH_METHOD_ORG = """
func (r *{cls}Resource) updatePath(scopeID string) string {{
\treturn strings.NewReplacer("{{organization_id}}", scopeID).Replace("{update_path}")
}}
"""

WS_STRUCT_FIELD = "\t// cachedWorkspaceID memoizes the resolved workspace id per project id, so a\n\t// config using project_id overrides across multiple projects resolves the\n\t// correct workspace for each instead of reusing the first one resolved.\n\tcachedWorkspaceID map[string]string\n"

COLLECTION_PATH_BODY_PLAIN = (
    '\treturn strings.NewReplacer("{{project_id}}", projectID).Replace("{collection}")'
)
INSTANCE_PATH_BODY_PLAIN = (
    '\treturn strings.NewReplacer("{{project_id}}", projectID, "{{{id_param}}}", id).'
    'Replace("{instance}")'
)
COLLECTION_PATH_BODY_WS = (
    '\treturn strings.NewReplacer("{{project_id}}", projectID, '
    '"{{workspace_id}}", r.workspaceID(projectID)).Replace("{collection}")'
)
INSTANCE_PATH_BODY_WS = (
    '\treturn strings.NewReplacer("{{project_id}}", projectID, '
    '"{{workspace_id}}", r.workspaceID(projectID), '
    '"{{{id_param}}}", id).Replace("{instance}")'
)

WORKSPACE_RESOLVER_TMPL = """// workspaceID returns the workspace id used to template the {entity} CRUD path.
// The project-only {entity} route requires a workspace, so we target the
// project's canonical workspace (global "All Project Data", else default),
// memoized per project id for the lifetime of this resource instance so that a
// config targeting multiple projects resolves the correct workspace for each.
func (r *{cls}Resource) workspaceID(projectID string) string {{
\tif r.cachedWorkspaceID == nil {{
\t\tr.cachedWorkspaceID = map[string]string{{}}
\t}}
\tif ws, ok := r.cachedWorkspaceID[projectID]; ok && ws != "" {{
\t\treturn ws
\t}}
\tif ws, err := r.client.DefaultWorkspaceID(context.Background(), projectID); err == nil && ws != "" {{
\t\tr.cachedWorkspaceID[projectID] = ws
\t}}
\treturn r.cachedWorkspaceID[projectID]
}}

"""


PROJECT_ID_WITH_ATTR = """	pid, err := stringAttrFromRaw(raw, "project_id")
	if err == nil && pid != "" {
		return r.client.ProjectID(pid), nil
	}
	return r.client.ProjectID(""), nil"""

PROJECT_ID_DEFAULT = """	_ = raw
	return r.client.ProjectID(""), nil"""

PROJECT_ID_UNSCOPED = """	_ = raw
	return "", nil"""

# For org-scoped entities the "projectID" slot carries the ORGANIZATION id. It is
# read from the resource's organization_id attribute (Optional+Computed int64),
# falling back to the provider default organization. The value is templated into
# the {organization_id} URL segment by collectionPath/instancePath.
ORG_ID_WITH_ATTR = """	oid, err := stringAttrFromRaw(raw, "organization_id")
	if err == nil && oid != "" {
		return r.client.OrganizationID(oid), nil
	}
	return r.client.OrganizationID(""), nil"""

# Org-scoped path bodies: the `projectID` slot carries the ORGANIZATION id, which
# is templated into the {organization_id} URL segment (mirrors the project plain
# variants but for the organizations/{organization_id}/... routes).
COLLECTION_PATH_BODY_ORG = '\treturn strings.NewReplacer("{{organization_id}}", projectID).Replace("{collection}")'
INSTANCE_PATH_BODY_ORG = (
    '\treturn strings.NewReplacer("{{organization_id}}", projectID, "{{{id_param}}}", id).'
    'Replace("{instance}")'
)


DATASOURCE_TMPL = """// Code generated by gen/crudgen.py. DO NOT EDIT.

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"{module}/internal/client"
	dsc "{module}/internal/provider/datasource_{entity}"
)

var (
	_ datasource.DataSource              = (*{cls}DataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*{cls}DataSource)(nil)
)

// keep the generated data-source schema package and schema builder imported.
var _ = dschema.StringAttribute{{}}

// New{cls}DataSource constructs the {entity} data source.
func New{cls}DataSource() datasource.DataSource {{
	return &{cls}DataSource{{}}
}}

type {cls}DataSource struct {{
	client *client.Client
}}

func (d *{cls}DataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {{
	resp.TypeName = req.ProviderTypeName + "_{entity}"
}}

func (d *{cls}DataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {{
	s := dsc.{cls}DataSourceSchema(ctx)
{ds_schema_inject}
	resp.Schema = s
}}

func (d *{cls}DataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {{
	if req.ProviderData == nil {{
		return
	}}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {{
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}}
	d.client = c
}}

func (d *{cls}DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {{
	spec := {cls}AttrSpec()
{ds_project_id}
	id, err := stringAttrFromRaw(req.Config.Raw, "{selector_attr}")
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity} id", err.Error())
		return
	}}
	path := strings.NewReplacer("{{{ds_scope_param}}}", projectID, "{{{id_param}}}", id).Replace("{instance}")
	respBody, err := d.client.Do(ctx, "GET", path, nil)
	if err != nil {{
		resp.Diagnostics.AddError("Reading {entity}", err.Error())
		return
	}}
	wire, err := unwrapBody(respBody, {enveloped})
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} response", err.Error())
		return
	}}
	wire = unwrapResultsMap(wire, {results_map})
{ds_wrap_singleton}	extras := map[string]any{{
		"{selector_attr}": id,
	}}
{ds_extras_project}
	schemaType := schemaTypeOfDataSource(ctx, resp.State)
	val, err := client.RawFromWire(schemaType, wire, extras, spec)
	if err != nil {{
		resp.Diagnostics.AddError("Building {entity} state", err.Error())
		return
	}}
	resp.State.Raw = val
	_ = tftypes.Value{{}}
	_ = tfsdk.State{{}}
}}
"""


# Plural "list" data source: returns the raw ids and "<project_id>:<id>" composite
# import ids of every entity in a project. Hand-shaped schema (no per-entity schema
# import) keeps it self-contained; bulk discovery + for_each import is fully served
# by ids + import_ids without round-tripping each item through the typed bridge.
DATASOURCE_LIST_TMPL = """// Code generated by gen/crudgen.py. DO NOT EDIT.

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"{module}/internal/client"
)

var (
	_ datasource.DataSource              = (*{cls}ListDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*{cls}ListDataSource)(nil)
)

// New{cls}ListDataSource constructs the {entity} list data source.
func New{cls}ListDataSource() datasource.DataSource {{
	return &{cls}ListDataSource{{}}
}}

type {cls}ListDataSource struct {{
	client *client.Client
}}

// {cls}ListModel is the state model for mixpanel_{entity}s.
type {cls}ListModel struct {{
	ProjectID types.String `tfsdk:"project_id"`
	IDs       types.List   `tfsdk:"ids"`
	ImportIDs types.List   `tfsdk:"import_ids"`
}}

func (d *{cls}ListDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {{
	resp.TypeName = req.ProviderTypeName + "_{entity}s"
}}

func (d *{cls}ListDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {{
	resp.Schema = dschema.Schema{{
		MarkdownDescription: "List of all {entity} objects in a project. " +
			"`ids` are the raw server ids; `import_ids` are `<project_id>:<id>` " +
			"composites ready to feed a for_each import block.",
		Attributes: map[string]dschema.Attribute{{
			"project_id": dschema.StringAttribute{{
				Optional:            true,
				MarkdownDescription: "Project id. Falls back to the provider default project.",
			}},
			"ids": dschema.ListAttribute{{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Raw ids of every {entity} in the project.",
			}},
			"import_ids": dschema.ListAttribute{{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "`<project_id>:<id>` composite import ids for every {entity}.",
			}},
		}},
	}}
}}

func (d *{cls}ListDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {{
	if req.ProviderData == nil {{
		return
	}}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {{
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}}
	d.client = c
}}

func (d *{cls}ListDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {{
	var cfg {cls}ListModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {{
		return
	}}
	projectID := d.client.ProjectID(cfg.ProjectID.ValueString())
	listPath := {list_path_expr}
	respBody, err := d.client.Do(ctx, "GET", listPath, nil)
	if err != nil {{
		resp.Diagnostics.AddError("Listing {entity}s", err.Error())
		return
	}}
	ids, err := collectIDsFromList(respBody, {enveloped}, "{id_json_path}")
	if err != nil {{
		resp.Diagnostics.AddError("Decoding {entity} list response", err.Error())
		return
	}}
	importIDs := make([]string, len(ids))
	for i, id := range ids {{
		importIDs[i] = compositeImportID(projectID, id)
	}}
	idList, di := types.ListValueFrom(ctx, types.StringType, ids)
	resp.Diagnostics.Append(di...)
	impList, di2 := types.ListValueFrom(ctx, types.StringType, importIDs)
	resp.Diagnostics.Append(di2...)
	if resp.Diagnostics.HasError() {{
		return
	}}
	state := {cls}ListModel{{
		ProjectID: types.StringValue(projectID),
		IDs:       idList,
		ImportIDs: impList,
	}}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}}
"""


SPEC_TMPL = """// Code generated by gen/crudgen.py. DO NOT EDIT.

package provider

import "{module}/internal/client"

// {cls}AttrSpec describes the synthetic / jsonencode attributes of the {entity}
// entity to the generic tftypes<->JSON bridge in the client package. It is shared
// by the {entity} resource and data source.
func {cls}AttrSpec() client.AttrSpec {{
	return client.AttrSpec{{
		IDAttr:           "{identity_attr}",
		ProjectIDAttr:    "{project_attr}",
		PathParamAttrs:   map[string]bool{{ {path_param_set} }},
		JSONEncodeAttrs:  map[string]bool{{ {jsonencode_set} }},
		JSONStringAttrs:  map[string]bool{{ {jsonstring_set} }},
		JSONEncodeWireKey: map[string]string{{ {wire_key_set} }},
		OutputOnlyAttrs:  map[string]bool{{ {output_only_set} }},
		SpreadAttrs:      map[string]bool{{ {spread_set} }},
	}}
}}
"""


def inject_schema_lines(ent, attr_kind, force_new_attrs=None):
    """Return Go source injecting jsonencode + identity-computed attributes.

    attr_kind is "schema" (resource) or "dschema" (datasource). force_new_attrs,
    when given (resources with no update operation), names the top-level
    attributes to mark RequiresReplace via the requireReplace helper.
    """
    lines = []
    # Singleton synthetic id: a Computed string identity absent from the request
    # schema. Emitted before the jsonencode passthroughs.
    if ent.get("inject_id"):
        lines.append(
            '\ts.Attributes["%s"] = %s.StringAttribute{Computed: true}'
            % (ent["identity_attr"], attr_kind)
        )
    for f in ent["inject_jsonencode"]:
        lines.append(
            '\ts.Attributes["%s"] = %s.StringAttribute{Optional: true, Computed: true}'
            % (f, attr_kind)
        )
    # Repair nested required leaves dropped from the IR (see SCHEMA_OVERRIDES).
    # Resource only: the override rebuilds attributes with Required:true, which is
    # meaningless for a (read-only) data source and would also reference the
    # data-source schema package incorrectly.
    if attr_kind == "schema":
        ov = SCHEMA_OVERRIDES.get(ent["name"])
        if ov:
            lines.append(ov.format(pkg=attr_kind))
        if force_new_attrs:
            names = ", ".join('"%s"' % n for n in force_new_attrs)
            lines.append("\trequireReplace(s.Attributes, %s)" % names)
        # Stabilize every Computed attribute with UseStateForUnknown so a
        # server-populated value already in state is preserved across plans
        # instead of being re-marked "(known after apply)". Without this, a
        # computed field the API echoes (e.g. annotation.user / user_id, the
        # resolved project_id, the server id) flips to unknown on every plan and
        # produces a spurious in-place change -- which, after `terraform import`,
        # means the imported resource never reaches a clean plan. Emitted last so
        # it sees the injected jsonencode/identity attributes too.
        lines.append("\tstabilizeComputed(s.Attributes)")
    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Import examples (Terraform Registry convention)
#
# For every generated resource we emit two files under
#   examples/resources/mixpanel_<entity>/
#     import.sh  -- the `terraform import` CLI command with the correct
#                   composite import-id for that entity's scope class.
#     import.tf  -- the equivalent config-driven `import {}` block (Terraform
#                   1.5+ / OpenTofu), pointing at a resource address.
#
# The import-id format MUST match the live ImportState parser
# (IMPORT_COMPOSITE / IMPORT_SINGLETON / IMPORT_BARE), keyed off the same
# scope classification used to pick the import_state_body:
#   project-scoped : "<PROJECT_ID>:<ID>"
#   org-scoped     : "<ORGANIZATION_ID>:<ID>"
#   unscoped       : "<ID>"
#   singleton      : "<PROJECT_ID>"   (single segment; sets both project_id+id)
# ---------------------------------------------------------------------------

IMPORT_SH_TMPL = """# {comment}
# Import format: {id_format}
terraform import mixpanel_{entity}.example "{id_value}"
"""

IMPORT_TF_TMPL = """# {comment}
# Config-driven import (Terraform 1.5+ / OpenTofu). Add a matching
# resource "mixpanel_{entity}" "example" {{ ... }} block, then `terraform plan`
# / `apply` to bring the existing object under management.
import {{
  to = mixpanel_{entity}.example
  id = "{id_value}"
}}
"""


def import_example_for(ent):
    """Return (id_format, id_value, comment) for an entity's import example.

    Mirrors the scope classification that selects import_state_body so the
    documented import-id is byte-compatible with the live parser."""
    entity = ent["name"]
    if ent["singleton"]:
        # One segment that is the project id (the singleton has no separate id).
        return (
            "<PROJECT_ID>",
            "1234567",
            "Import the singleton %s for a project by the project id." % entity,
        )
    if not ent["project_scoped"] and not ent["org_scoped"]:
        # Bare id, no scope segment.
        return (
            "<ID>",
            "7654321",
            "Import a %s by its id (this resource is not project-scoped)." % entity,
        )
    if ent["org_scoped"] and ent["has_org_attr"]:
        return (
            "<ORGANIZATION_ID>:<ID>",
            "1234567:7654321",
            'Import a %s by "<organization_id>:<id>".' % entity,
        )
    # project / workspace scoped (workspace auto-resolves at Read).
    return (
        "<PROJECT_ID>:<ID>",
        "1234567:7654321",
        'Import a %s by "<project_id>:<id>".' % entity,
    )


def write_import_examples(ent, written):
    """Emit import.sh + import.tf for one resource entity."""
    entity = ent["name"]
    id_format, id_value, comment = import_example_for(ent)
    ex_dir = os.path.join(EXAMPLES_DIR, "resources", "mixpanel_%s" % entity)
    os.makedirs(ex_dir, exist_ok=True)

    sh_path = os.path.join(ex_dir, "import.sh")
    open(sh_path, "w").write(
        IMPORT_SH_TMPL.format(
            comment=comment, id_format=id_format, id_value=id_value, entity=entity
        )
    )
    written.append(sh_path)

    tf_path = os.path.join(ex_dir, "import.tf")
    open(tf_path, "w").write(
        IMPORT_TF_TMPL.format(comment=comment, id_value=id_value, entity=entity)
    )
    written.append(tf_path)


# ===========================================================================
# RPC ASSOCIATION capability.
#
# Mixpanel RBAC (teams + membership/grant associations) is NOT collection/instance
# CRUD. It is a set of ORG-scoped RPC verbs with UNTYPED request bodies and untyped
# list responses:
#
#   create  = POST   <add-verb>     (e.g. organizations/{org}/add-teams/)
#   read    = GET     <list-path>   (e.g. organizations/{org}/teams) -> untyped list
#   update  = POST   <update-verb>  (optional; team has teams/{team_id}/update)
#   delete  = POST   <delete-verb>  (e.g. organizations/{org}/delete-teams/)
#
# Because the request/response bodies carry no OpenAPI schema, tfplugingen-openapi
# emits "no compatible schema found" and the entity never reaches the IR. So the
# rpc_assoc capability is FULLY SELF-CONTAINED: crudgen emits BOTH a synthetic
# schema package (resource_<entity>/<entity>_resource_gen.go) AND a typed-model
# resource (<entity>_resource.go) that depends only on the framework + the shared
# client. No generic tftypes bridge, no AttrSpec.
#
# Schema (every rpc_assoc resource):
#   id              Computed string  -- synthetic composite key "<org>:<key>"
#   organization_id Optional+Computed string -- org scope (provider default if unset)
#   key             Required string  -- the read-selection key (team name; or the
#                                       "user:project:role" tuple for a grant). Forces
#                                       replace (it is the identity of the association).
#   payload         Required string  -- jsonencode'd request body for the add/delete
#                                       verbs. The API bodies are untyped, so the whole
#                                       body is passed through verbatim. ForceNew unless
#                                       an update verb exists.
#
# Read confirms the managed association still exists: it GETs the list path, decodes
# the untyped JSON, and searches recursively for `key` (as a "name" field value, or as
# a substring of any string value). If absent, the resource is removed from state.
# This is intentionally permissive: the list shapes are open (additionalProperties)
# and we cannot assume a column layout, so existence-by-key is the safe contract. A
# user importing/reading drift on the typed columns is out of scope for v1 (the
# config-owned `payload`/`key` are preserved verbatim on read).
# ===========================================================================

RPC_ASSOC_SCHEMA_TMPL = """// Code generated by gen/crudgen.py (rpc_assoc). DO NOT EDIT.

package resource_{entity}

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

// {cls}ResourceSchema is the synthetic schema for the {entity} RPC association
// resource. The Mixpanel RBAC {entity} verbs carry untyped bodies, so the request
// body is passed through as the `payload` jsonencode string; `key` selects the
// managed row from the (untyped) list on read.
func {cls}ResourceSchema(ctx context.Context) schema.Schema {{
	_ = ctx
	return schema.Schema{{
		MarkdownDescription: "Mixpanel RBAC {entity} association (org-scoped RPC verbs; untyped bodies passed via the jsonencode `payload`).",
		Attributes: map[string]schema.Attribute{{
			"id": schema.StringAttribute{{
				Computed:            true,
				MarkdownDescription: "Synthetic composite identity: \\"<organization_id>:<key>\\".",
				PlanModifiers: []planmodifier.String{{
					stringplanmodifier.UseStateForUnknown(),
				}},
			}},
			"organization_id": schema.StringAttribute{{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Organization id that scopes the RPC verbs. Defaults to the provider organization.",
				PlanModifiers: []planmodifier.String{{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				}},
			}},
			"key": schema.StringAttribute{{
				Required:            true,
				MarkdownDescription: "Read-selection key for this association ({key_doc}). Changing it forces a new association.",
				PlanModifiers: []planmodifier.String{{
					stringplanmodifier.RequiresReplace(),
				}},
			}},
			"payload": schema.StringAttribute{{
				Required:            true,
				MarkdownDescription: "jsonencode'd request body for the create/{mutate_doc} verbs (the API bodies are untyped).{payload_force}",
				PlanModifiers: []planmodifier.String{{{payload_pm}
				}},
			}},
		}},
	}}
}}
"""

RPC_ASSOC_RESOURCE_TMPL = """// Code generated by gen/crudgen.py (rpc_assoc). DO NOT EDIT.

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"{module}/internal/client"
	rsc "{module}/internal/provider/resource_{entity}"
)

var (
	_ resource.Resource                = (*{cls}Resource)(nil)
	_ resource.ResourceWithConfigure   = (*{cls}Resource)(nil)
	_ resource.ResourceWithImportState = (*{cls}Resource)(nil)
)

// New{cls}Resource constructs the {entity} RPC association resource.
func New{cls}Resource() resource.Resource {{
	return &{cls}Resource{{}}
}}

type {cls}Resource struct {{
	client *client.Client
}}

// {cls}Model is the typed state model for the {entity} association.
type {cls}Model struct {{
	ID             types.String `tfsdk:"id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Key            types.String `tfsdk:"key"`
	Payload        types.String `tfsdk:"payload"`
}}

func (r *{cls}Resource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {{
	resp.TypeName = req.ProviderTypeName + "_{entity}"
}}

func (r *{cls}Resource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {{
	resp.Schema = rsc.{cls}ResourceSchema(ctx)
}}

func (r *{cls}Resource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {{
	if req.ProviderData == nil {{
		return
	}}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {{
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}}
	r.client = c
}}

// orgID resolves the organization scope from the model (falling back to the
// provider default organization).
func (r *{cls}Resource) orgID(m *{cls}Model) string {{
	if !m.OrganizationID.IsNull() && !m.OrganizationID.IsUnknown() && m.OrganizationID.ValueString() != "" {{
		return r.client.OrganizationID(m.OrganizationID.ValueString())
	}}
	return r.client.OrganizationID("")
}}

// decodePayload parses the jsonencode payload string into a request body.
func (r *{cls}Resource) decodePayload(raw string) (any, error) {{
	raw = strings.TrimSpace(raw)
	if raw == "" {{
		return map[string]any{{}}, nil
	}}
	var body any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {{
		return nil, fmt.Errorf("payload is not valid JSON: %w", err)
	}}
	return body, nil
}}

func (r *{cls}Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {{
	var m {cls}Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {{
		return
	}}
	org := r.orgID(&m)
	body, err := r.decodePayload(m.Payload.ValueString())
	if err != nil {{
		resp.Diagnostics.AddError("Encoding {entity} payload", err.Error())
		return
	}}
	createPath := strings.NewReplacer("{{organization_id}}", org).Replace("{create_path}")
	if _, err := r.client.Do(ctx, "POST", createPath, body); err != nil {{
		resp.Diagnostics.AddError("Creating {entity}", err.Error())
		return
	}}
	m.OrganizationID = types.StringValue(org)
	m.ID = types.StringValue(org + ":" + m.Key.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}}

func (r *{cls}Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {{
	var m {cls}Model
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {{
		return
	}}
	org := r.orgID(&m)
	listPath := strings.NewReplacer("{{organization_id}}", org).Replace("{list_path}")
	respBody, err := r.client.Do(ctx, "GET", listPath, nil)
	if err != nil {{
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {{
			resp.State.RemoveResource(ctx)
			return
		}}
		resp.Diagnostics.AddError("Reading {entity}", err.Error())
		return
	}}
	unwrapped, err := client.UnwrapEnvelope(respBody)
	if err != nil {{
		// Tolerate an un-enveloped body (search the raw bytes).
		unwrapped = respBody
	}}
	if !rpcAssocKeyPresent(unwrapped, m.Key.ValueString()) {{
		resp.State.RemoveResource(ctx)
		return
	}}
	// The association still exists; preserve the config-owned columns and refresh
	// the synthetic identity.
	m.OrganizationID = types.StringValue(org)
	m.ID = types.StringValue(org + ":" + m.Key.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}}

func (r *{cls}Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {{
{update_body}
}}

func (r *{cls}Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {{
	var m {cls}Model
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {{
		return
	}}
	org := r.orgID(&m)
	body, err := r.decodePayload(m.Payload.ValueString())
	if err != nil {{
		resp.Diagnostics.AddError("Encoding {entity} delete payload", err.Error())
		return
	}}
	deletePath := strings.NewReplacer("{{organization_id}}", org).Replace("{delete_path}")
	if _, err := r.client.Do(ctx, "POST", deletePath, body); err != nil {{
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {{
			return
		}}
		resp.Diagnostics.AddError("Deleting {entity}", err.Error())
		return
	}}
}}

func (r *{cls}Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {{
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}}
"""

# Update body for rpc_assoc entities WITH an update verb (e.g. team): POST the
# update path with the payload. The update path may carry a {key} placeholder (the
# team_id-style instance segment) which we substitute from the key.
RPC_ASSOC_UPDATE_VERB = """	var m {cls}Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {{
		return
	}}
	org := r.orgID(&m)
	body, err := r.decodePayload(m.Payload.ValueString())
	if err != nil {{
		resp.Diagnostics.AddError("Encoding {entity} update payload", err.Error())
		return
	}}
	updatePath := strings.NewReplacer("{{organization_id}}", org, "{{key}}", m.Key.ValueString()).Replace("{update_path}")
	if _, err := r.client.Do(ctx, "POST", updatePath, body); err != nil {{
		resp.Diagnostics.AddError("Updating {entity}", err.Error())
		return
	}}
	m.OrganizationID = types.StringValue(org)
	m.ID = types.StringValue(org + ":" + m.Key.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)"""

# Update body for ForceNew rpc_assoc entities (no update verb, e.g. a grant): the
# schema marks key+payload+org RequiresReplace, so Update is never reached for a
# real change. Emit a defensive no-op that just persists the plan.
RPC_ASSOC_UPDATE_FORCENEW = """	// {entity} has no update verb: key, payload and organization_id all force
	// replacement, so a real change recreates the association. This persists the
	// plan defensively (Update is only reached when nothing meaningful changed).
	var m {cls}Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {{
		return
	}}
	org := r.orgID(&m)
	m.OrganizationID = types.StringValue(org)
	m.ID = types.StringValue(org + ":" + m.Key.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)"""


# Shared runtime helper for rpc_assoc reads: recursively confirm a key is present
# anywhere in the (untyped) decoded list body. Emitted once into crud_helpers.go.
RPC_ASSOC_HELPER_GO = """
// rpcAssocKeyPresent reports whether key appears anywhere in the (untyped) JSON
// body of an RPC-association list response. The org RBAC list endpoints
// (organizations/{organization_id}/teams, .../users, ...) return open
// (additionalProperties) shapes with no guaranteed column layout, so existence is
// confirmed permissively: key matches a "name"/"id"/"email" field value, a map
// key, or any scalar leaf equal to key. Used by every rpc_assoc resource's Read.
func rpcAssocKeyPresent(body []byte, key string) bool {
	if key == "" {
		return false
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		// Undecodable list body: we cannot confirm presence. A literal substring
		// scan (the previous behavior) matches a key embedded in an unrelated value
		// -- e.g. a short role name or an email that recurs elsewhere -- and so
		// MASKS a real deletion. Report absent and let the next apply re-create.
		return false
	}
	return rpcAssocWalk(decoded, key)
}

func rpcAssocWalk(node any, key string) bool {
	switch v := node.(type) {
	case map[string]any:
		for k, val := range v {
			if k == key {
				return true
			}
			if rpcAssocWalk(val, key) {
				return true
			}
		}
	case []any:
		for _, e := range v {
			if rpcAssocWalk(e, key) {
				return true
			}
		}
	case string:
		return v == key
	}
	return false
}
"""


def emit_rpc_assoc(name, man, outdir, module):
    """Emit the schema package + resource file for one rpc_assoc entity.

    Returns the list of written file paths. The manifest entry supplies:
      create_path / list_path / delete_path  (org-templated RPC verb paths)
      update_path  (optional; templates {organization_id} and {key})
      key_doc      (human description of the `key` attribute)
    """
    cls = pascal(name)
    create_path = man["create_path"]
    list_path = man["list_path"]
    delete_path = man["delete_path"]
    update_path = man.get("update_path")

    def go_str(s):
        """Escape a Python string for embedding inside a Go double-quoted literal."""
        return s.replace("\\", "\\\\").replace('"', '\\"')

    key_doc = go_str(man.get("key_doc", "the association key"))

    if update_path:
        update_body = RPC_ASSOC_UPDATE_VERB.format(
            entity=name, cls=cls, update_path=update_path
        )
        payload_force = ""
        payload_pm = ""
        mutate_doc = "update/delete"
    else:
        update_body = RPC_ASSOC_UPDATE_FORCENEW.format(entity=name, cls=cls)
        payload_force = " Changing it forces a new association."
        payload_pm = "\n\t\t\t\t\tstringplanmodifier.RequiresReplace(),"
        mutate_doc = "delete"

    pkg_dir = os.path.join(outdir, "resource_%s" % name)
    os.makedirs(pkg_dir, exist_ok=True)
    schema_src = RPC_ASSOC_SCHEMA_TMPL.format(
        entity=name,
        cls=cls,
        key_doc=key_doc,
        mutate_doc=mutate_doc,
        payload_force=payload_force,
        payload_pm=payload_pm,
    )
    schema_path = os.path.join(pkg_dir, "%s_resource_gen.go" % name)
    open(schema_path, "w").write(schema_src)

    res_src = RPC_ASSOC_RESOURCE_TMPL.format(
        module=module,
        entity=name,
        cls=cls,
        create_path=create_path,
        list_path=list_path,
        delete_path=delete_path,
        update_body=update_body,
    )
    res_path = os.path.join(outdir, "%s_resource.go" % name)
    open(res_path, "w").write(res_src)
    return [schema_path, res_path]


def main():
    spec = json.load(open(IR))
    manifest = json.load(open(MANIFEST))
    mans = {e["resource_name"]: e for e in manifest["entities"]}

    res_specs = {r["name"]: r for r in spec["resources"]}
    ds_specs = {d["name"]: d for d in spec["datasources"]}

    merged = _load_merged_spec()

    written = []

    # Per-entity AttrSpec files (union of resources + data sources). One file per
    # entity so both the resource and the data source can reference {cls}AttrSpec.
    all_names = sorted(set(res_specs) | set(ds_specs))
    for name in all_names:
        spec_obj = res_specs.get(name) or ds_specs.get(name)
        attrs = spec_obj["schema"]["attributes"]
        attr_names = {a["name"] for a in attrs}
        man = mans.get(name, {})
        ent = resolve_entity(name, man, attr_names, attrs, merged=merged)
        path_param_set = ""
        if ent["id_param"] and ent["id_param"] != ent["identity_attr"]:
            path_param_set = '"%s": true' % ent["id_param"]
        jsonencode_set = ", ".join('"%s": true' % f for f in ent["top_jsonencode"])
        jsonstring_set = ", ".join('"%s": true' % f for f in ent["top_jsonstring"])
        wire_key_set = ", ".join(
            '"%s": "%s"' % (k, v) for k, v in sorted(ent["wire_key_map"].items())
        )
        output_only_set = ", ".join(
            '"%s": true' % f for f in sorted(ent["output_only"])
        )
        spread_set = ", ".join('"%s": true' % f for f in ent["top_spread"])
        # ProjectIDAttr is the scope attribute the generic bridge keeps out of the
        # request body and restores into state from extras. For org-scoped entities
        # that attribute is organization_id (it scopes the URL, not the body).
        if ent["org_scoped"] and ent["has_org_attr"]:
            project_attr = "organization_id"
        elif ent["has_project_attr"]:
            project_attr = "project_id"
        else:
            project_attr = ""
        src = SPEC_TMPL.format(
            module=MODULE,
            entity=name,
            cls=ent["cls"],
            identity_attr=ent["identity_attr"],
            project_attr=project_attr,
            path_param_set=path_param_set,
            jsonencode_set=jsonencode_set,
            jsonstring_set=jsonstring_set,
            wire_key_set=wire_key_set,
            output_only_set=output_only_set,
            spread_set=spread_set,
        )
        p = os.path.join(OUTDIR, "%s_spec.go" % name)
        open(p, "w").write(src)
        written.append(p)

    # Resources
    for name in sorted(res_specs):
        rspec = res_specs[name]
        attr_names = {a["name"] for a in rspec["schema"]["attributes"]}
        man = mans.get(name, {})
        ent = resolve_entity(
            name, man, attr_names, rspec["schema"]["attributes"], merged=merged
        )

        # scope (projectID slot) resolver body. For org-scoped entities the slot
        # carries the organization id, resolved from the organization_id attr.
        if ent["org_scoped"]:
            project_id_body = ORG_ID_WITH_ATTR
        elif not ent["project_scoped"]:
            project_id_body = PROJECT_ID_UNSCOPED
        elif ent["has_project_attr"]:
            project_id_body = PROJECT_ID_WITH_ATTR
        else:
            project_id_body = PROJECT_ID_DEFAULT

        # The wire key the identity is written under in a body-id payload. The id
        # attr is typically "id"; honor any wire-key override (camelCase, etc.).
        id_wire_key = ent["wire_key_map"].get(
            ent["identity_attr"], ent["identity_attr"]
        )
        update_verb = {"put": "PUT", "patch": "PATCH", "post": "POST"}.get(
            ent["update"] or ""
        )

        # create body: default POST-to-collection, or client-id upsert
        # (POST-to-instance with a config-supplied identity).
        if ent["rpc_lifecycle"]:
            create_body = CREATE_RPC_LIFECYCLE.format(
                entity=name,
                cls=ent["cls"],
                enveloped=go_bool(ent["enveloped"]),
                id_json_path=ent["id_json_path"],
                match_attr=ent["create_match_attr"],
                create_name_key=ent["create_name_key"],
                create_path=ent["create_path"],
            )
        elif ent["settings_passthrough"]:
            create_body = CREATE_SETTINGS_SINGLETON.format(
                entity=name,
                cls=ent["cls"],
                read_wrap_key=ent["read_wrap_key"],
            )
        elif ent["singleton"]:
            create_body = CREATE_SINGLETON.format(
                entity=name,
                cls=ent["cls"],
                read_wrap_key=ent["read_wrap_key"],
            )
        elif ent["collection_body_id"]:
            create_body = CREATE_COLLECTION_BODY_ID.format(
                entity=name,
                cls=ent["cls"],
                enveloped=go_bool(ent["enveloped"]),
                id_json_path=ent["id_json_path"],
                match_attr=ent["create_match_attr"],
            )
        elif ent["create_to_instance"]:
            create_body = CREATE_INSTANCE_UPSERT.format(
                entity=name,
                cls=ent["cls"],
                identity_attr=ent["identity_attr"],
            )
        elif ent["read_after_create"]:
            create_call = (
                'r.client.DoForm(ctx, "POST", r.collectionPath(projectID), body)'
                if ent["form_encoded"]
                else 'r.client.Do(ctx, "POST", r.collectionPath(projectID), body)'
            )
            create_body = CREATE_READ_AFTER.format(
                entity=name,
                cls=ent["cls"],
                create_call=create_call,
                enveloped=go_bool(ent["enveloped"]),
                id_json_path=ent["id_json_path"],
            )
        else:
            create_call = (
                'r.client.DoForm(ctx, "POST", r.collectionPath(projectID), body)'
                if ent["form_encoded"]
                else 'r.client.Do(ctx, "POST", r.collectionPath(projectID), body)'
            )
            create_body = CREATE_COLLECTION.format(
                entity=name,
                cls=ent["cls"],
                create_call=create_call,
            )

        # update body
        if ent["settings_passthrough"]:
            update_body = UPDATE_SETTINGS_SINGLETON.format(
                entity=name,
                cls=ent["cls"],
                read_wrap_key=ent["read_wrap_key"],
            )
        elif ent["singleton"]:
            update_body = UPDATE_SINGLETON.format(
                entity=name,
                cls=ent["cls"],
                read_wrap_key=ent["read_wrap_key"],
            )
        elif ent["collection_body_id"] and ent["update"]:
            update_body = UPDATE_COLLECTION_BODY_ID.format(
                entity=name,
                cls=ent["cls"],
                identity_attr=ent["identity_attr"],
                enveloped=go_bool(ent["enveloped"]),
                id_json_path=ent["id_json_path"],
                id_wire_key=id_wire_key,
                update_verb=update_verb,
            )
        elif ent["update"]:
            verb = update_verb
            update_call = (
                'r.client.DoForm(ctx, "%s", r.instancePath(projectID, id), body)' % verb
                if ent["form_encoded"]
                else 'r.client.Do(ctx, "%s", r.instancePath(projectID, id), body)'
                % verb
            )
            update_body = UPDATE_PUT_PATCH.format(
                entity=name,
                cls=ent["cls"],
                identity_attr=ent["identity_attr"],
                update_call=update_call,
            )
        else:
            update_body = UPDATE_FORCENEW.format(entity=name)

        # For entities with no update operation (UPDATE_FORCENEW above), every
        # user-settable top-level attribute must force replacement: an in-place
        # change is impossible, so a changed attribute can only be applied by
        # recreating the resource. Computed-only outputs are excluded (the user
        # never sets them, so they would never trigger a replace anyway).
        force_new_attrs = None
        if not ent["singleton"] and not ent["update"]:
            force_new_attrs = sorted(
                a["name"]
                for a in rspec["schema"]["attributes"]
                if attr_cor(a) != "computed"
            )
        elif ent["singleton"]:
            # A singleton's scope (organization_id / project_id) is its only real
            # identity: changing it points at a DIFFERENT org/project's settings,
            # which must DESTROY the old-scope resource and CREATE in the new scope
            # -- not an in-place Update that silently rewrites another scope. Mark
            # the scope attr RequiresReplace so the change routes through replace.
            singleton_scope = (
                "organization_id"
                if (ent["org_scoped"] and ent["has_org_attr"])
                else ("project_id" if ent["has_project_attr"] else None)
            )
            if singleton_scope:
                force_new_attrs = [singleton_scope]

        # read body: singleton GET, read-from-list (collection GET + select by id),
        # or the default instance GET.
        if ent["singleton"]:
            read_body = READ_SINGLETON.format(
                entity=name,
                cls=ent["cls"],
                read_wrap_key=ent["read_wrap_key"],
            )
        elif ent["read_from_list"]:
            read_body = READ_FROM_LIST.format(
                entity=name,
                cls=ent["cls"],
                identity_attr=ent["identity_attr"],
                enveloped=go_bool(ent["enveloped"]),
                id_json_path=ent["id_json_path"],
            )
        else:
            read_body = READ_INSTANCE.format(
                entity=name,
                cls=ent["cls"],
                identity_attr=ent["identity_attr"],
            )

        # delete body: rpc-lifecycle POST-to-delete-verb, singleton no-op, collection
        # body-id DELETE-with-body, or the default instance DELETE.
        if ent["rpc_lifecycle"]:
            delete_body = DELETE_RPC_LIFECYCLE.format(
                entity=name,
                identity_attr=ent["identity_attr"],
                delete_path=ent["delete_path"],
                id_list_key=ent["id_list_key"],
            )
        elif ent["singleton"]:
            delete_body = DELETE_SINGLETON.format(entity=name)
        elif ent["collection_body_id"]:
            delete_body = DELETE_COLLECTION_BODY_ID.format(
                entity=name,
                identity_attr=ent["identity_attr"],
                id_wire_key=id_wire_key,
            )
        else:
            delete_body = DELETE_INSTANCE.format(
                entity=name,
                identity_attr=ent["identity_attr"],
            )

        # extras: scope-id passthrough into state when it's a schema attr. For
        # org-scoped entities the scope value is the organization id; for
        # project-scoped entities it's the project id.
        if ent["org_scoped"] and ent["has_org_attr"]:
            extras_project = (
                '\tif projectID != "" {\n\t\textras["organization_id"] = projectID\n\t}'
            )
        elif ent["has_project_attr"] and ent["project_scoped"]:
            extras_project = (
                '\tif projectID != "" {\n\t\textras["project_id"] = projectID\n\t}'
            )
        else:
            extras_project = "\t_ = projectID"
        # Populate the path-param attribute (e.g. annotation_id) with the id when
        # it is a distinct schema attribute from the identity attr. The API never
        # echoes this synthetic attr, so without seeding it from the id it stays
        # unknown in state and shows a spurious "(known after apply)" diff on every
        # plan -- breaking clean-plan-after-import. (Harmless when it is not a real
        # schema attr: RawFromWireMerged drops extras absent from the schema type.)
        if ent["id_param"] and ent["id_param"] != ent["identity_attr"]:
            extras_project += '\n\textras["%s"] = id' % ent["id_param"]

        path_param_set = ""
        if ent["id_param"] and ent["id_param"] != ent["identity_attr"]:
            path_param_set = '"%s": true' % ent["id_param"]

        jsonencode_set = ", ".join('"%s": true' % f for f in ent["top_jsonencode"])
        if ent["org_scoped"] and ent["has_org_attr"]:
            project_attr = "organization_id"
        elif ent["has_project_attr"]:
            project_attr = "project_id"
        else:
            project_attr = ""

        # path wiring. Org-scoped entities template {organization_id}; workspace-
        # scoped entities template {project_id}+{workspace_id}; the default plain
        # variant templates {project_id} only.
        if ent["org_scoped"]:
            ws_struct_field = ""
            workspace_resolver = ""
            collection_path_body = COLLECTION_PATH_BODY_ORG
            instance_path_body = INSTANCE_PATH_BODY_ORG
        elif ent["workspace_scoped"]:
            ws_struct_field = WS_STRUCT_FIELD
            workspace_resolver = WORKSPACE_RESOLVER_TMPL.format(
                entity=name, cls=ent["cls"]
            )
            collection_path_body = COLLECTION_PATH_BODY_WS
            instance_path_body = INSTANCE_PATH_BODY_WS
        else:
            ws_struct_field = ""
            workspace_resolver = ""
            collection_path_body = COLLECTION_PATH_BODY_PLAIN
            instance_path_body = INSTANCE_PATH_BODY_PLAIN

        # The path-body constants carry their own {collection}/{instance}/{id_param}
        # placeholders. They are inserted into RESOURCE_TMPL as opaque values, so
        # str.format would not recurse into them -- pre-format them here to concrete
        # Go before passing them in.
        _path_fmt = dict(
            collection=ent["collection"] or "",
            instance=ent["instance"] or "",
            id_param=ent["id_param"] or ent["identity_attr"],
        )
        collection_path_body = collection_path_body.format(**_path_fmt)
        instance_path_body = instance_path_body.format(**_path_fmt)

        # ImportState body. Composite "scope:id" parsing, scope-aware. Checked in
        # the order singleton -> unscoped -> project/workspace/org.
        if ent["singleton"]:
            # The singleton's scope attr is organization_id for org-scoped settings
            # and project_id otherwise. Hardcoding project_id made ImportState set a
            # nonexistent attribute on org-scoped settings (their schema only has
            # organization_id), so import failed outright.
            singleton_scope_attr = (
                "organization_id"
                if (ent["org_scoped"] and ent["has_org_attr"])
                else "project_id"
            )
            import_state_body = IMPORT_SINGLETON.format(
                scope_attr=singleton_scope_attr,
                scope_kind=ent["scope_kind"],
                identity_attr=ent["identity_attr"],
                identity_kind=ent["identity_kind"],
            )
        elif not ent["project_scoped"] and not ent["org_scoped"]:
            import_state_body = IMPORT_BARE.format(
                identity_attr=ent["identity_attr"],
                identity_kind=ent["identity_kind"],
            )
        else:
            scope_attr = (
                "organization_id"
                if (ent["org_scoped"] and ent["has_org_attr"])
                else "project_id"
            )
            import_state_body = IMPORT_COMPOSITE.format(
                scope_attr=scope_attr,
                scope_label=scope_attr,
                scope_kind=ent["scope_kind"],
                identity_attr=ent["identity_attr"],
                identity_kind=ent["identity_kind"],
            )
        # update_path_method: emit the updatePath helper only for settings singletons
        # (a DISTINCT write POST target). The scope segment matches the entity scope.
        update_path_method = ""
        if ent["update_path"]:
            tmpl = (
                UPDATE_PATH_METHOD_ORG
                if ent["org_scoped"]
                else UPDATE_PATH_METHOD_PROJECT
            )
            update_path_method = tmpl.format(
                cls=ent["cls"], update_path=ent["update_path"]
            )

        src = RESOURCE_TMPL.format(
            module=MODULE,
            entity=name,
            cls=ent["cls"],
            identity_attr=ent["identity_attr"],
            project_attr=project_attr,
            path_param_set=path_param_set,
            jsonencode_set=jsonencode_set,
            schema_inject=inject_schema_lines(ent, "schema", force_new_attrs),
            project_id_body=project_id_body,
            collection=ent["collection"] or "",
            instance=ent["instance"] or "",
            id_param=ent["id_param"] or ent["identity_attr"],
            create_body=create_body,
            update_body=update_body,
            read_body=read_body,
            delete_body=delete_body,
            extras_project=extras_project,
            id_json_path=ent["id_json_path"],
            enveloped=go_bool(ent["enveloped"]),
            results_map=go_bool(ent["results_map"]),
            ws_struct_field=ws_struct_field,
            workspace_resolver=workspace_resolver,
            collection_path_body=collection_path_body,
            instance_path_body=instance_path_body,
            import_state_body=import_state_body,
            update_path_method=update_path_method,
        )
        path = os.path.join(OUTDIR, "%s_resource.go" % name)
        open(path, "w").write(src)
        written.append(path)

        # Generated mock-server acceptance test for this resource.
        test_src = render_test_file(name, ent, rspec["schema"]["attributes"])
        test_path = os.path.join(OUTDIR, "%s_resource_test.go" % name)
        open(test_path, "w").write(test_src)
        written.append(test_path)

        # Registry-convention import examples (import.sh + config-driven import.tf).
        write_import_examples(ent, written)

    # Data sources
    for name in sorted(ds_specs):
        dspec = ds_specs[name]
        attr_names = {a["name"] for a in dspec["schema"]["attributes"]}
        man = mans.get(name, {})
        ent = resolve_entity(name, man, attr_names, merged=merged)

        # The data source "projectID" slot carries the URL scope value. For org-
        # scoped entities that is the organization id; ds_scope_param is the URL
        # placeholder it fills (organization_id vs project_id).
        if ent["org_scoped"]:
            ds_scope_param = "organization_id"
            ds_project_id = (
                '\tprojectID := d.client.OrganizationID("")\n'
                '\tif oid, oerr := stringAttrFromRaw(req.Config.Raw, "organization_id"); oerr == nil && oid != "" {\n'
                "\t\tprojectID = d.client.OrganizationID(oid)\n\t}"
            )
        elif not ent["project_scoped"]:
            ds_scope_param = "project_id"
            ds_project_id = '\tprojectID := ""\n\t_ = req.Config.Raw'
        elif ent["has_project_attr"]:
            ds_scope_param = "project_id"
            ds_project_id = (
                '\tprojectID := d.client.ProjectID("")\n'
                '\tif pid, perr := stringAttrFromRaw(req.Config.Raw, "project_id"); perr == nil && pid != "" {\n'
                "\t\tprojectID = d.client.ProjectID(pid)\n\t}"
            )
        else:
            ds_scope_param = "project_id"
            ds_project_id = (
                '\tprojectID := d.client.ProjectID("")\n\t_ = req.Config.Raw'
            )

        if ent["org_scoped"] and ent["has_org_attr"]:
            ds_extras_project = (
                '\tif projectID != "" {\n\t\textras["organization_id"] = projectID\n\t}'
            )
        elif ent["has_project_attr"] and ent["project_scoped"]:
            ds_extras_project = (
                '\tif projectID != "" {\n\t\textras["project_id"] = projectID\n\t}'
            )
        else:
            ds_extras_project = "\t_ = projectID"

        # The data source's USER-FACING selector is the Required path-param attr
        # (e.g. metric_id / dashboard_id) that tfplugingen derives from the GET path
        # parameter -- NOT the body identity attr (id), which is Computed and thus
        # always null in config. Reading identity_attr here returned "" for every
        # singular data source, so the GET hit the collection with an empty id. Use
        # id_param when it is a real schema attr; fall back to identity_attr only for
        # singletons/entities whose selector IS the id attr.
        ds_selector_attr = (
            ent["id_param"]
            if (ent["id_param"] and ent["id_param"] in attr_names)
            else ent["identity_attr"]
        )
        # Settings/singleton data sources whose GET returns the settings fields flat
        # at the top level must wrap them under read_wrap_key (e.g. "settings") so
        # the (jsonencode) attribute is populated -- mirroring the resource Read.
        # Without this the Computed settings attribute is always null.
        if ent.get("read_wrap_key"):
            ds_wrap_singleton = (
                '\twire = wrapSingleton(wire, "%s")\n' % ent["read_wrap_key"]
            )
        else:
            ds_wrap_singleton = ""

        src = DATASOURCE_TMPL.format(
            module=MODULE,
            entity=name,
            cls=ent["cls"],
            identity_attr=ent["identity_attr"],
            selector_attr=ds_selector_attr,
            ds_wrap_singleton=ds_wrap_singleton,
            ds_schema_inject=inject_schema_lines(ent, "dschema"),
            ds_project_id=ds_project_id,
            ds_scope_param=ds_scope_param,
            instance=ent["instance"] or "",
            id_param=ent["id_param"] or ent["identity_attr"],
            enveloped=go_bool(ent["enveloped"]),
            results_map=go_bool(ent["results_map"]),
            ds_extras_project=ds_extras_project,
        )
        path = os.path.join(OUTDIR, "%s_data_source.go" % name)
        open(path, "w").write(src)
        written.append(path)

        # Plural "list" data source for the GREEN-10. The list path is the entity's
        # collection (already workspace-resolved for feature_flag via OVERRIDES);
        # workspace-scoped entities resolve {workspace_id} at runtime.
        if name in LIST_DATASOURCES:
            if ent["workspace_scoped"]:
                list_path_expr = (
                    "func() string {\n"
                    "\t\tws, _ := d.client.DefaultWorkspaceID(ctx, projectID)\n"
                    '\t\treturn strings.NewReplacer("{project_id}", projectID, '
                    '"{workspace_id}", ws).Replace("%s")\n\t}()' % ent["collection"]
                )
            else:
                list_path_expr = (
                    'strings.NewReplacer("{project_id}", projectID).Replace("%s")'
                    % ent["collection"]
                )
            list_src = DATASOURCE_LIST_TMPL.format(
                module=MODULE,
                entity=name,
                cls=ent["cls"],
                enveloped=go_bool(ent["enveloped"]),
                id_json_path=ent["id_json_path"],
                list_path_expr=list_path_expr,
            )
            lp = os.path.join(OUTDIR, "%s_list_data_source.go" % name)
            open(lp, "w").write(list_src)
            written.append(lp)

    # RPC ASSOCIATION resources (rpc_assoc capability). These never reach the IR
    # (untyped bodies), so they are emitted fully self-contained from the manifest:
    # a synthetic schema package + a typed-model resource. See emit_rpc_assoc.
    rpc_assoc_emitted = []
    for ent_man in manifest["entities"]:
        if not ent_man.get("keep") or ent_man.get("capability") != "rpc_assoc":
            continue
        rn = ent_man["resource_name"]
        files = emit_rpc_assoc(rn, ent_man, OUTDIR, MODULE)
        written.extend(files)
        rpc_assoc_emitted.append(rn)

    # Shared helpers file (idempotent). Append the rpc_assoc runtime helper when any
    # rpc_assoc entity was emitted (so the json/strings imports it needs are present).
    helpers = HELPERS_GO.replace("__MODULE__", MODULE)
    if rpc_assoc_emitted:
        helpers = helpers + RPC_ASSOC_HELPER_GO
    open(os.path.join(OUTDIR, "crud_helpers.go"), "w").write(helpers)
    written.append(os.path.join(OUTDIR, "crud_helpers.go"))

    # gofmt the emitted Go so output is canonical and idempotent.
    import subprocess

    try:
        subprocess.run(["gofmt", "-w", OUTDIR], check=True)
    except Exception as exc:  # noqa: BLE001 - gofmt is best-effort
        print("warning: gofmt failed: %s" % exc)

    print(
        "wrote %d files (%d resources, %d data sources)"
        % (len(written), len(res_specs), len(ds_specs))
    )


HELPERS_GO = """// Code generated by gen/crudgen.py. DO NOT EDIT.
//
// Shared runtime helpers used by every generated entity resource /
// data source. These are package-level utilities over the generic
// client bridge; they do not depend on any specific entity.

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/numberplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"__MODULE__/internal/client"
)

// diagAppender is the subset of diag.Diagnostics used by generated state writers.
type diagAppender = diag.Diagnostics

// requireReplace marks the named top-level schema attributes as RequiresReplace,
// so any change to one forces the resource to be replaced (destroy + create). It
// is used by resources whose API has no update operation: an in-place change is
// impossible, so the only correct apply for a changed attribute is a replacement.
// The plan-modifier value is typed per attribute kind, hence the type switch; an
// unknown attribute name or an attribute kind not listed here is skipped.
func requireReplace(attrs map[string]schema.Attribute, names ...string) {
	for _, n := range names {
		switch a := attrs[n].(type) {
		case schema.BoolAttribute:
			a.PlanModifiers = append(a.PlanModifiers, boolplanmodifier.RequiresReplace())
			attrs[n] = a
		case schema.Float64Attribute:
			a.PlanModifiers = append(a.PlanModifiers, float64planmodifier.RequiresReplace())
			attrs[n] = a
		case schema.Int64Attribute:
			a.PlanModifiers = append(a.PlanModifiers, int64planmodifier.RequiresReplace())
			attrs[n] = a
		case schema.NumberAttribute:
			a.PlanModifiers = append(a.PlanModifiers, numberplanmodifier.RequiresReplace())
			attrs[n] = a
		case schema.StringAttribute:
			a.PlanModifiers = append(a.PlanModifiers, stringplanmodifier.RequiresReplace())
			attrs[n] = a
		case schema.ListAttribute:
			a.PlanModifiers = append(a.PlanModifiers, listplanmodifier.RequiresReplace())
			attrs[n] = a
		case schema.ListNestedAttribute:
			a.PlanModifiers = append(a.PlanModifiers, listplanmodifier.RequiresReplace())
			attrs[n] = a
		case schema.MapAttribute:
			a.PlanModifiers = append(a.PlanModifiers, mapplanmodifier.RequiresReplace())
			attrs[n] = a
		case schema.MapNestedAttribute:
			a.PlanModifiers = append(a.PlanModifiers, mapplanmodifier.RequiresReplace())
			attrs[n] = a
		case schema.SetAttribute:
			a.PlanModifiers = append(a.PlanModifiers, setplanmodifier.RequiresReplace())
			attrs[n] = a
		case schema.SetNestedAttribute:
			a.PlanModifiers = append(a.PlanModifiers, setplanmodifier.RequiresReplace())
			attrs[n] = a
		case schema.ObjectAttribute:
			a.PlanModifiers = append(a.PlanModifiers, objectplanmodifier.RequiresReplace())
			attrs[n] = a
		case schema.SingleNestedAttribute:
			a.PlanModifiers = append(a.PlanModifiers, objectplanmodifier.RequiresReplace())
			attrs[n] = a
		}
	}
}

// stabilizeComputed adds a type-appropriate UseStateForUnknown plan modifier to
// every Computed attribute, so a value the server populated (already in state) is
// preserved across plans instead of being re-planned as "(known after apply)".
// This keeps applies idempotent and -- critically -- lets an IMPORTED resource
// reach a clean plan: without it, computed fields the API echoes
// (annotation.user/user_id, the resolved project_id, the server id) flip to
// unknown on every plan and force a spurious in-place change.
func stabilizeComputed(attrs map[string]schema.Attribute) {
	for n := range attrs {
		switch a := attrs[n].(type) {
		case schema.BoolAttribute:
			if a.Computed {
				a.PlanModifiers = append(a.PlanModifiers, boolplanmodifier.UseStateForUnknown())
				attrs[n] = a
			}
		case schema.Float64Attribute:
			if a.Computed {
				a.PlanModifiers = append(a.PlanModifiers, float64planmodifier.UseStateForUnknown())
				attrs[n] = a
			}
		case schema.Int64Attribute:
			if a.Computed {
				a.PlanModifiers = append(a.PlanModifiers, int64planmodifier.UseStateForUnknown())
				attrs[n] = a
			}
		case schema.NumberAttribute:
			if a.Computed {
				a.PlanModifiers = append(a.PlanModifiers, numberplanmodifier.UseStateForUnknown())
				attrs[n] = a
			}
		case schema.StringAttribute:
			if a.Computed {
				a.PlanModifiers = append(a.PlanModifiers, stringplanmodifier.UseStateForUnknown())
				attrs[n] = a
			}
		}
	}
}

// unwrapBody optionally unwraps the BaseOkResponseModel envelope and decodes the
// (unenveloped) body into a map. A non-object body yields an empty map.
func unwrapBody(respBody []byte, enveloped bool) (map[string]any, error) {
	body := respBody
	if enveloped {
		inner, err := client.UnwrapEnvelope(respBody)
		if err != nil {
			return nil, err
		}
		body = inner
	}
	if len(body) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		// Body was not a JSON object (e.g. bare value); nothing to map.
		return map[string]any{}, nil
	}
	return normalizeNumbers(m).(map[string]any), nil
}

// unwrapResultsMap peels an envelope `results` shaped as a single-entry map of
// id -> object (Mixpanel's themes_to_dict_map convention) down to the inner
// object, injecting the map key as a synthetic "id" so the standard id path
// (nestedID(wire,"id")) and the state merge see the real entity body. When
// resultsMap is false, or the body is not exactly a single {key: object} pair,
// the body is returned unchanged so non-map entities are unaffected.
func unwrapResultsMap(body map[string]any, resultsMap bool) map[string]any {
	if !resultsMap || len(body) != 1 {
		return body
	}
	for k, v := range body {
		inner, ok := v.(map[string]any)
		if !ok {
			return body
		}
		// Copy so we never mutate the caller's decoded map; inject the map key
		// as "id" only when the inner object does not already carry one.
		out := make(map[string]any, len(inner)+1)
		for ik, iv := range inner {
			out[ik] = iv
		}
		if _, has := out["id"]; !has {
			out["id"] = k
		}
		return out
	}
	return body
}

// findInList selects, from a collection GET response, the single object whose
// identity value (at dottedPath, default "id") string-equals wantID. The body is
// optionally unwrapped from the BaseOkResponseModel envelope; the unwrapped value
// must be a JSON array of objects (the Mixpanel list convention). Returns the
// matched object as a normalized map, a found flag, and any decode error. This is
// the read path for entities that expose no instance GET route (read-from-list).
func findInList(respBody []byte, enveloped bool, dottedPath, wantID string) (map[string]any, bool, error) {
	body := respBody
	if enveloped {
		inner, err := client.UnwrapEnvelope(respBody)
		if err != nil {
			return nil, false, err
		}
		body = inner
	}
	if len(body) == 0 {
		return nil, false, nil
	}
	var arr []any
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	if err := dec.Decode(&arr); err != nil {
		// Not a JSON array (e.g. an object/bare value); nothing to select.
		return nil, false, nil
	}
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		nm := normalizeNumbers(m).(map[string]any)
		if got, ok := nestedID(nm, dottedPath); ok && got == wantID {
			return nm, true, nil
		}
	}
	return nil, false, nil
}

// collectIDsFromList unwraps a collection GET response (optionally enveloped) into
// its `results` array and returns the identity value of every element at dottedPath
// (default "id"), rendered as a string. Null / missing ids are skipped (some list
// item id fields are nullable, e.g. custom_alert). The order of the API response is
// preserved so the result is deterministic. Used by the plural list data sources.
func collectIDsFromList(respBody []byte, enveloped bool, dottedPath string) ([]string, error) {
	body := respBody
	if enveloped {
		inner, err := client.UnwrapEnvelope(respBody)
		if err != nil {
			return nil, err
		}
		body = inner
	}
	out := []string{}
	if len(body) == 0 {
		return out, nil
	}
	var arr []any
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	if err := dec.Decode(&arr); err != nil {
		// Not a JSON array (unexpected for these endpoints) -- return empty, not
		// error, so an empty/odd list yields zero ids rather than failing the plan.
		return out, nil
	}
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		nm := normalizeNumbers(m).(map[string]any)
		if id, ok := nestedID(nm, dottedPath); ok && id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

// compositeImportID renders the "<project_id>:<id>" composite import id consumed by
// the resource ImportState. When projectID is empty (unscoped entity) the bare id is
// returned so the value is still a valid single-segment import id.
func compositeImportID(projectID, id string) string {
	if projectID == "" {
		return id
	}
	return projectID + ":" + id
}

// selectNewestFromList selects, from a collection mutation response (the FULL
// list returned by a body-id create), the element matching matchAttr==matchVal,
// choosing the numerically-largest id at dottedPath. Body-id creates (e.g. event
// drop filters) return the whole list rather than the created row, and the new row
// is unique by its match attribute (duplicates are rejected server-side); when more
// than one historically matched, the largest id is the just-created one. Returns
// the matched object, its id rendered as a string, and any decode error.
func selectNewestFromList(respBody []byte, enveloped bool, dottedPath, matchAttr, matchVal string) (map[string]any, string, error) {
	body := respBody
	if enveloped {
		inner, err := client.UnwrapEnvelope(respBody)
		if err != nil {
			return nil, "", err
		}
		body = inner
	}
	if len(body) == 0 {
		return nil, "", nil
	}
	var arr []any
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	if err := dec.Decode(&arr); err != nil {
		// Not a JSON array; nothing to select.
		return nil, "", nil
	}
	var best map[string]any
	var bestID string
	var bestNum *big.Float
	haveBest := false
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		nm := normalizeNumbers(m).(map[string]any)
		got, ok := nestedID(nm, matchAttr)
		if !ok || got != matchVal {
			continue
		}
		idStr, ok := nestedID(nm, dottedPath)
		if !ok {
			continue
		}
		num, _, perr := big.ParseFloat(idStr, 10, 256, big.ToNearestEven)
		if perr != nil {
			// Non-numeric id: take the first match deterministically.
			if !haveBest {
				best, bestID, haveBest = nm, idStr, true
			}
			continue
		}
		if !haveBest || bestNum == nil || num.Cmp(bestNum) > 0 {
			best, bestID, bestNum, haveBest = nm, idStr, num, true
		}
	}
	if !haveBest {
		return nil, "", nil
	}
	return best, bestID, nil
}

// wrapSingleton wraps a singleton's unwrapped GET body under wrapKey so it matches
// the request-derived resource schema (e.g. the data-standards GET returns the
// DataStandards root directly, but the schema -- built from the POST request --
// nests it under `data_standards`). When wrapKey is empty, or the body is already
// shaped with that single key, the body is returned unchanged.
func wrapSingleton(body map[string]any, wrapKey string) map[string]any {
	if wrapKey == "" {
		return body
	}
	if _, ok := body[wrapKey]; ok && len(body) == 1 {
		return body
	}
	return map[string]any{wrapKey: body}
}

// jsonNumberOrString renders an id for an outbound JSON body: a numeric-looking id
// is emitted as a json.Number (so the wire carries a JSON number, which the
// Mixpanel filter-id APIs expect), otherwise it is sent verbatim as a string.
func jsonNumberOrString(id string) any {
	if id == "" {
		return id
	}
	if _, _, err := big.ParseFloat(id, 10, 256, big.ToNearestEven); err == nil {
		return json.Number(id)
	}
	return id
}

// flatCreateID extracts a server-assigned id from a create response whose body
// does NOT match the read schema. The (optionally enveloped) body may be either a
// single flat object {id,...} or a single-element list [{id,...}]; in both cases
// the id is read from the first object at dottedPath (default "id"). Returns the
// id as a string, or "" when none is present. Used by read_after_create entities
// (e.g. event_definition) where the POST returns a flat id-bearing object/list.
func flatCreateID(respBody []byte, enveloped bool, dottedPath string) (string, error) {
	body := respBody
	if enveloped {
		inner, err := client.UnwrapEnvelope(respBody)
		if err != nil {
			return "", err
		}
		body = inner
	}
	if len(body) == 0 {
		return "", nil
	}
	// Try a JSON object first.
	var m map[string]any
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	if err := dec.Decode(&m); err == nil {
		nm := normalizeNumbers(m).(map[string]any)
		if got, ok := nestedID(nm, dottedPath); ok {
			return got, nil
		}
		return "", nil
	}
	// Fall back to a JSON array; use the first object element.
	var arr []any
	dec = json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	if err := dec.Decode(&arr); err != nil {
		return "", nil
	}
	for _, e := range arr {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		nm := normalizeNumbers(em).(map[string]any)
		if got, ok := nestedID(nm, dottedPath); ok {
			return got, nil
		}
	}
	return "", nil
}

// normalizeNumbers converts json.Number values produced by UseNumber back to
// float64 so the generic tftypes bridge sees a uniform numeric type.
func normalizeNumbers(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, ev := range x {
			x[k] = normalizeNumbers(ev)
		}
		return x
	case []any:
		for i, ev := range x {
			x[i] = normalizeNumbers(ev)
		}
		return x
	case json.Number:
		if f, err := x.Float64(); err == nil {
			return f
		}
		return x.String()
	default:
		return v
	}
}

// stringAttrFromRaw reads a top-level attribute from a raw object value and
// renders it as a string (handling string / number identity attributes).
func stringAttrFromRaw(raw tftypes.Value, name string) (string, error) {
	if raw.IsNull() || !raw.IsKnown() {
		return "", nil
	}
	obj := map[string]tftypes.Value{}
	if err := raw.As(&obj); err != nil {
		return "", fmt.Errorf("decoding object: %w", err)
	}
	v, ok := obj[name]
	if !ok || v.IsNull() || !v.IsKnown() {
		return "", nil
	}
	t := v.Type()
	switch {
	case t.Is(tftypes.String):
		var s string
		if err := v.As(&s); err != nil {
			return "", err
		}
		return s, nil
	case t.Is(tftypes.Number):
		var n big.Float
		if err := v.As(&n); err != nil {
			return "", err
		}
		return n.Text('f', -1), nil
	default:
		return "", fmt.Errorf("attribute %q is not a string or number", name)
	}
}

// nestedID walks a dotted json path within an unwrapped body and renders the
// value as a string.
func nestedID(wire map[string]any, dottedPath string) (string, bool) {
	parts := strings.Split(dottedPath, ".")
	var cur any = wire
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		cur, ok = m[p]
		if !ok || cur == nil {
			return "", false
		}
	}
	switch x := cur.(type) {
	case string:
		return x, true
	case float64:
		return big.NewFloat(x).Text('f', -1), true
	default:
		return fmt.Sprintf("%v", x), true
	}
}

// schemaTypeOfDataSource returns the tftypes.Type of a data source state schema.
func schemaTypeOfDataSource(ctx context.Context, state tfsdk.State) tftypes.Type {
	return state.Schema.Type().TerraformType(ctx)
}

// setImportID sets a parsed import-id segment onto the named root attribute of
// import state, choosing the wire type from kind ("string" | "int64" | "number").
// Terraform import ids are always strings, but the target attribute may be an
// Int64Attribute or NumberAttribute, in which case the raw segment is parsed.
// On a parse failure it appends an "Invalid import ID" diagnostic and returns.
func setImportID(ctx context.Context, state *tfsdk.State, diags *diag.Diagnostics, attr, raw, kind string) {
	switch kind {
	case "int64":
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			diags.AddError(
				"Invalid import ID",
				fmt.Sprintf("attribute %q expects an integer id, got %q: %s", attr, raw, err),
			)
			return
		}
		diags.Append(state.SetAttribute(ctx, path.Root(attr), n)...)
	case "number", "float64":
		f, _, err := big.ParseFloat(raw, 10, 512, big.ToNearestEven)
		if err != nil {
			diags.AddError(
				"Invalid import ID",
				fmt.Sprintf("attribute %q expects a numeric id, got %q: %s", attr, raw, err),
			)
			return
		}
		diags.Append(state.SetAttribute(ctx, path.Root(attr), f)...)
	default: // "string"
		diags.Append(state.SetAttribute(ctx, path.Root(attr), raw)...)
	}
}
"""


if __name__ == "__main__":
    main()
