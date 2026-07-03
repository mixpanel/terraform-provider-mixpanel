// Acceptance tests for mixpanel_dashboard resource.
// Run with: TF_ACC=1 go test -v -run TestAccDashboard

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func TestAccDashboard_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	title := accRandomName("tf-acc-dashboard")
	titleUpdated := accRandomName("tf-acc-dashboard-updated")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDashboardDestroy(),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_dashboard" "test" {
  title       = %q
  project_id  = %s
  description = "Acceptance test dashboard"
}
`, title, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_dashboard.test"),
					resource.TestCheckResourceAttr("mixpanel_dashboard.test", "title", title),
					resource.TestCheckResourceAttr("mixpanel_dashboard.test", "description", "Acceptance test dashboard"),
					accCheckResourceAttrIsInt("mixpanel_dashboard.test", "id"),
				),
			},
			{
				// Update title and description
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_dashboard" "test" {
  title       = %q
  project_id  = %s
  description = "Updated description"
}
`, titleUpdated, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_dashboard.test"),
					resource.TestCheckResourceAttr("mixpanel_dashboard.test", "title", titleUpdated),
					resource.TestCheckResourceAttr("mixpanel_dashboard.test", "description", "Updated description"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("mixpanel_dashboard.test", plancheck.ResourceActionUpdate),
					},
				},
			},
			{
				// Import
				ResourceName:      "mixpanel_dashboard.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importIDFunc("mixpanel_dashboard.test", "id", "project_id"),
				ImportStateVerifyIgnore: []string{
					"created", "creator", "creator_email", "creator_id", "creator_name",
					"is_favorited", "layout_version", "modified", "pinned_date",
					"template_type", "total_view_count", "unique_view_count",
				},
			},
		},
	})
}

func TestAccDashboard_withLayout(t *testing.T) {
	skipIfNotAcceptance(t)

	title := accRandomName("tf-acc-dashboard-layout")
	projectID := projectPool.nextProject()

	// The layout attribute carries the dashboards PATCH WRITE format
	// ({"rows":[...],"rows_order":[...]}); the GET shape (rows dict + order +
	// version) is transformed back into it on refresh. An empty layout is the
	// only shape expressible without real content cell ids (rows with cells
	// must reference board content).
	layoutConfig := accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_dashboard" "test" {
  title      = %q
  project_id = %s
  layout     = jsonencode({
    "rows"       = []
    "rows_order" = []
  })
}
`, title, projectID)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDashboardDestroy(),
		Steps: []resource.TestStep{
			{
				// Create with layout (POST + follow-up layout PATCH).
				Config: layoutConfig,
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_dashboard.test"),
					resource.TestCheckResourceAttr("mixpanel_dashboard.test", "title", title),
					accCheckResourceAttrSet("mixpanel_dashboard.test", "layout"),
				),
			},
			{
				// Verify idempotency (no drift): the refresh reads the GET shape
				// and must transform it back to a semantically equal write format.
				Config:             layoutConfig,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestAccDashboard_privacySettings(t *testing.T) {
	skipIfNotAcceptance(t)

	// Live-verified 2026-07-03 against the dev app API: the server IGNORES
	// is_private on both POST (create) and PATCH (update) for service-account
	// credentials — the echo and a fresh GET both report is_private=false
	// regardless of what was sent. With wire-preferred Read refresh the real
	// server value (false) is refreshed into state, so any config asserting
	// is_private=true fails with genuine (server-side) drift. Until the API
	// persists is_private for service accounts there is nothing to test here.
	t.Skip("live app API ignores is_private for service accounts (POST and PATCH; live-verified 2026-07-03) — is_private=true cannot be provisioned")

	title := accRandomName("tf-acc-dashboard-privacy")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDashboardDestroy(),
		Steps: []resource.TestStep{
			{
				// Create private dashboard
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_dashboard" "test" {
  title       = %q
  project_id  = %s
  is_private  = true
}
`, title, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_dashboard.test"),
					resource.TestCheckResourceAttr("mixpanel_dashboard.test", "is_private", "true"),
				),
			},
			{
				// Update to public
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_dashboard" "test" {
  title       = %q
  project_id  = %s
  is_private  = false
}
`, title, projectID),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("mixpanel_dashboard.test", "is_private", "false"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("mixpanel_dashboard.test", plancheck.ResourceActionUpdate),
					},
				},
			},
		},
	})
}
