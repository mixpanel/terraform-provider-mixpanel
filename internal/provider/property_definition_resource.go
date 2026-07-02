// MANUALLY MAINTAINED — DO NOT REGENERATE (not driven by gen/crudgen.py).
//
// property_definition manages one property's Lexicon metadata (Data
// Definitions "properties" module). The API surface is unusual and was
// live-verified against the webapp source on 2026-07-02:
//
//   - The urlconf (app_api/projects/data_definitions/urls.py) exposes
//     GET/PATCH/DELETE on the COLLECTION path /data-definitions/properties;
//     there is no per-instance path and no POST. The property is addressed by
//     `name` + `resourceType` ("Event" or "User") carried in the query string
//     (GET) or JSON body (PATCH/DELETE).
//   - PATCH without a `properties` array is the single-property path
//     (views.crud_property -> PropertyDefinitionAPI.patch): it UPSERTS — when
//     no appdb row exists for name+resourceType the synthetic definition is
//     materialized on save. Create therefore IS a PATCH. The response echoes
//     the full definition JSON. This path accepts description, displayName,
//     exampleValue, type, hidden, dropped and sensitive in one request, and
//     `name` in the body is the selector (harmless), unlike the bulk
//     `properties` path whose rows have per-field quirks — the provider never
//     uses the bulk path.
//   - GET by name NEVER 404s for a missing definition: it returns a synthetic
//     JSON with "id": 0 (PropertyDefinition.to_json renders `self.id or 0`).
//     id==0 is therefore the "definition row does not exist" signal used by
//     Read to detect external deletion.
//   - DELETE hard-deletes the row (204). It returns 400 "Must specify property
//     to DELETE" when the row is already gone (the name resolves to a
//     synthetic unsaved definition), and 409 when a metric/cohort still
//     references the property.
//
// ROUTING: the data-definitions urlconf is dual-mounted at
// /api/app/projects/{project_id}/data-definitions AND
// /api/app/workspaces/{workspace_id}/data-definitions over the same
// project-keyed rows. The provider prefers the workspace mount (canonical
// workspace: global > default > first — the mount the Mixpanel UI uses) and
// falls back to the project mount on 404, because the workspace mount is
// membership-gated and 404s before the view runs for non-members
// (live-verified: workspace 79 -> 200, non-member global workspace 75 -> 404).
// Same pattern as lexicon_tag.

package provider

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/mixpanel/terraform-provider-mixpanel/internal/client"
)

var (
	_ resource.Resource                = (*PropertyDefinitionResource)(nil)
	_ resource.ResourceWithConfigure   = (*PropertyDefinitionResource)(nil)
	_ resource.ResourceWithImportState = (*PropertyDefinitionResource)(nil)
)

// NewPropertyDefinitionResource constructs the property_definition resource.
func NewPropertyDefinitionResource() resource.Resource {
	return &PropertyDefinitionResource{}
}

type PropertyDefinitionResource struct {
	client               *client.Client
	workspacePathBuilder *client.WorkspacePathBuilder
}

// PropertyDefinitionModel is the typed state model for one property's Lexicon metadata.
type PropertyDefinitionModel struct {
	ID           types.Int64  `tfsdk:"id"`
	ProjectID    types.Int64  `tfsdk:"project_id"`
	Name         types.String `tfsdk:"name"`
	ResourceType types.String `tfsdk:"resource_type"`
	DisplayName  types.String `tfsdk:"display_name"`
	Description  types.String `tfsdk:"description"`
	ExampleValue types.String `tfsdk:"example_value"`
	Type         types.String `tfsdk:"type"`
	Hidden       types.Bool   `tfsdk:"hidden"`
	Dropped      types.Bool   `tfsdk:"dropped"`
	Sensitive    types.Bool   `tfsdk:"sensitive"`
}

func (r *PropertyDefinitionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_property_definition"
}

