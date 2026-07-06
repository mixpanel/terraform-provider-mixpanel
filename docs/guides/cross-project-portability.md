# Cross-Project Portability Patterns

This guide demonstrates how to create reusable, portable Terraform configurations
that work across multiple Mixpanel projects and environments (dev/staging/production).

The patterns shown here are production-tested and address the most common
cross-project use cases: cloning cohorts, boards (dashboards), and entire
project configurations across environments.

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
  selector = jsonencode({
    filter = {
      property = "session_count"
      operator = ">"
      value    = each.value.cohort_size_threshold
    }
  })
}
```

**Why this works:**
- Single source of truth for the cohort logic
- Environment-specific values via `each.value`
- Easy to add new environments
- Clear naming: `mixpanel_cohort.power_users["dev"]`

---

### 2. Using `jsonencode` with Terraform references

For complex nested structures (cohort selectors, dashboard queries, alert
conditions), use `jsonencode` with Terraform references rather than embedding
raw JSON strings.

**❌ Bad: Raw JSON strings**
```hcl
resource "mixpanel_cohort" "engaged_users" {
  project_id = var.project_id
  name       = "Engaged Users"
  
  # Hard-coded IDs make this non-portable
  selector = "{\"and\":[{\"data_group_id\":\"42\"}]}"
}
```

**✅ Good: `jsonencode` with Terraform references**
```hcl
# First, create the data group
resource "mixpanel_data_group" "user_profiles" {
  project_id = var.project_id
  name       = "User Profiles"
}

# Reference it via Terraform
resource "mixpanel_cohort" "engaged_users" {
  project_id = var.project_id
  name       = "Engaged Users"
  
  selector = jsonencode({
    and = [
      {
        # Terraform resolves this at apply time
        data_group_id = mixpanel_data_group.user_profiles.data_group_id
      }
    ]
  })
}
```

**Why this works:**
- IDs are resolved automatically in each environment
- Type-safe: Terraform validates the structure
- Readable: HCL is clearer than JSON strings
- Refactorable: rename resources without breaking references

---

### 3. By-name data sources for ID resolution

When referencing existing Mixpanel objects that weren't created by Terraform
(e.g., manually-created custom properties), use data sources to look them up
by name rather than hard-coding IDs.

```hcl
# Look up an existing custom property by name
data "mixpanel_custom_property" "user_tier" {
  project_id = var.project_id
  name       = "User Tier"
}

# Use it in a dashboard query
resource "mixpanel_dashboard" "customer_health" {
  project_id = var.project_id
  name       = "Customer Health"
  
  # The custom property ID differs per project
  # but the NAME is consistent
  metadata = jsonencode({
    charts = [
      {
        type = "segmentation"
        query = {
          breakdown = {
            # Resolved via data source
            property_id = data.mixpanel_custom_property.user_tier.id
          }
        }
      }
    ]
  })
}
```

**Why this works:**
- Works even when IDs differ across projects
- Self-documenting: the name is the contract
- Fails fast: Terraform errors if the property doesn't exist
- No manual ID lookups

---

### 4. Dependency ordering for nested references

Terraform automatically handles dependencies when you reference one resource
from another. For complex configurations, make dependencies explicit to ensure
correct ordering.

```hcl
# 1. Create custom properties first
resource "mixpanel_custom_property" "lifetime_value" {
  project_id = var.project_id
  name       = "Lifetime Value"
  data_type  = "number"
}

# 2. Create cohorts that reference the properties
resource "mixpanel_cohort" "high_value_users" {
  project_id = var.project_id
  name       = "High-Value Users"
  
  selector = jsonencode({
    filter = {
      # Terraform ensures the property exists first
      property_id = mixpanel_custom_property.lifetime_value.id
      operator    = ">"
      value       = 1000
    }
  })
  
  # Explicit dependency (usually not needed, but makes intent clear)
  depends_on = [mixpanel_custom_property.lifetime_value]
}

