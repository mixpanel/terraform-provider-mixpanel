// Acceptance tests for mixpanel_metric resource.
// Run with: TF_ACC=1 go test -v -run TestAccMetric

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func TestAccMetric_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-metric")
	nameUpdated := accRandomName("tf-acc-metric-updated")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_metric"),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_metric" "test" {
  name       = %q
  type       = "general"
  project_id = %s
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_metric.test"),
					resource.TestCheckResourceAttr("mixpanel_metric.test", "name", name),
					resource.TestCheckResourceAttr("mixpanel_metric.test", "type", "general"),
					accCheckResourceAttrIsInt("mixpanel_metric.test", "id"),
				),
			},
			{
				// Update name
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_metric" "test" {
  name       = %q
  type       = "general"
  project_id = %s
}
`, nameUpdated, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_metric.test"),
					resource.TestCheckResourceAttr("mixpanel_metric.test", "name", nameUpdated),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("mixpanel_metric.test", plancheck.ResourceActionUpdate),
					},
				},
			},
			{
				// Import
				ResourceName:      "mixpanel_metric.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importIDFunc("mixpanel_metric.test", "id", "project_id"),
				ImportStateVerifyIgnore: []string{
					"allow_staff_override", "can_share", "can_update_basic",
					"can_update_restricted", "can_update_visibility", "can_view",
					"created", "created_by", "definition", "is_locked",
					"is_superadmin", "is_visible", "last_verified",
					"last_verified_by", "modified",
				},
			},
		},
	})
}

func TestAccMetric_withDefinition(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-metric-def")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_metric"),
		Steps: []resource.TestStep{
			{
				// Create with definition
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_metric" "test" {
  name       = %q
  type       = "general"
  project_id = %s
  definition = jsonencode({
    "math" = "total"
    "measurement" = {
      "event" = "pageview"
      "type" = "total"
    }
  })
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_metric.test"),
					resource.TestCheckResourceAttr("mixpanel_metric.test", "name", name),
					accCheckResourceAttrSet("mixpanel_metric.test", "definition"),
				),
			},
			{
				// Update definition
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_metric" "test" {
  name       = %q
  type       = "general"
  project_id = %s
  definition = jsonencode({
    "math" = "total"
    "measurement" = {
      "event" = "click"
      "type" = "total"
    }
  })
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_metric.test"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("mixpanel_metric.test", plancheck.ResourceActionUpdate),
					},
				},
			},
		},
	})
}

func TestAccMetric_corruptDefinition(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-metric-corrupt")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		Steps: []resource.TestStep{
			{
				// Missing required measurement fields - may succeed but break query builder
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_metric" "test" {
  name       = %q
  type       = "general"
  project_id = %s
  definition = jsonencode({
    "math" = "total"
    "measurement" = {
      "property" = null
    }
  })
}
`, name, projectID),
				// Note: This documents the "2xx but corrupt" class from gaps-and-gotchas §3.2
				// When validation is added, update to expect proper error
			},
		},
	})
}
