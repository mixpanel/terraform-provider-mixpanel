#!/usr/bin/env python3
"""Patch to add writable field extraction to crudgen.py"""

def writable_fields_for_entity(merged, man, is_create=False):
    """Extract the allowlist of writable fields from the entity's create/update
    request schema. Returns a set of snake_case Terraform attribute names that are
    writable (present in the request schema's properties). Used to filter full-body
    GETs at the PATCH write boundary so read-only fields are excluded from updates.

    For create: uses create_req_schema. For update: tries update_req_schema, then
    falls back to inferring {Pascal}PatchRequest or {Pascal}UpdateRequest from the
    entity name, and finally to create_req_schema. Returns empty set when no schema
    is available."""
    import re

    def snake(n):
        """Convert to snake_case (copied from crudgen.py)"""
        s = re.sub(r"[-\s]+", "_", n)
        s = re.sub(r"(.)([A-Z][a-z]+)", r"\1_\2", s)
        s = re.sub(r"([a-z0-9])([A-Z])", r"\1_\2", s)
        s = re.sub(r"__+", "_", s)
        return s.lower()

    def pascal(n):
        """Convert to PascalCase (copied from crudgen.py)"""
        return "".join(p.capitalize() for p in n.split("_"))

    if not merged:
        return set()
    schemas = (merged.get("components") or {}).get("schemas") or {}

    if is_create:
        schema_key = man.get("create_req_schema")
    else:
        # For update, try these in order:
        # 1. Explicit update_req_schema from manifest
        schema_key = man.get("update_req_schema")
        # 2. Infer {Pascal}PatchRequest (most common)
        if not schema_key:
            cls = pascal(man["resource_name"])
            inferred = f"{cls}PatchRequest"
            if inferred in schemas:
                schema_key = inferred
        # 3. Infer {Pascal}UpdateRequest
        if not schema_key:
            inferred = f"{cls}UpdateRequest"
            if inferred in schemas:
                schema_key = inferred
        # 4. Fall back to create schema
        if not schema_key:
            schema_key = man.get("create_req_schema")

    if not schema_key:
        return set()
    sch = schemas.get(schema_key)
    if not isinstance(sch, dict):
        return set()
    props = sch.get("properties") or {}
    # Return snake_case TF attr names (the bridge sends wire keys, but the allowlist
    # is checked against TF attribute names before wire-key translation).
    return {snake(p) for p in props.keys()}


# Test the function
if __name__ == "__main__":
    import json
    merged = json.load(open("spec/openapi.pruned.json"))
    man = {"resource_name": "cohort", "create_req_schema": "CohortCreateRequest"}

    create_fields = writable_fields_for_entity(merged, man, is_create=True)
    update_fields = writable_fields_for_entity(merged, man, is_create=False)

    print("Create writable fields for cohort:")
    print(sorted(create_fields))
    print("\nUpdate writable fields for cohort:")
    print(sorted(update_fields))