func (r *PropertyDefinitionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Lexicon metadata (display name, description, type, visibility flags) of one event or profile property.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:    true,
				Description: "The Lexicon definition row id. 0 is never stored: it means the definition does not exist.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "The project ID (defaults to the provider project).",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The property key this metadata attaches to (e.g. \"plan_type\" or \"$city\").",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"resource_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("Event"),
				Description: "Whether this is an event property (\"Event\") or a user-profile property (\"User\"). Defaults to \"Event\".",
				Validators: []validator.String{
					stringvalidator.OneOf("Event", "User"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				Optional:    true,
				Description: "Human-friendly display name shown in the Mixpanel UI.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Description of the property's meaning and usage.",
			},
			"example_value": schema.StringAttribute{
				Optional:    true,
				Description: "Example value(s) shown in Lexicon.",
			},
			"type": schema.StringAttribute{
				Optional:    true,
				Description: "Declared data type. One of: string, number, datetime, boolean, list, object, blob, null, dimension, unknown.",
				Validators: []validator.String{
					stringvalidator.OneOf("unknown", "string", "number", "datetime", "boolean", "list", "object", "blob", "null", "dimension"),
				},
			},
			"hidden": schema.BoolAttribute{
				Optional:    true,
				Description: "Hide the property from pickers in the Mixpanel UI.",
			},
			"dropped": schema.BoolAttribute{
				Optional:    true,
				Description: "Drop the property at ingestion (event properties only; the API rejects dropping user properties and Mixpanel-default properties).",
			},
			"sensitive": schema.BoolAttribute{
				Optional:    true,
				Description: "Mark the property as classified/sensitive (data governance).",
			},
		},
	}
}

func (r *PropertyDefinitionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}
	r.client = c
	r.workspacePathBuilder = client.NewWorkspacePathBuilder(c)
}

// projectID resolves the project scope from the model, falling back to the
// provider default.
func (r *PropertyDefinitionResource) projectID(m *PropertyDefinitionModel) string {
	if !m.ProjectID.IsNull() && !m.ProjectID.IsUnknown() {
		return r.client.ProjectID(strconv.FormatInt(m.ProjectID.ValueInt64(), 10))
	}
	return r.client.ProjectID("")
}

// dataDefinitionsBases returns the ordered data-definitions base paths to try
// (workspace mount first when resolvable, then the project mount). See the
// file header for the dual-mount / membership-gate rationale.
func (r *PropertyDefinitionResource) dataDefinitionsBases(ctx context.Context, projectID string) []string {
	projectBase := "/api/app/projects/" + projectID + "/data-definitions"
	if r.workspacePathBuilder != nil {
		if wid, err := r.workspacePathBuilder.WorkspaceID(ctx, projectID); err == nil && wid != "" {
			return []string{"/api/app/workspaces/" + wid + "/data-definitions", projectBase}
		}
	}
	return []string{projectBase}
}

// doDataDefinitions issues one data-definitions request, preferring the
// workspace mount and retrying the project mount on 404 (the membership check
// rejects non-members with 404 before the view runs, so no side effect has
// occurred when the retry fires).
func (r *PropertyDefinitionResource) doDataDefinitions(ctx context.Context, method, projectID, suffix string, body any) ([]byte, error) {
	bases := r.dataDefinitionsBases(ctx, projectID)
	var lastErr error
	for i, base := range bases {
		respBody, err := r.client.Do(ctx, method, base+suffix, body)
		if err == nil {
			return respBody, nil
		}
		lastErr = err
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 && i < len(bases)-1 {
			continue
		}
		return nil, err
	}
	return nil, lastErr
}

// resourceTypeOrDefault returns the effective resourceType selector.
func resourceTypeOrDefault(m *PropertyDefinitionModel) string {
	if !m.ResourceType.IsNull() && !m.ResourceType.IsUnknown() && m.ResourceType.ValueString() != "" {
		return m.ResourceType.ValueString()
	}
	return "Event"
}

