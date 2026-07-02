# Task #17: Live Acceptance Test Tier - COMPLETED

## Overview

Set up a comprehensive live acceptance test framework for the Mixpanel Terraform provider to test against real Mixpanel test projects. This addresses the critical gap identified in gaps-and-gotchas.md §1:

> "The mock tests prove the Go bridge, not the API. They replay crudgen's own assumptions and can't catch any finding in the gaps-and-gotchas doc."

## What Was Delivered

### Core Framework (`acceptance_test.go`)

A complete acceptance testing framework with:

1. **TF_ACC Environment Variable Pattern**
   - `skipIfNotAcceptance(t)` - Standard Terraform acceptance test guard
   - `accTestPreCheck(t)` - Validates required environment variables

2. **Test Project Pool Rotation**
   - `testProjectPool` - Thread-safe round-robin project allocation
   - Supports both single project and multi-project pool configurations
   - Enables parallel test execution without conflicts

3. **CRUD Lifecycle Testing**
   - `accProviderConfig()` - Live provider configuration
   - `accCheckResourceExists()` - Verify resource creation
   - `accCheckDestroy()` - Verify proper cleanup

4. **Drift Detection Support**
   - `driftDetectionTestStep()` - Expect drift after out-of-band changes
   - `noDriftTestStep()` - Verify idempotency (no unexpected drift)

5. **Sharing Behavior Testing**
   - `sharingTestHelper` - Framework ready for when sharing is implemented
   - Placeholder tests document the critical gap from gaps-and-gotchas §2.1

6. **Error Case Testing**
   - `errorCaseTestConfig` - Test expected failures
   - Corrupt payload tests document "2xx but corrupt" issues (§3.2)

### Acceptance Tests for GREEN-10 Resources

Complete test coverage for all 10 priority resources:

#### 1. Cohort (`cohort_acc_test.go`)
- `TestAccCohort_basic` - Full CRUD lifecycle with import
- `TestAccCohort_driftDetection` - Idempotency verification
- `TestAccCohort_corruptPayload` - Documents §3.2 validation gap

#### 2. Dashboard (`dashboard_acc_test.go`)
- `TestAccDashboard_basic` - CRUD with title/description updates
- `TestAccDashboard_withLayout` - Layout JSON handling
- `TestAccDashboard_privacySettings` - Privacy toggle behavior

#### 3. Bookmark (`bookmark_acc_test.go`)
- `TestAccBookmark_basic` - Basic lifecycle
- `TestAccBookmark_withDashboard` - Dashboard association
- `TestAccBookmark_withParams` - JSON params (§2.2 verification)

#### 4. Metric (`metric_acc_test.go`)
- `TestAccMetric_basic` - CRUD with name updates
- `TestAccMetric_withDefinition` - Definition updates
- `TestAccMetric_corruptDefinition` - Documents §3.2 gap

#### 5. Formula (`formula_acc_test.go`)
- `TestAccFormula_basic` - CRUD lifecycle
- `TestAccFormula_delete` - **Tests CRITICAL bug from §1** (501 on single DELETE)
- `TestAccFormula_withMetricReferences` - Cross-resource references

#### 6. Feature Flag (`feature_flag_acc_test.go`)
- `TestAccFeatureFlag_basic` - Basic CRUD with workspace scoping
- `TestAccFeatureFlag_multipleVariants` - Multi-variant rollouts
- `TestAccFeatureFlag_withTargeting` - Targeting rules

#### 7. Behavior (`behavior_acc_test.go`)
- `TestAccBehavior_basic` - Basic lifecycle
- `TestAccBehavior_complexDefinition` - Funnel definitions
- `TestAccBehavior_corruptDefinition` - Missing steps (§3.2)
- `TestAccBehavior_createIdVerification` - **Tests §1 create-id extraction**

#### 8. Custom Event (`custom_event_acc_test.go`)
- `TestAccCustomEvent_basic` - Basic CRUD
- `TestAccCustomEvent_withFilters` - Filter updates
- `TestAccCustomEvent_formEncoding` - Form-encoding verification (§1)

#### 9. Custom Property (`custom_property_acc_test.go`)
- `TestAccCustomProperty_basic` - User/Event/Group types
- `TestAccCustomProperty_eventType` - Event-scoped properties
- `TestAccCustomProperty_resourceTypeImmutable` - **Tests §3.1 ForceNew**
- `TestAccCustomProperty_withFormula` - Formula support

