// Acceptance tests for mixpanel_custom_property resource.
// Run with: TF_ACC=1 go test -v -run TestAccCustomProperty

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCustomProperty_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-custom-prop")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_custom_property"),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_custom_property" "test" {
  name         = %q
  project_id   = %s
  resourceType = "User"
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_property.test"),
					resource.TestCheckResourceAttr("mixpanel_custom_property.test", "name", name),
					resource.TestCheckResourceAttr("mixpanel_custom_property.test", "resourceType", "User"),
					// custom_property_id is the identity field
					accCheckResourceAttrIsInt("mixpanel_custom_property.test", "custom_property_id"),
				),
			},
			{
				// Import
				ResourceName:                         "mixpanel_custom_property.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "custom_property_id",
				ImportStateIdFunc:                    importIDFunc("mixpanel_custom_property.test", "custom_property_id", "project_id"),
				ImportStateVerifyIgnore: []string{
					"behavior", "can_update_basic", "composed_properties", "created",
					"display_options", "is_session_scoped", "join_property_type",
					"mapped_data_group_id", "modified", "project", "property_type",
					"referenced_by", "referenced_directly_by", "referenced_raw_event_properties",
					"references_borrowed_property", "user",
				},
			},
		},
	})
}

func TestAccCustomProperty_eventType(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-custom-prop-event")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_custom_property"),
		Steps: []resource.TestStep{
			{
				// Create Event-scoped custom property
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_custom_property" "test" {
  name         = %q
  project_id   = %s
  resourceType = "Event"
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_property.test"),
					resource.TestCheckResourceAttr("mixpanel_custom_property.test", "resourceType", "Event"),
				),
			},
		},
	})
}

func TestAccCustomProperty_resourceTypeImmutable(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-custom-prop-immut")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_custom_property"),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_custom_property" "test" {
  name         = %q
  project_id   = %s
  resourceType = "User"
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_property.test"),
				),
			},
			{
				// Try to change resourceType - should force replacement or error
				// Per gaps-and-gotchas §3.1: resourceType is create-only, should be ForceNew
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_custom_property" "test" {
  name         = %q
  project_id   = %s
  resourceType = "Event"
}
`, name, projectID),
				// This should either force replacement or fail with 400
				// When ForceNew is properly set, this will trigger replacement
			},
		},
	})
}

func TestAccCustomProperty_withFormula(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-custom-prop-formula")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_custom_property"),
		Steps: []resource.TestStep{
			{
				// Create with formula
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_custom_property" "test" {
  name         = %q
  project_id   = %s
  resourceType = "User"
  formula      = jsonencode({
    "expression" = "1 + 1"
  })
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_property.test"),
					accCheckResourceAttrSet("mixpanel_custom_property.test", "formula"),
				),
			},
		},
	})
}
