# Implementation Plan for Tasks #14 and #15

## Task #14: Service Account Project Grants

### Objective
Add RPC association support for service account project grants (add-to-project/remove-from-project).

### Implementation Steps

1. **Add manifest entry for service_account_project** in `gen/refined_manifest.json`:
   ```json
   {
     "keep": true,
     "capability": "rpc_assoc",
     "resource_name": "service_account_project",
     "create_path": "/organizations/{organization_id}/add-to-project/",
     "delete_path": "/organizations/{organization_id}/remove-from-project/",
     "list_path": "/organizations/{organization_id}/service-accounts",
     "key_doc": "composite key identifying the service account and project association (format: serviceaccount_id:project_id)",
     "notes": "RPC association for granting/revoking service account access to projects. Uses add-to-project (create) and remove-from-project (delete) endpoints. The key format is 'serviceaccount_id:project_id' to uniquely identify the grant."
   }
   ```

2. **Register the resource** in `internal/provider/registry.go`:
   - Add `NewServiceAccountProjectResource` to `providerResources()`

3. **Regenerate** using `./gen/regen.sh`

4. **Test** the generated resource

## Task #15: Lexicon Bulk Operations

### Objective
Expand lexicon coverage for high-demand production operations:
- Property definitions bulk PATCH (setSchema)
- Bulk visibility operations (setVisibility)
- Bulk description updates (updateDescriptions)  
- Event/property merge/unmerge (mergeEvents)
- Dropped-event flags on definitions (dropEvents)

### Analysis
These operations are bulk/batch operations that don't fit the standard CRUD or RPC association patterns. They need custom implementation.

### Implementation Approach

Since these are bulk operations that don't fit the standard patterns, we have two options:

**Option A**: Create custom resources for each operation type
- More granular control
- Each operation is its own resource
- Follows Terraform patterns better

**Option B**: Create a single bulk operations resource
- Simpler from a resource count perspective
- More complex schema

We'll go with **Option A** for better maintainability.

### Resources to Create

1. **property_definitions_bulk** - Bulk PATCH for property schemas
   - Endpoint: `/api/app/projects/{project_id}/data-definitions/properties` (PATCH)
   - Manages multiple property definitions at once

2. **lexicon_visibility_bulk** - Bulk visibility updates
   - Endpoint: TBD (setVisibility reference)
   - Updates visibility for multiple events/properties

3. **lexicon_descriptions_bulk** - Bulk description updates
   - Endpoint: TBD (updateDescriptions reference)
   - Updates descriptions for multiple items

4. **event_merge** - Merge/unmerge events
   - Endpoint: TBD (mergeEvents reference)  
   - Handles event merging operations

5. **event_drop_flags** - Dropped-event flags on definitions
   - Endpoint: TBD (dropEvents reference)
   - Different from event_drop_filter

### Next Steps

1. Research the exact API endpoints from the spec or powertools
2. Determine the correct capability type for these resources
3. Add manifest entries
4. Implement custom resource files if needed
5. Register resources
6. Regenerate and test

## Notes

- The powertools index.js references (lines 5323, 4846, 4255, 4379, 4735, 4341) need to be examined to understand exact API patterns
- These bulk operations may require custom resource implementation beyond what the generator can produce
- May need to add a new capability type (e.g., "bulk_operation") to crudgen.py

## Decision Needed

Before proceeding with Task #15, we need to:
1. Access the powertools index.js file to understand the exact API endpoints
2. OR examine the OpenAPI spec for these endpoints
3. Determine if these operations are already in the spec

For now, I'll proceed with Task #14 since it follows the established RPC association pattern.
