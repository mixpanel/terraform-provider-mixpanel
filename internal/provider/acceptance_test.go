// Acceptance test framework for live Mixpanel API testing.
//
// These tests run against real Mixpanel test projects when TF_ACC=1 is set.
// They verify:
//   - Full CRUD lifecycles against the actual API
//   - Sharing behavior
//   - Drift detection
//   - Error cases (allowlist violations, corrupt payloads)
//   - Import/export round-trips
//
// Test project pool rotation is managed by the MIXPANEL_TEST_PROJECT_POOL
// environment variable (comma-separated project IDs).

package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// skipIfNotAcceptance skips the test unless TF_ACC=1 is set.
func skipIfNotAcceptance(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless env TF_ACC is set")
	}
}

// testProjectPool manages a pool of test project IDs for parallel test execution.
type testProjectPool struct {
	mu       sync.Mutex
	projects []string
	index    int
}

var projectPool = &testProjectPool{}

// init loads the project pool from MIXPANEL_TEST_PROJECT_POOL env var.
func init() {
	if pool := os.Getenv("MIXPANEL_TEST_PROJECT_POOL"); pool != "" {
		projectPool.projects = strings.Split(pool, ",")
		for i, p := range projectPool.projects {
			projectPool.projects[i] = strings.TrimSpace(p)
		}
	}
}

// nextProject returns the next project ID from the pool in round-robin fashion.
// Falls back to MIXPANEL_PROJECT_ID if no pool is configured.
func (p *testProjectPool) nextProject() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.projects) == 0 {
		// Fall back to single project from env
		return os.Getenv("MIXPANEL_PROJECT_ID")
	}

	project := p.projects[p.index]
	p.index = (p.index + 1) % len(p.projects)
	return project
}

// accProviderConfig returns a provider config for acceptance tests.
// Uses live credentials from environment variables.
func accProviderConfig(projectID string) string {
	// Allow override for specific test requirements
	if projectID == "" {
		projectID = projectPool.nextProject()
	}

	baseURL := os.Getenv("MIXPANEL_BASE_URL")
	if baseURL == "" {
		baseURL = "https://mixpanel.com"
	}

	orgID := os.Getenv("MIXPANEL_ORGANIZATION_ID")
	if orgID == "" {
		orgID = "1" // Default for most tests
	}

	return fmt.Sprintf(`
provider "mixpanel" {
  service_account        = %q
  service_account_secret = %q
  project_id             = %q
  organization_id        = %q
  base_url               = %q
}
`, os.Getenv("MIXPANEL_SERVICE_ACCOUNT"),
		os.Getenv("MIXPANEL_SERVICE_ACCOUNT_SECRET"),
		projectID,
		orgID,
		baseURL)
}

// accCheckResourceExists is a TestCheckFunc that verifies a resource exists in state.
func accCheckResourceExists(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource %s not found in state", name)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("resource %s has no ID set", name)
		}
		return nil
	}
}

// accCheckResourceAttrSet verifies an attribute is set to a non-empty value.
func accCheckResourceAttrSet(name, attr string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource %s not found in state", name)
		}
		val, ok := rs.Primary.Attributes[attr]
		if !ok || val == "" {
			return fmt.Errorf("resource %s attribute %s not set or empty", name, attr)
		}
		return nil
	}
}

// accCheckResourceAttrIsInt verifies an attribute is a valid integer.
func accCheckResourceAttrIsInt(name, attr string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource %s not found in state", name)
		}
		val, ok := rs.Primary.Attributes[attr]
		if !ok {
			return fmt.Errorf("resource %s attribute %s not set", name, attr)
		}
		if _, err := strconv.Atoi(val); err != nil {
			return fmt.Errorf("resource %s attribute %s is not a valid integer: %v", name, attr, err)
		}
		return nil
	}
}

// accCheckDestroy verifies that resources of a given type are destroyed.
// This is a generic destroy check that can be used for any resource type.
func accCheckDestroy(resourceType string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != resourceType {
				continue
			}
			// Resource still exists in state after destroy
			return fmt.Errorf("resource %s %s still exists after destroy", resourceType, rs.Primary.ID)
		}
		return nil
	}
}

// accTestPreCheck verifies that all required environment variables are set.
func accTestPreCheck(t *testing.T) {
	required := []string{
		"MIXPANEL_SERVICE_ACCOUNT",
		"MIXPANEL_SERVICE_ACCOUNT_SECRET",
	}

	// At least one project source must be configured
	hasProject := os.Getenv("MIXPANEL_PROJECT_ID") != "" || os.Getenv("MIXPANEL_TEST_PROJECT_POOL") != ""

	var missing []string
	for _, env := range required {
		if os.Getenv(env) == "" {
			missing = append(missing, env)
		}
	}

	if !hasProject {
		missing = append(missing, "MIXPANEL_PROJECT_ID or MIXPANEL_TEST_PROJECT_POOL")
	}

	if len(missing) > 0 {
		// Skip rather than fail: TF_ACC=1 alone runs the mock-server tier;
		// the live tier additionally needs credentials. Failing here would
		// make the mock tier unrunnable without live credentials.
		t.Skipf("Skipping live acceptance test; missing environment variables: %s", strings.Join(missing, ", "))
	}
}

