// Package provider: shared-entity project sharing.
//
// Entities created through the App API by a service account are PRIVATE to
// that service account until they are shared. A Terraform user who applies a
// config with dashboards/cohorts/etc. and then opens the Mixpanel webapp sees
// nothing, because the entities exist but are invisible to every human user.
//
// This file implements the fix: every shareable resource carries a
// `share_with_project` attribute (Optional+Computed, defaulting to true) and
// the provider performs a share-after-create against the shared-entities API:
//
//	POST /api/app/projects/{p}/shared-entities/{entity_type}/{entity_id}/upsert
//	  body: {"id": <entity_id>, "projectShares": [{"id": <p>, "canEdit": true}]}
//	POST .../delete
//	  body: {"id": <entity_id>, "projectShares": [<p>]}
//	GET  .../shared-entities/{entity_type}/{entity_id}
//	  results: {"id":..., "userShares":[], "teamShares":[],
//	            "projectShares":[{"id":<p>,"name":...,"canEdit":true}], ...}
//
// (Request/response shapes verified live against the webapp on 2026-07-02;
// see also analytics/webapp/app_api/projects/shared_entities/.)
//
// The entity_type path segment is an EXACT-MATCH key (a wrong one 404s). Note
// the singular forms for metric and behavior, and that formulas ARE metrics.
//
// Failure mode: entity create succeeded but the share POST failed. The apply
// is NOT failed outright (the entity exists and is tracked in state); instead
// a warning diagnostic with remediation is emitted and share_with_project is
// recorded as false so a subsequent apply retries the share. When the user
// EXPLICITLY set share_with_project = true in config, Terraform core will
// additionally report its "inconsistent result after apply" error for the
// attribute — state is still persisted, so a re-apply retries the share.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/mixpanel/terraform-provider-mixpanel/internal/client"
)

// shareAttrName is the schema attribute controlling project sharing.
const shareAttrName = "share_with_project"

// Shared-entities entity_type path segments. These are exact-match keys in the
// webapp (app_api/constants.py); a wrong value 404s. metric and behavior are
// SINGULAR, and formulas are metrics.
const (
	sharedEntityTypeCohort         = "cohorts"
	sharedEntityTypeCustomEvent    = "custom-events"
	sharedEntityTypeCustomProperty = "custom-properties"
	sharedEntityTypeDashboard      = "dashboards"
	sharedEntityTypeMetric         = "metric"
	sharedEntityTypeBehavior       = "behavior"
	sharedEntityTypeFeatureFlag    = "feature-flags"
	sharedEntityTypeBookmark       = "bookmarks"
	sharedEntityTypeExperiment     = "experiments"
)

// shareWithProjectAttribute returns the schema attribute added to every
// shareable resource. Optional+Computed with the default applied in Create
// (the codebase does not use static schema defaults): unset ⇒ share.
func shareWithProjectAttribute() schema.BoolAttribute {
	return schema.BoolAttribute{
		Optional: true,
		Computed: true,
		MarkdownDescription: "Whether to share this entity with the whole project after creation. " +
			"Entities created by a service account are otherwise visible only to that service account. " +
			"Defaults to `true`.",
	}
}

// sharedEntityBasePath builds the project-scoped shared-entities path for one
// entity. entityID may be an integer id or a UUID (feature flags).
func sharedEntityBasePath(projectID, entityType, entityID string) string {
	return "/api/app/projects/" + projectID + "/shared-entities/" + entityType + "/" + entityID
}

// shareWireID renders an id for the shared-entities request body: numeric ids
// are sent as JSON numbers (matching the webapp contract), UUIDs as strings.
func shareWireID(id string) any {
	if _, err := strconv.ParseInt(id, 10, 64); err == nil {
		return json.Number(id)
	}
	return id
}

// shareEntityWithProject upserts a project share so the entity is visible
// project-wide. Verified live: POST .../upsert with
// {"id":<eid>,"projectShares":[{"id":<pid>,"canEdit":true}]} returns the full
// share dict enveloped in {"status":"ok","results":{...}}.
func shareEntityWithProject(ctx context.Context, c *client.Client, projectID, entityType, entityID string, canEdit bool) error {
	body := map[string]any{
		"id": shareWireID(entityID),
		"projectShares": []any{
			map[string]any{"id": shareWireID(projectID), "canEdit": canEdit},
		},
	}
	// DoUnwrap (out=nil) still unwraps the envelope so an HTTP-200
	// {"status":"error"} body fails the call.
	return c.DoUnwrap(ctx, "POST", sharedEntityBasePath(projectID, entityType, entityID)+"/upsert", body, nil)
}

// unshareEntityFromProject removes the project share. Verified live: POST
// .../delete with {"id":<eid>,"projectShares":[<pid>]} (bare id list, unlike
// the upsert's object list) empties projectShares.
func unshareEntityFromProject(ctx context.Context, c *client.Client, projectID, entityType, entityID string) error {
	body := map[string]any{
		"id":            shareWireID(entityID),
		"projectShares": []any{shareWireID(projectID)},
	}
	return c.DoUnwrap(ctx, "POST", sharedEntityBasePath(projectID, entityType, entityID)+"/delete", body, nil)
}