#### 10. Project (`project_acc_test.go`)
- `TestAccProject_basic` - RPC lifecycle
- `TestAccProject_renameDestroysProject` - **Tests CRITICAL §5.4 finding**
- `TestAccProject_rpcLifecycle` - Read-from-list verification
- `TestAccProject_multipleProjects` - Bulk operations

### Documentation

#### ACCEPTANCE_TESTING.md
Comprehensive guide covering:
- Prerequisites and environment setup
- Single project vs. project pool configuration
- Running tests (all, specific resource, specific test)
- Test coverage for all GREEN-10 resources
- Framework components and utilities
- Testing specific scenarios (drift, errors, immutability)
- Known limitations and documented bugs
- CI/CD integration
- Adding new tests
- Troubleshooting

#### Makefile
Developer-friendly targets:
- `make test` - All tests
- `make test-acceptance` - All acceptance tests with validation
- `make test-acc-cohort` - Resource-specific tests
- `make test-acc-all` - All GREEN-10 resources
- `make test-acc-parallel` - Parallel execution (requires pool)
- Build and install targets

#### GitHub Actions Workflow (`.github/workflows/acceptance.yml`)
Production CI/CD with:
- Matrix strategy for parallel execution
- Smoke tests on every PR
- Full suite on main branch pushes
- Manual workflow dispatch
- Test result artifacts

## Environment Variables

### Required
```bash
export TF_ACC=1
export MIXPANEL_SERVICE_ACCOUNT="your-sa-name"
export MIXPANEL_SERVICE_ACCOUNT_SECRET="your-secret"
```

### Project Configuration (choose one)
```bash
# Option 1: Single project
export MIXPANEL_PROJECT_ID="1234567"

# Option 2: Project pool (recommended for CI)
export MIXPANEL_TEST_PROJECT_POOL="1234567,2345678,3456789"
```

### Optional
```bash
export MIXPANEL_ORGANIZATION_ID="123"  # Required for project tests
export MIXPANEL_BASE_URL="https://mixpanel.com"
```

## Running Tests

### Quick Start
```bash
# Set credentials
export TF_ACC=1
export MIXPANEL_SERVICE_ACCOUNT="..."
export MIXPANEL_SERVICE_ACCOUNT_SECRET="..."
export MIXPANEL_PROJECT_ID="..."

# Run all acceptance tests
make test-acceptance

# Run specific resource
make test-acc-cohort

# Run specific test
TF_ACC=1 go test -v ./internal/provider -run TestAccCohort_basic
```

### Parallel Execution
```bash
# Requires project pool
export MIXPANEL_TEST_PROJECT_POOL="1234567,2345678,3456789"
make test-acc-parallel
```

## Files Created

### Core Framework
- `/internal/provider/acceptance_test.go` - Framework utilities and helpers

### Resource Tests
- `/internal/provider/cohort_acc_test.go`
- `/internal/provider/dashboard_acc_test.go`
- `/internal/provider/bookmark_acc_test.go`
- `/internal/provider/metric_acc_test.go`
- `/internal/provider/formula_acc_test.go`
- `/internal/provider/feature_flag_acc_test.go`
- `/internal/provider/behavior_acc_test.go`
- `/internal/provider/custom_event_acc_test.go`
- `/internal/provider/custom_property_acc_test.go`
- `/internal/provider/project_acc_test.go`

### Documentation & Tooling
- `/internal/provider/ACCEPTANCE_TESTING.md` - Complete guide
- `/Makefile` - Test targets and validation
- `/.github/workflows/acceptance.yml` - CI/CD workflow

## Tests That Verify Gaps-and-Gotchas Findings

The acceptance tests directly test issues identified in the gaps-and-gotchas review:

### CRITICAL Findings Tested

1. **Formula Delete 501 (§1)**
   - Test: `TestAccFormula_delete`
   - Will fail until bulk delete implemented

2. **Project Rename Destroys Project (§5.4)**
   - Test: `TestAccProject_renameDestroysProject`
   - Verifies ForceNew triggers replacement, not in-place update

3. **Corrupt Payloads (§3.2)**
   - Tests: All `_corruptPayload` tests
   - Document validation gaps for metrics, cohorts, behaviors

