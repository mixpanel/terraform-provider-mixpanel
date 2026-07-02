// Acceptance tests for mixpanel_formula resource.
// Run with: TF_ACC=1 go test -v -run TestAccFormula

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func TestAccFormula_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-formula")
	nameUpdated := accRandomName("tf-acc-formula-updated")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_formula"),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_formula" "test" {
  name       = %q
  type       = "formula"
  project_id = %s
  definition = jsonencode({
    "definition" = "A + B"
    "referencedMetrics" = []
  })
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_formula.test"),
					resource.TestCheckResourceAttr("mixpanel_formula.test", "name", name),
					resource.TestCheckResourceAttr("mixpanel_formula.test", "type", "formula"),
					accCheckResourceAttrIsInt("mixpanel_formula.test", "id"),
				),
			},
			{
				// Update name
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_formula" "test" {
  name       = %q
  type       = "formula"
  project_id = %s
  definition = jsonencode({
    "definition" = "A + B"
    "referencedMetrics" = []
  })
}
`, nameUpdated, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_formula.test"),
					resource.TestCheckResourceAttr("mixpanel_formula.test", "name", nameUpdated),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("mixpanel_formula.test", plancheck.ResourceActionUpdate),
					},
				},
			},
			{
				// Import
				ResourceName:      "mixpanel_formula.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importIDFunc("mixpanel_formula.test", "id", "project_id"),
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

func TestAccFormula_delete(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-formula-del")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_formula"),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_formula" "test" {
  name       = %q
  type       = "formula"
  project_id = %s
  definition = jsonencode({
    "definition" = "1 + 1"
    "referencedMetrics" = []
  })
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_formula.test"),
				),
			},
			// Note: The destroy step tests the CRITICAL issue from gaps-and-gotchas §1:
			// "Formula delete always 501s - single DELETE unsupported, must use bulk DELETE"
			// This test will fail until bulk delete is implemented.
		},
	})
}

func TestAccFormula_withMetricReferences(t *testing.T) {
	skipIfNotAcceptance(t)

	formulaName := accRandomName("tf-acc-formula-ref")
	metricName := accRandomName("tf-acc-metric-ref")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_formula"),
		Steps: []resource.TestStep{
			{
				// Create metric and formula that references it
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_metric" "ref" {
  name       = %q
  type       = "general"
  project_id = %s
}

resource "mixpanel_formula" "test" {
  name       = %q
  type       = "formula"
  project_id = %s
  definition = jsonencode({
    "definition" = format("A + %%s", mixpanel_metric.ref.id)
    "referencedMetrics" = [mixpanel_metric.ref.id]
  })
}
`, metricName, projectID, formulaName, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_metric.ref"),
					accCheckResourceExists("mixpanel_formula.test"),
					resource.TestCheckResourceAttr("mixpanel_formula.test", "name", formulaName),
				),
			},
		},
	})
}
