// MANUALLY MAINTAINED — DO NOT REGENERATE.
//
// Mock-backed lifecycle test for the lookup_table resource, covering the full
// create→upload-url→storage-PUT→register→poll handshake. The mock's first
// registration takes the ASYNC path (returns uploadId; upload-status reports
// PENDING once, then SUCCESS) so the polling loop is exercised; replacement
// uploads take the synchronous path ({"id": ...}) so both response shapes are
// covered. Dedicated server (not mock_test.go's generic echo) because the
// handshake spans four endpoints plus external storage.

package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// lookupTableMockID is a full-range negative int64 (matching real data-group
// ids like "-9199707515373727904") to prove the provider never routes the id
// through float64.
const lookupTableMockID = "-9199707515373727904"

type lookupTableMock struct {
	mu       sync.Mutex
	srvURL   string
	blobs    map[string]string // storage key -> uploaded CSV bytes
	keySeq   int
	deleted  bool
	name     string
	desc     string
	csv      string
	pollSeen int  // upload-status calls for the pending task
	pending  bool // async task outstanding
}

func newLookupTableMockServer(t *testing.T) (*httptest.Server, *lookupTableMock) {
	t.Helper()
	m := &lookupTableMock{blobs: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(m.handle))
	m.srvURL = srv.URL
	t.Cleanup(srv.Close)
	return srv, m
}

