---
page_title: "Cross-project portability"
subcategory: "Guides"
description: |-
  Deploy one analytics configuration across dev, staging, and production projects.
---

# Cross-Project Portability Patterns

This guide demonstrates how to create reusable, portable Terraform configurations
that work across multiple Mixpanel projects and environments (dev/staging/production).

The patterns below are the standard Terraform portability idioms applied to Mixpanel.

---

## Why portability matters

Mixpanel configurations often need to be:
- **Replicated across environments**: dev, staging, and production
- **Promoted through a pipeline**: test in dev, deploy to prod
- **Shared across teams**: common cohorts, dashboards, and metrics
- **Version-controlled**: tracked in git, reviewed in PRs

Hand-copying configurations via the UI is error-prone and doesn't scale. Terraform
makes these configurations **code**, enabling:
- Consistent deployments across projects
- Automated testing and validation
- Audit trails via git history
- Rollback capability

---

## Core patterns

### 1. `for_each` over environments

The most common pattern: define a configuration once and deploy it to multiple
projects using `for_each`.

```hcl
# Define your environments
locals {
  environments = {
    dev = {
      project_id = 1234567
      cohort_size_threshold = 100
    }
    staging = {
      project_id = 2345678
      cohort_size_threshold = 500
    }
    prod = {
      project_id = 3456789
      cohort_size_threshold = 1000
    }
  }
}

# Deploy a cohort to all environments
resource "mixpanel_cohort" "power_users" {
  for_each = local.environments

  project_id  = each.value.project_id
  name        = "Power Users - ${each.key}"
  description = "Users who performed key actions"
  
  # Environment-specific tuning
  groups = jsonencode([
    {
      event = {
        resourceType = "cohort"
        value        = "$all_users"
        label        = "All Users"
      }
      filters = [
        {
          property = "session_count"
          operator = ">"
          value    = each.value.cohort_size_threshold
        }
      ]
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])
}
```

**Why this works:**
- Single source of truth for the cohort logic
- Environment-specific values via `each.value`
- Easy to add new environments
- Clear naming: `mixpanel_cohort.power_users["dev"]`

---

### 2. Using `jsonencode` with Terraform references

For complex nested structures (cohort groups, cohort filters), use `jsonencode`
with Terraform references rather than embedding raw JSON strings.

**❌ Bad: Raw JSON strings**
```hcl
resource "mixpanel_cohort" "engaged_users" {
  project_id = var.project_id
  name       = "Engaged Users"
  
  # Hard-coded IDs make this non-portable
  groups = "[{\"event\":{\"resourceType\":\"cohort\",\"value\":\"$all_users\"},\"filters\":[{\"data_group_id\":\"42\"}]}]"
}
```

**✅ Good: `jsonencode` with Terraform references**
```hcl
# First, create the data group
resource "mixpanel_data_group" "user_profiles" {
  project_id    = var.project_id
  display_name  = "User Profiles"
  property_name = "user_profile_id"
}

# Reference it via Terraform
resource "mixpanel_cohort" "engaged_users" {
  project_id = var.project_id
  name       = "Engaged Users"
  
  groups = jsonencode([
    {
      event = {
        resourceType = "cohort"
        value        = "$all_users"
        label        = "All Users"
      }
      filters = [
        {
          # Terraform resolves this at apply time
          data_group_id = mixpanel_data_group.user_profiles.data_group_id
          operator      = "is_set"
        }
      ]
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])
```

**Why this works:**
- IDs are resolved automatically in each environment
- Type-safe: Terraform validates the structure
- Readable: HCL is clearer than JSON strings
- Refactorable: rename resources without breaking references

---

### 3. Referencing existing Mixpanel objects

When your Terraform configuration needs to reference Mixpanel objects that
already exist, you have two honest patterns:

**Pattern A: Reference resources you manage**

If Terraform creates the object, reference it directly:

