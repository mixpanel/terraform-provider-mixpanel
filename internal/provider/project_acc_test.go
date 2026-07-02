// Acceptance tests for mixpanel_project resource.
// Run with: TF_ACC=1 go test -v -run TestAccProject

package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccProject_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-project")
	orgID := os.Getenv("MIXPANEL_ORGANIZATION_ID")
	if orgID == "" {
		t.Skip("MIXPANEL_ORGANIZATION_ID required for project tests")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_project"),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accProviderConfig("") + fmt.Sprintf(`
resource "mixpanel_project" "test" {
  name            = %q
  organization_id = %s
}
`, name, orgID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_project.test"),
					resource.TestCheckResourceAttr("mixpanel_project.test", "name", name),
					accCheckResourceAttrIsInt("mixpanel_project.test", "id"),
				),
			},
			{
				// Import
				ResourceName:      "mixpanel_project.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importIDFunc("mixpanel_project.test", "id", "organization_id"),
				ImportStateVerifyIgnore: []string{
					"api_key", "api_secret", "timezone_name", "token", "url",
				},
			},
		},
	})
}

func TestAccProject_renameDestroysProject(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-project-rename")
	nameUpdated := accRandomName("tf-acc-project-renamed")
	orgID := os.Getenv("MIXPANEL_ORGANIZATION_ID")
	if orgID == "" {
		t.Skip("MIXPANEL_ORGANIZATION_ID required for project tests")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_project"),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accProviderConfig("") + fmt.Sprintf(`
resource "mixpanel_project" "test" {
  name            = %q
  organization_id = %s
}
`, name, orgID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_project.test"),
				),
			},
			{
				// CRITICAL TEST: Rename should trigger replacement, not update
				// Per gaps-and-gotchas §5.4: "Project rename destroys the project"
				// This is the single highest-stakes line in the provider.
				Config: accProviderConfig("") + fmt.Sprintf(`
resource "mixpanel_project" "test" {
  name            = %q
  organization_id = %s
}
`, nameUpdated, orgID),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						// Name is ForceNew, so this should be a replace action
						plancheck.ExpectResourceAction("mixpanel_project.test", plancheck.ResourceActionReplace),
					},
				},
			},
		},
	})
}

func TestAccProject_rpcLifecycle(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-project-rpc")
	orgID := os.Getenv("MIXPANEL_ORGANIZATION_ID")
	if orgID == "" {
		t.Skip("MIXPANEL_ORGANIZATION_ID required for project tests")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_project"),
		Steps: []resource.TestStep{
			{
				// Create - tests RPC lifecycle (create-projects endpoint)
				Config: accProviderConfig("") + fmt.Sprintf(`
resource "mixpanel_project" "test" {
  name            = %q
  organization_id = %s
}
`, name, orgID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_project.test"),
					resource.TestCheckResourceAttr("mixpanel_project.test", "name", name),
					// Verify ID extraction from RPC response
					accCheckResourceAttrIsInt("mixpanel_project.test", "id"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["mixpanel_project.test"]
						projectID := rs.Primary.Attributes["id"]
						t.Logf("Created project with id: %s", projectID)
						return nil
					},
				),
			},
			{
				// Verify read-from-list works (projects use list endpoint for reads)
				Config: accProviderConfig("") + fmt.Sprintf(`
resource "mixpanel_project" "test" {
  name            = %q
  organization_id = %s
}
`, name, orgID),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestAccProject_multipleProjects(t *testing.T) {
	skipIfNotAcceptance(t)

	name1 := accRandomName("tf-acc-project-1")
	name2 := accRandomName("tf-acc-project-2")
	orgID := os.Getenv("MIXPANEL_ORGANIZATION_ID")
	if orgID == "" {
		t.Skip("MIXPANEL_ORGANIZATION_ID required for project tests")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_project"),
		Steps: []resource.TestStep{
			{
				// Create multiple projects to test bulk create endpoint
				Config: accProviderConfig("") + fmt.Sprintf(`
resource "mixpanel_project" "test1" {
  name            = %q
  organization_id = %s
}

resource "mixpanel_project" "test2" {
  name            = %q
  organization_id = %s
}
`, name1, orgID, name2, orgID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_project.test1"),
					accCheckResourceExists("mixpanel_project.test2"),
					resource.TestCheckResourceAttr("mixpanel_project.test1", "name", name1),
					resource.TestCheckResourceAttr("mixpanel_project.test2", "name", name2),
				),
			},
		},
	})
}
