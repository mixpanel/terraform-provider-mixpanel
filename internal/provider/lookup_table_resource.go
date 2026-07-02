// MANUALLY MAINTAINED — DO NOT REGENERATE (not driven by gen/crudgen.py).
//
// lookup_table manages one Mixpanel lookup table (CSV → dimension data group).
// The API is a stateful create→upload handshake, verified against the webapp
// source (app_api/projects/data_definitions/{views,lookup_table_tasks}.py) and
// live-probed on 2026-07-02:
//
//  1. GET  .../data-definitions/lookup-tables/upload-url?content-type=text/csv
//     → {"url": <signed GCS PUT url, 60-min expiry>, "path": "<pid>/<uuid>", "key": "<uuid>"}
//     (the url is minted FIRST; it does not take an uploadId — the notes in
//     older design docs describing a POST-first flow are wrong).
//  2. PUT  <signed url> with the raw CSV bytes and Content-Type: text/csv
//     (external storage; no Mixpanel auth). Live-verified: HTTP 200.
//  3. POST .../data-definitions/lookup-tables as FORM data (request.POST):
//     name, path, key [, data-group-id to REPLACE an existing table's rows,
//     defer-mark-ready]. Files < 5 MB are ingested synchronously and the
//     response is {"id": <data_group_id>} or {"error": "..."}; files ≥ 5 MB
//     go through Celery and the response is {"uploadId": "<task id>"}.
//  4. GET  .../lookup-tables/upload-status?upload-id=<task id> polls the async
//     path. uploadStatus is a Celery state: PENDING/STARTED/RETRY → keep
//     polling; SUCCESS → "result" carries the terminal {"id":...}/{"error":...}
//     object; FAILURE/REVOKED → failed; NOTFOUND → result row not visible yet.
//  5. PATCH .../lookup-tables with JSON {"data-group-id": N, "name"?, "description"?}
//     renames/redescribes (404s when the table does not exist).
//  6. DELETE .../lookup-tables with JSON {"data-group-ids": [N]} removes tables.
//     Deletion is gated server-side by the "can-delete-data-groups" config and
//     returns a descriptive 400 when the environment forbids it (live-verified).
//
// GET .../lookup-tables?data-group-id=N reads one table (enveloped single-item
// list; 404 when the id is unknown). Table ids are data-group ids serialized
// as STRINGS because they are full-range int64s (e.g. "-9199707515373727904");
// all id handling here stays in string space (json.Number decode) to avoid
// float64 precision loss.
//
// Live devbox caveat: steps 1–2 succeed against the real GCS bucket, but the
// devbox's arb backend rejects data-group creation (gRPC "GetRetentionPolicyDays
// failed ... Access denied for user 'readonly_sys'"), so step 3 returns a 500
// there. The Create error path surfaces the server body plus a hint for this
// class of environment failure.
//
// ROUTING: lookup-tables live in the dual-mounted data-definitions urlconf;
// the provider prefers the workspace mount with 404 fallback to the project
// mount, same as lexicon_tag / property_definition.

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/mixpanel/terraform-provider-mixpanel/internal/client"
)

var (
	_ resource.Resource                = (*LookupTableResource)(nil)
	_ resource.ResourceWithConfigure   = (*LookupTableResource)(nil)
	_ resource.ResourceWithImportState = (*LookupTableResource)(nil)
)

// lookupTablePollTimeout bounds the async upload-status polling loop.
const lookupTablePollTimeout = 10 * time.Minute

// NewLookupTableResource constructs the lookup_table resource.
func NewLookupTableResource() resource.Resource {
	return &LookupTableResource{}
}

type LookupTableResource struct {
	client               *client.Client
	workspacePathBuilder *client.WorkspacePathBuilder
}

