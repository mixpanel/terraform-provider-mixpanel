# Acceptance Testing Framework

This document describes the live acceptance test tier for the Mixpanel Terraform provider.

## Overview

The acceptance test framework runs tests against **real Mixpanel test projects** to verify:

- Full CRUD lifecycles (Create, Read, Update, Delete)
- Import/export round-trips
- Drift detection
- Sharing behavior (when implemented)
- Error cases (allowlist violations, corrupt payloads)
- API-specific gotchas identified in the gaps-and-gotchas document

Unlike the mock tests in `mock_test.go`, acceptance tests exercise the real Mixpanel API and can catch issues that mock tests cannot detect.

## Prerequisites

### Required Environment Variables

```bash
export TF_ACC=1                                    # Enable acceptance tests
export MIXPANEL_SERVICE_ACCOUNT="your-sa-name"    # Service account username
export MIXPANEL_SERVICE_ACCOUNT_SECRET="secret"   # Service account secret
```

### Project Configuration

You must configure at least one test project using **one** of these methods:

#### Option 1: Single Test Project

```bash
export MIXPANEL_PROJECT_ID="1234567"
```

Best for: Local development, quick test runs

#### Option 2: Project Pool (Recommended for CI)

```bash
export MIXPANEL_TEST_PROJECT_POOL="1234567,2345678,3456789"
```

Best for: Parallel test execution, CI/CD pipelines

The pool rotates projects in round-robin fashion, allowing multiple tests to run concurrently without conflicts.

### Optional Environment Variables

```bash
export MIXPANEL_ORGANIZATION_ID="123"                    # Required for project resource tests
export MIXPANEL_BASE_URL="https://mixpanel.com"         # Default: https://mixpanel.com
```

## Running Tests

### Run All Acceptance Tests

```bash
TF_ACC=1 go test -v ./internal/provider -run TestAcc
```

### Run Tests for Specific Resource

```bash
# Cohort tests
TF_ACC=1 go test -v ./internal/provider -run TestAccCohort

# Dashboard tests
TF_ACC=1 go test -v ./internal/provider -run TestAccDashboard

# Metric tests
TF_ACC=1 go test -v ./internal/provider -run TestAccMetric
```

### Run Specific Test Case

```bash
TF_ACC=1 go test -v ./internal/provider -run TestAccCohort_basic
```

### Run Tests in Parallel

```bash
TF_ACC=1 go test -v -parallel 4 ./internal/provider -run TestAcc
```

**Note:** Configure a project pool when running tests in parallel to avoid conflicts.

## Test Coverage: GREEN-10 Resources

The following resources have comprehensive acceptance tests:

1. **mixpanel_cohort** (`cohort_acc_test.go`)
   - Basic CRUD lifecycle
   - Drift detection
   - Corrupt payload handling

2. **mixpanel_dashboard** (`dashboard_acc_test.go`)
   - Basic CRUD lifecycle
   - Layout handling
   - Privacy settings

3. **mixpanel_bookmark** (`bookmark_acc_test.go`)
   - Basic CRUD lifecycle
   - Dashboard association
   - JSON params handling

4. **mixpanel_metric** (`metric_acc_test.go`)
   - Basic CRUD lifecycle
   - Definition updates
   - Corrupt definition handling

5. **mixpanel_formula** (`formula_acc_test.go`)
   - Basic CRUD lifecycle
   - Delete behavior (tests CRITICAL bug from gaps-and-gotchas)
   - Metric references

6. **mixpanel_feature_flag** (`feature_flag_acc_test.go`)
   - Basic CRUD lifecycle
   - Multiple variants
   - Targeting rules

7. **mixpanel_behavior** (`behavior_acc_test.go`)
   - Basic CRUD lifecycle
   - Complex definitions (funnels)
   - Create-ID verification (tests gaps-and-gotchas §1 finding)

8. **mixpanel_custom_event** (`custom_event_acc_test.go`)
   - Basic CRUD lifecycle
   - Filter handling
   - Form-encoding verification

9. **mixpanel_custom_property** (`custom_property_acc_test.go`)
   - Basic CRUD lifecycle
   - resourceType immutability (tests gaps-and-gotchas §3.1)
   - Formula support

10. **mixpanel_project** (`project_acc_test.go`)
    - RPC lifecycle verification
    - Rename behavior (tests CRITICAL finding from gaps-and-gotchas §5.4)
    - Bulk operations

## Framework Components

### Core Test Utilities (`acceptance_test.go`)

#### Skip Guard

```go
skipIfNotAcceptance(t)  // Skip test unless TF_ACC=1
```

