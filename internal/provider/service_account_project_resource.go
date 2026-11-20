// Hand-written RPC association resource. gen/crudgen.py preserves this file.
// Service-account membership is a numeric account/project pair in the nested
// service-accounts response, not a free-text key in a generic RPC list.

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/mixpanel/terraform-provider-mixpanel/internal/client"
	rsc "github.com/mixpanel/terraform-provider-mixpanel/internal/provider/resource_service_account_project"
)

var (
	_ resource.Resource                = (*ServiceAccountProjectResource)(nil)
	_ resource.ResourceWithConfigure   = (*ServiceAccountProjectResource)(nil)
	_ resource.ResourceWithImportState = (*ServiceAccountProjectResource)(nil)
)

// NewServiceAccountProjectResource constructs the service_account_project RPC association resource.
func NewServiceAccountProjectResource() resource.Resource {
	return &ServiceAccountProjectResource{}
}

type ServiceAccountProjectResource struct {
	client *client.Client
}

// ServiceAccountProjectModel is the typed state model for the service_account_project association.
type ServiceAccountProjectModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Key            types.String `tfsdk:"key"`
	Payload        types.String `tfsdk:"payload"`
}

func (r *ServiceAccountProjectResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_account_project"
}

func (r *ServiceAccountProjectResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = rsc.ServiceAccountProjectResourceSchema(ctx)
}

func (r *ServiceAccountProjectResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
}

// orgID resolves the organization scope from the model (falling back to the
// provider default organization).
func (r *ServiceAccountProjectResource) orgID(m *ServiceAccountProjectModel) string {
	if !m.OrganizationID.IsNull() && !m.OrganizationID.IsUnknown() && m.OrganizationID.ValueString() != "" {
		return r.client.OrganizationID(m.OrganizationID.ValueString())
	}
	return r.client.OrganizationID("")
}

// decodePayload parses the jsonencode payload string into a request body.
func (r *ServiceAccountProjectResource) decodePayload(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}
	var body any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil, fmt.Errorf("payload is not valid JSON: %w", err)
	}
	return body, nil
}