// LookupTableModel is the typed state model for one lookup table.
type LookupTableModel struct {
	ID          types.String `tfsdk:"id"`
	ProjectID   types.Int64  `tfsdk:"project_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	CSVContent  types.String `tfsdk:"csv_content"`
}

func (r *LookupTableResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lookup_table"
}

func (r *LookupTableResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Mixpanel lookup table: uploads CSV content through the signed-URL handshake and keeps the table's name/description in sync.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The lookup table id (the dimension data-group id, an int64 rendered as a string).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
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
				Description: "The lookup table name (renamed in place via PATCH).",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Human-readable description.",
			},
			"csv_content": schema.StringAttribute{
				Required: true,
				Description: "The CSV content of the table (header row first; first column is the join key). " +
					"Changing it re-uploads the table in place through the signed-URL handshake. " +
					"Use file(\"...\") to source it from a file. The server cannot echo the raw CSV back, " +
					"so drift is detected against the configured value only.",
			},
		},
	}
}

func (r *LookupTableResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *LookupTableResource) projectID(m *LookupTableModel) string {
	if !m.ProjectID.IsNull() && !m.ProjectID.IsUnknown() {
		return r.client.ProjectID(strconv.FormatInt(m.ProjectID.ValueInt64(), 10))
	}
	return r.client.ProjectID("")
}

// dataDefinitionsBases mirrors the lexicon_tag routing: workspace mount first
// when resolvable, project mount as the 404 fallback.
func (r *LookupTableResource) dataDefinitionsBases(ctx context.Context, projectID string) []string {
	projectBase := "/api/app/projects/" + projectID + "/data-definitions"
	if r.workspacePathBuilder != nil {
		if wid, err := r.workspacePathBuilder.WorkspaceID(ctx, projectID); err == nil && wid != "" {
			return []string{"/api/app/workspaces/" + wid + "/data-definitions", projectBase}
		}
	}
	return []string{projectBase}
}

// doDataDefinitions issues one JSON data-definitions request with the
// workspace-preferred, 404-fallback mount order.
func (r *LookupTableResource) doDataDefinitions(ctx context.Context, method, projectID, suffix string, body any) ([]byte, error) {
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

// doDataDefinitionsForm is doDataDefinitions for form-encoded requests (the
// lookup-table create/replace POST reads request.POST, not a JSON body).
func (r *LookupTableResource) doDataDefinitionsForm(ctx context.Context, method, projectID, suffix string, values map[string]any) ([]byte, error) {
	bases := r.dataDefinitionsBases(ctx, projectID)
	var lastErr error
	for i, base := range bases {
		respBody, err := r.client.DoForm(ctx, method, base+suffix, values)
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

// decodeLookupResults unwraps the {"status":"ok","results":...} envelope and
// decodes results with json.Number preserved (data-group ids are full-range
// int64s that float64 would corrupt).
func decodeLookupResults(respBody []byte) (any, error) {
	raw, err := client.UnwrapEnvelope(respBody)
	if err != nil {
		return nil, fmt.Errorf("unexpected response envelope: %w (body: %s)", err, bodySnippet(respBody))
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding results: %w (body: %s)", err, bodySnippet(respBody))
	}
	return out, nil
}

// lookupIDString renders an id value (json.Number from a create response, or
// string from the list endpoint) as its canonical decimal string.
func lookupIDString(v any) string {
	switch x := v.(type) {
	case json.Number:
		return x.String()
	case string:
		return x
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

// putSignedURL uploads the CSV bytes to the signed storage URL (step 2 of the
// handshake). This is external storage — no Mixpanel path or auth — so it goes
// through the raw HTTP client with a small bounded retry for transient
// storage-side errors.
func (r *LookupTableResource) putSignedURL(ctx context.Context, signedURL, csv string) error {
	httpClient := r.client.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, signedURL, strings.NewReader(csv))
		if err != nil {
			return err
		}
		// Must match the content-type the signed URL was minted for.
		req.Header.Set("Content-Type", "text/csv")
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		lastErr = fmt.Errorf("storage PUT returned HTTP %d: %s", resp.StatusCode, bodySnippet(body))
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			// Signed-URL 4xx (expired/signature) will not heal by retrying the
			// same URL.
			return lastErr
		}
	}
	return lastErr
}

// uploadCSV runs the full handshake (upload-url → storage PUT → form POST →
// optional upload-status polling) and returns the table's data-group id.
// dataGroupID == "" creates a new table; otherwise the existing table's rows
// are replaced in place.
func (r *LookupTableResource) uploadCSV(ctx context.Context, projectID, name, csv, dataGroupID string) (string, error) {
	// Step 1: mint the signed upload URL.
	respBody, err := r.doDataDefinitions(ctx, "GET", projectID, "/lookup-tables/upload-url?content-type="+url.QueryEscape("text/csv"), nil)
	if err != nil {
		return "", fmt.Errorf("requesting upload URL: %w", err)
	}
	res, err := decodeLookupResults(respBody)
	if err != nil {
		return "", fmt.Errorf("decoding upload-url response: %w", err)
	}
	urlInfo, _ := res.(map[string]any)
	signedURL, _ := urlInfo["url"].(string)
	blobPath, _ := urlInfo["path"].(string)
	blobKey, _ := urlInfo["key"].(string)
	if signedURL == "" || blobPath == "" || blobKey == "" {
		return "", fmt.Errorf("upload-url response missing url/path/key (body: %s)", bodySnippet(respBody))
	}

	// Step 2: PUT the CSV bytes to storage.
	if err := r.putSignedURL(ctx, signedURL, csv); err != nil {
		return "", fmt.Errorf("uploading CSV to signed storage URL: %w", err)
	}

	// Step 3: form POST registers the blob (and creates or replaces the table).
	form := map[string]any{
		"name": name,
		"path": blobPath,
		"key":  blobKey,
	}
	if dataGroupID != "" {
		form["data-group-id"] = dataGroupID
	}
	respBody, err = r.doDataDefinitionsForm(ctx, "POST", projectID, "/lookup-tables", form)
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode >= 500 {
			return "", fmt.Errorf("registering the uploaded CSV failed with HTTP %d. "+
				"This step creates the dimension data group in the ingestion backend; environments without "+
				"a working lookup-table ingestion pipeline (arb/GCS) reject it even though the CSV upload itself succeeded. "+
				"Server response: %s", apiErr.StatusCode, bodySnippet([]byte(apiErr.Body)))
		}
		return "", fmt.Errorf("registering the uploaded CSV: %w", err)
	}
	res, err = decodeLookupResults(respBody)
	if err != nil {
		return "", fmt.Errorf("decoding lookup-table create response: %w", err)
	}
	createRes, _ := res.(map[string]any)
	if errMsg, ok := createRes["error"]; ok && errMsg != nil {
		return "", fmt.Errorf("lookup table ingestion rejected the CSV: %v", errMsg)
	}
	if id, ok := createRes["id"]; ok && id != nil {
		// Synchronous path (< 5 MB): done.
		return lookupIDString(id), nil
	}
	uploadID, _ := createRes["uploadId"].(string)
	if uploadID == "" {
		return "", fmt.Errorf("lookup-table create response carried neither id nor uploadId (body: %s)", bodySnippet(respBody))
	}

	// Step 4: async path — poll upload-status until the Celery task lands.
	return r.pollUploadStatus(ctx, projectID, uploadID)
}

// pollUploadStatus polls .../lookup-tables/upload-status?upload-id=X with
// backoff until the task reaches a terminal Celery state or the poll budget is
// exhausted.
func (r *LookupTableResource) pollUploadStatus(ctx context.Context, projectID, uploadID string) (string, error) {
	deadline := time.Now().Add(lookupTablePollTimeout)
	delay := 2 * time.Second
	lastStatus := ""
	for {
		respBody, err := r.doDataDefinitions(ctx, "GET", projectID, "/lookup-tables/upload-status?upload-id="+url.QueryEscape(uploadID), nil)
		if err != nil {
			return "", fmt.Errorf("polling upload status: %w", err)
		}
		res, err := decodeLookupResults(respBody)
		if err != nil {
			return "", fmt.Errorf("decoding upload-status response: %w", err)
		}
		statusInfo, _ := res.(map[string]any)
		status, _ := statusInfo["uploadStatus"].(string)
		lastStatus = status
		switch status {
		case "SUCCESS":
			result, _ := statusInfo["result"].(map[string]any)
			if errMsg, ok := result["error"]; ok && errMsg != nil {
				return "", fmt.Errorf("lookup table ingestion rejected the CSV: %v", errMsg)
			}
			if id, ok := result["id"]; ok && id != nil {
				return lookupIDString(id), nil
			}
			return "", fmt.Errorf("upload finished but the result carried no table id (body: %s)", bodySnippet(respBody))
		case "FAILURE", "REVOKED":
			return "", fmt.Errorf("lookup table upload task ended in state %s (body: %s)", status, bodySnippet(respBody))
		}
		// PENDING / STARTED / RETRY / NOTFOUND (result row not visible yet):
		// keep polling with capped backoff.
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out after %s waiting for the lookup table upload to finish (last status: %q)", lookupTablePollTimeout, lastStatus)
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
		if delay < 10*time.Second {
			delay += delay / 2
		}
	}
}

// patchLookupTable PATCHes name/description onto an existing table.
func (r *LookupTableResource) patchLookupTable(ctx context.Context, projectID, id string, m *LookupTableModel) error {
	body := map[string]any{
		// The server int()s this, so the decimal string is safe (and lossless
		// for full-range int64 ids, unlike a JSON number round-trip).
		"data-group-id": id,
		"name":          m.Name.ValueString(),
	}
	if !m.Description.IsNull() && !m.Description.IsUnknown() {
		body["description"] = m.Description.ValueString()
	}
	_, err := r.doDataDefinitions(ctx, "PATCH", projectID, "/lookup-tables", body)
	return err
}

// finishState resolves the computed scope attribute and normalizes the model.
func (r *LookupTableResource) finishState(m *LookupTableModel, projectID string, diags *diagAppender) {
	pid, err := strconv.ParseInt(projectID, 10, 64)
	if err != nil {
		diags.AddError("Resolving project_id", fmt.Sprintf("project id %q is not numeric: %s", projectID, err))
		return
	}
	m.ProjectID = types.Int64Value(pid)
}

func (r *LookupTableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan LookupTableModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := r.projectID(&plan)
	id, err := r.uploadCSV(ctx, projectID, plan.Name.ValueString(), plan.CSVContent.ValueString(), "")
	if err != nil {
		resp.Diagnostics.AddError("Creating lookup_table", err.Error())
		return
	}
	plan.ID = types.StringValue(id)
	// The create form has no description field; attach it with a follow-up
	// PATCH when configured.
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		if err := r.patchLookupTable(ctx, projectID, id, &plan); err != nil {
			resp.Diagnostics.AddError("Setting lookup_table description after create", err.Error())
			return
		}
	}
	r.finishState(&plan, projectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *LookupTableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state LookupTableModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := r.projectID(&state)
	id := state.ID.ValueString()
	respBody, err := r.doDataDefinitions(ctx, "GET", projectID, "/lookup-tables?data-group-id="+url.QueryEscape(id), nil)
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading lookup_table", err.Error())
		return
	}
	res, err := decodeLookupResults(respBody)
	if err != nil {
		resp.Diagnostics.AddError("Decoding lookup_table response", err.Error())
		return
	}
	list, _ := res.([]any)
	var table map[string]any
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if lookupIDString(entry["id"]) == id {
			table = entry
			break
		}
	}
	if table == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	if name, ok := table["name"].(string); ok && name != "" {
		state.Name = types.StringValue(name)
	}
	// description: refresh managed values; a null (unmanaged) description only
	// adopts a non-empty server value (import support) — the server default ""
	// is left null to avoid null/"" churn.
	if desc, ok := table["description"].(string); ok {
		if !state.Description.IsNull() || desc != "" {
			state.Description = types.StringValue(desc)
		}
	}
	// csv_content cannot be read back from the API (the download endpoint
	// reconstructs rows from live profile queries, not the uploaded file), so
	// the prior state value is preserved as-is.
	r.finishState(&state, projectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *LookupTableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state LookupTableModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := r.projectID(&plan)
	id := state.ID.ValueString()

	// Changed CSV content → replace the table rows in place via the same
	// handshake, keyed by data-group-id.
	if !plan.CSVContent.Equal(state.CSVContent) {
		newID, err := r.uploadCSV(ctx, projectID, plan.Name.ValueString(), plan.CSVContent.ValueString(), id)
		if err != nil {
			resp.Diagnostics.AddError("Re-uploading lookup_table CSV", err.Error())
			return
		}
		if newID != "" {
			id = newID
		}
	}

	// Changed name/description → metadata PATCH. (Always safe to send both.)
	if !plan.Name.Equal(state.Name) || !plan.Description.Equal(state.Description) {
		if err := r.patchLookupTable(ctx, projectID, id, &plan); err != nil {
			resp.Diagnostics.AddError("Updating lookup_table", err.Error())
			return
		}
	}

	plan.ID = types.StringValue(id)
	r.finishState(&plan, projectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *LookupTableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state LookupTableModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := r.projectID(&state)
	// The server int()s each element, so decimal strings are safe.
	body := map[string]any{"data-group-ids": []string{state.ID.ValueString()}}
	if _, err := r.doDataDefinitions(ctx, "DELETE", projectID, "/lookup-tables", body); err != nil {
		if apiErr, ok := err.(*client.APIError); ok {
			if apiErr.StatusCode == 404 {
				return
			}
			// Deletion is gated by the server-side "can-delete-data-groups"
			// config; surface that state clearly instead of a bare 400.
			if apiErr.StatusCode == 400 && strings.Contains(apiErr.Body, "can-delete-data-groups") {
				resp.Diagnostics.AddError(
					"Deleting lookup_table",
					"This Mixpanel environment forbids deleting lookup tables (data groups): "+
						"the server-side 'can-delete-data-groups' setting is disabled. "+
						"Remove the resource from state with `terraform state rm` if you want Terraform to stop managing it. "+
						"Server response: "+bodySnippet([]byte(apiErr.Body)),
				)
				return
			}
		}
		resp.Diagnostics.AddError("Deleting lookup_table", err.Error())
		return
	}
}

func (r *LookupTableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID: "project_id:data_group_id". The data-group id may be negative
	// (full-range int64), so split on the FIRST colon only.
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("expected import ID in the form \"project_id:<data_group_id>\", got %q", req.ID),
		)
		return
	}
	pid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("project_id %q is not numeric", parts[0]))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), pid)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
