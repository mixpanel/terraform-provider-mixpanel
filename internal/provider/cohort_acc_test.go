// Acceptance tests for mixpanel_cohort resource.
// Run with: TF_ACC=1 go test -v -run TestAccCohort
//
// Live-verified groups shape (2026-07-03, devbox project 3): the cohort
// create path resolves referenced cohorts through
// api/version_2_0/cohorts/parser.py _validate_filter_group, which requires
// every filter group to carry event = {resourceType: "cohort", value:
// "$all_users" | <cohort id>} in addition to filters/filtersOperator/
// behavioralFiltersOperator (groupingOperator on every group but the last).
// A group without the event clause is rejected with 400 "Missing or invalid
// 'resourceType' field in filter group 0".

package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// accCheckCohortDestroy verifies via the live API that cohorts are gone
// (GET of a deleted cohort returns 404).
func accCheckCohortDestroy() resource.TestCheckFunc {
	return accCheckEntityGone("mixpanel_cohort", "id",
		"/api/app/projects/%s/cohorts/%s", 404)
}

// accCohortGroupsHCL is the minimal live-valid cohort groups blob: one
// all-users filter group. The server echoes it back verbatim, so plans stay
// idempotent.
const accCohortGroupsHCL = `jsonencode([
    {
      event = {
        resourceType = "cohort"
        value        = "$all_users"
        label        = "All Users"
      }
      filters                   = []
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])`

// accCohortConfig renders a cohort with the live-valid groups fixture.
func accCohortConfig(projectID, name string) string {
	return accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_cohort" "test" {
  name       = %q
  project_id = %s
  groups     = `+accCohortGroupsHCL+`
}
`, name, projectID)
}

func TestAccCohort_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-cohort")
	nameUpdated := accRandomName("tf-acc-cohort-updated")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckCohortDestroy(),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accCohortConfig(projectID, name),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_cohort.test"),
					resource.TestCheckResourceAttr("mixpanel_cohort.test", "name", name),
					accCheckResourceAttrIsInt("mixpanel_cohort.test", "id"),
				),
			},
			{
				// Update name
				Config: accCohortConfig(projectID, nameUpdated),
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
		CheckDestroy:             accCheckCohortDestroy(),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accCohortConfig(projectID, name),
			},
			{
				// Verify no drift after create (idempotency check)
				Config:             accCohortConfig(projectID, name),
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
				// Plan-time validation (analytics_validate.go) now rejects this
				// shape before it reaches the API: a group without
				// filtersOperator/behavioralFiltersOperator saves with 200 but
				// breaks the webapp query builder (gaps-and-gotchas §3.2).
				ExpectError: regexp.MustCompile(`filtersOperator: missing`),
			},
		},
	})
}