#### Provider Configuration

```go
// Auto-rotates through project pool
config := accProviderConfig("")

// Use specific project
config := accProviderConfig("1234567")
```

#### Existence Checks

```go
accCheckResourceExists("mixpanel_cohort.test")
accCheckResourceAttrSet("mixpanel_cohort.test", "name")
accCheckResourceAttrIsInt("mixpanel_cohort.test", "id")
```

#### Destroy Verification

```go
CheckDestroy: accCheckDestroy("mixpanel_cohort")
```

#### Drift Detection

```go
// Expect drift
driftDetectionTestStep(config)

// Expect no drift (idempotency)
noDriftTestStep(config)
```

#### Test Helpers

```go
// Generate unique names
name := accRandomName("tf-acc-cohort")

// Project pool
projectID := projectPool.nextProject()

// Pre-checks
PreCheck: func() { accTestPreCheck(t) }
```

### Test Structure Pattern

All acceptance tests follow this standard pattern:

```go
func TestAccResource_basic(t *testing.T) {
    skipIfNotAcceptance(t)
    
    name := accRandomName("tf-acc-resource")
    projectID := projectPool.nextProject()
    
    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testProtoV6,
        PreCheck:                 func() { accTestPreCheck(t) },
        CheckDestroy:             accCheckDestroy("mixpanel_resource"),
        Steps: []resource.TestStep{
            {
                // Create
                Config: accProviderConfig(projectID) + `...`,
                Check: resource.ComposeTestCheckFunc(
                    accCheckResourceExists("mixpanel_resource.test"),
                    // Additional checks...
                ),
            },
            {
                // Update (optional)
                Config: accProviderConfig(projectID) + `...`,
                Check: resource.ComposeTestCheckFunc(...),
            },
            {
                // Import
                ResourceName:      "mixpanel_resource.test",
                ImportState:       true,
                ImportStateVerify: true,
                ImportStateIdFunc: importIDFunc(...),
                ImportStateVerifyIgnore: []string{...},
            },
        },
    })
}
```

## Testing Specific Scenarios

### Drift Detection

Tests that verify the provider detects out-of-band changes:

```go
func TestAccCohort_driftDetection(t *testing.T) {
    // ...
    Steps: []resource.TestStep{
        {
            Config: createConfig,
        },
        {
            // Verify no drift after create
            Config:             createConfig,
            PlanOnly:           true,
            ExpectNonEmptyPlan: false,
        },
    }
}
```

**Note:** Currently blocked by merge-base Read issue (gaps-and-gotchas §1, HIGH severity).

### Error Cases

Tests that verify proper error handling:

```go
func TestAccCohort_corruptPayload(t *testing.T) {
    // ...
    Steps: []resource.TestStep{
        {
            Config:      corruptConfig,
            ExpectError: resource.ComposeTestCheckFunc(),
            // Will fail when validation is added
        },
    }
}
```

These tests document the "2xx but corrupt" class from gaps-and-gotchas §3.2.

### Immutability

Tests that verify ForceNew behavior:

```go
ConfigPlanChecks: resource.ConfigPlanChecks{
    PreApply: []plancheck.PlanCheck{
        plancheck.ExpectResourceAction("mixpanel_resource.test", plancheck.ResourceActionReplace),
    },
}
```

## Known Limitations

### Issues Tested But Not Yet Fixed

These tests document known issues from the gaps-and-gotchas review:

1. **Formula Delete (CRITICAL)**
   - Test: `TestAccFormula_delete`
   - Issue: Single DELETE returns 501, needs bulk endpoint
   - Status: Will fail until bulk delete implemented

2. **Project Rename (CRITICAL)**
   - Test: `TestAccProject_renameDestroysProject`
   - Issue: Name is ForceNew, rename triggers full destroy
   - Status: Tests that replacement action is triggered

3. **Drift Detection (HIGH)**
   - Tests: All `_driftDetection` tests
   - Issue: Merge-base Read disables drift detection
   - Status: Tests will pass but won't detect real drift

4. **Corrupt Payloads (CRITICAL)**
   - Tests: All `_corruptPayload` tests
   - Issue: No validation, bad payloads may save successfully
   - Status: Tests document expected failures

5. **Behavior Create-ID (MEDIUM)**
   - Test: `TestAccBehavior_createIdVerification`
   - Issue: Create response id extraction path needs verification
   - Status: Tests against live API

### Sharing Tests

Sharing tests exist but are placeholders:

```go
helper := newSharingTestHelper(t, projectID)
Check: helper.checkSharedState("mixpanel_cohort.test", true)
```

