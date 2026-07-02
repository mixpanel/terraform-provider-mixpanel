# Task #15 Analysis: Lexicon Bulk Operations

## Summary
Task #15 requires expanding lexicon coverage for high production demand operations. Based on the task description, the following operations need to be implemented:

### Required Operations

1. **Property definitions bulk PATCH (setSchema)**
   - Reference: powertools index.js:4846
   - Endpoint: `/api/app/projects/{project_id}/data-definitions/properties` (PATCH)
   - Purpose: Bulk update property schemas

2. **Bulk visibility operations (setVisibility)**
   - Reference: powertools index.js:4255  
   - Endpoint: TBD
   - Purpose: Update visibility for multiple events/properties

3. **Bulk description updates (updateDescriptions)**
   - Reference: powertools index.js:4379
   - Endpoint: TBD
   - Purpose: Update descriptions for multiple items

4. **Event/property merge/unmerge (mergeEvents)**
   - Reference: powertools index.js:4735
   - Endpoint: TBD
   - Purpose: Merge or unmerge events with resourceType per kind

5. **Dropped-event flags on definitions (dropEvents)**
   - Reference: powertools index.js:4341
   - Endpoint: TBD
   - Purpose: Different from event_drop_filter - flags on definitions themselves

## Implementation Challenges

### Missing Information
1. **No access to powertools index.js** - The reference file containing the exact API patterns is not available in the repository
2. **OpenAPI spec required** - Need to examine `gen/spec/openapi.pruned.json` to find these endpoints
3. **API endpoint discovery** - Without the powertools reference or spec, exact endpoint paths are unknown

### Implementation Approach

These operations don't fit the standard CRUD or RPC association patterns. They require one of:

**Option A: Custom resource implementation**
- Create individual resources for each operation
- Implement custom Create/Read/Update/Delete logic
- May require special handling in crudgen.py

**Option B: New capability type**
- Add a new "bulk_operation" capability to crudgen.py
- Similar to rpc_assoc but for batch operations
- Would allow generator to handle these uniformly

**Option C: Settings-style resources**
- Model each as a settings-singleton that accepts batch payloads
- Use the existing settings_singleton pattern

## Next Steps to Complete Task #15

1. **Access the OpenAPI spec**:
   ```bash
   # Fetch the spec (requires webapp access)
   python3 gen/prune_spec.py
   # Then examine gen/spec/openapi.pruned.json for these endpoints
   ```

2. **Search for endpoints**:
   ```bash
   cat gen/spec/openapi.pruned.json | jq '.paths | keys' | grep -E "properties|visibility|descriptions|merge|drop"
   ```

3. **Examine endpoint details**:
   - Request/response schemas
   - HTTP methods
   - Path parameters
   - Body structure

4. **Determine implementation pattern**:
   - If endpoints follow CRUD: Add to manifest as standard resources
   - If RPC-style: Add as rpc_assoc or similar
   - If completely custom: Implement hand-written resources

5. **Add to manifest** (example for property definitions):
   ```json
   {
     "keep": true,
     "resource_name": "property_definitions_bulk",
     "capability": "bulk_operation",
     "update_path": "/api/app/projects/{project_id}/data-definitions/properties",
     "notes": "Bulk PATCH for property schemas..."
   }
   ```

6. **Extend crudgen.py if needed**:
   - Add bulk_operation capability handler
   - Generate appropriate CRUD operations
   - Handle batch payloads

7. **Register resources** in `internal/provider/registry.go`

8. **Regenerate and test**

## Current Status

**Task #14**: ✅ Complete
- Service account project grants implemented
- Resource generated and tested
- Committed to repository

**Task #15**: ⏸️ Blocked
- Requires access to OpenAPI spec or powertools reference
- Need to identify exact API endpoints
- Implementation pattern to be determined based on endpoint structure

## Recommendation

To unblock Task #15:
1. Obtain the OpenAPI spec file
2. OR examine the powertools index.js file at the referenced lines
3. OR consult with the API team to understand these endpoints

Once endpoints are identified, implementation should follow the established patterns in the provider (CRUD, RPC assoc, or custom).