# 3. Create dashboards that reference the cohorts
resource "mixpanel_dashboard" "value_analysis" {
  project_id = var.project_id
  name       = "Value Analysis"
  
  metadata = jsonencode({
    filters = {
      # Terraform ensures the cohort exists first
      cohort_id = mixpanel_cohort.high_value_users.id
    }
  })
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
      selector = {
        filter = {
          property = "session_count"
          operator = ">="
          value    = 10
        }
      }
    }
    
    recent_signups = {
      name        = "Recent Signups"
      description = "Signed up in last 7 days"
      selector = {
        filter = {
          property = "signup_date"
          operator = "within"
          value    = "7d"
        }
      }
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
  selector    = jsonencode(local.cohorts[each.value.cohort].selector)
}

# Access as: mixpanel_cohort.cohorts["dev-power_users"]
#            mixpanel_cohort.cohorts["prod-recent_signups"]
```

---

### Example 2: Clone dashboards with data source references

```hcl
# Look up existing custom properties (created manually or in another stack)
data "mixpanel_custom_property" "user_tier" {
  for_each = local.environments
  
  project_id = each.value.project_id
  name       = "User Tier"  # Must exist in all projects
}

# Clone dashboard across environments
resource "mixpanel_dashboard" "executive_overview" {
  for_each = local.environments
  
  project_id = each.value.project_id
  name       = "Executive Overview"
  
  metadata = jsonencode({
    charts = [
      {
        type  = "insights"
        title = "Users by Tier"
        query = {
          breakdown = {
            # ID resolved per environment
            property_id = data.mixpanel_custom_property.user_tier[each.key].id
          }
        }
      }
    ]
  })
}
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
    user_tier       = { data_type = "string" }
    lifetime_value  = { data_type = "number" }
    last_seen       = { data_type = "datetime" }
  }
  
  project_id = var.project_id
  name       = title(replace(each.key, "_", " "))
  data_type  = each.value.data_type
}

# Cohorts
resource "mixpanel_cohort" "power_users" {
  project_id  = var.project_id
  name        = "Power Users"
  description = "Managed by Terraform"
  
  selector = jsonencode({
    filter = {
      property_id = mixpanel_custom_property.properties["lifetime_value"].id
      operator    = ">"
      value       = 500
    }
  })
}

# Dashboards
resource "mixpanel_dashboard" "main" {
  project_id = var.project_id
  name       = "Main Dashboard - ${var.environment}"
  
  metadata = jsonencode({
    filters = {
      cohort_id = mixpanel_cohort.power_users.id
    }
  })
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
selector = "{\"cohort_id\":42}"  # Won't work in other projects
```

**Solution:**
```hcl
selector = jsonencode({
  cohort_id = mixpanel_cohort.power_users.id
})
```

---

### Pitfall: Circular dependencies

**Problem:**
```hcl
resource "mixpanel_cohort" "a" {
  selector = jsonencode({
    cohort_id = mixpanel_cohort.b.id  # A depends on B
  })
}

resource "mixpanel_cohort" "b" {
  selector = jsonencode({
    cohort_id = mixpanel_cohort.a.id  # B depends on A
  })
}
```

**Solution:**  
Restructure to break the cycle or use `terraform_data` with explicit
lifecycle rules.

---

### Pitfall: Inconsistent naming across environments

**Problem:**
```hcl
# Dev uses "UserTier", prod uses "User Tier"
data "mixpanel_custom_property" "tier" {
  name = var.custom_property_name  # Fragile
}
```

**Solution:**  
Establish naming conventions and enforce them:
```hcl
# Enforce consistent naming in all projects
locals {
  property_names = {
    user_tier = "User Tier"  # Single source of truth
  }
}

data "mixpanel_custom_property" "tier" {
  for_each = local.environments
  
  project_id = each.value.project_id
  name       = local.property_names.user_tier
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
  
  selector = var.enable_new_cohort_logic ? jsonencode({
    # New logic
    filter = { /* ... */ }
  }) : jsonencode({
    # Old logic
    filter = { /* ... */ }
  })
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
| `jsonencode` with TF refs | Embed IDs that differ per project | `jsonencode({ cohort_id = mixpanel_cohort.x.id })` |
| By-name data sources | Reference existing objects by name | `data "mixpanel_custom_property" "tier" { name = "User Tier" }` |
| Explicit dependencies | Control apply order | `depends_on = [mixpanel_custom_property.x]` |
| Modules | Encapsulate full project config | `module "mp_dev" { source = "./modules/project" }` |