```hcl
# Terraform manages this cohort
resource "mixpanel_cohort" "power_users" {
  project_id = var.project_id
  name       = "Power Users"
  # ... configuration
}

# Reference it in a dashboard (reports attach via mixpanel_bookmark)
# Dashboard content is managed via mixpanel_bookmark resources, not inline metadata
resource "mixpanel_dashboard" "customer_health" {
  project_id  = var.project_id
  title       = "Customer Health"
  description = "Managed by Terraform"
}
```

**Pattern B: Adopt objects Terraform doesn't manage**

For manually-created objects or those managed outside Terraform, use the
[import guide](./import.md) to bring them under Terraform management. This
enables cross-references and drift detection.

**By-name lookup data sources** (e.g., fetch a custom property by name instead
of ID) are a provider roadmap item.

---

### 4. Dependency ordering for nested references

Terraform automatically handles dependencies when you reference one resource
from another. For complex configurations, make dependencies explicit to ensure
correct ordering.

```hcl
# 1. Create custom properties first
resource "mixpanel_custom_property" "lifetime_value" {
  project_id   = var.project_id
  name         = "Lifetime Value"
  display_name = "Lifetime Value"
  description  = "Customer lifetime value in USD"
}

# 2. Create cohorts that reference the properties
resource "mixpanel_cohort" "high_value_users" {
  project_id = var.project_id
  name       = "High-Value Users"
  
  groups = jsonencode([
    {
      event = {
        resourceType = "cohort"
        value        = "$all_users"
        label        = "All Users"
      }
      filters = [
        {
          # Terraform ensures the property exists first
          property_id = mixpanel_custom_property.lifetime_value.id
          operator    = ">"
          value       = 1000
        }
      ]
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])
  
  # Explicit dependency (usually not needed, but makes intent clear)
  depends_on = [mixpanel_custom_property.lifetime_value]
}

# 3. Create dashboards (content managed via mixpanel_bookmark, not inline)
resource "mixpanel_dashboard" "value_analysis" {
  project_id  = var.project_id
  title       = "Value Analysis"
  description = "Dashboard for high-value user metrics"
}
```

**Dependency graph:**
```
custom_property -> cohort -> dashboard
```

Terraform applies them in order, and destroys them in reverse order.

---

## Complete examples

### Example 1: Clone cohorts across environments

```hcl
# environments.tf
locals {
  environments = {
    dev     = { project_id = 1111111 }
    staging = { project_id = 2222222 }
    prod    = { project_id = 3333333 }
  }
  
  # Define cohorts once
  cohorts = {
    power_users = {
      name        = "Power Users"
      description = "Users with 10+ sessions"
      threshold   = 10
    }
    
    recent_signups = {
      name        = "Recent Signups"
      description = "Signed up in last 7 days"
      days        = 7
    }
  }
}

# cohorts.tf
resource "mixpanel_cohort" "cohorts" {
  # Cartesian product: every cohort in every environment
  for_each = {
    for pair in setproduct(keys(local.environments), keys(local.cohorts)) :
    "${pair[0]}-${pair[1]}" => {
      env    = pair[0]
      cohort = pair[1]
    }
  }
  
  project_id  = local.environments[each.value.env].project_id
  name        = "${local.cohorts[each.value.cohort].name} [${each.value.env}]"
  description = local.cohorts[each.value.cohort].description
  
  # Use the verified groups structure
  groups = jsonencode([
    {
      event = {
        resourceType = "cohort"
        value        = "$all_users"
        label        = "All Users"
      }
      filters = each.value.cohort == "power_users" ? [
        {
          property = "session_count"
          operator = ">="
          value    = local.cohorts[each.value.cohort].threshold
        }
      ] : [
        {
          property = "signup_date"
          operator = "within"
          value    = "${local.cohorts[each.value.cohort].days}d"
        }
      ]
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])
}

# Access as: mixpanel_cohort.cohorts["dev-power_users"]
#            mixpanel_cohort.cohorts["prod-recent_signups"]
```

---

### Example 2: Clone dashboards across environments

