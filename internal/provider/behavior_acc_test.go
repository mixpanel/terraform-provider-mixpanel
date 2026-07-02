// Acceptance tests for mixpanel_behavior resource.
// Run with: TF_ACC=1 go test -v -run TestAccBehavior

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBehavior_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-behavior")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_behavior"),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_behavior" "test" {
  name       = %q
  project_id = %s
  definition = jsonencode({
    "type" = "did"
    "event" = "pageview"
  })
}
`, name, projectID),
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

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_behavior"),
		Steps: []resource.TestStep{
			{
				// Create with complex definition (funnel-style)
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_behavior" "test" {
  name       = %q
  project_id = %s
  definition = jsonencode({
    "type" = "funnel"
    "steps" = [
      {
        "event" = "page_view"
      },
      {
        "event" = "signup"
      }
    ]
  })
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_behavior.test"),
					accCheckResourceAttrSet("mixpanel_behavior.test", "definition"),
				),
			},
			{
				// Verify idempotency
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_behavior" "test" {
  name       = %q
  project_id = %s
  definition = jsonencode({
    "type" = "funnel"
    "steps" = [
      {
        "event" = "page_view"
      },
      {
        "event" = "signup"
      }
    ]
  })
}
`, name, projectID),
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
				// Funnel with only 1 step (requires 2+) - documents §3.2 gap
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_behavior" "test" {
  name       = %q
  project_id = %s
  definition = jsonencode({
    "type" = "funnel"
    "steps" = [
      {
        "event" = "page_view"
      }
    ]
  })
}
`, name, projectID),
				// Note: May succeed but break query builder
				// ExpectError can be added here if API returns validation error
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
		CheckDestroy:             accCheckDestroy("mixpanel_behavior"),
		Steps: []resource.TestStep{
			{
				// This test verifies the create-id path mentioned in gaps-and-gotchas §1
				// "Behavior create-id path is a guess... needs verification against project"
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_behavior" "test" {
  name       = %q
  project_id = %s
  definition = jsonencode({
    "type" = "did"
    "event" = "test_event"
  })
}
`, name, projectID),
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
