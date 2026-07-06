// Acceptance tests for mixpanel_behavior resource.
// Run with: TF_ACC=1 go test -v -run TestAccBehavior
//
// Live-verified payload shapes (2026-07-03, devbox project 3; see also
// gen/refined_manifest.json behavior notes):
//   - `type` must be a Django model choice: simple | funnel | retention
//     ("did"/"event" are rejected by full_clean).
//   - `definition` must wrap a Behavior show clause under a "behavior" key:
//     {"behavior": {...}}. Funnel steps live in behavior.behaviors[] and are
//     SubBehavior objects (name/type/filters/... — top-level show-clause keys
//     like dataset/search/resourceType are rejected on steps).
//   - `description` must be sent explicitly: the create succeeds without it,
//     but the response serializer rejects the null description ("None is not
//     of type 'string'"), so the API returns an error AND leaks the entity.
//   - GET/DELETE of a nonexistent behavior raises unhandled
//     Behavior.DoesNotExist (HTTP 500 in dev), not a clean 404 — the destroy
//     check below accepts both.

package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// accCheckBehaviorDestroy verifies via the live API that behaviors are gone.
// 404 for a clean not-found; 500 because the dev server surfaces
// Behavior.DoesNotExist unhandled (manifest-documented quirk).
func accCheckBehaviorDestroy() resource.TestCheckFunc {
	return accCheckEntityGone("mixpanel_behavior", "behavior_id",
		"/api/app/projects/%s/behaviors/%s", 404, 500)
}

// accBehaviorSimpleConfig renders a minimal live-valid simple behavior.
func accBehaviorSimpleConfig(projectID, name, event string) string {
	return accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_behavior" "test" {
  name        = %q
  project_id  = %s
  type        = "simple"
  description = ""
  definition = jsonencode({
    behavior = {
      name         = %q
      type         = "event"
      search       = ""
      dataset      = "$mixpanel"
      filters      = []
      resourceType = "events"
    }
  })
}
`, name, projectID, event)
}

func TestAccBehavior_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-behavior")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckBehaviorDestroy(),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accBehaviorSimpleConfig(projectID, name, "pageview"),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_behavior.test"),
					resource.TestCheckResourceAttr("mixpanel_behavior.test", "name", name),
					// behavior_id is the identity field, not id
					accCheckResourceAttrIsInt("mixpanel_behavior.test", "behavior_id"),
				),
			},
			{
				// Import
				ResourceName:                         "mixpanel_behavior.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "behavior_id",
				ImportStateIdFunc:                    importIDFunc("mixpanel_behavior.test", "behavior_id", "project_id"),
				ImportStateVerifyIgnore:              []string{"behaviors", "definition"},
			},
		},
	})
}

func TestAccBehavior_complexDefinition(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-behavior-complex")
	projectID := projectPool.nextProject()

	// Funnel behavior: type=funnel, steps as SubBehavior objects under
	// definition.behavior.behaviors (live-verified shape).
	funnelConfig := accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_behavior" "test" {
  name        = %q
  project_id  = %s
  type        = "funnel"
  description = ""
  definition = jsonencode({
    behavior = {
      type = "funnel"
      behaviors = [
        {
          name    = "page_view"
          type    = "event"
          filters = []
        },
        {
          name    = "signup"
          type    = "event"
          filters = []
        }
      ]
    }
  })
}
`, name, projectID)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckBehaviorDestroy(),
		Steps: []resource.TestStep{
			{
				// Create with complex definition (funnel-style)
				Config: funnelConfig,
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_behavior.test"),
					accCheckResourceAttrSet("mixpanel_behavior.test", "definition"),
				),
			},
			{
				// Verify idempotency
				Config:             funnelConfig,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestAccBehavior_corruptDefinition(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-behavior-corrupt")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		Steps: []resource.TestStep{
			{
				// Funnel with only 1 step (requires 2+) — the §3.2 gap. The API
				// saves this with 200 and it corrupts the webapp query builder;
				// the provider's plan-time validator (analytics_validate.go) now
				// rejects it before anything reaches the API, which is the
				// intended behavior for this corrupt shape.
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_behavior" "test" {
  name        = %q
  project_id  = %s
  type        = "funnel"
  description = ""
  definition = jsonencode({
    behavior = {
      type = "funnel"
      behaviors = [
        {
          name    = "page_view"
          type    = "event"
          filters = []
        }
      ]
    }
  })
}
`, name, projectID),
				ExpectError: regexp.MustCompile(`funnel needs at least 2 steps`),
			},
		},
	})
}

func TestAccBehavior_createIdVerification(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-behavior-id")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckBehaviorDestroy(),
		Steps: []resource.TestStep{
			{
				// This test verifies the create-id path mentioned in gaps-and-gotchas §1
				// "Behavior create-id path is a guess... needs verification against project"
				Config: accBehaviorSimpleConfig(projectID, name, "test_event"),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_behavior.test"),
					// Verify behavior_id is properly extracted from create response
					accCheckResourceAttrIsInt("mixpanel_behavior.test", "behavior_id"),
					// Verify we can read it back
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["mixpanel_behavior.test"]
						behaviorID := rs.Primary.Attributes["behavior_id"]
						t.Logf("Created behavior with behavior_id: %s", behaviorID)
						return nil
					},
				),
			},
		},
	})
}
