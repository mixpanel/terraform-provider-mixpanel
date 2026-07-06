// Hand-written mock lifecycle test for the service_account_project rpc_assoc
// resource. crudgen.py does not emit tests for rpc_assoc entities (their create/
// delete verbs are untyped RPC POSTs, not REST CRUD), so this follows the
// generated *_resource_test.go pattern by hand against the shared mock server:
// create POSTs /organizations/{org}/add-to-project/, read confirms the `key`
// is present in the GET /organizations/{org}/service-accounts list, delete
// POSTs /organizations/{org}/remove-from-project/.
package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func TestAccServiceAccountProject_lifecycle(t *testing.T) {
	// The generic mock suffices: the create POST stores the payload body (which
	// carries the key as a scalar value), and the read GET to the non-instance
	// path .../service-accounts falls through to the collection branch and
	// returns every stored object, so rpcAssocKeyPresent finds the key.
	srv := newMockServer(t, mockOpts{enveloped: true})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		Steps: []resource.TestStep{
			{
				// Create; the implicit post-apply refresh+plan asserts idempotency
				// (Read must re-find the key in the service-accounts list).
				Config: providerConfig(srv.URL, `
resource "mixpanel_service_account_project" "test" {
  key = "tf-acc-sa:1"
  payload = jsonencode({
    serviceaccount = "tf-acc-sa:1"
    role           = "admin"
  })
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mixpanel_service_account_project.test", "id", "1:tf-acc-sa:1"),
					resource.TestCheckResourceAttr("mixpanel_service_account_project.test", "organization_id", "1"),
				),
			},
			{
				// key and payload both force replacement (there is no update verb):
				// a change must plan as destroy (remove-from-project) + create.
				Config: providerConfig(srv.URL, `
resource "mixpanel_service_account_project" "test" {
  key = "tf-acc-sa:2"
  payload = jsonencode({
    serviceaccount = "tf-acc-sa:2"
    role           = "analyst"
  })
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("mixpanel_service_account_project.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.TestCheckResourceAttr("mixpanel_service_account_project.test", "id", "1:tf-acc-sa:2"),
			},
			// No import step: like the other rpc_assoc resources (user_project_role,
			// team), import is passthrough-id only — the untyped RPC surface cannot
			// reconstruct the required `payload` (and `key`) from the API, so an
			// ImportStateVerify round-trip is not meaningful here.
		},
	})
}
