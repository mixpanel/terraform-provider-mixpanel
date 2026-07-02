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
		CheckDestroy:             accCheckDestroy("mixpanel_dashboard"),
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

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_dashboard"),
		Steps: []resource.TestStep{
			{
				// Create with layout
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_dashboard" "test" {
  title      = %q
  project_id = %s
  layout     = jsonencode({
    "rows_order" = []
    "rows" = {}
  })
}
`, title, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_dashboard.test"),
					resource.TestCheckResourceAttr("mixpanel_dashboard.test", "title", title),
					accCheckResourceAttrSet("mixpanel_dashboard.test", "layout"),
				),
			},
			{
				// Verify idempotency (no drift)
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_dashboard" "test" {
  title      = %q
  project_id = %s
  layout     = jsonencode({
    "rows_order" = []
    "rows" = {}
  })
}
`, title, projectID),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestAccDashboard_privacySettings(t *testing.T) {
	skipIfNotAcceptance(t)

	title := accRandomName("tf-acc-dashboard-privacy")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_dashboard"),
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
