// Mock-tier tests for the plan-time analytics validators: prove that a
// corrupt config FAILS AT PLAN/VALIDATE (before any API call), that a valid
// config still applies, that the service_account `expires` format is enforced
// at plan time, and that user_project_role payload ids reach the wire as JSON
// numbers.
package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccCohort_corruptGroupsRejectedAtPlan proves the scariest failure mode is
// blocked before the API is touched: a cohort group without a `filters` array
// saves with 200 on the real API and then crashes the webapp cohort builder;
// here it must fail during plan/validate — the mock records zero requests.
func TestAccCohort_corruptGroupsRejectedAtPlan(t *testing.T) {
	srv := newMockServer(t, mockOpts{enveloped: true})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		Steps: []resource.TestStep{
			{
				// groups[0] has no filters array (confirmed webapp-crash shape).
				Config: providerConfig(srv.URL, `
resource "mixpanel_cohort" "corrupt" {
  name = "tf-corrupt"
  groups = jsonencode([
    {
      event                     = { value = "$all_users", resourceType = "cohort", label = "All Users" }
      filtersOperator           = "and"
      behavioralFiltersOperator = "and"
    }
  ])
}`),
				ExpectError: regexp.MustCompile(`groups\[0\]\.filters: missing or null`),
			},
			{
				// Missing operators are also caught, with the JSON path named.
				Config: providerConfig(srv.URL, `
resource "mixpanel_cohort" "corrupt" {
  name = "tf-corrupt"
  groups = jsonencode([
    {
      event   = { value = "$all_users", resourceType = "cohort", label = "All Users" }
      filters = []
    }
  ])
}`),
				ExpectError: regexp.MustCompile(`groups\[0\]\.behavioralFiltersOperator: missing`),
			},
			{
				// The live-verified valid shape passes validation and applies.
				Config: providerConfig(srv.URL, `
resource "mixpanel_cohort" "ok" {
  name = "tf-ok"
  groups = jsonencode([
    {
      event                     = { value = "$all_users", resourceType = "cohort", label = "All Users" }
      filters                   = []
      filtersOperator           = "and"
      behavioralFiltersOperator = "and"
      groupingOperator          = null
    }
  ])
}`),
				Check: resource.TestCheckResourceAttr("mixpanel_cohort.ok", "name", "tf-ok"),
			},
		},
	})

	// The corrupt configs must never have reached the API.
	for _, cr := range srv.requests("POST") {
		var decoded map[string]any
		b, _ := json.Marshal(cr.body)
		_ = json.Unmarshal(b, &decoded)
		if name, _ := decoded["name"].(string); name == "tf-corrupt" {
			t.Fatalf("corrupt cohort config reached the API: %v", cr)
		}
	}
}

// TestAccCohort_duplicateName409IsActionable models the live-verified 409
// semantics (a LIVE cohort with the planned name already exists; soft-deleted
// cohorts never conflict) and asserts the provider surfaces the conflicting id
// and an import hint instead of a bare API error.
func TestAccCohort_duplicateName409IsActionable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			// The exact live shape: 409 + Mixpanel error envelope.
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"status": "error", "error": "Cohort with name \"dup\" already exists."}`)
		case http.MethodGet:
			// The live listing carries the conflicting cohort.
			fmt.Fprint(w, `{"status": "ok", "results": [{"id": 555, "name": "dup"}]}`)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL, `
resource "mixpanel_cohort" "dup" {
  name = "dup"
}`),
				// The diagnostic must carry the conflicting id and the import
				// hint (patterns kept short: Terraform wraps diagnostics).
				ExpectError: regexp.MustCompile(`(?s)cohort has id 555.*terraform import`),
			},
		},
	})
}

// TestAccBehavior_corruptDefinitionRejectedAtPlan covers the funnel step rules.
func TestAccBehavior_corruptDefinitionRejectedAtPlan(t *testing.T) {
	srv := newMockServer(t, mockOpts{enveloped: true, resultsMap: true, createIDField: "id"})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		Steps: []resource.TestStep{
			{
				// One-step funnel: confirmed query-builder crash shape.
				Config: providerConfig(srv.URL, `
resource "mixpanel_behavior" "corrupt" {
  name = "tf-corrupt-behavior"
  type = "funnel"
  definition = jsonencode({
    behavior = {
      type      = "funnel"
      behaviors = [{ type = "event", name = "Sign Up" }]
    }
  })
}`),
				ExpectError: regexp.MustCompile(`a funnel needs at least 2 steps, got 1`),
			},
			{
				// A step that names no event: confirmed crash shape.
				Config: providerConfig(srv.URL, `
