// Acceptance tests for mixpanel_custom_property resource.
// Run with: TF_ACC=1 go test -v -run TestAccCustomProperty
//
// Live-verified payload rules (2026-07-03, devbox project 3; webapp
// app_api/projects/custom_properties/views.py):
//   - the attribute is snake_case `resource_type` (wire resourceType) and the
//     accepted values are "events" | "people" — NOT "Event"/"User";
//   - one of displayFormula/behavior is required, and displayFormula requires
//     composedProperties (an object; {} is valid for a formula that
//     references no properties);
//   - description and exampleValue must be sent explicitly: leaving them
//     null makes the dev server's insert fail with an IntegrityError;
//   - resource_type is server-immutable (IMMUTABLE_FIELDS) and marked
//     RequiresReplace in the provider schema, so changing it plans a REPLACE.

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// accCheckCustomPropertyDestroy verifies via the live API that custom
// properties are gone (GET of a deleted custom property returns 404).
func accCheckCustomPropertyDestroy() resource.TestCheckFunc {
	return accCheckEntityGone("mixpanel_custom_property", "custom_property_id",
		"/api/app/projects/%s/custom_properties/%s", 404)
}

// accCustomPropertyConfig renders a minimal live-valid custom property with
// a constant display formula.
func accCustomPropertyConfig(projectID, name, resourceType, formula string) string {
	return accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_custom_property" "test" {
  name                = %q
  project_id          = %s
  resource_type       = %q
  display_formula     = %q
  composed_properties = jsonencode({})
  description         = ""
  example_value       = ""
}
`, name, projectID, resourceType, formula)
}

func TestAccCustomProperty_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-custom-prop")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckCustomPropertyDestroy(),
		Steps: []resource.TestStep{
			{
				// Create a people-scoped ("User" in UI terms) custom property.
				Config: accCustomPropertyConfig(projectID, name, "people", "1 + 1"),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_property.test"),
					resource.TestCheckResourceAttr("mixpanel_custom_property.test", "name", name),
					resource.TestCheckResourceAttr("mixpanel_custom_property.test", "resource_type", "people"),
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
		CheckDestroy:             accCheckCustomPropertyDestroy(),
		Steps: []resource.TestStep{
			{
				// Create an events-scoped custom property.
				Config: accCustomPropertyConfig(projectID, name, "events", "1 + 1"),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_property.test"),
					resource.TestCheckResourceAttr("mixpanel_custom_property.test", "resource_type", "events"),
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
		CheckDestroy:             accCheckCustomPropertyDestroy(),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accCustomPropertyConfig(projectID, name, "people", "1 + 1"),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_property.test"),
				),
			},
			{
				// Change resource_type: per gaps-and-gotchas §3.1 it is
				// create-only on the server (the PUT validator rejects a
				// changed value), and the provider marks it RequiresReplace —
				// the plan must be a REPLACE, never an in-place update.
				Config: accCustomPropertyConfig(projectID, name, "events", "1 + 1"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("mixpanel_custom_property.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_property.test"),
					resource.TestCheckResourceAttr("mixpanel_custom_property.test", "resource_type", "events"),
				),
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
		CheckDestroy:             accCheckCustomPropertyDestroy(),
		Steps: []resource.TestStep{
			{
				// A formula that references a real event property through the
				// composed-properties variable map (variable A -> property);
				// the server validates the formula against composedProperties
				// and infers propertyType from it (live-verified).
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_custom_property" "test" {
  name            = %q
  project_id      = %s
  resource_type   = "events"
  display_formula = "A + 1"
  composed_properties = jsonencode({
    A = {
      resourceType = "event"
      name         = "mp_processing_time_ms"
    }
  })
  description   = ""
  example_value = ""
}
`, name, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_property.test"),
					accCheckResourceAttrSet("mixpanel_custom_property.test", "display_formula"),
					// propertyType is inferred server-side from the formula.
					resource.TestCheckResourceAttr("mixpanel_custom_property.test", "property_type", "number"),
				),
			},
		},
	})
}