// propertyPatchBody builds the single-property PATCH body from the plan.
// `name` + `resourceType` are the selectors; the metadata fields are included
// only when the plan sets them — except that a field transitioning from a
// non-null prior value to null is explicitly cleared (empty string / false),
// because omitting it would leave the server value in place and the state
// null, i.e. silent divergence. prior may be nil (Create).
func propertyPatchBody(plan, prior *PropertyDefinitionModel) map[string]any {
	body := map[string]any{
		"name":         plan.Name.ValueString(),
		"resourceType": resourceTypeOrDefault(plan),
	}
	setString := func(key string, pv types.String, sv func(*PropertyDefinitionModel) types.String) {
		if !pv.IsNull() && !pv.IsUnknown() {
			body[key] = pv.ValueString()
		} else if prior != nil && !sv(prior).IsNull() {
			body[key] = ""
		}
	}
	setBool := func(key string, pv types.Bool, sv func(*PropertyDefinitionModel) types.Bool) {
		if !pv.IsNull() && !pv.IsUnknown() {
			body[key] = pv.ValueBool()
		} else if prior != nil && !sv(prior).IsNull() {
			body[key] = false
		}
	}
	setString("description", plan.Description, func(m *PropertyDefinitionModel) types.String { return m.Description })
	setString("displayName", plan.DisplayName, func(m *PropertyDefinitionModel) types.String { return m.DisplayName })
	setString("exampleValue", plan.ExampleValue, func(m *PropertyDefinitionModel) types.String { return m.ExampleValue })
	// `type` cannot be meaningfully cleared server-side (there is no "unset");
	// it is only sent when configured.
	if !plan.Type.IsNull() && !plan.Type.IsUnknown() {
		body["type"] = plan.Type.ValueString()
	}
	setBool("hidden", plan.Hidden, func(m *PropertyDefinitionModel) types.Bool { return m.Hidden })
	setBool("dropped", plan.Dropped, func(m *PropertyDefinitionModel) types.Bool { return m.Dropped })
	setBool("sensitive", plan.Sensitive, func(m *PropertyDefinitionModel) types.Bool { return m.Sensitive })
	return body
}

// propertySelectorSuffix renders the GET query for a property selector.
func propertySelectorSuffix(name, resourceType string) string {
	return "/properties?name=" + url.QueryEscape(name) + "&resourceType=" + url.QueryEscape(resourceType)
}