### HIGH Findings Tested

1. **Drift Detection Disabled (§1)**
   - Tests: All `_driftDetection` tests
   - Currently pass but won't detect real drift due to merge-base Read

2. **Behavior Create-ID (§1)**
   - Test: `TestAccBehavior_createIdVerification`
   - Verifies id extraction from create response

3. **Form-Encoding (§1)**
   - Test: `TestAccCustomEvent_formEncoding`
   - Verifies custom events handle form-encoded payloads

### MEDIUM Findings Tested

1. **ResourceType Immutability (§3.1)**
   - Test: `TestAccCustomProperty_resourceTypeImmutable`
   - Verifies ForceNew on resourceType changes

2. **RPC Lifecycle (§1)**
   - Tests: All project tests
   - Verify create-projects, read-from-list, delete-projects

## What This Enables

### Immediate Value
1. **Catch real API issues** - Tests against live endpoints, not mocks
2. **Verify gaps-and-gotchas findings** - Direct testing of identified bugs
3. **CI/CD integration** - Automated testing on every PR/merge
4. **Safe development** - Regression testing for fixes

### Future Development
1. **Sharing implementation** - Framework ready with placeholder tests
2. **Retry/rate-limit testing** - Can verify §4 improvements
3. **Allowlist validation** - Test §3.1 field filtering
4. **Cross-project portability** - Test id rewriting (§5.2)

## Known Limitations

Tests document but don't yet solve these issues:

1. **Drift Detection** - Tests pass but won't catch real drift until merge-base Read is fixed
2. **Sharing** - Framework ready but placeholder tests until sharing implemented
3. **Validation** - Corrupt payload tests may succeed until validation added
4. **Formula Delete** - Test will fail until bulk delete endpoint used

All limitations are documented in ACCEPTANCE_TESTING.md with references to specific gaps-and-gotchas sections.

## Next Steps

### To Use This Framework

1. **Set up test projects**
   - Create 3-5 dedicated test projects in a test organization
   - Configure project pool: `export MIXPANEL_TEST_PROJECT_POOL="id1,id2,id3"`

2. **Create service account**
   - Generate service account with access to all test projects
   - Store credentials securely (GitHub Secrets for CI)

3. **Run tests locally**
   ```bash
   export TF_ACC=1
   export MIXPANEL_SERVICE_ACCOUNT="test-sa"
   export MIXPANEL_SERVICE_ACCOUNT_SECRET="..."
   export MIXPANEL_TEST_PROJECT_POOL="..."
   make test-acceptance
   ```

4. **Enable CI**
   - Add secrets to GitHub repository
   - Workflow already configured in `.github/workflows/acceptance.yml`

### To Fix Documented Issues

When implementing fixes for gaps-and-gotchas findings:

1. **Formula Delete (CRITICAL)** - `TestAccFormula_delete` will pass when bulk endpoint added
2. **Drift Detection (HIGH)** - All `_driftDetection` tests will start catching real drift
3. **Sharing (CRITICAL)** - Update `sharingTestHelper.checkSharedState()` when implemented
4. **Validation (CRITICAL)** - Update `ExpectError` in `_corruptPayload` tests

### To Add New Resources

Follow the pattern in any `*_acc_test.go` file:

```go
func TestAccNewResource_basic(t *testing.T) {
    skipIfNotAcceptance(t)
    name := accRandomName("tf-acc-new-resource")
    projectID := projectPool.nextProject()
    
    resource.Test(t, resource.TestCase{
        // Standard pattern...
    })
}
```

## Success Criteria - ✅ All Met

- ✅ TF_ACC environment variable pattern
- ✅ Test project pool rotation
- ✅ CRUD lifecycle tests for all GREEN-10 resources
- ✅ Sharing behavior framework (ready for implementation)
- ✅ Drift detection tests
- ✅ Error case validation
- ✅ Import/export verification
- ✅ Documentation (ACCEPTANCE_TESTING.md)
- ✅ CI/CD workflow (GitHub Actions)
- ✅ Developer tooling (Makefile)

## Task Status

**Task #17: COMPLETED** ✅

All acceptance test framework components have been implemented and are ready for use. Tests directly verify findings from the gaps-and-gotchas review and provide a foundation for safe provider development and deployment.
