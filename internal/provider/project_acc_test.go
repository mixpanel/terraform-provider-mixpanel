// Acceptance tests for mixpanel_project resource.
// Run with: TF_ACC=1 go test -v -run TestAccProject

package provider

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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
		CheckDestroy:             accCheckProjectGone(),
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

func TestAccProject_renameInPlace(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-project-rename")
	nameUpdated := accRandomName("tf-acc-project-renamed")
	orgID := os.Getenv("MIXPANEL_ORGANIZATION_ID")
	if orgID == "" {
		t.Skip("MIXPANEL_ORGANIZATION_ID required for project tests")
	}

	var createdID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckProjectGone(),
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
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["mixpanel_project.test"]
						createdID = rs.Primary.Attributes["id"]
						return nil
					},
				),
			},
			{
				// CRITICAL TEST: renaming a project must be an in-place update via
				// POST /projects/update/{id}/, NEVER a replace. A replace would run
				// Delete (RPC delete-projects), destroying the project and ALL of
				// its analytics data. Per gaps-and-gotchas §5.4 this is the single
				// highest-stakes line in the provider.
				Config: accProviderConfig("") + fmt.Sprintf(`
resource "mixpanel_project" "test" {
  name            = %q
  organization_id = %s
}
`, nameUpdated, orgID),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("mixpanel_project.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("mixpanel_project.test", "name", nameUpdated),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["mixpanel_project.test"]
						if got := rs.Primary.Attributes["id"]; got != createdID {
							return fmt.Errorf("project id changed across rename: %s -> %s (rename must not replace the project)", createdID, got)
						}
						return nil
					},
				),
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
		CheckDestroy:             accCheckProjectGone(),
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
		CheckDestroy:             accCheckProjectGone(),
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

// accCheckProjectGone verifies via the live API that every mixpanel_project in
// the (pre-destroy) state is absent from the organization's project list.
// Project delete is a soft delete (delete-projects RPC); a deleted project
// drops out of GET /organizations/{org}/projects/, which is also what the
// resource's Read consults. The state-emptiness accCheckDestroy cannot work
// here (plugin-testing hands CheckDestroy the pre-destroy state).
func accCheckProjectGone() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		orgID := os.Getenv("MIXPANEL_ORGANIZATION_ID")
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "mixpanel_project" {
				continue
			}
			listing, err := accAPIGet(fmt.Sprintf("/organizations/%s/projects/", orgID))
			if err != nil {
				return fmt.Errorf("listing org projects after destroy: %w", err)
			}
			if strings.Contains(listing, fmt.Sprintf(`"id": %s,`, rs.Primary.ID)) ||
				strings.Contains(listing, fmt.Sprintf(`"id":%s,`, rs.Primary.ID)) {
				return fmt.Errorf("project %s still present in org listing after destroy", rs.Primary.ID)
			}
		}
		return nil
	}
}

// accAPIGet performs an authenticated GET against the live test API and
// returns the response body as a string.
func accAPIGet(path string) (string, error) {
	base := os.Getenv("MIXPANEL_BASE_URL")
	if base == "" {
		base = "https://mixpanel.com"
	}
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(os.Getenv("MIXPANEL_SERVICE_ACCOUNT"), os.Getenv("MIXPANEL_SERVICE_ACCOUNT_SECRET"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s returned %d", path, resp.StatusCode)
	}
	return string(body), nil
}
