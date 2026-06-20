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
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// importIDFunc returns an ImportStateIdFunc that reconstructs the import id from the
// prior state of the named resource. idAttr is the resource's identity attribute
// (ent.identity_attr). scopeAttr names the scope attribute whose value prefixes the
// id as "<scope>:<id>" — "project_id" for project/workspace-scoped resources,
// "organization_id" for org-scoped resources, and "" for unscoped/singleton
// resources (the bare id, which for a singleton already equals the project id).
// This matches the composite, scope-aware ImportState parser. The default provider
// scope value ("1") backstops an empty prior-state scope attribute.
func importIDFunc(resourceName, idAttr, scopeAttr string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", resourceName)
		}
		id := rs.Primary.Attributes[idAttr]
		if id == "" {
			id = rs.Primary.ID
		}
		if scopeAttr == "" {
			return id, nil
		}
		scope := rs.Primary.Attributes[scopeAttr]
		if scope == "" {
			// Provider default from providerConfig.
			scope = "1"
		}
		return scope + ":" + id, nil
	}
}

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
	// createIDField, when set and different from idField, is the field name the
	// CREATE response carries the server-assigned id under. A read_after_create
	// entity whose create response is a flat id-bearing object can return the id
	// under a key ("id") that differs from the canonical read identity field
	// (e.g. behavior_id): the create handler extracts the id from this key, then
	// re-reads via the instance GET (which returns the id under idField).
	createIDField string
	// rpcLifecycle models an org-scoped RPC entity (project) that has no REST CRUD:
	// create POSTs {createNameKey:[<name>]} to .../create-<plural>/ and the
	// enveloped response is an ARRAY of created rows (each {idField, matchAttr:name});
	// read GETs the list path and returns every stored row; delete POSTs
	// {idListKey:[<id>]} to .../delete-<plural>/. matchAttr defaults to "name".
	rpcLifecycle  bool
	createNameKey string
	idListKey     string
	matchAttr     string
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

	if m.opts.rpcLifecycle {
		m.handleRPC(w, r)
		return
	}

	switch r.Method {
	case http.MethodPost:
		body := m.parseBody(r)
		var idStr string
		if seg := lastSegment(r.URL.Path); !m.opts.listCreate {
			if _, exists := m.store[seg]; exists {
				// POST to an existing instance id is an in-place update (some
				// endpoints use POST, not PUT/PATCH, as their update verb, e.g.
				// data_group). Merge into the stored object under the same id so the
				// object is mutated rather than duplicated under a fresh id -- which
				// matches the real API and keeps the post-update id stable for the
				// import round-trip.
				obj := m.store[seg]
				for k, v := range body {
					obj[k] = v
				}
				obj[m.opts.idField] = m.idValue(seg)
				m.store[seg] = obj
				m.respond(w, seg, obj)
				return
			}
		}
		if m.opts.upsert {
			// Upsert: the id is supplied by the configuration and templated into
			// the POST path (create-to-instance), not assigned by the server.
			idStr = lastSegment(r.URL.Path)
		} else {
			m.counter++
			idStr = strconv.Itoa(m.counter)
		}
		body[m.opts.idField] = m.idValue(idStr)
		if m.opts.createIDField != "" && m.opts.createIDField != m.opts.idField {
			// read_after_create entities whose create response carries the id under a
			// different key than the read identity field (e.g. behavior: create -> id,
			// read -> behavior_id). Both keys are harmless on the stored object.
			body[m.opts.createIDField] = m.idValue(idStr)
		}
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

// handleRPC answers the org-scoped RPC lifecycle (project): create-<plural>,
// the list path, and delete-<plural>. The caller holds m.mu.
func (m *mockServer) handleRPC(w http.ResponseWriter, r *http.Request) {
	matchAttr := m.opts.matchAttr
	if matchAttr == "" {
		matchAttr = "name"
	}
	seg := lastSegment(r.URL.Path)
	switch {
	case r.Method == http.MethodPost && strings.HasPrefix(seg, "create-"):
		body := m.parseBody(r)
		names, _ := body[m.opts.createNameKey].([]any)
		created := make([]any, 0, len(names))
		for _, n := range names {
			m.counter++
			idStr := strconv.Itoa(m.counter)
			obj := map[string]any{m.opts.idField: m.idValue(idStr), matchAttr: n}
			m.store[idStr] = obj
			created = append(created, obj)
		}
		m.respondValue(w, created)
	case r.Method == http.MethodPost && strings.HasPrefix(seg, "delete-"):
		body := m.parseBody(r)
		ids, _ := body[m.opts.idListKey].([]any)
		for _, id := range ids {
			delete(m.store, idToKey(id))
		}
		m.respondValue(w, map[string]any{})
	case r.Method == http.MethodGet:
		// List path: return every stored row (read_from_list selects by id).
		list := make([]any, 0, len(m.store))
		for _, o := range m.store {
			list = append(list, o)
		}
		m.respondValue(w, list)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// idToKey renders a JSON-decoded id (number or string) as the store key, matching
// the decimal string strconv.Itoa produced when the row was created.
func idToKey(id any) string {
	switch x := id.(type) {
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	case string:
		return x
	default:
		return fmt.Sprintf("%v", x)
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
