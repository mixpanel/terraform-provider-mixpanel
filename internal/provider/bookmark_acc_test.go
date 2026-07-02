// Acceptance tests for mixpanel_bookmark resource.
// Run with: TF_ACC=1 go test -v -run TestAccBookmark

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBookmark_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-bookmark")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_bookmark"),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_bookmark" "test" {
  name       = %q
  project_id = %s
  type       = "insights"
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_bookmark.test"),
					resource.TestCheckResourceAttr("mixpanel_bookmark.test", "name", name),
					resource.TestCheckResourceAttr("mixpanel_bookmark.test", "type", "insights"),
					accCheckResourceAttrIsInt("mixpanel_bookmark.test", "id"),
				),
			},
			{
				// Import
				ResourceName:      "mixpanel_bookmark.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importIDFunc("mixpanel_bookmark.test", "id", "project_id"),
				ImportStateVerifyIgnore: []string{
					"allow_staff_override", "can_share", "can_update_basic", "can_view",
					"created", "creator", "creator_email", "creator_id", "creator_name",
					"generation_type", "include_in_dashboard", "is_default", "is_superadmin",
					"last_modified_by_email", "last_modified_by_id", "last_modified_by_name",
					"metadata", "modified", "original_type", "params",
					"total_view_count", "unique_view_count", "workspace_id",
				},
			},
		},
	})
}

func TestAccBookmark_withDashboard(t *testing.T) {
	skipIfNotAcceptance(t)

	dashTitle := accRandomName("tf-acc-dash")
	bookmarkName := accRandomName("tf-acc-bookmark")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_bookmark"),
		Steps: []resource.TestStep{
			{
				// Create dashboard and bookmark
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_dashboard" "test" {
  title      = %q
  project_id = %s
}

resource "mixpanel_bookmark" "test" {
  name         = %q
  project_id   = %s
  type         = "insights"
  dashboard_id = mixpanel_dashboard.test.id
}
`, dashTitle, projectID, bookmarkName, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_dashboard.test"),
					accCheckResourceExists("mixpanel_bookmark.test"),
					resource.TestCheckResourceAttr("mixpanel_bookmark.test", "name", bookmarkName),
					resource.TestCheckResourceAttrPair("mixpanel_bookmark.test", "dashboard_id", "mixpanel_dashboard.test", "id"),
				),
			},
		},
	})
}

func TestAccBookmark_withParams(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-bookmark-params")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_bookmark"),
		Steps: []resource.TestStep{
			{
				// Create with params
				// Note: params should be a JSON string per gaps-and-gotchas §2.2
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_bookmark" "test" {
  name       = %q
  project_id = %s
  type       = "insights"
  params     = jsonencode({
    "chartType" = "line"
    "events" = []
  })
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_bookmark.test"),
					accCheckResourceAttrSet("mixpanel_bookmark.test", "params"),
				),
			},
			{
				// Verify idempotency
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_bookmark" "test" {
  name       = %q
  project_id = %s
  type       = "insights"
  params     = jsonencode({
    "chartType" = "line"
    "events" = []
  })
}
`, name, projectID),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}
