# Task #15: API Endpoints Identified

## Summary
Successfully located all the required API endpoints in the OpenAPI spec (`gen/spec/openapi.pruned.json`).

## Identified Endpoints

### 1. Property Definitions Bulk PATCH (setSchema) ✅ FOUND
**Endpoint:** `PATCH /api/app/projects/{project_id}/data-definitions/properties`

**Request Schema:** `CrudPropertyRequest`
- Polymorphic PATCH body for bulk or single edits
- Fields: id, dropped, hidden, merged, description, display_name, tags, etc.
- Additional properties: true (flexible schema)

**Purpose:** Bulk update property schemas including:
- Visibility (`hidden` field)
- Dropped status (`dropped` field)
- Descriptions (`description` field)
- Display names (`display_name` field)
- Tags

**Implementation:** This single endpoint handles multiple Task #15 requirements!

### 2. Event Merge ✅ FOUND  
**Endpoint:** `POST /api/app/projects/{project_id}/data-definitions/lexicon/events/merges`

**Request Schema:** `MergeRequest`
```json
{
  "merge": [
    {
      // MergeGroup objects
    }
  ]
}
```

**Undo Endpoint:** `POST /api/app/projects/{project_id}/data-definitions/lexicon/events/merges/undo`

**Purpose:** Merge events with support for undo

### 3. Property Merge ✅ FOUND
**Endpoint:** `POST /api/app/projects/{project_id}/data-definitions/lexicon/properties/merges`

**Request Schema:** `MergeRequest` (same as events)

**Undo Endpoint:** `POST /api/app/projects/{project_id}/data-definitions/lexicon/properties/merges/undo`

**Purpose:** Merge properties with support for undo

### 4. Visibility (Hidden Properties) ✅ FOUND
**Endpoints:**
- Collection: `GET/POST /api/app/projects/{project_id}/hidden-properties`
- Instance: `GET/PATCH/DELETE /api/app/projects/{project_id}/hidden-properties/{property_id}`

**Also available for events:**
- Collection: `GET/POST /api/app/projects/{project_id}/hidden-events`
- Instance: `GET/PATCH/DELETE /api/app/projects/{project_id}/hidden-events/{event_id}`

**Note:** Visibility can ALSO be managed via the bulk PATCH endpoint above using the `hidden` field!

### 5. Drop Filters (Different from Definitions) ✅ FOUND
**Endpoint:** `GET/POST/PATCH/DELETE /api/app/projects/{project_id}/data-definitions/events/drop-filters`

**Purpose:** Manage event drop filters (note: this is different from the `dropped` field on definitions)

### Additional Related Endpoints Found

**Events Endpoint:**
- `GET/PATCH /api/app/projects/{project_id}/data-definitions/events`
- Similar bulk PATCH capability for events

**Audit Endpoints:**
- `/api/app/projects/{project_id}/data-definitions/audit`
- `/api/app/projects/{project_id}/data-definitions/audit-events-only`

**History:**
- `/api/app/projects/{project_id}/data-definitions/events/{event_name}/history`
- `/api/app/projects/{project_id}/data-definitions/properties/{property_name}/history`

## Key Findings

### The Big Discovery: Single Bulk Endpoint
The **PATCH /data-definitions/properties** endpoint handles MULTIPLE Task #15 requirements:
1. ✅ Bulk schema updates (`setSchema`)
2. ✅ Visibility management (`hidden` field - `setVisibility`)
3. ✅ Description updates (`description` field - `updateDescriptions`)
4. ✅ Dropped flags (`dropped` field - `dropEvents`)
5. ✅ Display names (`display_name` field)
6. ✅ Tags management

### Implementation Simplification
Instead of 5 separate resources, we can implement:

**Option A: Single Bulk Resource**
- `property_definitions_bulk` - Uses PATCH /data-definitions/properties
- Handles all property updates in one resource

**Option B: Specialized Resources**
- Still create separate resources for each operation
- All use the same endpoint but with different field subsets

### Merge Operations
- Separate endpoints for event and property merges
- Both support undo operations
- Use standard POST verbs

## Recommended Implementation

### Phase 1: Core Bulk Operations
1. **property_definitions_bulk** resource
   - Uses: `PATCH /api/app/projects/{project_id}/data-definitions/properties`
   - Capability: Custom or new `bulk_patch` capability
   - Handles: visibility, descriptions, dropped flags, display names, tags

