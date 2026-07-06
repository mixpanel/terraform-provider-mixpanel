// MANUALLY MAINTAINED — DO NOT REGENERATE.
//
// Mock-backed lifecycle test for the property_definition resource. The
// property-definitions API is not plain CRUD (collection-path GET/PATCH/DELETE
// selected by name+resourceType, PATCH-as-upsert, synthetic id==0 body instead
// of 404), so it gets a dedicated in-process mock rather than the generic echo
// server in mock_test.go.

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// propertyDefinitionMock models the verified crud_property behavior:
//   - GET ?name=&resourceType=  -> full definition JSON; a missing row is a
//     synthetic body with id 0 (never a 404).
//   - PATCH {name, resourceType, ...fields} (no `properties` array) -> upsert;
//     echoes the full definition.
//   - DELETE {name, resourceType} -> 204; 400 "Must specify property to
//     DELETE" when the row does not exist.
type propertyDefinitionMock struct {
	mu      sync.Mutex
	store   map[string]map[string]any // "name|resourceType" -> definition
	counter int
}

func newPropertyDefinitionMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	m := &propertyDefinitionMock{store: map[string]map[string]any{}, counter: 5000}
	srv := httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(srv.Close)
	return srv
}

func (m *propertyDefinitionMock) newDefinition(name, resourceType string) map[string]any {
	m.counter++
	return map[string]any{
		"id":           float64(m.counter),
		"name":         name,
		"resourceType": resourceType,
		"displayName":  "",
		"description":  "",
		"exampleValue": "",
		"type":         "unknown",
		"hidden":       false,
		"dropped":      false,
		"sensitive":    false,
		"merged":       false,
		"status":       "live",
	}
}

func (m *propertyDefinitionMock) handle(w http.ResponseWriter, r *http.Request) {
	// Workspace resolution (the resource prefers the workspace mount).
	if r.Method == http.MethodGet && strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/workspaces") {
		writeJSON(w, map[string]any{
			"status": "ok",
			"results": []any{
				map[string]any{"id": 1.0, "is_global": true, "is_default": true, "name": "All Project Data"},
			},
		})
		return
	}
	if !strings.Contains(r.URL.Path, "/data-definitions/properties") {
		http.Error(w, `{"status":"error","error":"unexpected path"}`, http.StatusNotFound)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	switch r.Method {
	case http.MethodGet:
		name := r.URL.Query().Get("name")
		rt := r.URL.Query().Get("resourceType")
		if rt == "" {
			rt = "Event"
		}
		if def, ok := m.store[name+"|"+rt]; ok {
			writeJSON(w, map[string]any{"status": "ok", "results": def})
			return
		}
		// Missing definitions come back synthetic with id 0, never 404.
		synthetic := m.newDefinition(name, rt)
		m.counter-- // synthetic bodies don't consume ids
		synthetic["id"] = float64(0)
		writeJSON(w, map[string]any{"status": "ok", "results": synthetic})
	case http.MethodPatch:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		name, _ := body["name"].(string)
		rt, _ := body["resourceType"].(string)
		if rt == "" {
			rt = "Event"
		}
		key := name + "|" + rt
		def, ok := m.store[key]
		if !ok {
			// PATCH-as-upsert: materialize the row.
			def = m.newDefinition(name, rt)
		}
		for wireKey, target := range map[string]string{
			"description":  "description",
			"displayName":  "displayName",
			"exampleValue": "exampleValue",
			"type":         "type",
		} {
			if v, ok := body[wireKey].(string); ok {
				def[target] = v
			}
		}
		for _, k := range []string{"hidden", "dropped", "sensitive"} {
			if v, ok := body[k].(bool); ok {
				def[k] = v
			}
		}
		m.store[key] = def
		writeJSON(w, map[string]any{"status": "ok", "results": def})
	case http.MethodDelete:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		name, _ := body["name"].(string)
		rt, _ := body["resourceType"].(string)
		if rt == "" {
			rt = "Event"
		}
		key := name + "|" + rt
		if _, ok := m.store[key]; !ok {
			http.Error(w, `{"status":"error","error":"Must specify property to DELETE"}`, http.StatusBadRequest)
			return
		}
		delete(m.store, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func TestAccPropertyDefinition_lifecycle(t *testing.T) {
	srv := newPropertyDefinitionMockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		Steps: []resource.TestStep{
			{
				// Create (a PATCH upsert); the implicit post-apply refresh+plan
				// asserts idempotency, including that unmanaged fields the API
				// materializes as ""/false do not churn against null.
				Config: providerConfig(srv.URL, `
resource "mixpanel_property_definition" "test" {
  project_id    = 1
  name          = "plan_type"
  resource_type = "Event"
  display_name  = "Plan Type"
  description   = "The subscription plan of the account"
  type          = "string"
  hidden        = true
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("mixpanel_property_definition.test", "id"),
					resource.TestCheckResourceAttr("mixpanel_property_definition.test", "name", "plan_type"),
					resource.TestCheckResourceAttr("mixpanel_property_definition.test", "resource_type", "Event"),
					resource.TestCheckResourceAttr("mixpanel_property_definition.test", "description", "The subscription plan of the account"),
				),
			},
			{
				// Metadata change must plan (and apply) as an in-place update.
				Config: providerConfig(srv.URL, `
resource "mixpanel_property_definition" "test" {
  project_id    = 1
  name          = "plan_type"
  resource_type = "Event"
  display_name  = "Plan Type"
  description   = "Updated description"
  type          = "string"
  hidden        = true
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("mixpanel_property_definition.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr("mixpanel_property_definition.test", "description", "Updated description"),
			},
			{
				// Changing the property key must force replacement (the key IS
				// the identity; there is no rename).
				Config: providerConfig(srv.URL, `
resource "mixpanel_property_definition" "test" {
  project_id    = 1
  name          = "plan_tier"
  resource_type = "Event"
  display_name  = "Plan Type"
  description   = "Updated description"
  type          = "string"
  hidden        = true
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("mixpanel_property_definition.test", plancheck.ResourceActionReplace),
					},
				},
			},
			{
				// Import by "project_id:resource_type:name" and assert state
				// round-trips through Read.
				ResourceName:                         "mixpanel_property_definition.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["mixpanel_property_definition.test"]
					if !ok {
						return "", fmt.Errorf("resource not found in state")
					}
					return fmt.Sprintf("%s:%s:%s",
						rs.Primary.Attributes["project_id"],
						rs.Primary.Attributes["resource_type"],
						rs.Primary.Attributes["name"]), nil
				},
			},
		},
	})
}
