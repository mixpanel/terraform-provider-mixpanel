// Acceptance tests for mixpanel_feature_flag resource.
// Run with: TF_ACC=1 go test -v -run TestAccFeatureFlag

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccFeatureFlag_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf_acc_flag")
	key := accRandomName("tf_acc_flag")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_feature_flag"),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_feature_flag" "test" {
  name           = %q
  key            = %q
  context        = "client"
  serving_method = "client"
  project_id     = %s
  ruleset = {
    rollout = [{
      rollout_percentage = 100
      variant_splits     = { on = 100 }
    }]
    variants = [{
      is_control = true
      key        = "on"
      split      = 100
      value      = "true"
    }]
  }
}
`, name, key, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_feature_flag.test"),
					resource.TestCheckResourceAttr("mixpanel_feature_flag.test", "name", name),
					resource.TestCheckResourceAttr("mixpanel_feature_flag.test", "key", key),
					resource.TestCheckResourceAttr("mixpanel_feature_flag.test", "context", "client"),
					// ID is string for feature flags
					accCheckResourceAttrSet("mixpanel_feature_flag.test", "id"),
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

	name := accRandomName("tf_acc_flag_multi")
	key := accRandomName("tf_acc_flag_multi")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_feature_flag"),
		Steps: []resource.TestStep{
			{
				// Create with multiple variants
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_feature_flag" "test" {
  name           = %q
  key            = %q
  context        = "client"
  serving_method = "client"
  project_id     = %s
  ruleset = {
    rollout = [{
      rollout_percentage = 100
      variant_splits     = {
        control = 33
        variant_a = 33
        variant_b = 34
      }
    }]
    variants = [
      {
        is_control = true
        key        = "control"
        split      = 33
        value      = "false"
      },
      {
        is_control = false
        key        = "variant_a"
        split      = 33
        value      = "a"
      },
      {
        is_control = false
        key        = "variant_b"
        split      = 34
        value      = "b"
      }
    ]
  }
}
`, name, key, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_feature_flag.test"),
					resource.TestCheckResourceAttr("mixpanel_feature_flag.test", "name", name),
				),
			},
			{
				// Verify idempotency
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_feature_flag" "test" {
  name           = %q
  key            = %q
  context        = "client"
  serving_method = "client"
  project_id     = %s
  ruleset = {
    rollout = [{
      rollout_percentage = 100
      variant_splits     = {
        control = 33
        variant_a = 33
        variant_b = 34
      }
    }]
    variants = [
      {
        is_control = true
        key        = "control"
        split      = 33
        value      = "false"
      },
      {
        is_control = false
        key        = "variant_a"
        split      = 33
        value      = "a"
      },
      {
        is_control = false
        key        = "variant_b"
        split      = 34
        value      = "b"
      }
    ]
  }
}
`, name, key, projectID),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestAccFeatureFlag_withTargeting(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf_acc_flag_target")
	key := accRandomName("tf_acc_flag_target")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckDestroy("mixpanel_feature_flag"),
		Steps: []resource.TestStep{
			{
				// Create with targeting rules
				Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_feature_flag" "test" {
  name           = %q
  key            = %q
  context        = "client"
  serving_method = "client"
  project_id     = %s
  ruleset = {
    rollout = [{
      rollout_percentage = 50
      variant_splits     = { on = 100 }
      targeting = [{
        property = "distinct_id"
        operator = "equals"
        value    = "test_user"
      }]
    }]
    variants = [{
      is_control = true
      key        = "on"
      split      = 100
      value      = "true"
    }]
  }
}
`, name, key, projectID),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_feature_flag.test"),
					resource.TestCheckResourceAttr("mixpanel_feature_flag.test", "name", name),
				),
			},
		},
	})
}