// propertyWireID extracts the numeric id from an unwrapped response body
// (unwrapBody normalizes JSON numbers to int64, but float64 is tolerated for
// robustness).
func propertyWireID(wire map[string]any) (int64, bool) {
	switch v := wire["id"].(type) {
	case int64:
		return v, true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

// applyPropertyWire copies API response fields into the model. Non-null model
// values are refreshed from the response (drift on managed fields is visible);
// null model values adopt the API value only when it differs from the server
// default ("", false, "unknown"), so unmanaged fields the server materializes
// as empty do not churn between null and "" — while import (an all-null prior)
// still adopts every meaningful value.
func applyPropertyWire(m *PropertyDefinitionModel, wire map[string]any) {
	if v, ok := propertyWireID(wire); ok {
		m.ID = types.Int64Value(v)
	}
	if v, ok := wire["resourceType"].(string); ok && v != "" {
		m.ResourceType = types.StringValue(v)
	}
	refreshString := func(dst *types.String, key, defaultVal string) {
		v, ok := wire[key].(string)
		if !ok {
			return
		}
		if !dst.IsNull() || v != defaultVal {
			*dst = types.StringValue(v)
		}
	}
	refreshBool := func(dst *types.Bool, key string) {
		v, ok := wire[key].(bool)
		if !ok {
			return
		}
		if !dst.IsNull() || v {
			*dst = types.BoolValue(v)
		}
	}
	refreshString(&m.Description, "description", "")
	refreshString(&m.DisplayName, "displayName", "")
	refreshString(&m.ExampleValue, "exampleValue", "")
	refreshString(&m.Type, "type", "unknown")
	refreshBool(&m.Hidden, "hidden")
	refreshBool(&m.Dropped, "dropped")
	refreshBool(&m.Sensitive, "sensitive")
}

// finishState resolves the computed scope attributes and persists the model.
func (r *PropertyDefinitionResource) finishState(m *PropertyDefinitionModel, projectID string, diags *diagAppender) {
	pid, err := strconv.ParseInt(projectID, 10, 64)
	if err != nil {
		diags.AddError("Resolving project_id", fmt.Sprintf("project id %q is not numeric: %s", projectID, err))
		return
	}
	m.ProjectID = types.Int64Value(pid)
	if m.ResourceType.IsNull() || m.ResourceType.IsUnknown() {
		m.ResourceType = types.StringValue("Event")
	}
}

func (r *PropertyDefinitionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PropertyDefinitionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := r.projectID(&plan)
	// Single-property PATCH upserts: it materializes the definition row when
	// none exists yet, so create and update are the same request.
	respBody, err := r.doDataDefinitions(ctx, "PATCH", projectID, "/properties", propertyPatchBody(&plan, nil))
	if err != nil {
		resp.Diagnostics.AddError("Creating property_definition", err.Error())
		return
	}
	wire, err := unwrapBody(respBody, true)
	if err != nil {
		resp.Diagnostics.AddError("Decoding property_definition response", err.Error())
		return
	}
	applyPropertyWire(&plan, wire)
	r.finishState(&plan, projectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PropertyDefinitionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PropertyDefinitionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := r.projectID(&state)
	respBody, err := r.doDataDefinitions(ctx, "GET", projectID,
		propertySelectorSuffix(state.Name.ValueString(), resourceTypeOrDefault(&state)), nil)
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading property_definition", err.Error())
		return
	}
	wire, err := unwrapBody(respBody, true)
	if err != nil {
		resp.Diagnostics.AddError("Decoding property_definition response", err.Error())
		return
	}
	// The API never 404s a missing definition: it synthesizes a JSON body with
	// id == 0. That is the "row deleted externally" signal.
	if id, ok := propertyWireID(wire); ok && id == 0 {
		resp.State.RemoveResource(ctx)
		return
	}
	applyPropertyWire(&state, wire)
	r.finishState(&state, projectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PropertyDefinitionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state PropertyDefinitionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := r.projectID(&plan)
	respBody, err := r.doDataDefinitions(ctx, "PATCH", projectID, "/properties", propertyPatchBody(&plan, &state))
	if err != nil {
		resp.Diagnostics.AddError("Updating property_definition", err.Error())
		return
	}
	wire, err := unwrapBody(respBody, true)
	if err != nil {
		resp.Diagnostics.AddError("Decoding property_definition response", err.Error())
		return
	}
	applyPropertyWire(&plan, wire)
	r.finishState(&plan, projectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PropertyDefinitionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PropertyDefinitionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := r.projectID(&state)
	body := map[string]any{
		"name":         state.Name.ValueString(),
		"resourceType": resourceTypeOrDefault(&state),
	}
	if _, err := r.doDataDefinitions(ctx, "DELETE", projectID, "/properties", body); err != nil {
		if apiErr, ok := err.(*client.APIError); ok {
			if apiErr.StatusCode == 404 {
				return
			}
			// DELETE on an already-deleted definition resolves to a synthetic
			// unsaved row and the view rejects it with this specific 400; the
			// row is gone, which is the desired end state.
			if apiErr.StatusCode == 400 && strings.Contains(apiErr.Body, "Must specify property to DELETE") {
				return
			}
		}
		resp.Diagnostics.AddError("Deleting property_definition", err.Error())
		return
	}
}

func (r *PropertyDefinitionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID: "project_id:resource_type:name". name is last (and taken
	// verbatim, colons included) because property keys may contain ':'.
	parts := strings.SplitN(req.ID, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("expected import ID in the form \"project_id:resource_type:name\" (resource_type is Event or User), got %q", req.ID),
		)
		return
	}
	if parts[1] != "Event" && parts[1] != "User" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("resource_type must be \"Event\" or \"User\", got %q", parts[1]),
		)
		return
	}
	pid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("project_id %q is not numeric", parts[0]))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), pid)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("resource_type"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[2])...)
}
