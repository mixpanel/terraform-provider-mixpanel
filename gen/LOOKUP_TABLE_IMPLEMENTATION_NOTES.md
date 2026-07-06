# Lookup Table Implementation Notes

The `lookup_table` resource has been added to `refined_manifest.json` but requires
**custom CRUD implementation** due to its asynchronous upload workflow.

## Current Status

✅ **Completed:**
- Added to `refined_manifest.json` (after `data_governance_settings`)
- Documentation created in `docs/resources/lookup_table.md`
- Examples created in `examples/resources/mixpanel_lookup_table/`
- CSV sample data provided

❌ **Requires custom implementation:**
- The standard CRUD generator cannot handle the async upload handshake
- Custom Go code needed in `crudgen.py` OVERRIDES or manual implementation

## API Workflow

The Mixpanel lookup table API uses an asynchronous upload workflow that differs
from standard CRUD:

### Standard CRUD (most resources)
```
POST /collection → {id, ...fields}    (create)
GET  /instance   → {id, ...fields}    (read)
PATCH /instance  → {id, ...fields}    (update)
DELETE /instance → {}                 (delete)
```

### Lookup Table Upload Workflow
```
1. POST /lookup-tables
   Body: {"name": "...", "mark-ready": false}
   Response: {"uploadId": "abc123", "id": "42"}

2. GET /upload-url?uploadId=abc123
   Response: {"url": "https://storage.../", "method": "PUT"}

3. PUT https://storage.../  (external storage, not Mixpanel API)
   Body: <CSV file bytes>
   Response: 200 OK

4. GET /upload-status?uploadId=abc123  (poll until complete)
   Response: {"uploadStatus": "processing", "result": null}
   Then: {"uploadStatus": "complete", "result": {...}}

5. PATCH /lookup-tables  (optional, mark ready)
   Body: {"data-group-id": "42", "mark-ready": true}
   Response: {id, name, ...}
```

## Implementation Options

### Option 1: Add to crudgen.py OVERRIDES (recommended)

Add a custom override in `gen/crudgen.py` similar to `event_definition`'s
`read_after_create` pattern:

```python
OVERRIDES = {
    # ... existing overrides ...
    
    "lookup_table": {
        "collection": "/api/app/projects/{project_id}/data-definitions/lookup-tables",
        "instance": "/api/app/projects/{project_id}/data-definitions/lookup-tables",
        "id_param": "lookup_table_id",
        "update": "patch",
        "delete": "delete",
        "project_scoped": True,
        "enveloped": True,
        
        # Custom upload workflow flags
        "async_upload": True,
        "upload_url_path": "/api/app/projects/{project_id}/data-definitions/lookup-tables/upload-url",
        "upload_status_path": "/api/app/projects/{project_id}/data-definitions/lookup-tables/upload-status",
        
        # CSV file attribute
        "file_upload_attr": "csv_file",
    },
}
```

Then extend the CRUD template in `crudgen.py` to generate:

```go
func (r *LookupTableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
    // 1. Extract csv_file path from plan
    var csvFile string
    req.Plan.GetAttribute(ctx, path.Root("csv_file"), &csvFile)
    
    // 2. POST /lookup-tables to get uploadId
    body := map[string]interface{}{
        "name": name,
        "mark-ready": false,
    }
    createResp, err := r.client.Do(ctx, "POST", r.collectionPath(projectID), body)
    uploadID := createResp["uploadId"].(string)
    id := createResp["id"].(string)
    
    // 3. GET /upload-url?uploadId=X
    uploadURLResp, err := r.client.Do(ctx, "GET", 
        fmt.Sprintf("%s?uploadId=%s", r.uploadURLPath(projectID), uploadID), nil)
    signedURL := uploadURLResp["url"].(string)
    method := uploadURLResp["method"].(string)
    
    // 4. Upload CSV to signed URL
    csvBytes, err := os.ReadFile(csvFile)
    req, _ := http.NewRequestWithContext(ctx, method, signedURL, bytes.NewReader(csvBytes))
    req.Header.Set("Content-Type", "text/csv")
    _, err = http.DefaultClient.Do(req)
    
    // 5. Poll /upload-status?uploadId=X until complete
    for {
        statusResp, _ := r.client.Do(ctx, "GET",
            fmt.Sprintf("%s?uploadId=%s", r.uploadStatusPath(projectID), uploadID), nil)
        
        status := statusResp["uploadStatus"].(string)
        if status == "complete" {
            break
        } else if status == "failed" {
            resp.Diagnostics.AddError("Upload failed", statusResp["error"])
            return
        }
        
        time.Sleep(5 * time.Second)  // Poll every 5s
    }
    
    // 6. Optionally PATCH to mark-ready
    if markReady {
        r.client.Do(ctx, "PATCH", r.instancePath(projectID, id), map[string]interface{}{
            "data-group-id": id,
            "mark-ready": true,
        })
    }
    
    // 7. Populate state
    state.ID = types.StringValue(id)
    resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
```

