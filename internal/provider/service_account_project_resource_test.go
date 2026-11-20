// Hand-written tests use nested service-account rows rather than the generic
// echo server. They exercise HTTP and framework state without live credentials.
// Mock acceptance of a role field does not prove the add endpoint accepts it.
package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/mixpanel/terraform-provider-mixpanel/internal/client"
	rsc "github.com/mixpanel/terraform-provider-mixpanel/internal/provider/resource_service_account_project"
)

func serviceAccountProjectTestState(t *testing.T, key, payload string) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: rsc.ServiceAccountProjectResourceSchema(context.Background())}
	diags := state.Set(context.Background(), &ServiceAccountProjectModel{
		ID: types.StringValue("1:" + key), OrganizationID: types.StringValue("1"),
		Key: types.StringValue(key), Payload: types.StringValue(payload),
	})
	if diags.HasError() {
		t.Fatalf("setting test state: %v", diags)
	}
	return state
}

func TestServiceAccountProjectRead(t *testing.T) {
	const payload = `{"serviceaccount_id":10,"project_id":20,"role":"admin"}`
	tests := []struct {
		name, body, key, payload, role string
		absent, invalid                bool
		status                         int
	}{
		{name: "nested pair", body: `{"status":"ok","results":[{"id":10,"projects":[{"id":20,"role":"admin"}]}]}`, role: "admin"},
		{name: "raw array", body: `[{"id":10,"projects":[{"id":20,"role":"admin"}]}]`, role: "admin"},
		{name: "wrong project", body: `[{"id":10,"projects":[{"id":21,"role":"admin"}]}]`, absent: true},
		{name: "unrelated account and strings", body: `[{"id":11,"username":"10:20","projects":[{"id":20,"role":"admin"}]},{"id":10,"username":"prefix10:20suffix","projects":[]}]`, absent: true},
		{name: "role drift", body: `[{"id":10,"projects":[{"id":20,"role":"analyst"}]}]`, role: "analyst"},
		{name: "no role ownership", body: `[{"id":10,"projects":[{"id":20,"role":"analyst"}]}]`, payload: `{"serviceaccount_id":10,"project_id":20}`},
		{name: "deleted account", body: `[]`, absent: true},
		{name: "deleted project", body: `[{"id":10,"projects":[]}]`, absent: true},
		{name: "large numeric IDs", body: `[{"id":9007199254740993,"projects":[{"id":9007199254740995,"role":"admin"}]}]`, key: "9007199254740993:9007199254740995", payload: `{"serviceaccount_id":9007199254740993,"project_id":9007199254740995}`, role: ""},
		{name: "HTTP 404 preserves state", body: `{"error":"not found"}`, status: 404, invalid: true},
		{name: "invalid composite key", body: `[]`, key: "10", invalid: true},
		{name: "malformed owned payload", body: `[{"id":10,"projects":[{"id":20,"role":"admin"}]}]`, payload: `[]`, invalid: true},
		{name: "malformed JSON", body: `[`, invalid: true},
		{name: "unknown list root", body: `{"service_accounts":[]}`, invalid: true},
		{name: "null list", body: `null`, invalid: true},
		{name: "API error envelope", body: `{"status":"error","error":"forbidden"}`, invalid: true},
		{name: "missing account ID", body: `[{"projects":[]}]`, invalid: true},
		{name: "string account ID", body: `[{"id":"10","projects":[]}]`, invalid: true},
		{name: "fractional account ID", body: `[{"id":10.5,"projects":[]}]`, invalid: true},
		{name: "missing projects", body: `[{"id":10}]`, invalid: true},
		{name: "missing project ID", body: `[{"id":10,"projects":[{"role":"admin"}]}]`, invalid: true},
		{name: "missing project role", body: `[{"id":10,"projects":[{"id":20}]}]`, invalid: true},
		{name: "duplicate grant", body: `[{"id":10,"projects":[{"id":20,"role":"admin"},{"id":20,"role":"admin"}]}]`, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/app/organizations/1/service-accounts" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				if tt.status != 0 {
					w.WriteHeader(tt.status)
				}
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			key := tt.key
			if key == "" {
				key = "10:20"
			}
			p := tt.payload
			if p == "" {
				p = payload
			}
			state := serviceAccountProjectTestState(t, key, p)
			res := ServiceAccountProjectResource{client: client.New(client.Config{BaseURL: srv.URL, DefaultOrganizationID: "1"})}
			resp := resource.ReadResponse{State: state}
			res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
			if got := resp.Diagnostics.HasError(); got != tt.invalid {
				t.Fatalf("diagnostic error=%v want %v: %v", got, tt.invalid, resp.Diagnostics)
			}
			if tt.invalid {
				if !resp.State.Raw.Equal(state.Raw) {
					t.Fatal("malformed response changed state")
				}
				return
			}
			if resp.State.Raw.IsNull() != tt.absent {
				t.Fatalf("state removed=%v want %v", resp.State.Raw.IsNull(), tt.absent)
			}
			if tt.absent {
				return
			}
			var m ServiceAccountProjectModel
			if diags := resp.State.Get(context.Background(), &m); diags.HasError() {
				t.Fatal(diags)
			}
			if m.Key.ValueString() != key || m.ID.ValueString() != "1:"+key {
				t.Fatalf("incorrect identity: %+v", m)
			}
			var got map[string]json.RawMessage
			_ = json.Unmarshal([]byte(m.Payload.ValueString()), &got)
			if tt.role != "" {
				var role string
				_ = json.Unmarshal(got["role"], &role)
				if role != tt.role {
					t.Fatalf("role=%q want %q", role, tt.role)
				}
			} else if m.Payload.ValueString() != p {
				t.Fatalf("unowned payload changed: %s", m.Payload.ValueString())
			}
		})
	}
}

