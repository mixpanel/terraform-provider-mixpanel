// Mock-server test harness shared by the generated per-entity lifecycle tests.
//
// These are acceptance tests (resource.Test, gated by TF_ACC) but they run
// against an in-process httptest echo server instead of the real Mixpanel API:
// the provider's base_url is pointed at the mock, so a full plan -> apply ->
// refresh -> destroy cycle exercises the real Terraform graph and the generic
// tftypes<->JSON bridge with no credentials and nothing created externally.
//
// The mock is deliberately generic: it round-trips whatever the provider sends
// (POST stores the body and assigns an id; GET returns it; PATCH/PUT merges;
// DELETE removes), enveloping the response per the entity's contract. That makes
// the post-apply empty-plan check a real idempotency test of the bridge for
// every entity, without per-entity recorded response fixtures.
package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testProtoV6 wires the provider under test for resource.Test.
var testProtoV6 = map[string]func() (tfprotov6.ProviderServer, error){
	"mixpanel": providerserver.NewProtocol6WithError(New("test")()),
}

// mockOpts describes the response contract of one entity so the generic echo
// server can answer it faithfully. Everything else (paths, request shape) the
// server infers from the request itself.
type mockOpts struct {
	enveloped  bool   // wrap the response in {"status":"ok","results": ...}
	idField    string // identity field injected into the stored object (default "id")
	stringID   bool   // render the assigned id as a JSON string rather than a number
	resultsMap bool   // shape results as {id: obj} (themes_to_dict_map convention)
	upsert     bool   // create POSTs to an instance path with a config-supplied id
	listCreate bool   // create response is a list the provider selects from (collection-body-id)
}

// mockServer is an in-memory echo backend for the Mixpanel App API.
type mockServer struct {
	*httptest.Server
	opts    mockOpts
	mu      sync.Mutex
	store   map[string]map[string]any
	counter int
}

// newMockServer starts an echo server and registers cleanup. idField defaults to
// "id" when empty.
func newMockServer(t *testing.T, opts mockOpts) *mockServer {
	t.Helper()
	if opts.idField == "" {
		opts.idField = "id"
	}
	m := &mockServer{opts: opts, store: map[string]map[string]any{}, counter: 1000}
	m.Server = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.Close)
	return m
}

func (m *mockServer) handle(w http.ResponseWriter, r *http.Request) {
	// Workspace resolution: feature-flag-style routes resolve the project's
	// canonical workspace via GET .../workspaces before their CRUD calls.
	if r.Method == http.MethodGet && strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/workspaces") {
		writeJSON(w, map[string]any{
			"status": "ok",
			"results": []any{
				map[string]any{"id": 1.0, "is_global": true, "is_default": true, "name": "All Project Data"},
			},
		})
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	switch r.Method {
	case http.MethodPost:
		body := m.parseBody(r)
		var idStr string
		if m.opts.upsert {
			// Upsert: the id is supplied by the configuration and templated into
			// the POST path (create-to-instance), not assigned by the server.
			idStr = lastSegment(r.URL.Path)
		} else {
			m.counter++
			idStr = strconv.Itoa(m.counter)
		}
		body[m.opts.idField] = m.idValue(idStr)
		m.store[idStr] = body
		if m.opts.listCreate {
			// Collection-body-id create: the provider selects the new element from
			// a returned list rather than reading a single object.
			m.respondValue(w, []any{body})
			return
		}
		m.respond(w, idStr, body)
	case http.MethodGet:
		id := lastSegment(r.URL.Path)
		if obj, ok := m.store[id]; ok {
			m.respond(w, id, obj)
			return
		}
		// Collection GET (read-from-list entities): return every stored object.
		list := make([]any, 0, len(m.store))
		for _, o := range m.store {
			list = append(list, o)
		}
		m.respondValue(w, list)
	case http.MethodPut, http.MethodPatch:
		id := lastSegment(r.URL.Path)
		obj, ok := m.store[id]
		if !ok {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		for k, v := range m.parseBody(r) {
			obj[k] = v
		}
		obj[m.opts.idField] = m.idValue(id)
		m.store[id] = obj
		m.respond(w, id, obj)
	case http.MethodDelete:
		delete(m.store, lastSegment(r.URL.Path))
		m.respond(w, "", map[string]any{})
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// idValue renders the assigned id as the type the schema expects.
func (m *mockServer) idValue(idStr string) any {
	if m.opts.stringID {
		return idStr
	}
	f, _ := strconv.ParseFloat(idStr, 64)
	return f
}

// parseBody decodes a JSON or form-urlencoded request body into a map. Form
// fields are JSON-decoded when possible (the form convention used by a few legacy
// endpoints, where lists/objects arrive as JSON-encoded form values).
func (m *mockServer) parseBody(r *http.Request) map[string]any {
	out := map[string]any{}
	if strings.Contains(r.Header.Get("Content-Type"), "form-urlencoded") {
		_ = r.ParseForm()
		for k, vs := range r.PostForm {
			if len(vs) == 0 {
				continue
			}
			var v any
			if err := json.Unmarshal([]byte(vs[0]), &v); err == nil {
				out[k] = v
			} else {
				out[k] = vs[0]
			}
		}
		return out
	}
	_ = json.NewDecoder(r.Body).Decode(&out)
	return out
}

// respond writes the entity body, applying the results-map and envelope shapes.
func (m *mockServer) respond(w http.ResponseWriter, idStr string, obj map[string]any) {
	if m.opts.resultsMap && idStr != "" {
		m.respondValue(w, map[string]any{idStr: obj})
		return
	}
	m.respondValue(w, obj)
}

// respondValue writes an arbitrary results value, enveloped when configured.
func (m *mockServer) respondValue(w http.ResponseWriter, results any) {
	if m.opts.enveloped {
		writeJSON(w, map[string]any{"status": "ok", "results": results})
		return
	}
	writeJSON(w, results)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// lastSegment returns the final non-empty path segment (the instance id).
func lastSegment(p string) string {
	p = strings.Trim(p, "/")
	if p == "" {
		return ""
	}
	parts := strings.Split(p, "/")
	return parts[len(parts)-1]
}

// providerConfig renders the provider block (pointed at the mock) plus the given
// resource/data-source HCL.
func providerConfig(baseURL, body string) string {
	return fmt.Sprintf(`
provider "mixpanel" {
  service_account        = "test"
  service_account_secret = "test"
  project_id             = "1"
  organization_id        = "1"
  base_url               = %q
}
`, baseURL) + body
}