### Option 2: Manual implementation (fallback)

If the generator cannot easily support this pattern:

1. Generate the base resource via `./gen/regen.sh`
2. Manually edit `internal/provider/lookup_table_resource.go`
3. Override `Create()` and `Update()` with the async upload logic
4. Mark the file with `// MANUALLY MAINTAINED — DO NOT REGENERATE`

### Option 3: Use a provisioner (NOT recommended)

Terraform provisioners (`local-exec`, `remote-exec`) are a last resort and
should be avoided. They don't integrate with Terraform's state management.

## Required Schema Changes

The generated schema needs a **computed** `upload_status` attribute and a
**required** `csv_file` attribute:

```go
// In internal/provider/resource_lookup_table/lookup_table_resource_gen.go
func LookupTableResourceSchema(ctx context.Context) schema.Schema {
    return schema.Schema{
        Attributes: map[string]schema.Attribute{
            // Generated attributes from OpenAPI
            "id": schema.StringAttribute{Computed: true},
            "name": schema.StringAttribute{Required: true},
            "description": schema.StringAttribute{Optional: true},
            "data_group_id": schema.StringAttribute{Optional: true, Computed: true},
            
            // CUSTOM: CSV file path (not in OpenAPI spec)
            "csv_file": schema.StringAttribute{
                Required: true,
                Description: "Path to CSV file to upload. Changes trigger re-upload.",
            },
            
            // CUSTOM: Upload status (computed)
            "upload_status": schema.StringAttribute{
                Computed: true,
                Description: "Current upload status: pending, processing, complete, failed",
            },
            
            // CUSTOM: Mark ready flag
            "mark_ready": schema.BoolAttribute{
                Optional: true,
                Computed: true,
                Default: booldefault.StaticBool(true),
                Description: "Mark table as ready for queries after upload",
            },
        },
    }
}
```

## File Change Detection

Use `terraform_data` or a file hash to detect CSV changes:

```go
// Compute file hash for change detection
func computeFileHash(filePath string) (string, error) {
    f, err := os.Open(filePath)
    if err != nil {
        return "", err
    }
    defer f.Close()
    
    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil {
        return "", err
    }
    
    return hex.EncodeToString(h.Sum(nil)), nil
}
```

Store the hash in state and compare on Read/Update.

## Testing

Add acceptance tests in `internal/provider/lookup_table_resource_test.go`:

```go
func TestAccLookupTableResource(t *testing.T) {
    resource.Test(t, resource.TestCase{
        PreCheck:                 func() { testAccPreCheck(t) },
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testAccLookupTableResourceConfig("test_table", "user_attributes.csv"),
                Check: resource.ComposeAggregateTestCheckFunc(
                    resource.TestCheckResourceAttr("mixpanel_lookup_table.test", "name", "test_table"),
                    resource.TestCheckResourceAttr("mixpanel_lookup_table.test", "upload_status", "complete"),
                    resource.TestCheckResourceAttrSet("mixpanel_lookup_table.test", "id"),
                ),
            },
            // Test CSV file update
            {
                Config: testAccLookupTableResourceConfig("test_table", "user_attributes_v2.csv"),
                Check: resource.ComposeAggregateTestCheckFunc(
                    resource.TestCheckResourceAttr("mixpanel_lookup_table.test", "upload_status", "complete"),
                ),
            },
        },
    })
}
```

## Production Considerations

1. **Polling timeout**: Set max poll duration (e.g., 5 minutes) to avoid infinite loops
2. **Retry logic**: Implement exponential backoff for transient failures
3. **Large files**: Stream uploads rather than loading entire file into memory
4. **Signed URL expiration**: Handle expired URLs by requesting a fresh one
5. **Concurrent uploads**: Prevent multiple simultaneous uploads to the same table

## References

- OpenAPI paths: `/api/app/projects/{project_id}/data-definitions/lookup-tables/*`
- Schemas: `LookupTableResult`, `CrudLookupTableJsonRequest`, `LookupTableUploadStatusResponse`
- Production usage: Based on logs showing clone_cohorts/clone_boards demand (27/9 runs)
- Related: `powertools index.js:8644` (reference implementation pattern)

## Next Steps

1. Decide on implementation approach (Option 1 recommended)
2. Extend `crudgen.py` with async upload support
3. Generate initial resource code
4. Manually add CSV file handling logic
5. Add file hash change detection
6. Write acceptance tests
7. Test with real Mixpanel project
8. Document limitations (file size, upload time, etc.)

## Status: Ready for Implementation

All design artifacts are complete:
- ✅ Manifest entry added
- ✅ Documentation written
- ✅ Examples created
- ✅ API workflow documented
- ⏳ Custom CRUD implementation needed

The resource is production-ready **after** implementing the async upload workflow.
