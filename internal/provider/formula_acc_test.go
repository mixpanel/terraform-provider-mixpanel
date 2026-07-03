// Acceptance tests for mixpanel_formula resource.
// Run with: TF_ACC=1 go test -v -run TestAccFormula
//
// Live-verified payload shape (2026-07-03, devbox project 3; matches the
// manifest's minimal accepted FormulaMetricDefinition): definition =
// {formula: {definition: "A", referencedMetrics: [<ReferencedMetricClause>]}}
// where each referenced metric is a full show clause (display/behavior/
// measurement) optionally carrying metric_id for a saved-metric reference.
// A bare {"definition": "A + B", "referencedMetrics": []} without the
// "formula" wrapper is rejected by the MetricsRequest schema.

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// accCheckFormulaDestroy verifies via the live API that formulas are gone
// (GET of a deleted metric/formula returns 404).
func accCheckFormulaDestroy() resource.TestCheckFunc {
	return accCheckEntityGone("mixpanel_formula", "id",
		"/api/app/projects/%s/metrics/%s", 404)
}

// accFormulaConfig renders a live-valid formula whose variable A is an
// inline unique-count of `pageview`.
func accFormulaConfig(projectID, name, expression string) string {
	return accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_formula" "test" {
  name       = %q
  type       = "formula"
  project_id = %s
  definition = jsonencode({
    formula = {
      definition = %q
      referencedMetrics = [{
        display = {}
        behavior = {
          name         = "pageview"
          type         = "event"
          search       = ""
          dataset      = "$mixpanel"
          filters      = []
          resourceType = "events"
        }
        measurement = {
          math       = "unique"
          cumulative = false
        }
      }]
    }
  })
}
`, name, projectID, expression)
}

func TestAccFormula_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-formula")
	nameUpdated := accRandomName("tf-acc-formula-updated")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckFormulaDestroy(),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accFormulaConfig(projectID, name, "A"),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_formula.test"),
					resource.TestCheckResourceAttr("mixpanel_formula.test", "name", name),
					resource.TestCheckResourceAttr("mixpanel_formula.test", "type", "formula"),
					accCheckResourceAttrIsInt("mixpanel_formula.test", "id"),
				),
			},
			{
				// Update name
				Config: accFormulaConfig(projectID, nameUpdated, "A"),
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
		// The destroy step exercises the CRITICAL issue from gaps-and-gotchas
		// §1: single DELETE /metrics/{id} always 501s; the provider deletes
		// through the bulk collection DELETE ({"metrics":[{"id":...}]}), and
		// the API-backed CheckDestroy verifies the formula is really gone.
		CheckDestroy: accCheckFormulaDestroy(),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accFormulaConfig(projectID, name, "A"),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_formula.test"),
				),
			},
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
		CheckDestroy:             accCheckFormulaDestroy(),
		Steps: []resource.TestStep{
			{
				// Create a saved metric and a formula that references it: a
				// ReferencedMetricClause is the metric's show clause plus a
				// metric_id string pointing at the saved metric (live-verified).
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_metric" "ref" {
  name       = %q
  type       = "metric"
  project_id = %s
  definition = jsonencode({
    display = {}
    behavior = {
      name         = "pageview"
      type         = "event"
      search       = ""
      dataset      = "$mixpanel"
      filters      = []
      resourceType = "events"
    }
    measurement = {
      math       = "unique"
      cumulative = false
    }
  })
}

resource "mixpanel_formula" "test" {
  name       = %q
  type       = "formula"
  project_id = %s
  definition = jsonencode({
    formula = {
      definition = "A * 2"
      referencedMetrics = [{
        metric_id = tostring(mixpanel_metric.ref.id)
        display   = {}
        behavior = {
          name         = "pageview"
          type         = "event"
          search       = ""
          dataset      = "$mixpanel"
          filters      = []
          resourceType = "events"
        }
        measurement = {
          math       = "unique"
          cumulative = false
        }
      }]
    }
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
