# Cross-Project Portability Examples

This directory contains practical examples demonstrating how to create portable
Terraform configurations that work across multiple Mixpanel projects.

These examples implement the patterns documented in
[docs/guides/cross-project-portability.md](../../../docs/guides/cross-project-portability.md).

## Examples

### 1. `clone-cohorts.tf`

**Pattern:** `for_each` over environments

Deploy the same cohort definitions to dev, staging, and production projects
with environment-specific thresholds.

**What you'll learn:**
- Using `for_each` to iterate over environments
- Environment-specific configuration via `each.value`
- Using `jsonencode` for cohort groups
- Outputting results for verification

**Usage:**
```bash
cd clone-cohorts
terraform init
terraform plan
terraform apply
```

---

### 2. `clone-dashboards.tf`

**Pattern:** Data source references with `jsonencode`

Create dashboards that reference existing custom properties by name, with
IDs resolved automatically per environment.

**What you'll learn:**
- Looking up existing resources by name using data sources
- Referencing data sources in `jsonencode` blocks
- Cross-referencing cohorts created in the same config
- Building complex dashboard configurations

**Usage:**
```bash
cd clone-dashboards
terraform init
terraform plan
terraform apply
```

**Prerequisites:**
- Custom properties named "User Tier" and "Lifetime Value" must exist in each project
  (create them manually first, or use the module example)

---

### 3. `clone-project-module/`

**Pattern:** Module encapsulation for complete project setup

A reusable module that encapsulates a full Mixpanel project configuration.
Deploy identical setups to multiple environments with environment-specific tuning.

**What you'll learn:**
- Creating reusable Terraform modules
- Managing dependencies between resources
- Using `depends_on` for explicit ordering
- Passing environment-specific variables
- Aggregating outputs across modules

**Usage:**
```bash
cd clone-project-module
terraform init
terraform plan
terraform apply
```

**Files:**
- `main.tf` - The module definition
- `usage.tf` - Example usage deploying to 3 environments

---

## Quick Start

1. **Set up credentials:**
   ```bash
   export MIXPANEL_SERVICE_ACCOUNT="your-service-account"
   export MIXPANEL_SERVICE_ACCOUNT_SECRET="your-secret"
   ```

2. **Update project IDs:**
   Edit the example files to use your actual Mixpanel project IDs:
   ```hcl
   locals {
     environments = {
       dev  = { project_id = 1234567 }  # Your dev project
       prod = { project_id = 3456789 }  # Your prod project
     }
   }
   ```

3. **Run an example:**
   ```bash
   cd clone-cohorts
   terraform init
   terraform plan   # Review changes
   terraform apply  # Deploy
   ```

4. **Verify in Mixpanel:**
   Check the Mixpanel UI to see the created resources in each project.

---

## Production Usage

For production deployments, consider:

1. **Separate state files per environment:**
   ```
   environments/
   ├── dev/
   │   ├── main.tf
   │   └── terraform.tfstate
   ├── staging/
   │   └── main.tf
   └── prod/
       └── main.tf
   ```

2. **Use remote state backends:**
   ```hcl
   terraform {
     backend "s3" {
       bucket = "my-terraform-state"
       key    = "mixpanel/dev/terraform.tfstate"
       region = "us-west-2"
     }
   }
   ```

3. **Progressive rollout:**
   - Apply to dev first
   - Validate in Mixpanel UI
   - Apply to staging
   - Validate again
   - Apply to prod

4. **Version control:**
   - Commit these configs to git
   - Review changes via pull requests
   - Use CI/CD for automated deployments

---

## Common Patterns Reference

| Pattern | Example File | Key Technique |
|---------|--------------|---------------|
| Multi-environment deployment | `clone-cohorts.tf` | `for_each = local.environments` |
| Data source by name | `clone-dashboards.tf` | `data "mixpanel_custom_property" { name = "..." }` |
| Terraform references in JSON | All | `jsonencode({ cohort_id = mixpanel_cohort.x.id })` |
| Dependency ordering | `clone-project-module/main.tf` | `depends_on = [...]` |
| Module reuse | `clone-project-module/usage.tf` | `module "mp_dev" { source = "..." }` |

---

## Troubleshooting

### "Custom property not found"

**Problem:**
```
Error: Custom property "User Tier" not found in project 1234567
```

**Solution:**
Create the custom property in the Mixpanel UI first, or add it to your Terraform config:
```hcl
resource "mixpanel_custom_property" "user_tier" {
  project_id   = var.project_id
  name         = "user_tier"
  display_name = "User Tier"
  description  = "Customer tier level"
}
```

---

### "Inconsistent result after apply"

**Problem:**
```
Error: Provider produced inconsistent result after apply
```

**Solution:**
You likely have a hard-coded ID in a JSON string. Use `jsonencode` with Terraform references:
```hcl
# ❌ Bad
groups = "[{\"event\":{...},\"filters\":[{\"cohort_id\":42}]}]"

# ✅ Good
groups = jsonencode([
  {
    event = {
      resourceType = "cohort"
      value        = "$all_users"
      label        = "All Users"
    }
    filters = [
      {
        cohort_id = mixpanel_cohort.power_users.id
        operator  = "in"
      }
    ]
    filtersOperator           = "and"
    behavioralFilters         = []
    behavioralFiltersOperator = "or"
  }
])
```

---

### "Circular dependency"

**Problem:**
```
Error: Cycle: mixpanel_cohort.a -> mixpanel_cohort.b -> mixpanel_cohort.a
```

**Solution:**
Restructure your cohorts to break the cycle. Cohort A cannot reference cohort B
if B also references A.

---

## Further Reading

- [Cross-Project Portability Guide](../../../docs/guides/cross-project-portability.md)
- [Import Guide](../../../docs/guides/import.md)
- [Provider Documentation](../../../docs/index.md)
