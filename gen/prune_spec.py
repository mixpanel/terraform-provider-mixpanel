#!/usr/bin/env python3
"""Deny-list spec pruning for the Mixpanel Terraform provider generator.

The merged OpenAPI spec contains the full Mixpanel app surface (~780 paths),
most of which is admin/internal plumbing, billing, GDPR/destructive RPCs, and
per-user UI/assistant endpoints that have no place in a config-management
Terraform provider. This script produces a "lightly pruned" spec: it keeps the
configuration surface and drops the noise via a prefix- and segment-based
deny-list.

Design notes
------------
* Matching is **prefix + path-segment** based, never loose substring. A segment
  rule fires only when a full URL segment equals a deny token (so a token like
  ``ai`` does not accidentally match ``available`` or ``maintenance``), and a
  prefix rule fires only on a full-segment boundary (so ``/api/app/me`` does not
  match ``/api/app/metrics``).
* Path template parameters (``{project_id}``, ``{id}`` ...) are normalized to a
  single placeholder (``{x}``) when comparing a path against the manifest, so
  differently-named params still compare equal.
* Components/schemas are intentionally **left untouched** -- we only prune the
  ``paths`` dict. The generator resolves ``$ref``s against ``components`` and
  tolerates unreferenced schemas, so pruning them would add risk for no benefit.
* Two layers win over the deny-list, checked *before* any deny rule:
    1. The segment keep-override ({audit-logs, usage, metadata}).
    2. Every path referenced by the provider's entity manifest. The manifest is
       the contract for what the provider manages; those paths must survive no
       matter what. This is also asserted at the end as a hard invariant, so a
       future deny rule can never silently amputate a managed resource.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

# --------------------------------------------------------------------------- #
# Default locations (overridable via CLI). These point at the frozen spec-gen
# spec location so the script is runnable with no arguments in the gen/ dir.
# --------------------------------------------------------------------------- #
_HERE = Path(__file__).resolve().parent
DEFAULT_SPEC = _HERE / "spec" / "openapi.merged.json"
DEFAULT_MANIFEST = _HERE / "refined_manifest.json"
DEFAULT_OUTPUT = _HERE / "spec" / "openapi.pruned.json"

# --------------------------------------------------------------------------- #
# Deny-list definitions.
# --------------------------------------------------------------------------- #

# Full-segment prefixes. A path is denied if it equals the prefix or begins with
# ``prefix + "/"``. The bare ``"/''"`` entry is the legacy unauthenticated
# session surface (paths literally namespaced under a ``''`` segment).
DENY_PREFIXES = [
    # admin / internal / auth / third-party webhooks
    "/admin/internal",
    "/api/appscim",
    "/oauth",
    "/slack",
    "/github/webhook",
    "/liveblocks",
    "/security/passkeys",
    "/''",  # legacy session surface
    # billing
    "/api/app/billing_v2",
    "/api/app/pricing",
    "/api/app/payments",
    "/api/app/invoice_info",
    "/api/app/attach_payment_method",
    "/api/app/create_setup_intent",
    "/api/app/credits_info",
    "/api/app/get-stripe-publishable-key",
    "/api/app/quote_tax",
    "/api/app/recommend_plan",
    "/api/app/tax-information",
    # destructive / GDPR
    "/api/app/data-deletions",
    "/api/app/data-retrievals",
    "/api/app/gdpr",
    # per-user UI / assistant
    "/api/app/me",
    "/api/app/nps_survey",
    "/api/app/education",
    "/api/app/product_updates",
    "/api/app/demo_project",
    "/api/app/avatars",
    "/api/app/user-media",
    "/api/app/user_support_info",
]

# Any path containing a full segment equal to "billing".
DENY_SEGMENT_BILLING = {"billing"}

# Destructive / GDPR action segments (anywhere in the path).
DENY_SEGMENT_DESTRUCTIVE = {
    "data-deletions",
    "data-retrievals",
    "reset",
    "reset_keys",
    "undelete",
    "transfer-project",
    "secure_passwords",
    "replay-redirect",
    "delete-projects",
    "delete-users",
    "delete-teams",
    "leave",
    "account_deletion_workflows",
    "start_account_deletion_workflow",
}

# Per-user UI / assistant segments (anywhere in the path).
DENY_SEGMENT_UI = {
    "rca",
    "ai",
    "ai-skills",
    "copilot",
    "search",
    "banners",
    "sendbird",
    "setup-guide",
    "onboarding-checklist",
    "set-onboarding-checklist-flag",
    "product-feedback",
    "content-sidenav",
    "homepage-dashboard",
    "top-creators",
}

# Invite / session segments (anywhere in the path).
DENY_SEGMENT_INVITE = {
    "regenerate-invite-url",
    "resend-invites",
    "accept-invite",
    "cancel_project_member_invite",
    "request_access",
    "send_magic_link",
    "register",
    "get_login_flow",
    "onboarding-questionnaire",
}

# Verbs for the top-level (non-/api/app) ``/projects/...`` RPC surface. These
# appear at varying positions -- e.g. ``/projects/delete/{project_id}`` (verb
# first) and ``/projects/{project_id}/experiment-settings`` (verb after the id)
# -- so the rule matches the verb as any non-parameter segment of a ``/projects``
# path.
DENY_PROJECTS_VERBS = {
    "delete",
    "reset",
    "undelete",
    "update",
    "session-settings",
    "experiment-settings",
    "id-merge-settings",
    "time-period-settings",
    "approve-access-request",
    "delete-project-access-requests",
}

# Verbs for the top-level ``/organizations/...`` RPC surface (matched as any
# non-parameter segment of an ``/organizations`` path).
DENY_ORGANIZATIONS_VERBS = {
    "delete-projects",
    "delete-teams",
    "delete-users",
    "undelete-projects",
    "transfer-project",
    "leave",
    "data-deletions",
    "data-retrievals",
}

# Keep-override segments: a path whose segments include any of these survives
# even if a deny rule would otherwise match. Checked before the deny-list.
KEEP_SEGMENTS = {"audit-logs", "usage", "metadata"}

# Manifest keys that hold a real path template (vs. metadata). ``connector``
# stores a bare short-name placeholder, not a path; the caller filters it out.
_MANIFEST_PATH_KEYS = (
    "collection",
    "instance",
    "list_path",
    "read_path",
    "update_path",
    "create_path",
    "delete_path",
)

_PARAM_RE = re.compile(r"\{[^}]+\}")


# --------------------------------------------------------------------------- #
# Helpers.
# --------------------------------------------------------------------------- #
def normalize(path: str) -> str:
    """Normalize template params so ``{foo}`` and ``{bar}`` compare equal."""
    return _PARAM_RE.sub("{x}", path)


def segments(path: str) -> list[str]:
    """Split a path into non-empty segments (keeps a literal ``''`` segment)."""
    return [s for s in path.split("/") if s != ""]


def _is_param(seg: str) -> bool:
    return seg.startswith("{") and seg.endswith("}")


def _matches_prefix(path: str, prefix: str) -> bool:
    """True if ``path`` is ``prefix`` or sits under it on a segment boundary."""
    return path == prefix or path.startswith(prefix + "/")


def deny_reason(path: str) -> str | None:
    """Return a human-readable deny reason for ``path``, or None to keep.

    The keep-override (KEEP_SEGMENTS) is the caller's responsibility and is
    applied before this function; here we only evaluate the deny-list itself.
    """
    segs = segments(path)
    nonparam = [s for s in segs if not _is_param(s)]

    # Full-segment prefixes.
    for prefix in DENY_PREFIXES:
        if _matches_prefix(path, prefix):
            return f"prefix:{prefix}"

    # Top-level /projects/<verb> RPC surface (verb at any non-param position).
    if segs and segs[0] == "projects":
        hit = next((s for s in segs[1:] if s in DENY_PROJECTS_VERBS), None)
        if hit:
            return f"projects-verb:{hit}"

    # Top-level /organizations/<verb> RPC surface.
    if segs and segs[0] == "organizations":
        hit = next((s for s in segs[1:] if s in DENY_ORGANIZATIONS_VERBS), None)
        if hit:
            return f"organizations-verb:{hit}"

    # Segment-set rules (any matching full segment anywhere in the path).
    for seg in nonparam:
        if seg in DENY_SEGMENT_BILLING:
            return "segment:billing"
    for seg in nonparam:
        if seg in DENY_SEGMENT_DESTRUCTIVE:
            return f"segment-destructive:{seg}"
    for seg in nonparam:
        if seg in DENY_SEGMENT_UI:
            return f"segment-ui:{seg}"
    for seg in nonparam:
        if seg in DENY_SEGMENT_INVITE:
            return f"segment-invite:{seg}"

    return None


def collect_manifest_paths(manifest: dict) -> set[str]:
    """Return the normalized set of real path templates the manifest references.

    The ``connector`` entity stores a bare short-name token ("connector") in its
    collection/instance fields as a placeholder rather than a real path; any
    value lacking a leading slash is treated as a non-path token and skipped.
    """
    out: set[str] = set()
    for entity in manifest.get("entities", []):
        for key in _MANIFEST_PATH_KEYS:
            value = entity.get(key)
            if value and value.startswith("/"):
                out.add(normalize(value))
    return out


def prune(spec: dict, manifest: dict) -> tuple[dict, list[str], list[tuple[str, str]]]:
    """Prune ``spec['paths']`` in place-free fashion.

    Returns (pruned_spec, kept_paths, dropped_pairs) where dropped_pairs is a
    list of (path, reason). Components are deliberately left untouched.
    """
    paths = spec.get("paths", {})
    manifest_paths = collect_manifest_paths(manifest)

    kept: list[str] = []
    dropped: list[tuple[str, str]] = []
    new_paths: dict = {}

    for path, item in paths.items():
        segs_nonparam = [s for s in segments(path) if not _is_param(s)]

        # Keep-override #1: protected segments win over everything.
        if any(s in KEEP_SEGMENTS for s in segs_nonparam):
            new_paths[path] = item
            kept.append(path)
            continue

        # Keep-override #2: anything the manifest manages must survive.
        if normalize(path) in manifest_paths:
            new_paths[path] = item
            kept.append(path)
            continue

        reason = deny_reason(path)
        if reason is None:
            new_paths[path] = item
            kept.append(path)
        else:
            dropped.append((path, reason))

    spec["paths"] = new_paths
    return spec, kept, dropped


def assert_invariant(kept: list[str], manifest: dict) -> None:
    """Fail loudly if any manifest path did not survive the prune."""
    kept_norm = {normalize(p) for p in kept}
    manifest_paths = collect_manifest_paths(manifest)
    missing = sorted(m for m in manifest_paths if m not in kept_norm)
    if missing:
        sys.stderr.write(
            "INVARIANT VIOLATION: the following manifest paths were pruned "
            "(every collection/instance path in the manifest MUST survive):\n"
        )
        for m in missing:
            sys.stderr.write(f"  - {m}\n")
        sys.exit(1)


# --------------------------------------------------------------------------- #
# CLI.
# --------------------------------------------------------------------------- #
def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=(__doc__ or "").splitlines()[0])
    parser.add_argument(
        "--spec",
        type=Path,
        default=DEFAULT_SPEC,
        help=f"Input merged OpenAPI spec (default: {DEFAULT_SPEC})",
    )
    parser.add_argument(
        "--manifest",
        type=Path,
        default=DEFAULT_MANIFEST,
        help=f"Provider entity manifest (default: {DEFAULT_MANIFEST})",
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=DEFAULT_OUTPUT,
        help=f"Where to write the pruned spec (default: {DEFAULT_OUTPUT})",
    )
    args = parser.parse_args(argv)

    spec = json.loads(args.spec.read_text())
    manifest = json.loads(args.manifest.read_text())

    total = len(spec.get("paths", {}))
    spec, kept, dropped = prune(spec, manifest)
    assert_invariant(kept, manifest)

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(spec, indent=2) + "\n")

    print(f"KEPT {len(kept)} / DROPPED {len(dropped)} of TOTAL {total}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