func (m *lookupTableMock) handle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p := r.URL.Path
	switch {
	// Workspace resolution (the resource prefers the workspace mount).
	case r.Method == http.MethodGet && strings.HasSuffix(strings.TrimRight(p, "/"), "/workspaces"):
		writeJSON(w, map[string]any{
			"status": "ok",
			"results": []any{
				map[string]any{"id": 1.0, "is_global": true, "is_default": true, "name": "All Project Data"},
			},
		})

	// Step 1: mint a "signed" URL pointing back at this server.
	case r.Method == http.MethodGet && strings.Contains(p, "/lookup-tables/upload-url"):
		m.keySeq++
		key := fmt.Sprintf("mock-key-%d", m.keySeq)
		writeJSON(w, map[string]any{
			"status": "ok",
			"results": map[string]any{
				"url":  m.srvURL + "/signed-storage/" + key,
				"path": "1/" + key,
				"key":  key,
			},
		})

	// Step 2: external storage PUT (no Mixpanel envelope).
	case r.Method == http.MethodPut && strings.HasPrefix(p, "/signed-storage/"):
		if r.Header.Get("Content-Type") != "text/csv" {
			http.Error(w, "content-type must match the signed url", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		m.blobs[strings.TrimPrefix(p, "/signed-storage/")] = string(body)
		w.WriteHeader(http.StatusOK)

	// Step 4: upload-status polling (PENDING once, then SUCCESS).
	case r.Method == http.MethodGet && strings.Contains(p, "/lookup-tables/upload-status"):
		if !m.pending {
			writeJSON(w, map[string]any{"status": "ok", "results": map[string]any{"uploadStatus": "NOTFOUND"}})
			return
		}
		m.pollSeen++
		if m.pollSeen < 2 {
			writeJSON(w, map[string]any{"status": "ok", "results": map[string]any{"uploadStatus": "PENDING"}})
			return
		}
		m.pending = false
		m.deleted = false
		writeJSON(w, map[string]any{
			"status": "ok",
			"results": map[string]any{
				"uploadStatus": "SUCCESS",
				// json.RawMessage keeps the full-range id a bare JSON number,
				// as the real Celery result does.
				"result": map[string]any{"id": json.RawMessage(lookupTableMockID)},
			},
		})

	// Step 3: form-POST registration (create or replace).
	case r.Method == http.MethodPost && strings.Contains(p, "/lookup-tables"):
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "form-urlencoded") {
			http.Error(w, `{"status":"error","error":"expected form-encoded body"}`, http.StatusBadRequest)
			return
		}
		_ = r.ParseForm()
		name := r.PostForm.Get("name")
		key := r.PostForm.Get("key")
		csv, ok := m.blobs[key]
		if !ok || r.PostForm.Get("path") != "1/"+key {
			http.Error(w, `{"status":"error","error":"no uploaded blob for key"}`, http.StatusBadRequest)
			return
		}
		m.name = name
		m.csv = csv
		if dgid := r.PostForm.Get("data-group-id"); dgid != "" {
			// Replacement of an existing table: synchronous response shape.
			if dgid != lookupTableMockID {
				http.Error(w, `{"status":"error","error":"unknown data-group-id"}`, http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{"status": "ok", "results": map[string]any{"id": json.RawMessage(lookupTableMockID)}})
			return
		}
		// New table: async path — hand back an uploadId and let the provider poll.
		m.pending = true
		m.pollSeen = 0
		writeJSON(w, map[string]any{"status": "ok", "results": map[string]any{"uploadId": "mock-task-1"}})

	// Read: single-table GET by data-group-id.
	case r.Method == http.MethodGet && strings.Contains(p, "/lookup-tables"):
		if m.deleted || m.name == "" || r.URL.Query().Get("data-group-id") != lookupTableMockID {
			http.Error(w, `{"status":"error","error":"Lookup Table not Found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{
			"status": "ok",
			"results": []any{
				map[string]any{"id": lookupTableMockID, "name": m.name, "description": m.desc},
			},
		})

	// Metadata PATCH (rename / description).
	case r.Method == http.MethodPatch && strings.Contains(p, "/lookup-tables"):
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if fmt.Sprintf("%v", body["data-group-id"]) != lookupTableMockID || m.deleted {
			http.Error(w, `{"status":"error","error":"Lookup Table not found"}`, http.StatusNotFound)
			return
		}
		if v, ok := body["name"].(string); ok {
			m.name = v
		}
		if v, ok := body["description"].(string); ok {
			m.desc = v
		}
		writeJSON(w, map[string]any{"status": "ok", "results": nil})

	// Bulk-body DELETE.
	case r.Method == http.MethodDelete && strings.Contains(p, "/lookup-tables"):
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		ids, _ := body["data-group-ids"].([]any)
		for _, id := range ids {
			if fmt.Sprintf("%v", id) == lookupTableMockID {
				m.deleted = true
			}
		}
		writeJSON(w, map[string]any{"status": "ok", "results": map[string]any{}})

	default:
		http.Error(w, `{"error":"unexpected request"}`, http.StatusNotFound)
	}
}

func TestAccLookupTable_lifecycle(t *testing.T) {
	srv, mock := newLookupTableMockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		Steps: []resource.TestStep{
			{
				// Create runs the full async handshake (upload-url → storage
				// PUT → form POST → PENDING → SUCCESS polling).
				Config: providerConfig(srv.URL, `
resource "mixpanel_lookup_table" "test" {
  project_id  = 1
  name        = "tf-acc-lookup"
  description = "accounts by id"
  csv_content = "id,plan\nu1,free\nu2,pro\n"
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mixpanel_lookup_table.test", "id", lookupTableMockID),
					resource.TestCheckResourceAttr("mixpanel_lookup_table.test", "name", "tf-acc-lookup"),
					func(s *terraform.State) error {
						if mock.csv != "id,plan\nu1,free\nu2,pro\n" {
							return fmt.Errorf("mock received csv %q", mock.csv)
						}
						if mock.pollSeen < 2 {
							return fmt.Errorf("polling loop not exercised (pollSeen=%d)", mock.pollSeen)
						}
						return nil
					},
				),
			},
			{
				// Changed CSV content re-uploads in place (synchronous
				// replacement path, keyed by data-group-id) — an Update, not a
				// replace.
				Config: providerConfig(srv.URL, `
resource "mixpanel_lookup_table" "test" {
  project_id  = 1
  name        = "tf-acc-lookup"
  description = "accounts by id"
  csv_content = "id,plan\nu1,enterprise\nu2,pro\n"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("mixpanel_lookup_table.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mixpanel_lookup_table.test", "id", lookupTableMockID),
					func(s *terraform.State) error {
						if mock.csv != "id,plan\nu1,enterprise\nu2,pro\n" {
							return fmt.Errorf("replacement csv not uploaded, mock has %q", mock.csv)
						}
						return nil
					},
				),
			},
			{
				// Rename + description change ride the metadata PATCH (no
				// re-upload).
				Config: providerConfig(srv.URL, `
resource "mixpanel_lookup_table" "test" {
  project_id  = 1
  name        = "tf-acc-lookup-renamed"
  description = "accounts by id v2"
  csv_content = "id,plan\nu1,enterprise\nu2,pro\n"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("mixpanel_lookup_table.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: func(s *terraform.State) error {
					if mock.name != "tf-acc-lookup-renamed" {
						return fmt.Errorf("mock name = %q, want rename applied", mock.name)
					}
					return nil
				},
			},
			{
				// Import by "project_id:data_group_id". csv_content cannot be
				// read back from the API, so it is excluded from verification.
				ResourceName:                         "mixpanel_lookup_table.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "id",
				ImportStateIdFunc:                    importIDFunc("mixpanel_lookup_table.test", "id", "project_id"),
				ImportStateVerifyIgnore:              []string{"csv_content"},
			},
		},
	})
}