// accRandomName generates a unique name for test resources.
func accRandomName(prefix string) string {
	return resource.PrefixedUniqueId(prefix + "-")
}

// driftDetectionTestStep returns a TestStep that triggers a refresh and expects no changes.
// Use this after making out-of-band modifications to verify drift detection works.
func driftDetectionTestStep(config string) resource.TestStep {
	return resource.TestStep{
		Config:             config,
		PlanOnly:           true,
		ExpectNonEmptyPlan: true, // We expect to detect drift
	}
}

// noDriftTestStep returns a TestStep that verifies no drift is detected.
// Use this after apply to ensure idempotency.
func noDriftTestStep(config string) resource.TestStep {
	return resource.TestStep{
		Config:             config,
		PlanOnly:           true,
		ExpectNonEmptyPlan: false, // No changes expected
	}
}

// sharingTestHelper provides utilities for testing sharing behavior.
type sharingTestHelper struct {
	t         *testing.T
	projectID string
}

// newSharingTestHelper creates a new sharing test helper.
func newSharingTestHelper(t *testing.T, projectID string) *sharingTestHelper {
	return &sharingTestHelper{
		t:         t,
		projectID: projectID,
	}
}

// checkSharedState verifies that a resource has the expected sharing state.
// This is a placeholder for when sharing support is added.
func (h *sharingTestHelper) checkSharedState(resourceName string, expectShared bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		// TODO: Implement when sharing support is added (per gaps-and-gotchas §2.1)
		h.t.Logf("Sharing check for %s (expectShared=%v) - not yet implemented", resourceName, expectShared)
		return nil
	}
}

// errorCaseTestConfig represents a test case that should produce an error.
type errorCaseTestConfig struct {
	Config      string
	ExpectError string // Regex pattern to match in error message
}

// runErrorCaseTest runs a test case that should produce an error.
func runErrorCaseTest(t *testing.T, tc errorCaseTestConfig) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		Steps: []resource.TestStep{
			{
				Config:      tc.Config,
				ExpectError: regexp.MustCompile(tc.ExpectError),
			},
		},
	})
}

// lifecycleTestCase represents a full CRUD lifecycle test.
type lifecycleTestCase struct {
	ResourceType string
	CreateConfig string
	UpdateConfig string
	ImportIgnore []string // Attributes to ignore during import verification
	PreCheck     func()
	CheckDestroy resource.TestCheckFunc
}

// runLifecycleTest executes a standard CRUD lifecycle test.
func runLifecycleTest(t *testing.T, tc lifecycleTestCase) {
	if tc.PreCheck == nil {
		tc.PreCheck = func() { accTestPreCheck(t) }
	}
	if tc.CheckDestroy == nil {
		tc.CheckDestroy = accCheckDestroy(tc.ResourceType)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 tc.PreCheck,
		CheckDestroy:             tc.CheckDestroy,
		Steps: []resource.TestStep{
			{
				// Create and verify
				Config: tc.CreateConfig,
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists(tc.ResourceType + ".test"),
				),
			},
			{
				// Update and verify (if update config provided)
				Config:   tc.UpdateConfig,
				SkipFunc: func() (bool, error) { return tc.UpdateConfig == "", nil },
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists(tc.ResourceType + ".test"),
				),
			},
			{
				// Import and verify state round-trips
				ResourceName:            tc.ResourceType + ".test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: tc.ImportIgnore,
			},
		},
	})
}

// testCleanup provides cleanup utilities for acceptance tests.
type testCleanup struct {
	t         *testing.T
	cleanupFn []func(context.Context) error
}

// newTestCleanup creates a new test cleanup helper.
func newTestCleanup(t *testing.T) *testCleanup {
	tc := &testCleanup{t: t}
	t.Cleanup(func() {
		tc.runCleanup()
	})
	return tc
}

// add registers a cleanup function to run at test completion.
func (tc *testCleanup) add(fn func(context.Context) error) {
	tc.cleanupFn = append(tc.cleanupFn, fn)
}

// runCleanup executes all registered cleanup functions.
func (tc *testCleanup) runCleanup() {
	ctx := context.Background()
	for i := len(tc.cleanupFn) - 1; i >= 0; i-- {
		if err := tc.cleanupFn[i](ctx); err != nil {
			tc.t.Logf("Cleanup error: %v", err)
		}
	}
}