// readEntityProjectShare reports whether a project share for projectID exists
// on the entity. GET .../shared-entities/{type}/{id} returns
// {"status":"ok","results":{...,"projectShares":[{"id":<pid>,...}]}}.
func readEntityProjectShare(ctx context.Context, c *client.Client, projectID, entityType, entityID string) (bool, error) {
	respBody, err := c.Do(ctx, "GET", sharedEntityBasePath(projectID, entityType, entityID), nil)
	if err != nil {
		return false, err
	}
	wire, err := unwrapBody(respBody, true)
	if err != nil {
		return false, err
	}
	shares, _ := wire["projectShares"].([]any)
	for _, s := range shares {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := client.IDFromWire(m, "id"); ok && id == projectID {
			return true, nil
		}
	}
	return false, nil
}

// shareBoolFromRaw reads share_with_project from a raw root object. known is
// false when the attribute is absent, null, or unknown.
func shareBoolFromRaw(raw tftypes.Value) (val bool, known bool) {
	if raw.IsNull() || !raw.IsKnown() {
		return false, false
	}
	obj := map[string]tftypes.Value{}
	if err := raw.As(&obj); err != nil {
		return false, false
	}
	v, ok := obj[shareAttrName]
	if !ok || v.IsNull() || !v.IsKnown() || !v.Type().Is(tftypes.Bool) {
		return false, false
	}
	var b bool
	if err := v.As(&b); err != nil {
		return false, false
	}
	return b, true
}

// setShareState writes the effective share_with_project value into state.
func setShareState(ctx context.Context, state *tfsdk.State, diags *diag.Diagnostics, val bool) {
	diags.Append(state.SetAttribute(ctx, path.Root(shareAttrName), val)...)
}

// finishShareOnCreate runs after a successful create + state write: it applies
// the plan's share_with_project (default true when unset) and records the
// ACTUAL share outcome in state. A share failure is a warning, not an error —
// the entity exists and is tracked; recording false lets a re-apply retry.
func finishShareOnCreate(ctx context.Context, c *client.Client, state *tfsdk.State, diags *diag.Diagnostics, planRaw tftypes.Value, projectID, entityType, entityID string) {
	if diags.HasError() || entityID == "" {
		return
	}
	want := true
	if v, known := shareBoolFromRaw(planRaw); known {
		want = v
	}
	actual := false
	if want {
		if err := shareEntityWithProject(ctx, c, projectID, entityType, entityID, true); err != nil {
			diags.AddWarning(
				fmt.Sprintf("Created but not shared with project %s", projectID),
				fmt.Sprintf("The %s (id %s) was created, but sharing it with the project failed: %s\n\n"+
					"It is currently visible only to the creating service account. "+
					"share_with_project has been recorded as false in state; run `terraform apply` again to retry the share, "+
					"or share the entity manually from the Mixpanel UI.", entityType, entityID, err),
			)
		} else {
			actual = true
		}
	}
	setShareState(ctx, state, diags, actual)
}

// finishShareOnUpdate reconciles the planned share_with_project against the
// prior state after a successful update + state write. false→true shares,
// true→false unshares; failures warn and record the value that is actually in
// effect so a re-apply reconciles again.
func finishShareOnUpdate(ctx context.Context, c *client.Client, state *tfsdk.State, diags *diag.Diagnostics, planRaw, stateRaw tftypes.Value, projectID, entityType, entityID string) {
	if diags.HasError() || entityID == "" {
		return
	}
	want := true
	if v, known := shareBoolFromRaw(planRaw); known {
		want = v
	}
	have, haveKnown := shareBoolFromRaw(stateRaw)
	if haveKnown && want == have {
		setShareState(ctx, state, diags, want)
		return
	}
	// Prior value unknown (state predates this attribute) or differs: reconcile.
	// Both API calls are idempotent, so reconciling from an unknown prior is safe.
	if want {
		if err := shareEntityWithProject(ctx, c, projectID, entityType, entityID, true); err != nil {
			diags.AddWarning(
				fmt.Sprintf("Updated but not shared with project %s", projectID),
				fmt.Sprintf("The %s (id %s) was updated, but sharing it with the project failed: %s\n\n"+
					"share_with_project has been recorded as false in state; run `terraform apply` again to retry the share, "+
					"or share the entity manually from the Mixpanel UI.", entityType, entityID, err),
			)
			setShareState(ctx, state, diags, false)
			return
		}
		setShareState(ctx, state, diags, true)
		return
	}
	if err := unshareEntityFromProject(ctx, c, projectID, entityType, entityID); err != nil {
		diags.AddWarning(
			fmt.Sprintf("Could not unshare from project %s", projectID),
			fmt.Sprintf("The %s (id %s) was updated, but removing its project share failed: %s\n\n"+
				"share_with_project has been recorded as true in state; run `terraform apply` again to retry, "+
				"or unshare the entity manually from the Mixpanel UI.", entityType, entityID, err),
		)
		setShareState(ctx, state, diags, true)
		return
	}
	setShareState(ctx, state, diags, false)
}

// refreshShareOnRead refreshes share_with_project from the shared-entities API
// during Read/import. On a read error (permissions, sharing disabled for the
// project, transient failure) the prior state value is kept rather than
// failing the whole Read — the share state is auxiliary to the entity itself.
func refreshShareOnRead(ctx context.Context, c *client.Client, state *tfsdk.State, diags *diag.Diagnostics, projectID, entityType, entityID string) {
	if diags.HasError() || entityID == "" {
		return
	}
	shared, err := readEntityProjectShare(ctx, c, projectID, entityType, entityID)
	if err != nil {
		return
	}
	setShareState(ctx, state, diags, shared)
}
