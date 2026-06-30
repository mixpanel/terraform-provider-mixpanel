package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccFormulaResource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless env 'TF_ACC' set")
	}

	projectID := os.Getenv("MIXPANEL_PROJECT_ID")
	if projectID == "" {
		t.Fatal("MIXPANEL_PROJECT_ID must be set for acceptance tests")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccFormulaResourceConfig(projectID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mixpanel_formula.test", "name", "tf-test-formula"),
					resource.TestCheckResourceAttr("mixpanel_formula.test", "description", "Test formula for acceptance testing"),
					resource.TestCheckResourceAttrSet("mixpanel_formula.test", "id"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "mixpanel_formula.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update testing (if supported)
			{
				Config: testAccFormulaResourceConfigUpdated(projectID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mixpanel_formula.test", "description", "Updated formula description"),
				),
			},
		},
	})
}

func TestAccLexiconTagResource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless env 'TF_ACC' set")
	}

	projectID := os.Getenv("MIXPANEL_PROJECT_ID")
	if projectID == "" {
		t.Fatal("MIXPANEL_PROJECT_ID must be set for acceptance tests")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccLexiconTagResourceConfig(projectID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mixpanel_lexicon_tag.test", "name", "tf-test-tag"),
					resource.TestCheckResourceAttr("mixpanel_lexicon_tag.test", "description", "Test tag for acceptance testing"),
					resource.TestCheckResourceAttrSet("mixpanel_lexicon_tag.test", "id"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "mixpanel_lexicon_tag.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccEventDefinitionGovernance(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless env 'TF_ACC' set")
	}

	projectID := os.Getenv("MIXPANEL_PROJECT_ID")
	if projectID == "" {
		t.Fatal("MIXPANEL_PROJECT_ID must be set for acceptance tests")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with governance fields
			{
				Config: testAccEventDefinitionGovernanceConfig(projectID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mixpanel_event_definition.test", "event_name", "tf_test_event"),
					resource.TestCheckResourceAttr("mixpanel_event_definition.test", "display_name", "Test Event"),
					resource.TestCheckResourceAttr("mixpanel_event_definition.test", "description", "Test event for governance"),
					resource.TestCheckResourceAttr("mixpanel_event_definition.test", "verified", "true"),
					resource.TestCheckResourceAttr("mixpanel_event_definition.test", "hidden", "false"),
					resource.TestCheckResourceAttr("mixpanel_event_definition.test", "sensitive", "false"),
				),
			},
		},
	})
}

func TestAccSchemaGraphDataSource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless env 'TF_ACC' set")
	}

	projectID := os.Getenv("MIXPANEL_PROJECT_ID")
	if projectID == "" {
		t.Fatal("MIXPANEL_PROJECT_ID must be set for acceptance tests")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSchemaGraphDataSourceConfig(projectID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.mixpanel_schema_graph.test", "events"),
				),
			},
		},
	})
}

// Test configs

func testAccFormulaResourceConfig(projectID string) string {
	return fmt.Sprintf(`
resource "mixpanel_behavior" "test_a" {
  project_id = %[1]s
  name       = "tf-test-behavior-a"
  type       = "simple"
  steps      = jsonencode([{event = "test_event"}])
}

resource "mixpanel_metric" "test_a" {
  project_id = %[1]s
  name       = "tf-test-metric-a"
  math       = "total"
  steps      = jsonencode([mixpanel_behavior.test_a.id])
}

resource "mixpanel_metric" "test_b" {
  project_id = %[1]s
  name       = "tf-test-metric-b"
  math       = "total"
  steps      = jsonencode([mixpanel_behavior.test_a.id])
}

resource "mixpanel_formula" "test" {
  project_id  = %[1]s
  name        = "tf-test-formula"
  description = "Test formula for acceptance testing"
  definition  = jsonencode({
    definition        = "A + B"
    referencedMetrics = [
      mixpanel_metric.test_a.id,
      mixpanel_metric.test_b.id
    ]
  })
}
`, projectID)
}

func testAccFormulaResourceConfigUpdated(projectID string) string {
	return fmt.Sprintf(`
resource "mixpanel_behavior" "test_a" {
  project_id = %[1]s
  name       = "tf-test-behavior-a"
  type       = "simple"
  steps      = jsonencode([{event = "test_event"}])
}

resource "mixpanel_metric" "test_a" {
  project_id = %[1]s
  name       = "tf-test-metric-a"
  math       = "total"
  steps      = jsonencode([mixpanel_behavior.test_a.id])
}

resource "mixpanel_metric" "test_b" {
  project_id = %[1]s
  name       = "tf-test-metric-b"
  math       = "total"
  steps      = jsonencode([mixpanel_behavior.test_a.id])
}

resource "mixpanel_formula" "test" {
  project_id  = %[1]s
  name        = "tf-test-formula"
  description = "Updated formula description"
  definition  = jsonencode({
    definition        = "A + B"
    referencedMetrics = [
      mixpanel_metric.test_a.id,
      mixpanel_metric.test_b.id
    ]
  })
}
`, projectID)
}

func testAccLexiconTagResourceConfig(projectID string) string {
	return fmt.Sprintf(`
resource "mixpanel_lexicon_tag" "test" {
  project_id  = %[1]s
  name        = "tf-test-tag"
  description = "Test tag for acceptance testing"
  color       = "#FF6B6B"
}
`, projectID)
}

func testAccEventDefinitionGovernanceConfig(projectID string) string {
	return fmt.Sprintf(`
resource "mixpanel_event_definition" "test" {
  project_id    = %[1]s
  event_name    = "tf_test_event"
  display_name  = "Test Event"
  description   = "Test event for governance"
  example_value = "2024-01-15T10:30:00Z"
  verified      = true
  hidden        = false
  sensitive     = false
}
`, projectID)
}

func testAccSchemaGraphDataSourceConfig(projectID string) string {
	return fmt.Sprintf(`
data "mixpanel_schema_graph" "test" {
  project_id      = %[1]s
  include_density = true
}
`, projectID)
}