func (r *ServiceAccountProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m ServiceAccountProjectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	org := r.orgID(&m)
	body, err := r.decodePayload(m.Payload.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Encoding service_account_project payload", err.Error())
		return
	}
	createPath := strings.NewReplacer("{organization_id}", org).Replace("/organizations/{organization_id}/add-to-project/")
	if _, err := r.client.Do(ctx, "POST", createPath, body); err != nil {
		resp.Diagnostics.AddError("Creating service_account_project", err.Error())
		return
	}
	m.OrganizationID = types.StringValue(org)
	m.ID = types.StringValue(org + ":" + m.Key.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *ServiceAccountProjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m ServiceAccountProjectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	org := r.orgID(&m)
	listPath := strings.NewReplacer("{organization_id}", org).Replace("/api/app/organizations/{organization_id}/service-accounts")
	respBody, err := r.client.Do(ctx, "GET", listPath, nil)
	if err != nil {
		resp.Diagnostics.AddError("Reading service_account_project", err.Error())
		return
	}
	accountID, projectID, err := parseServiceAccountProjectKey(m.Key.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading service_account_project", err.Error())
		return
	}
	role, found, err := serviceAccountProjectRole(respBody, accountID, projectID)
	if err != nil {
		resp.Diagnostics.AddError("Reading service_account_project", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	// Only reconcile a role that the caller already owns in the untyped payload.
	// The read schema does not establish which fields the add verb accepts.
	payload, err := refreshServiceAccountProjectPayload(m.Payload.ValueString(), role)
	if err != nil {
		resp.Diagnostics.AddError("Reading service_account_project payload", err.Error())
		return
	}
	m.Payload = types.StringValue(payload)
	// The association still exists; preserve the config-owned columns and refresh
	// the synthetic identity.
	m.OrganizationID = types.StringValue(org)
	m.ID = types.StringValue(org + ":" + m.Key.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *ServiceAccountProjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// service_account_project has no update verb: key, payload and organization_id all force
	// replacement, so a real change recreates the association. This persists the
	// plan defensively (Update is only reached when nothing meaningful changed).
	var m ServiceAccountProjectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	org := r.orgID(&m)
	m.OrganizationID = types.StringValue(org)
	m.ID = types.StringValue(org + ":" + m.Key.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *ServiceAccountProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m ServiceAccountProjectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	org := r.orgID(&m)
	body, err := r.decodePayload(m.Payload.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Encoding service_account_project delete payload", err.Error())
		return
	}
	deletePath := strings.NewReplacer("{organization_id}", org).Replace("/organizations/{organization_id}/remove-from-project/")
	if _, err := r.client.Do(ctx, "POST", deletePath, body); err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Deleting service_account_project", err.Error())
		return
	}
}

func (r *ServiceAccountProjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 3 {
		resp.Diagnostics.AddError("Invalid import ID", "expected ORGANIZATION_ID:SERVICE_ACCOUNT_ID:PROJECT_ID with positive integer IDs")
		return
	}
	if _, err := serviceAccountProjectID(parts[0]); err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	key := parts[1] + ":" + parts[2]
	accountID, projectID, err := parseServiceAccountProjectKey(key)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	// These are the documented add/remove fields. A readable project role does
	// not imply that role is an accepted field on the add endpoint.
	payload, err := json.Marshal(map[string]int64{"serviceaccount_id": accountID, "project_id": projectID})
	if err != nil {
		resp.Diagnostics.AddError("Importing service_account_project", err.Error())
		return
	}
	m := ServiceAccountProjectModel{
		ID: types.StringValue(req.ID), OrganizationID: types.StringValue(parts[0]),
		Key: types.StringValue(key), Payload: types.StringValue(string(payload)),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func serviceAccountProjectID(raw string) (int64, error) {
	// IDs are integers in ServiceAccountModel and its nested projects schema.
	// Avoid float64 conversion so IDs above 2^53 retain their identity.
	for _, c := range raw {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("expected a positive integer ID, got %q", raw)
		}
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("expected a positive integer ID, got %q", raw)
	}
	return id, nil
}

func parseServiceAccountProjectKey(key string) (int64, int64, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected serviceaccount_id:project_id with positive integer IDs, got %q", key)
	}
	accountID, err := serviceAccountProjectID(parts[0])
	if err != nil {
		return 0, 0, err
	}
	projectID, err := serviceAccountProjectID(parts[1])
	return accountID, projectID, err
}

// serviceAccountProjectRole joins the numeric account ID to its nested project
// IDs. Unknown/malformed list shapes are errors, not evidence of revoked access.
func serviceAccountProjectRole(body []byte, accountID, projectID int64) (string, bool, error) {
	inner, err := client.UnwrapEnvelope(body)
	if err != nil {
		return "", false, err
	}
	var accounts []struct {
		ID       json.RawMessage `json:"id"`
		Projects json.RawMessage `json:"projects"`
	}
	if err := json.Unmarshal(inner, &accounts); err != nil || accounts == nil {
		return "", false, fmt.Errorf("expected a service-accounts array in the API response")
	}
	var role string
	found := false
	for _, account := range accounts {
		id, err := serviceAccountProjectID(string(account.ID))
		if err != nil {
			return "", false, fmt.Errorf("invalid service-account response ID: %w", err)
		}
		if id != accountID {
			continue
		}
		var projects []struct {
			ID   json.RawMessage `json:"id"`
			Role *string         `json:"role"`
		}
		if err := json.Unmarshal(account.Projects, &projects); err != nil || projects == nil {
			return "", false, fmt.Errorf("expected a projects array for service account %d", accountID)
		}
		for _, project := range projects {
			id, err := serviceAccountProjectID(string(project.ID))
			if err != nil {
				return "", false, fmt.Errorf("invalid project response ID: %w", err)
			}
			if id == projectID {
				if project.Role == nil || *project.Role == "" {
					return "", false, fmt.Errorf("missing project role for service account %d project %d", accountID, projectID)
				}
				if found {
					return "", false, fmt.Errorf("duplicate service account %d project %d association", accountID, projectID)
				}
				role, found = *project.Role, true
			}
		}
	}
	return role, found, nil
}

func refreshServiceAccountProjectPayload(raw, role string) (string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &payload); err != nil || payload == nil {
		return "", fmt.Errorf("expected a JSON object payload")
	}
	prior, ownsRole := payload["role"]
	if !ownsRole {
		return raw, nil
	}
	var oldRole string
	if err := json.Unmarshal(prior, &oldRole); err == nil && oldRole == role {
		return raw, nil
	}
	payload["role"], _ = json.Marshal(role)
	updated, err := json.Marshal(payload)
	return string(updated), err
}
