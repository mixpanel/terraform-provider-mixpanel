package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestEnvelopeStatusErrorHTTP200 verifies that an HTTP-200 body carrying the
// Mixpanel error envelope ({"status":"error","error":"..."} — see webapp
// app_api/response.py) is surfaced as an error including the server message,
// instead of being silently treated as a success payload.
func TestEnvelopeStatusErrorHTTP200(t *testing.T) {
	_, err := UnwrapEnvelope([]byte(`{"status": "error", "error": "project not found"}`))
	if err == nil {
		t.Fatal("UnwrapEnvelope: expected an error for status \"error\", got nil")
	}
	if !strings.Contains(err.Error(), "project not found") {
		t.Errorf("error %q does not include the server error message", err)
	}
}

// TestEnvelopeStatusErrorObjectPayload verifies a structured `error` payload
// (the agentic endpoints return {"code":..., "message":...} with HTTP 200) is
// included verbatim in the returned error.
func TestEnvelopeStatusErrorObjectPayload(t *testing.T) {
	body := `{"status": "error", "id": "x", "error": {"code": "requires_payment_credentials", "message": "Payment credentials are required"}}`
	_, err := UnwrapEnvelope([]byte(body))
	if err == nil {
		t.Fatal("UnwrapEnvelope: expected an error for status \"error\", got nil")
	}
	if !strings.Contains(err.Error(), "requires_payment_credentials") {
		t.Errorf("error %q does not include the structured error payload", err)
	}
}

// TestEnvelopeStatusErrorNoErrorField verifies the error path still fires (with
// the raw body as context) when the envelope omits the `error` field.
func TestEnvelopeStatusErrorNoErrorField(t *testing.T) {
	_, err := UnwrapEnvelope([]byte(`{"status": "error"}`))
	if err == nil {
		t.Fatal("UnwrapEnvelope: expected an error for status \"error\", got nil")
	}
}

// TestEnvelopeStatusOK verifies the normal success envelope still unwraps to
// its `results` payload.
func TestEnvelopeStatusOK(t *testing.T) {
	inner, err := UnwrapEnvelope([]byte(`{"status": "ok", "results": {"id": 42, "name": "x"}}`))
	if err != nil {
		t.Fatalf("UnwrapEnvelope: %v", err)
	}
	if !strings.Contains(string(inner), `"id": 42`) {
		t.Errorf("results not unwrapped: %s", inner)
	}
}

// TestEnvelopeNoStatusPassthrough verifies an unenveloped root entity (no
// `status` field, e.g. SCIM / custom_event responses) passes through unchanged.
func TestEnvelopeNoStatusPassthrough(t *testing.T) {
	body := `{"id": 7, "name": "entity at root"}`
	inner, err := UnwrapEnvelope([]byte(body))
	if err != nil {
		t.Fatalf("UnwrapEnvelope: %v", err)
	}
	if string(inner) != body {
		t.Errorf("body changed: got %s, want %s", inner, body)
	}
}

// TestEnvelopeNonObjectPassthrough verifies a non-object body (e.g. a bare
// JSON array) passes through unchanged.
func TestEnvelopeNonObjectPassthrough(t *testing.T) {
	body := `[{"id": 1}, {"id": 2}]`
	inner, err := UnwrapEnvelope([]byte(body))
	if err != nil {
		t.Fatalf("UnwrapEnvelope: %v", err)
	}
	if string(inner) != body {
		t.Errorf("body changed: got %s, want %s", inner, body)
	}
}

// TestEnvelopeDoUnwrapStatusError verifies the DoUnwrap path fails an HTTP-200
// error envelope with endpoint context, both when the caller wants the body and
// when it discards it (out == nil, e.g. deletes).
func TestEnvelopeDoUnwrapStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status": "error", "error": "boom from server"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ServiceAccount: "sa", ServiceSecret: "secret"})

	var out map[string]any
	err := c.DoUnwrap(context.Background(), "GET", "/api/app/projects/1/webhooks", nil, &out)
	if err == nil {
		t.Fatal("DoUnwrap: expected an error for a 200 error envelope, got nil")
	}
	if !strings.Contains(err.Error(), "boom from server") {
		t.Errorf("error %q does not include the server error message", err)
	}
	if !strings.Contains(err.Error(), "/api/app/projects/1/webhooks") {
		t.Errorf("error %q does not include endpoint context", err)
	}

	// out == nil (delete-style call) must not bypass the envelope check.
	err = c.DoUnwrap(context.Background(), "DELETE", "/api/app/projects/1/webhooks/9", nil, nil)
	if err == nil {
		t.Fatal("DoUnwrap(out=nil): expected an error for a 200 error envelope, got nil")
	}
}

// TestEnvelopeDoFormErrorEnvelope verifies the form-encoded path gets the same
// protection: DoForm returns the raw bytes, and unwrapping them (as every form
// caller does downstream) fails on an HTTP-200 error envelope.
func TestEnvelopeDoFormErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status": "error", "error": "custom event name already in use"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ServiceAccount: "sa", ServiceSecret: "secret"})
	respBody, err := c.DoForm(context.Background(), "POST", "/api/app/custom_events/1/", map[string]any{"name": "n"})
	if err != nil {
		t.Fatalf("DoForm: %v", err)
	}
	if _, err := UnwrapEnvelope(respBody); err == nil {
		t.Fatal("UnwrapEnvelope after DoForm: expected an error for a 200 error envelope, got nil")
	} else if !strings.Contains(err.Error(), "custom event name already in use") {
		t.Errorf("error %q does not include the server error message", err)
	}
}