These will be activated when sharing support is implemented (gaps-and-gotchas §2.1).

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Acceptance Tests

on:
  pull_request:
  push:
    branches: [main]

jobs:
  acceptance:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.23'
      
      - name: Run Acceptance Tests
        env:
          TF_ACC: 1
          MIXPANEL_SERVICE_ACCOUNT: ${{ secrets.MIXPANEL_SERVICE_ACCOUNT }}
          MIXPANEL_SERVICE_ACCOUNT_SECRET: ${{ secrets.MIXPANEL_SERVICE_ACCOUNT_SECRET }}
          MIXPANEL_TEST_PROJECT_POOL: ${{ secrets.MIXPANEL_TEST_PROJECT_POOL }}
          MIXPANEL_ORGANIZATION_ID: ${{ secrets.MIXPANEL_ORGANIZATION_ID }}
        run: |
          go test -v -parallel 4 ./internal/provider -run TestAcc
```

### Test Project Pool Setup

For CI, create 3-5 dedicated test projects:

1. Create projects in your test organization
2. Note the project IDs
3. Configure as comma-separated pool:
   ```
   MIXPANEL_TEST_PROJECT_POOL=1234567,2345678,3456789
   ```
4. Grant service account access to all projects

### Cleanup

Test resources are automatically cleaned up via `CheckDestroy` after each test. For orphaned resources:

```bash
# List test resources (they all start with tf-acc-)
# Manually clean up via Mixpanel UI or API
```

## Adding New Tests

### 1. Create Test File

```bash
touch internal/provider/new_resource_acc_test.go
```

### 2. Follow Standard Pattern

```go
package provider

import (
    "fmt"
    "testing"
    "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccNewResource_basic(t *testing.T) {
    skipIfNotAcceptance(t)
    
    name := accRandomName("tf-acc-new-resource")
    projectID := projectPool.nextProject()
    
    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testProtoV6,
        PreCheck:                 func() { accTestPreCheck(t) },
        CheckDestroy:             accCheckDestroy("mixpanel_new_resource"),
        Steps: []resource.TestStep{
            {
                Config: accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_new_resource" "test" {
  name       = %q
  project_id = %s
}
`, name, projectID),
                Check: resource.ComposeTestCheckFunc(
                    accCheckResourceExists("mixpanel_new_resource.test"),
                    resource.TestCheckResourceAttr("mixpanel_new_resource.test", "name", name),
                ),
            },
            {
                ResourceName:      "mixpanel_new_resource.test",
                ImportState:       true,
                ImportStateVerify: true,
                ImportStateIdFunc: importIDFunc("mixpanel_new_resource.test", "id", "project_id"),
            },
        },
    })
}
```

### 3. Add Test Cases

Include tests for:
- Basic CRUD lifecycle
- Updates (if applicable)
- Import/export
- Idempotency (no drift)
- Error cases
- Special behaviors (from gaps-and-gotchas)

## Troubleshooting

### Test Fails with "Missing Environment Variables"

Ensure all required env vars are set:
```bash
export TF_ACC=1
export MIXPANEL_SERVICE_ACCOUNT="..."
export MIXPANEL_SERVICE_ACCOUNT_SECRET="..."
export MIXPANEL_PROJECT_ID="..." # or MIXPANEL_TEST_PROJECT_POOL
```

### Test Fails with 401 Unauthorized

- Verify service account credentials are correct
- Ensure service account has access to test project(s)
- Check base URL matches your region (US/EU/IN)

### Test Fails with 429 Rate Limit

- Reduce parallel execution: `-parallel 1`
- Use project pool to distribute load
- Add delays between test runs
- Consider implementing retry logic (gaps-and-gotchas §4)

### Import Verification Fails

Add computed/read-only fields to `ImportStateVerifyIgnore`:

```go
ImportStateVerifyIgnore: []string{
    "created", "modified", "creator_id",
}
```

### Resource Not Destroyed

- Check `CheckDestroy` implementation
- Verify API actually deletes resource
- Check for soft-delete behavior
- Review gaps-and-gotchas §5.3 for known delete issues

## References

- [Terraform Plugin Testing](https://developer.hashicorp.com/terraform/plugin/testing)
- [gaps-and-gotchas.md](~/gaps-and-gotchas.md) - Known API issues
- [Hashicorp Testing Best Practices](https://developer.hashicorp.com/terraform/plugin/testing/acceptance-tests)

## Contact

For test infrastructure access (project pool, service accounts), contact the Mixpanel infrastructure team.