resource "mixpanel_behavior" "corrupt" {
  name = "tf-corrupt-behavior"
  type = "funnel"
  definition = jsonencode({
    behavior = {
      type      = "funnel"
      behaviors = [{ type = "event", name = "Sign Up" }, { type = "event" }]
    }
  })
}`),
				ExpectError: regexp.MustCompile(`behaviors\[1\]: step defines no event`),
			},
		},
	})
}

// TestAccMetric_corruptMeasurementRejectedAtPlan covers the measurement rules.
func TestAccMetric_corruptMeasurementRejectedAtPlan(t *testing.T) {
	srv := newMockServer(t, mockOpts{enveloped: true, resultsMap: true})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		Steps: []resource.TestStep{
			{
				// property-requiring math with property:null (confirmed crash).
				Config: providerConfig(srv.URL, `
resource "mixpanel_metric" "corrupt" {
  name = "tf-corrupt-metric"
  type = "metric"
  definition = jsonencode({
    behavior    = { type = "event", name = "Purchase" }
    measurement = { math = "average", property = null }
  })
}`),
				// Terraform wraps diagnostics at ~78 columns, so match a
				// fragment that fits on one line.
				ExpectError: regexp.MustCompile(`math "average" aggregates a property`),
			},
			{
				// measurement.property with the wrong JSON type (confirmed crash).
				Config: providerConfig(srv.URL, `
resource "mixpanel_metric" "corrupt" {
  name = "tf-corrupt-metric"
  type = "metric"
  definition = jsonencode({
    behavior    = { type = "event", name = "Purchase" }
    measurement = { math = "total", property = "Price" }
  })
}`),
				ExpectError: regexp.MustCompile(`property: must be a JSON object`),
			},
		},
	})
}

// TestAccServiceAccount_expiresValidatedAtPlan proves the exact-format check
// (%Y-%m-%dT%H:%M:%SZ) runs at plan time and that a valid expires is sent on
// the create wire.
func TestAccServiceAccount_expiresValidatedAtPlan(t *testing.T) {
	srv := newMockServer(t, mockOpts{enveloped: true})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL, `
resource "mixpanel_service_account" "bad" {
  username = "tf-acc-expiring"
  expires  = "2030-01-01 00:00:00"
}`),
				// Terraform wraps diagnostics at ~78 columns, so match a
				// fragment that fits on one line.
				ExpectError: regexp.MustCompile(`does not match the required format`),
			},
			{
				Config: providerConfig(srv.URL, `
resource "mixpanel_service_account" "bad" {
  username = "tf-acc-expiring"
  expires  = "2030-01-01T00:00:00.123Z"
}`),
				ExpectError: regexp.MustCompile(`does not match the required format`),
			},
			{
				Config: providerConfig(srv.URL, `
resource "mixpanel_service_account" "ok" {
  username = "tf-acc-expiring"
  expires  = "2030-01-01T00:00:00Z"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("mixpanel_service_account.ok", "expires", "2030-01-01T00:00:00Z"),
					func(s *terraform.State) error {
						for _, cr := range srv.requests("POST") {
							if v, ok := cr.body["expires"]; ok {
								if v != "2030-01-01T00:00:00Z" {
									return fmt.Errorf("expires reached the wire as %v", v)
								}
								return nil
							}
						}
						return fmt.Errorf("create request did not carry expires")
					},
				),
			},
		},
	})
}

// TestAccUserProjectRole_numericIDCoercion proves that string ids in the
// jsonencode payload reach the wire as JSON numbers (the real endpoint 500s
// on string ids).
func TestAccUserProjectRole_numericIDCoercion(t *testing.T) {
	srv := newMockServer(t, mockOpts{enveloped: true})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL, `
resource "mixpanel_user_project_role" "test" {
  key = "user@example.com"
  payload = jsonencode({
    projects = [
      {
        id    = "12345"
        users = [{ email = "user@example.com", role = "analyst" }]
      }
    ]
  })
}`),
				Check: func(s *terraform.State) error {
					for _, cr := range srv.requests("POST") {
						projects, ok := cr.body["projects"].([]any)
						if !ok || len(projects) == 0 {
							continue
						}
						entry, _ := projects[0].(map[string]any)
						if entry == nil {
							continue
						}
						// The mock decodes JSON numbers as float64; a string id
						// would surface here as a string.
						if id, ok := entry["id"].(float64); ok && id == 12345 {
							return nil
						}
						return fmt.Errorf("project id reached the wire as %T (%v), want a JSON number", entry["id"], entry["id"])
					}
					return fmt.Errorf("no add-users-to-projects POST captured")
				},
			},
		},
	})
}
