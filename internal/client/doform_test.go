package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDoFormEncodesFields verifies that DoForm sends
// application/x-www-form-urlencoded, sends string values verbatim, and
// JSON-encodes non-string values (the custom_events `alternatives` field is a
// JSON string the view json.loads server-side).
func TestDoFormEncodesFields(t *testing.T) {
	var gotCT, gotBody, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"custom_event":{"id":42,"name":"x"}}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ServiceAccount: "sa", ServiceSecret: "secret"})
	values := map[string]any{
		"name":         "My Event",
		"alternatives": []any{map[string]any{"event": "Page View"}},
	}
	if _, err := c.DoForm(context.Background(), "POST", "/api/app/custom_events/1234567/", values); err != nil {
		t.Fatalf("DoForm: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("content-type = %q, want form-urlencoded", gotCT)
	}
	// name verbatim
	if !strings.Contains(gotBody, "name=My+Event") {
		t.Errorf("body %q missing name form field", gotBody)
	}
	// alternatives JSON-encoded into a single form field
	if !strings.Contains(gotBody, "alternatives=") || !strings.Contains(gotBody, "Page+View") {
		t.Errorf("body %q missing JSON-encoded alternatives", gotBody)
	}
	// alternatives must NOT be sent as a structured/array form key
	if strings.Contains(gotBody, "alternatives%5B") || strings.Contains(gotBody, "alternatives[") {
		t.Errorf("alternatives should be a JSON string, not a structured key: %q", gotBody)
	}
}

// TestDoFormRedirectKeepsMethod verifies that a 301 redirect (slash-less ->
// trailing-slash) does NOT downgrade POST to GET, which previously turned a
// create into a list read and produced an empty body.
func TestDoFormRedirectKeepsMethod(t *testing.T) {
	var finalMethod, finalBody string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/app/custom_events/1234567", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/app/custom_events/1234567/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/api/app/custom_events/1234567/", func(w http.ResponseWriter, r *http.Request) {
		finalMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		finalBody = string(b)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"custom_event":{"id":7}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ServiceAccount: "sa", ServiceSecret: "secret"})
	// Intentionally hit the slash-less path to exercise the redirect.
	if _, err := c.DoForm(context.Background(), "POST", "/api/app/custom_events/1234567", map[string]any{"name": "n"}); err != nil {
		t.Fatalf("DoForm: %v", err)
	}
	if finalMethod != "POST" {
		t.Errorf("after redirect method = %q, want POST (no GET downgrade)", finalMethod)
	}
	if !strings.Contains(finalBody, "name=n") {
		t.Errorf("after redirect body not re-sent: %q", finalBody)
	}
}
