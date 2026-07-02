// Acceptance tests for mixpanel_custom_event resource.
// Run with: TF_ACC=1 go test -v -run TestAccCustomEvent

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCustomEvent_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-custom-event")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_custom_event"),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_custom_event" "test" {
  name       = %q
  project_id = %s
  custom_event = jsonencode({
    "event" = "page_view"
    "filters" = []
  })
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_event.test"),
					resource.TestCheckResourceAttr("mixpanel_custom_event.test", "name", name),
					// customevent_id is the identity field
					accCheckResourceAttrIsInt("mixpanel_custom_event.test", "customevent_id"),
					accCheckResourceAttrSet("mixpanel_custom_event.test", "custom_event"),
				),
			},
			{
				// Import
				ResourceName:                         "mixpanel_custom_event.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "customevent_id",
				ImportStateIdFunc:                    importIDFunc("mixpanel_custom_event.test", "customevent_id", "project_id"),
				ImportStateVerifyIgnore:              []string{"custom_event"},
			},
		},
	})
}

func TestAccCustomEvent_withFilters(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-custom-event-filter")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_custom_event"),
		Steps: []resource.TestStep{
			{
				// Create with filters
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_custom_event" "test" {
  name       = %q
  project_id = %s
  custom_event = jsonencode({
    "event" = "page_view"
    "filters" = [
      {
        "property" = "url"
        "operator" = "contains"
        "value" = "/product"
      }
    ]
  })
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_event.test"),
					resource.TestCheckResourceAttr("mixpanel_custom_event.test", "name", name),
				),
			},
			{
				// Update filters
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_custom_event" "test" {
  name       = %q
  project_id = %s
  custom_event = jsonencode({
    "event" = "page_view"
    "filters" = [
      {
        "property" = "url"
        "operator" = "contains"
        "value" = "/checkout"
      }
    ]
  })
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_event.test"),
				),
			},
		},
	})
}

func TestAccCustomEvent_formEncoding(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-custom-event-form")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_custom_event"),
		Steps: []resource.TestStep{
			{
				// Custom events use form-encoding per gaps-and-gotchas §1
				// This test verifies form-encoded payloads work correctly
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_custom_event" "test" {
  name       = %q
  project_id = %s
  custom_event = jsonencode({
    "event" = "test_event"
    "filters" = []
    "aggregationType" = "total"
  })
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_event.test"),
					accCheckResourceAttrIsInt("mixpanel_custom_event.test", "customevent_id"),
				),
			},
			{
				// Verify idempotency
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_custom_event" "test" {
  name       = %q
  project_id = %s
  custom_event = jsonencode({
    "event" = "test_event"
    "filters" = []
    "aggregationType" = "total"
  })
}
`, name, projectID),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}