```hcl
# Clone dashboard across environments
# Note: Dashboard content (report cells) is managed via mixpanel_bookmark resources
# rather than inline metadata. See the bookmark documentation for details.
resource "mixpanel_dashboard" "executive_overview" {
  for_each = local.environments
  
  project_id  = each.value.project_id
  title       = "Executive Overview [${upper(each.key)}]"
  description = "Managed by Terraform for ${each.key} environment"
}

# To populate the dashboard with reports, use mixpanel_bookmark resources
# that reference this dashboard's ID
```

---

### Example 3: Clone entire project configuration

For a complete project setup (properties, cohorts, dashboards, alerts), use
modules to encapsulate the configuration.

```hcl
# modules/mixpanel-project/main.tf
variable "project_id" {
  type = number
}

variable "environment" {
  type = string
}

# Custom properties
resource "mixpanel_custom_property" "properties" {
  for_each = {
    user_tier = {
      display_name = "User Tier"
      description  = "Customer tier level"
    }
    lifetime_value = {
      display_name = "Lifetime Value"
      description  = "Customer lifetime value in USD"
    }
    last_seen = {
      display_name = "Last Seen"
      description  = "Last activity timestamp"
    }
  }
  
  project_id   = var.project_id
  name         = each.key
  display_name = each.value.display_name
  description  = each.value.description
}

# Cohorts
resource "mixpanel_cohort" "power_users" {
  project_id  = var.project_id
  name        = "Power Users"
  description = "Managed by Terraform"
  
  groups = jsonencode([
    {
      event = {
        resourceType = "cohort"
        value        = "$all_users"
        label        = "All Users"
      }
      filters = [
        {
          property_id = mixpanel_custom_property.properties["lifetime_value"].id
          operator    = ">"
          value       = 500
        }
      ]
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])
}

# Dashboards
resource "mixpanel_dashboard" "main" {
  project_id  = var.project_id
  title       = "Main Dashboard - ${var.environment}"
  description = "Managed by Terraform"
}

# root main.tf
module "mixpanel_dev" {
  source = "./modules/mixpanel-project"
  
  project_id  = 1111111
  environment = "dev"
}

module "mixpanel_staging" {
  source = "./modules/mixpanel-project"
  
  project_id  = 2222222
  environment = "staging"
}

module "mixpanel_prod" {
  source = "./modules/mixpanel-project"
  
  project_id  = 3333333
  environment = "prod"
}
```

**Structure:**
```
.
├── main.tf
├── modules/
│   └── mixpanel-project/
│       ├── main.tf
│       ├── variables.tf
│       └── outputs.tf
```

---

## Common pitfalls and solutions

### Pitfall: Hard-coded IDs in JSON strings

**Problem:**
```hcl
groups = "{\"cohort_id\":42}"  # Won't work in other projects
```

