// Acceptance tests for mixpanel_metric resource.
// Run with: TF_ACC=1 go test -v -run TestAccMetric
//
// Live-verified payload shape (2026-07-03, devbox project 3): the /metrics
// POST body is the discriminated MetricsRequest — a behavior metric carries
// type="metric" (NOT "general"; the discriminator maps metric ->
// BehaviorMetricRequest) and a REQUIRED definition of the form
// {behavior: <Behavior show clause>, measurement: <BehaviorMeasurement>}
// (webapp app_api/projects/metrics/__types__/models.py).

package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// accCheckMetricDestroy verifies via the live API that metrics are gone
// (GET of a deleted metric returns 404).
func accCheckMetricDestroy() resource.TestCheckFunc {
	return accCheckEntityGone("mixpanel_metric", "id",
		"/api/app/projects/%s/metrics/%s", 404)
}

// accMetricDefinitionHCL renders the minimal live-valid behavior-metric
// definition counting uniques of the given event.
func accMetricDefinitionHCL(event string) string {
	return fmt.Sprintf(`jsonencode({
    display = {}
    behavior = {
      name         = %q
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
  })`, event)
}

// accMetricConfig renders a live-valid behavior metric.
func accMetricConfig(projectID, name, event string) string {
	return accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_metric" "test" {
  name       = %q
  type       = "metric"
  project_id = %s
  definition = `+accMetricDefinitionHCL(event)+`
}
`, name, projectID)
}

func TestAccMetric_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-metric")
	nameUpdated := accRandomName("tf-acc-metric-updated")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckMetricDestroy(),
		Steps: []resource.TestStep{
			{
				// Create. definition is required by the live schema, so even the
				// basic lifecycle carries the minimal valid one.
				Config: accMetricConfig(projectID, name, "pageview"),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_metric.test"),
					resource.TestCheckResourceAttr("mixpanel_metric.test", "name", name),
					resource.TestCheckResourceAttr("mixpanel_metric.test", "type", "metric"),
					accCheckResourceAttrIsInt("mixpanel_metric.test", "id"),
				),
			},
			{
				// Update name
				Config: accMetricConfig(projectID, nameUpdated, "pageview"),
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
		CheckDestroy:             accCheckMetricDestroy(),
		Steps: []resource.TestStep{
			{
				// Create with definition
				Config: accMetricConfig(projectID, name, "pageview"),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_metric.test"),
					resource.TestCheckResourceAttr("mixpanel_metric.test", "name", name),
					accCheckResourceAttrSet("mixpanel_metric.test", "definition"),
				),
			},
			{
				// Update definition (change the measured event)
				Config: accMetricConfig(projectID, name, "click"),
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
				// Property-aggregating math with a null property: the "2xx but
				// corrupt" class from gaps-and-gotchas §3.2 (Mixpanel saves it,
				// the webapp query builder then crashes). The provider's
				// plan-time validator (analytics_validate.go) now rejects it
				// before anything reaches the API.
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_metric" "test" {
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
      math       = "average"
      cumulative = false
      property   = null
    }
  })
}
`, name, projectID),
				// Keep the pattern short: the CLI line-wraps the full message.
				ExpectError: regexp.MustCompile(`aggregates a property`),
			},
		},
	})
}
