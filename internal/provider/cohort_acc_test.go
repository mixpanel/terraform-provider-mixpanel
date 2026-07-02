// Acceptance tests for mixpanel_cohort resource.
// Run with: TF_ACC=1 go test -v -run TestAccCohort

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func TestAccCohort_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-cohort")
	nameUpdated := accRandomName("tf-acc-cohort-updated")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_cohort"),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_cohort" "test" {
  name       = %q
  project_id = %s
  groups     = jsonencode([
    {
      "behavioralFilters" = []
      "behavioralFiltersOperator" = "or"
      "filters" = []
      "filtersOperator" = "and"
      "groupingOperator" = "and"
    }
  ])
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_cohort.test"),
					resource.TestCheckResourceAttr("mixpanel_cohort.test", "name", name),
					accCheckResourceAttrIsInt("mixpanel_cohort.test", "id"),
				),
			},
			{
				// Update name
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_cohort" "test" {
  name       = %q
  project_id = %s
  groups     = jsonencode([
    {
      "behavioralFilters" = []
      "behavioralFiltersOperator" = "or"
      "filters" = []
      "filtersOperator" = "and"
      "groupingOperator" = "and"
    }
  ])
}
`, nameUpdated, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_cohort.test"),
					resource.TestCheckResourceAttr("mixpanel_cohort.test", "name", nameUpdated),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("mixpanel_cohort.test", plancheck.ResourceActionUpdate),
					},
				},
			},
			{
				// Import
				ResourceName:            "mixpanel_cohort.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdFunc:       importIDFunc("mixpanel_cohort.test", "id", "project_id"),
				ImportStateVerifyIgnore: []string{"groups"},
			},
		},
	})
}

func TestAccCohort_driftDetection(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-cohort-drift")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_cohort"),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_cohort" "test" {
  name       = %q
  project_id = %s
  groups     = jsonencode([
    {
      "behavioralFilters" = []
      "behavioralFiltersOperator" = "or"
      "filters" = []
      "filtersOperator" = "and"
      "groupingOperator" = "and"
    }
  ])
}
`, name, projectID),
			},
			{
				// Verify no drift after create (idempotency check)
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_cohort" "test" {
  name       = %q
  project_id = %s
  groups     = jsonencode([
    {
      "behavioralFilters" = []
      "behavioralFiltersOperator" = "or"
      "filters" = []
      "filtersOperator" = "and"
      "groupingOperator" = "and"
    }
  ])
}
`, name, projectID),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestAccCohort_corruptPayload(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-cohort-corrupt")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		Steps: []resource.TestStep{
			{
				// Missing required fields in groups should fail
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_cohort" "test" {
  name       = %q
  project_id = %s
  groups     = jsonencode([
    {
      "filters" = []
    }
  ])
}
`, name, projectID),
				// Note: This may succeed on create but break query builder.
				// This test documents the gap identified in gaps-and-gotchas §3.2.
				// When validation is added, update ExpectError to match the validation message.
			},
		},
	})
}
