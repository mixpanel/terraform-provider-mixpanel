// Resource-level proof of the writable-field allowlists: a dashboard update
// PATCH body must contain ONLY keys from DashboardAttrSpec().UpdateWritableAttrs
// (dashboards/validate.py validate_dashboard_update_fields is a PREVENT_EXTRA
// voluptuous schema — any other key is a 400 on the real API). The mock server
// captures every wire body, so the assertion is on exactly what the provider
// sent, not on state.
package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDashboardUpdate_patchBodyOnlyWritableKeys(t *testing.T) {
	srv := newMockServer(t, mockOpts{enveloped: true})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		Steps: []resource.TestStep{
			{
				// Create with a create-only field (is_draft) set: it is accepted by
				// the POST schema but NOT by the PATCH allowlist, so it must appear
				// in the create body and be filtered out of the update body.
				Config: providerConfig(srv.URL, `
resource "mixpanel_dashboard" "test" {
  title       = "tf filter test"
  description = "before"
  is_draft    = false
}`),
			},
			{
				// In-place update: only allowlisted keys may reach the PATCH body.
				Config: providerConfig(srv.URL, `
resource "mixpanel_dashboard" "test" {
  title       = "tf filter test"
  description = "after"
  is_draft    = false
}`),
				Check: resource.TestCheckResourceAttr("mixpanel_dashboard.test", "description", "after"),
			},
		},
	})

	patches := []capturedRequest{}
	for _, cr := range srv.requests("PATCH") {
		if strings.Contains(cr.path, "/dashboards/") {
			patches = append(patches, cr)
		}
	}
	if len(patches) == 0 {
		t.Fatal("expected at least one dashboard PATCH request to be captured")
	}
	allowed := DashboardAttrSpec().UpdateWritableAttrs
	if len(allowed) == 0 {
		t.Fatal("DashboardAttrSpec().UpdateWritableAttrs must be populated for this test to be meaningful")
	}
	for _, p := range patches {
		for k := range p.body {
			if !allowed[k] {
				t.Errorf("dashboard PATCH body contains non-allowlisted key %q (body=%v)", k, p.body)
			}
		}
		if _, ok := p.body["is_draft"]; ok {
			t.Errorf("create-only field is_draft leaked into the PATCH body: %v", p.body)
		}
		if p.body["description"] != "after" {
			t.Errorf("PATCH body must carry the updated description, got %v", p.body)
		}
	}
}