2. **event_definitions_bulk** resource  
   - Uses: `PATCH /api/app/projects/{project_id}/data-definitions/events`
   - Similar to properties bulk

### Phase 2: Merge Operations
3. **lexicon_event_merge** resource
   - Uses: `POST /api/app/projects/{project_id}/data-definitions/lexicon/events/merges`
   - Undo: POST .../ merges/undo
   - Capability: Custom merge resource

4. **lexicon_property_merge** resource
   - Uses: `POST /api/app/projects/{project_id}/data-definitions/lexicon/properties/merges`
   - Undo: POST .../merges/undo
   - Capability: Custom merge resource

### Phase 3: Individual Hidden Resources (Optional)
These already exist as CRUD endpoints and could be standard resources:
- `hidden_property` - Standard CRUD
- `hidden_event` - Standard CRUD

## Manifest Entries

### property_definitions_bulk
```json
{
  "keep": true,
  "resource_name": "property_definitions_bulk",
  "capability": "bulk_patch",
  "read_path": "/api/app/projects/{project_id}/data-definitions/properties",
  "update_path": "/api/app/projects/{project_id}/data-definitions/properties",
  "update": "patch",
  "read_schema": "BaseOkResponseModel_list_PropertyDefinitionResult__",
  "update_req_schema": "CrudPropertyRequest",
  "jsonencode_fields": ["properties_update"],
  "notes": "Bulk PATCH for property definitions. Handles visibility (hidden), descriptions, dropped flags, display names, and tags in a single polymorphic request. Request body is flexible (additionalProperties:true) and passed via jsonencode. This endpoint consolidates multiple powertools operations: setSchema, setVisibility, updateDescriptions, dropEvents."
}
```

### lexicon_event_merge
```json
{
  "keep": true,
  "resource_name": "lexicon_event_merge",
  "capability": "rpc_lifecycle",
  "create_path": "/api/app/projects/{project_id}/data-definitions/lexicon/events/merges",
  "delete_path": "/api/app/projects/{project_id}/data-definitions/lexicon/events/merges/undo",
  "list_path": "/api/app/projects/{project_id}/data-definitions/events",
  "create_req_schema": "MergeRequest",
  "read_schema": "BaseOkResponseModel_MergeResponse_",
  "jsonencode_fields": ["merge_groups"],
  "notes": "Event merge operation with undo support. Create performs merge, delete performs undo. Read verifies merged state from events list."
}
```

### lexicon_property_merge  
```json
{
  "keep": true,
  "resource_name": "lexicon_property_merge",
  "capability": "rpc_lifecycle",
  "create_path": "/api/app/projects/{project_id}/data-definitions/lexicon/properties/merges",
  "delete_path": "/api/app/projects/{project_id}/data-definitions/lexicon/properties/merges/undo",
  "list_path": "/api/app/projects/{project_id}/data-definitions/properties",
  "create_req_schema": "MergeRequest",
  "read_schema": "BaseOkResponseModel_MergeResponse_",
  "jsonencode_fields": ["merge_groups"],
  "notes": "Property merge operation with undo support. Create performs merge, delete performs undo. Read verifies merged state from properties list."
}
```

## Next Steps

1. **Add manifest entries** for the three resources above
2. **Register resources** in registry.go
3. **Run regeneration**: `./gen/regen.sh`
4. **Test build**
5. **Create examples** showing usage
6. **Test against live API**

## Schema Details

### CrudPropertyRequest (Bulk PATCH)
```typescript
{
  id?: integer | string | null,
  dropped?: boolean | null,
  hidden?: boolean | null,
  merged?: boolean | null,
  description?: string | null,
  display_name?: string | null,
  tags?: array | null,
  // ... additional properties allowed
}
```

### MergeRequest
```typescript
{
  merge: MergeGroup[]  // 1-100 items
}
```

## Conclusion

All Task #15 requirements CAN be implemented! The API provides:
- ✅ Single bulk PATCH endpoint for most operations
- ✅ Dedicated merge endpoints with undo
- ✅ Clean, well-defined schemas
- ✅ Existing patterns (rpc_lifecycle for merge, custom for bulk PATCH)

The implementation is simpler than expected because one endpoint handles multiple requirements.
