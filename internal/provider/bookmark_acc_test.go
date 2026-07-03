// Acceptance tests for mixpanel_bookmark resource.
// Run with: TF_ACC=1 go test -v -run TestAccBookmark
//
// Board-type bookmarks (insights/retention/funnels/flows) are BOARD REPORTS:
// they require a dashboard_id and are created/deleted through the dashboards
// content API (the standalone POST silently drops dashboard_id and the
// standalone DELETE 500s for them — live-verified 2026-07-03). Every test
// here therefore pairs the bookmark with a dashboard and uses a VALID
// insights params blob (the server requires sections.show with at least one
// clause plus displayOptions.chartType).

package provider

import (
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// accCheckGoneViaAPI verifies via the live API that every resource of the
// given type recorded in the state is gone (GET returns 404).
// terraform-plugin-testing passes CheckDestroy the final state from BEFORE
// the destroy specifically so tests can query the backend per resource —
// checking that the state map is empty (accCheckDestroy) always fails.
// pathFmt receives (projectID, id).
func accCheckGoneViaAPI(resourceType, pathFmt string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		base := os.Getenv("MIXPANEL_BASE_URL")
		if base == "" {
			base = "https://mixpanel.com"
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != resourceType {
				continue
			}
			id := rs.Primary.Attributes["id"]
			if id == "" {
				id = rs.Primary.ID
			}
			projectID := rs.Primary.Attributes["project_id"]
			if projectID == "" {
				projectID = os.Getenv("MIXPANEL_PROJECT_ID")
			}
			req, err := http.NewRequest(http.MethodGet, base+fmt.Sprintf(pathFmt, projectID, id), nil)
			if err != nil {
				return err
			}
			req.SetBasicAuth(os.Getenv("MIXPANEL_SERVICE_ACCOUNT"), os.Getenv("MIXPANEL_SERVICE_ACCOUNT_SECRET"))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("checking %s %s after destroy: %w", resourceType, id, err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				return fmt.Errorf("%s %s still exists after destroy (GET returned %d, want 404)", resourceType, id, resp.StatusCode)
			}
		}
		return nil
	}
}

// accCheckBookmarkDestroy / accCheckDashboardDestroy are the API-backed
// destroy checks for the two board entities.
func accCheckBookmarkDestroy() resource.TestCheckFunc {
	return accCheckGoneViaAPI("mixpanel_bookmark", "/api/app/projects/%s/bookmarks/%s")
}

func accCheckDashboardDestroy() resource.TestCheckFunc {
	return accCheckGoneViaAPI("mixpanel_dashboard", "/api/app/projects/%s/dashboards/%s")
}

// accInsightsParamsHCL is the minimal VALID insights params blob accepted by
// the live server (live-verified 2026-07-03: sections.show needs >= 1 clause
// and displayOptions needs a chartType; an empty show is rejected with
// 400 InvalidParams).
const accInsightsParamsHCL = `jsonencode({
    sections = {
      show = [{
        dataset      = "$mixpanel"
        value        = { name = "$all_events", resourceType = "events" }
        resourceType = "events"
        profileType  = null
        search       = ""
        math         = "total"
        property     = null
      }]
    }
    displayOptions = {
      chartType = "line"
      plotStyle = "standard"
      analysis  = "linear"
      value     = "absolute"
    }
  })`

// accBookmarkConfig renders a dashboard plus an insights board report
// attached to it.
func accBookmarkConfig(projectID, dashTitle, bookmarkName string) string {
	return accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_dashboard" "test" {
  title      = %q
  project_id = %s
}

resource "mixpanel_bookmark" "test" {
  # project_id is read-only on bookmarks (frozen-spec schema); the provider
  # default project applies.
  name         = %q
  type         = "insights"
  dashboard_id = mixpanel_dashboard.test.id
  params       = `+accInsightsParamsHCL+`
}
`, dashTitle, projectID, bookmarkName)
}

func TestAccBookmark_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	dashTitle := accRandomName("tf-acc-bookmark-dash")
	name := accRandomName("tf-acc-bookmark")
	nameUpdated := accRandomName("tf-acc-bookmark-upd")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckBookmarkDestroy(),
		Steps: []resource.TestStep{
			{
				// Create: goes through the dashboards content PATCH, which sets
				// dashboard_id AND creates the board layout cell.
				Config: accBookmarkConfig(projectID, dashTitle, name),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_dashboard.test"),
					accCheckResourceExists("mixpanel_bookmark.test"),
					resource.TestCheckResourceAttr("mixpanel_bookmark.test", "name", name),
					resource.TestCheckResourceAttr("mixpanel_bookmark.test", "type", "insights"),
					resource.TestCheckResourceAttrPair("mixpanel_bookmark.test", "dashboard_id", "mixpanel_dashboard.test", "id"),
					accCheckResourceAttrIsInt("mixpanel_bookmark.test", "id"),
				),
			},
			{
				// Rename in place (standalone bookmark PATCH still works for
				// name/params).
				Config: accBookmarkConfig(projectID, dashTitle, nameUpdated),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("mixpanel_bookmark.test", "name", nameUpdated),
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

func TestAccBookmark_paramsIdempotency(t *testing.T) {
	skipIfNotAcceptance(t)

	dashTitle := accRandomName("tf-acc-bookmark-dash-p")
	name := accRandomName("tf-acc-bookmark-params")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckBookmarkDestroy(),
		Steps: []resource.TestStep{
			{
				// Create with params: the content-create echo rewrites params
				// server-side, but the GET round-trips the user's original string,
				// so the applied state must equal the plan.
				Config: accBookmarkConfig(projectID, dashTitle, name),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_bookmark.test"),
					accCheckResourceAttrSet("mixpanel_bookmark.test", "params"),
				),
			},
			{
				// Verify idempotency: refresh + plan against the same config must
				// be empty (no params drift from the server echo).
				Config:             accBookmarkConfig(projectID, dashTitle, name),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}
