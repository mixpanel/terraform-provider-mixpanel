// Acceptance tests for mixpanel_feature_flag resource.
// Run with: TF_ACC=1 go test -v -run TestAccFeatureFlag
//
// Live-verified rules (2026-07-03, devbox project 3; see also
// gen/refined_manifest.json feature_flag notes):
//   - project_id is READ-ONLY on this resource: flags are workspace-scoped
//     and the provider resolves the project from the provider config (the
//     resource injects project_id into state after create/read).
//   - variant `split`, rollout `rollout_percentage`, and `variant_splits`
//     values are 0.0-1.0 floats (the server 400s "Invalid value for field
//     split" for percentages like 100).
//   - flags live only under the workspace mount
//     /projects/{pid}/workspaces/{ws}/feature-flags; the destroy check below
//     therefore probes every workspace of the project.

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// accCheckFeatureFlagDestroy verifies via the live API that flags are gone.
// Flags are only reachable through workspace-mounted paths, so it lists the
// project's workspaces and asserts no workspace still serves the flag id.
func accCheckFeatureFlagDestroy() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		base := os.Getenv("MIXPANEL_BASE_URL")
		if base == "" {
			base = "https://mixpanel.com"
		}
		sa := os.Getenv("MIXPANEL_SERVICE_ACCOUNT")
		secret := os.Getenv("MIXPANEL_SERVICE_ACCOUNT_SECRET")
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "mixpanel_feature_flag" {
				continue
			}
			id := rs.Primary.Attributes["id"]
			if id == "" {
				id = rs.Primary.ID
			}
			projectID := rs.Primary.Attributes["project_id"]
			if projectID == "" {
				projectID = os.Getenv("MIXPANEL_PROJECT_ID")
			}
			// List the project's workspaces.
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/app/projects/%s/workspaces", base, projectID), nil)
			if err != nil {
				return err
			}
			req.SetBasicAuth(sa, secret)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("listing workspaces: %w", err)
			}
			var wsBody struct {
				Results []struct {
					ID int64 `json:"id"`
				} `json:"results"`
			}
			err = json.NewDecoder(resp.Body).Decode(&wsBody)
			resp.Body.Close()
			if err != nil {
				return fmt.Errorf("decoding workspaces: %w", err)
			}
			for _, ws := range wsBody.Results {
				req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/app/projects/%s/workspaces/%d/feature-flags/%s", base, projectID, ws.ID, id), nil)
				if err != nil {
					return err
				}
				req.SetBasicAuth(sa, secret)
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return fmt.Errorf("checking feature flag %s in workspace %d: %w", id, ws.ID, err)
				}
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return fmt.Errorf("feature flag %s still exists after destroy (workspace %d returned 200)", id, ws.ID)
				}
			}
		}
		return nil
	}
}

func TestAccFeatureFlag_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-flag")
	key := accRandomName("tf-acc-flag")
	projectID := projectPool.nextProject()

	// project_id intentionally NOT set: it is read-only on feature flags
	// (workspace-scoped resource; project comes from the provider config).
	// Splits are 0.0-1.0 floats.
	basicConfig := accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_feature_flag" "test" {
  name           = %q
  key            = %q
  context        = "client"
  serving_method = "client"
  ruleset = {
    rollout = [{
      rollout_percentage = 1
      variant_splits     = { on = 1 }
    }]
    variants = [{
      is_control = true
      key        = "on"
      split      = 1
      value      = "true"
    }]
  }
}
`, name, key)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckFeatureFlagDestroy(),
		Steps: []resource.TestStep{
			{
				// Create
				Config: basicConfig,
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_feature_flag.test"),
					resource.TestCheckResourceAttr("mixpanel_feature_flag.test", "name", name),
					resource.TestCheckResourceAttr("mixpanel_feature_flag.test", "key", key),
					resource.TestCheckResourceAttr("mixpanel_feature_flag.test", "context", "client"),
					// ID is string for feature flags
					accCheckResourceAttrSet("mixpanel_feature_flag.test", "id"),
					// project_id is computed from the provider configuration.
					resource.TestCheckResourceAttr("mixpanel_feature_flag.test", "project_id", projectID),
				),
			},
			{
				// Import
				ResourceName:      "mixpanel_feature_flag.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importIDFunc("mixpanel_feature_flag.test", "id", "project_id"),
				ImportStateVerifyIgnore: []string{
					"allow_staff_override", "can_pin", "can_share", "can_update_basic",
					"can_view", "content_environments", "content_type", "created",
					"creator_email", "creator_id", "creator_name", "deleted",
					"enabled_at", "is_favorited", "is_shared_with_project",
					"is_superadmin", "last_modified_by_email", "last_modified_by_id",
					"last_modified_by_name", "modified", "pinned_date", "project_name",
				},
			},
		},
	})
}

func TestAccFeatureFlag_multipleVariants(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-flag-multi")
	key := accRandomName("tf-acc-flag-multi")
	projectID := projectPool.nextProject()

	// Variant splits are 0.0-1.0 floats and must sum to 1. Binary-exact
	// fractions (0.25/0.25/0.5) are used deliberately: a decimal like 0.33
	// round-trips through the API's JSON float64 with different precision
	// than Terraform's decimal number parsing, which surfaces as a spurious
	// refresh-plan diff on the nested ruleset.
	multiConfig := accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_feature_flag" "test" {
  name           = %q
  key            = %q
  context        = "client"
  serving_method = "client"
  ruleset = {
    rollout = [{
      rollout_percentage = 1
      variant_splits = {
        control   = 0.25
        variant_a = 0.25
        variant_b = 0.5
      }
    }]
    variants = [
      {
        is_control = true
        key        = "control"
        split      = 0.25
        value      = "false"
      },
      {
        is_control = false
        key        = "variant_a"
        split      = 0.25
        value      = "a"
      },
      {
        is_control = false
        key        = "variant_b"
        split      = 0.5
        value      = "b"
      }
    ]
  }
}
`, name, key)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckFeatureFlagDestroy(),
		Steps: []resource.TestStep{
			{
				// Create with multiple variants
				Config: multiConfig,
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_feature_flag.test"),
					resource.TestCheckResourceAttr("mixpanel_feature_flag.test", "name", name),
				),
			},
			{
				// Verify idempotency
				Config:             multiConfig,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

// TestAccFeatureFlag_partialRollout covers a rollout below 100%. (The
// original blind-written test tried a `targeting` block, but targeting rules
// are not part of the provider's ruleset schema — rollout percentage is the
// supported targeting knob.)
func TestAccFeatureFlag_partialRollout(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-flag-partial")
	key := accRandomName("tf-acc-flag-partial")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckFeatureFlagDestroy(),
		Steps: []resource.TestStep{
			{
				// Create with a 50% rollout (0.0-1.0 float scale).
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_feature_flag" "test" {
  name           = %q
  key            = %q
  context        = "client"
  serving_method = "client"
  ruleset = {
    rollout = [{
      rollout_percentage = 0.5
      variant_splits     = { on = 1 }
    }]
    variants = [{
      is_control = true
      key        = "on"
      split      = 1
      value      = "true"
    }]
  }
}
`, name, key),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_feature_flag.test"),
					resource.TestCheckResourceAttr("mixpanel_feature_flag.test", "name", name),
					resource.TestCheckResourceAttr("mixpanel_feature_flag.test", "ruleset.rollout.0.rollout_percentage", "0.5"),
				),
			},
		},
	})
}