**Solution:**
```hcl
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

### Pitfall: Circular dependencies

**Problem:**
```hcl
resource "mixpanel_cohort" "a" {
  groups = jsonencode([
    {
      event = {
        resourceType = "cohort"
        value        = mixpanel_cohort.b.id  # A depends on B
        label        = "Cohort B"
      }
      filters                   = []
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])
}

resource "mixpanel_cohort" "b" {
  groups = jsonencode([
    {
      event = {
        resourceType = "cohort"
        value        = mixpanel_cohort.a.id  # B depends on A
        label        = "Cohort A"
      }
      filters                   = []
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])
}
```

**Solution:**  
Restructure to break the cycle or use `terraform_data` with explicit
lifecycle rules.

---

### Pitfall: Inconsistent naming across environments

**Problem:**
```hcl
# Dev uses "power_users", prod uses "Power Users"
# Can cause drift when objects aren't managed consistently
```

**Solution:**  
Establish naming conventions and enforce them via Terraform:
```hcl
# Enforce consistent naming in all projects
locals {
  cohort_names = {
    power_users = "Power Users"  # Single source of truth
  }
}

resource "mixpanel_cohort" "cohorts" {
  for_each = local.environments
  
  project_id = each.value.project_id
  name       = local.cohort_names.power_users
  # ... rest of configuration
}
```

---

### Pitfall: Not using workspaces correctly

**Problem:**  
Deploying all environments from one Terraform state causes blast radius issues.

**Solution:**  
Use [Terraform workspaces](https://developer.hashicorp.com/terraform/language/state/workspaces)
or separate state files per environment:

```bash
# Option 1: Workspaces
terraform workspace new dev
terraform workspace new staging
terraform workspace new prod

# Option 2: Separate directories (recommended for production)
environments/
├── dev/
│   └── main.tf
├── staging/
│   └── main.tf
└── prod/
    └── main.tf
```

---

## Production usage patterns

### Pattern: Progressive rollout

Deploy to dev → validate → promote to staging → validate → promote to prod.

```hcl
# dev.tfvars
project_id = 1111111
enable_experiment = true

# prod.tfvars
project_id = 3333333
enable_experiment = false  # Wait for dev validation
```

```bash
terraform apply -var-file=dev.tfvars
# Test in dev
terraform apply -var-file=prod.tfvars
```

---

### Pattern: Feature flags for risky changes

Use variables to gate risky configurations:

```hcl
variable "enable_new_cohort_logic" {
  type    = bool
  default = false
}

resource "mixpanel_cohort" "users" {
  project_id = var.project_id
  name       = "Users"
  
  groups = var.enable_new_cohort_logic ? jsonencode([
    {
      event = {
        resourceType = "cohort"
        value        = "$all_users"
        label        = "All Users"
      }
      # New logic
      filters                   = [ /* ... */ ]
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ]) : jsonencode([
    {
      event = {
        resourceType = "cohort"
        value        = "$all_users"
        label        = "All Users"
      }
      # Old logic
      filters                   = [ /* ... */ ]
      filtersOperator           = "and"
      behavioralFilters         = []
      behavioralFiltersOperator = "or"
    }
  ])
}
```

---

### Pattern: Import existing configurations

Before creating new resources, import existing ones to avoid duplicates:

```bash
# Discover existing cohorts
terraform plan -generate-config-out=generated.tf

# Review generated.tf, then apply
terraform apply
```

See the [Import Guide](./import.md) for details.

---

## Testing portability

Before deploying to production:

1. **Dry-run in dev:**
   ```bash
   terraform plan -var-file=dev.tfvars
   terraform apply -var-file=dev.tfvars
   ```

2. **Validate outputs:**
   ```hcl
   output "cohort_ids" {
     value = {
       for k, v in mixpanel_cohort.cohorts :
       k => v.id
     }
   }
   ```

3. **Check cross-references:**
   ```bash
   terraform show -json | jq '.values.root_module.resources[] | 
     select(.type == "mixpanel_cohort") | 
     {name: .name, id: .values.id}'
   ```

4. **Promote to staging/prod:**
   ```bash
   terraform plan -var-file=staging.tfvars
   # Review, then apply
   ```

---

## Further reading

- [Import Guide](./import.md) — Bulk import existing Mixpanel objects
- [Terraform Functions](https://developer.hashicorp.com/terraform/language/functions/jsonencode) — `jsonencode`, `setproduct`, `for`
- [Provider Configuration](../index.md) — Multi-project setup

---

## Quick reference

| Pattern | Use case | Example |
|---------|----------|---------|
| `for_each` over environments | Deploy same config to dev/staging/prod | `for_each = local.environments` |
| `jsonencode` with TF refs | Embed IDs that differ per project | `jsonencode([{ event = {...}, filters = [...] }])` |
| Reference managed resources | Use objects Terraform creates | `cohort_id = mixpanel_cohort.power_users.id` |
| Import external objects | Adopt manually-created objects | See [import guide](./import.md) |
| Explicit dependencies | Control apply order | `depends_on = [mixpanel_custom_property.x]` |
| Modules | Encapsulate full project config | `module "mp_dev" { source = "./modules/project" }` |