func TestServiceAccountProjectImport(t *testing.T) {
	for _, id := range []string{"1:10:20", "1:9007199254740993:9007199254740995", "1", "1:10", "1:10:20:30", ":10:20", "1::20", "1:10:", "abc:10:20", "1:-10:20", "1:10:0", "1:10:20.5", "1:10:9223372036854775808"} {
		t.Run(id, func(t *testing.T) {
			state := serviceAccountProjectTestState(t, "10:20", `{"serviceaccount_id":10,"project_id":20}`)
			resp := resource.ImportStateResponse{State: tfsdk.State{Schema: state.Schema}}
			res := ServiceAccountProjectResource{}
			res.ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &resp)
			valid := id == "1:10:20" || id == "1:9007199254740993:9007199254740995"
			if resp.Diagnostics.HasError() == valid {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
			if !valid {
				return
			}
			var m ServiceAccountProjectModel
			if diags := resp.State.Get(context.Background(), &m); diags.HasError() {
				t.Fatal(diags)
			}
			if m.ID.ValueString() != id || m.OrganizationID.ValueString() != "1" || "1:"+m.Key.ValueString() != id {
				t.Fatalf("incorrect imported identity: %+v", m)
			}
			var p map[string]json.RawMessage
			_ = json.Unmarshal([]byte(m.Payload.ValueString()), &p)
			a, pid, err := parseServiceAccountProjectKey(m.Key.ValueString())
			if err != nil {
				t.Fatal(err)
			}
			if string(p["serviceaccount_id"]) != jsonNumberString(a) || string(p["project_id"]) != jsonNumberString(pid) || len(p) != 2 {
				t.Fatalf("unexpected import payload %s", m.Payload.ValueString())
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"status":"ok","results":[{"id":` + jsonNumberString(a) + `,"projects":[{"id":` + jsonNumberString(pid) + `,"role":"admin"}]}]}`))
			}))
			defer srv.Close()
			res.client = client.New(client.Config{BaseURL: srv.URL, DefaultOrganizationID: "999"})
			read := resource.ReadResponse{State: resp.State}
			res.Read(context.Background(), resource.ReadRequest{State: resp.State}, &read)
			if read.Diagnostics.HasError() || read.State.Raw.IsNull() {
				t.Fatalf("import refresh failed: %v", read.Diagnostics)
			}
			if !read.State.Raw.Equal(resp.State.Raw) {
				t.Fatal("import refresh added unowned role or changed identity")
			}
		})
	}
}

func jsonNumberString(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestServiceAccountProjectCreateDelete(t *testing.T) {
	const payload = `{"serviceaccount_id":10,"project_id":20}`
	calls := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method %s", r.Method)
		}
		calls = append(calls, r.URL.Path)
		var got map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		if len(got) != 2 || string(got["serviceaccount_id"]) != "10" || string(got["project_id"]) != "20" {
			t.Errorf("unexpected payload %v", got)
		}
		_, _ = w.Write([]byte(`{"status":"ok","results":{}}`))
	}))
	defer srv.Close()
	res := ServiceAccountProjectResource{client: client.New(client.Config{BaseURL: srv.URL, DefaultOrganizationID: "1"})}
	state := serviceAccountProjectTestState(t, "10:20", payload)
	create := resource.CreateResponse{State: tfsdk.State{Schema: state.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}}, &create)
	if create.Diagnostics.HasError() {
		t.Fatal(create.Diagnostics)
	}
	del := resource.DeleteResponse{}
	res.Delete(context.Background(), resource.DeleteRequest{State: create.State}, &del)
	if del.Diagnostics.HasError() {
		t.Fatal(del.Diagnostics)
	}
	if len(calls) != 2 || calls[0] != "/organizations/1/add-to-project/" || calls[1] != "/organizations/1/remove-from-project/" {
		t.Fatalf("unexpected lifecycle requests %v", calls)
	}
}

func TestAccServiceAccountProject_lifecycle(t *testing.T) {
	// This mock models membership only, with the documented numeric write fields.
	// The account is external to the grant resource and survives grant replacement.
	projectID := int64(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /api/app/organizations/1/service-accounts":
			projects := []map[string]any{}
			if projectID != 0 {
				projects = append(projects, map[string]any{"id": projectID, "role": "admin"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "results": []map[string]any{{"id": 10, "projects": projects}}})
		case "POST /organizations/1/add-to-project/", "POST /organizations/1/remove-from-project/":
			var payload struct {
				Account int64 `json:"serviceaccount_id"`
				Project int64 `json:"project_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Account != 10 {
				t.Errorf("bad grant request: %+v %v", payload, err)
			}
			if r.URL.Path == "/organizations/1/add-to-project/" {
				projectID = payload.Project
			} else {
				projectID = 0
			}
			_, _ = w.Write([]byte(`{"status":"ok","results":{}}`))
		default:
			t.Errorf("unexpected account mutation: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 404)
		}
	}))
	defer srv.Close()
	config := func(project string) string {
		return providerConfig(srv.URL, `resource "mixpanel_service_account_project" "test" {
  key = "10:`+project+`"
  payload = jsonencode({serviceaccount_id = 10, project_id = `+project+`})
}`)
	}
	tfresource.Test(t, tfresource.TestCase{ProtoV6ProviderFactories: testProtoV6, Steps: []tfresource.TestStep{
		{Config: config("20"), Check: tfresource.TestCheckResourceAttr("mixpanel_service_account_project.test", "id", "1:10:20")},
		{ResourceName: "mixpanel_service_account_project.test", ImportState: true, ImportStateVerify: true, ImportStateId: "1:10:20"},
		{Config: config("21"), ConfigPlanChecks: tfresource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("mixpanel_service_account_project.test", plancheck.ResourceActionReplace)}}, Check: tfresource.TestCheckResourceAttr("mixpanel_service_account_project.test", "id", "1:10:21")},
		// Plan only: prove payload-only changes replace the grant without claiming
		// that the real API accepts role in an add request.
		{Config: providerConfig(srv.URL, `resource "mixpanel_service_account_project" "test" {
  key = "10:21"
  payload = jsonencode({serviceaccount_id = 10, project_id = 21, role = "analyst"})
}`), PlanOnly: true, ExpectNonEmptyPlan: true, ConfigPlanChecks: tfresource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectResourceAction("mixpanel_service_account_project.test", plancheck.ResourceActionReplace)}}},
	}})
}
