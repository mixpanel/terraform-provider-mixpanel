#!/usr/bin/env python3
"""Post-process the tfplugingen-openapi IR (provider_code_spec.json) in place.

tfplugingen-openapi snake-cases response-body field names, which can collide with
a snake_case path parameter of the same logical id (e.g. custom_property's body
field `customPropertyId` -> `custom_property_id`, which also exists as the path
param). The framework generator rejects duplicate attribute names, so we dedupe:
keep the path-param variant (computed_optional, settable for import/read) and drop
the pure-`computed` body duplicate.

We also strip `default` from any attribute that is purely `computed`. tfplugingen
copies an OpenAPI `default: <x>` onto the attribute even when the value is fully
server-populated (computed, not settable). The framework then turns that into a
`Default: <x>` schema option, so the plan shows the static default while the API
returns the real value -> "Provider produced inconsistent result after apply"
(a round-trip bug). A static default only makes sense on a settable
(optional/computed_optional) attribute; on a pure-computed attribute it is always
wrong. This runs recursively so nested object/list attributes are covered too.

Run after `tfplugingen-openapi generate`, before `tfplugingen-framework generate`.
"""
import json
import os

HERE = os.path.dirname(os.path.abspath(__file__))
IR = os.path.join(HERE, "provider_code_spec.json")


def dedupe_attrs(attrs):
    by_name = {}
    order = []
    for a in attrs:
        n = a["name"]
        if n not in by_name:
            by_name[n] = a
            order.append(n)
            continue
        # collision: prefer the variant that is computed_optional/optional/required
        # (a settable path param) over a pure-computed body duplicate.
        existing = by_name[n]
        if _settable_rank(a) > _settable_rank(existing):
            by_name[n] = a
    return [by_name[n] for n in order]


def _settable_rank(attr):
    for _, v in attr.items():
        if isinstance(v, dict) and "computed_optional_required" in v:
            cor = v["computed_optional_required"]
            return {
                "required": 3,
                "optional": 2,
                "computed_optional": 2,
                "computed": 1,
            }.get(cor, 0)
    return 0


def strip_computed_defaults(node):
    """Recursively drop `default` from any attribute typed block whose
    computed_optional_required == "computed" (pure-computed, server-populated).

    A typed block is the per-type dict tfplugingen emits, e.g.
    {"bool": {"computed_optional_required": "computed", "default": {...}}}.
    Returns the number of defaults removed.
    """
    removed = 0
    if isinstance(node, dict):
        if node.get("computed_optional_required") == "computed" and "default" in node:
            del node["default"]
            removed += 1
        for v in node.values():
            removed += strip_computed_defaults(v)
    elif isinstance(node, list):
        for v in node:
            removed += strip_computed_defaults(v)
    return removed


def main():
    spec = json.load(open(IR))
    fixed = 0
    for bucket in ("resources", "datasources"):
        for ent in spec.get(bucket, []):
            attrs = ent.get("schema", {}).get("attributes")
            if not attrs:
                continue
            names = [a["name"] for a in attrs]
            if len(names) != len(set(names)):
                ent["schema"]["attributes"] = dedupe_attrs(attrs)
                fixed += 1
    stripped = strip_computed_defaults(spec)
    json.dump(spec, open(IR, "w"), indent=2)
    print("deduped duplicate attributes in %d entity schemas" % fixed)
    print("stripped %d static defaults from pure-computed attributes" % stripped)


if __name__ == "__main__":
    main()
