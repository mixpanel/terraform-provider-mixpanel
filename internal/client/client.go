// Package client is a thin HTTP client for the Mixpanel App API, shared by every
// generated resource and data source. It handles project-scoped URL building,
// HTTP Basic auth with a service account, JSON request/response, and the
// BaseOkResponseModel `results` envelope that every entity endpoint returns.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// DefaultBaseURL is the Mixpanel API host. Paths are appended to it, e.g.
// /api/app/projects/{project_id}/dashboards.
const DefaultBaseURL = "https://mixpanel.com"

// Client talks to the Mixpanel App API with service-account Basic auth.
type Client struct {
	BaseURL               string
	ServiceAccount        string
	ServiceSecret         string
	DefaultProjectID      string
	DefaultOrganizationID string
	HTTPClient            *http.Client
}

// Config carries the resolved provider configuration into the client.
type Config struct {
	BaseURL               string
	ServiceAccount        string
	ServiceSecret         string
	DefaultProjectID      string
	DefaultOrganizationID string
}

// New builds a Client from resolved config, applying sensible defaults.
func New(cfg Config) *Client {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	return &Client{
		BaseURL:               strings.TrimRight(base, "/"),
		ServiceAccount:        cfg.ServiceAccount,
		ServiceSecret:         cfg.ServiceSecret,
		DefaultProjectID:      cfg.DefaultProjectID,
		DefaultOrganizationID: cfg.DefaultOrganizationID,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
			// Several App API routes 301-redirect a slash-less path to its
			// trailing-slash form. Go's default redirect policy downgrades
			// POST/PUT/DELETE to GET on a 301/302, which silently turns a
			// create/update into a list read (empty body -> "inconsistent
			// result after apply"). Preserve the original method and body so a
			// redirected write still writes. (Generated paths carry the
			// trailing slash where required, so this is a safety net.)
			CheckRedirect: preserveMethodOnRedirect,
		},
	}
}

// preserveMethodOnRedirect keeps the original HTTP method and body across
// redirects (Go's default would convert POST/PUT/DELETE to GET on 301/302/303).
func preserveMethodOnRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	if len(via) == 0 {
		return nil
	}
	prev := via[len(via)-1]
	req.Method = prev.Method
	req.Header.Set("Authorization", prev.Header.Get("Authorization"))
	if ct := prev.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if req.Body == nil && prev.GetBody != nil {
		if body, err := prev.GetBody(); err == nil {
			req.Body = body
			req.ContentLength = prev.ContentLength
		}
	}
	return nil
}

// ProjectID returns override if non-empty, else the provider default.
func (c *Client) ProjectID(override string) string {
	if override != "" {
		return override
	}
	return c.DefaultProjectID
}

// OrganizationID returns override if non-empty, else the provider default
// organization. Org-scoped resources (e.g. service_account) template the
// returned value into the {organization_id} URL segment.
func (c *Client) OrganizationID(override string) string {
	if override != "" {
		return override
	}
	return c.DefaultOrganizationID
}

// ProjectPath builds a project-scoped path: /api/app/projects/{project_id}/<suffix>.
// suffix should NOT include a leading slash.
func (c *Client) ProjectPath(projectID, suffix string) string {
	return fmt.Sprintf("/api/app/projects/%s/%s", projectID, strings.TrimLeft(suffix, "/"))
}

// URL joins the base URL with an absolute API path (path must start with "/").
func (c *Client) URL(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.BaseURL + path
}

// APIError is a non-2xx HTTP response from the API.
type APIError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("mixpanel API %s %s: status %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

// Do performs an HTTP request against an absolute API path. If body is non-nil it
// is JSON-encoded. The raw response body bytes are returned for 2xx responses
// (callers typically pass them through UnwrapEnvelope). DELETE that returns JSON
// (the Mixpanel convention, not 204) is handled like any other 2xx.
func (c *Client) Do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.URL(path), reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.SetBasicAuth(c.ServiceAccount, c.ServiceSecret)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}
	return respBody, nil
}

// DoForm performs an HTTP request whose body is application/x-www-form-urlencoded.
// A handful of legacy Mixpanel App API endpoints (e.g. custom_events) read their
// parameters from request.POST (form fields) rather than a JSON body; sending
// JSON to them yields HTTP 400 "missing required parameters". The values map is
// encoded as form fields: string values are sent verbatim, and any non-string
// value (list / object / number / bool) is JSON-encoded into its form field
// (these endpoints json.loads such fields server-side, e.g. `alternatives`).
//
// The raw response body bytes are returned for 2xx responses, matching Do.
func (c *Client) DoForm(ctx context.Context, method, path string, values map[string]any) ([]byte, error) {
	form := url.Values{}
	// Deterministic field order keeps requests reproducible.
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := values[k]
		switch s := v.(type) {
		case nil:
			continue
		case string:
			form.Set(k, s)
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("encoding form field %q: %w", k, err)
			}
			form.Set(k, string(b))
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.URL(path), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.ServiceAccount, c.ServiceSecret)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}
	return respBody, nil
}

// DoJSON performs Do and unmarshals the 2xx response body into out (if non-nil).
func (c *Client) DoJSON(ctx context.Context, method, path string, body, out any) error {
	respBody, err := c.Do(ctx, method, path, body)
	if err != nil {
		return err
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decoding response body: %w", err)
	}
	return nil
}

// baseOkResponse is the Mixpanel envelope: {"status": "ok", "results": <entity>}.
type baseOkResponse struct {
	Status  string          `json:"status"`
	Results json.RawMessage `json:"results"`
}

// UnwrapEnvelope extracts the `results` field of a BaseOkResponseModel response.
// If the payload is not enveloped (no top-level `results`), the original bytes are
// returned unchanged so it also works for the few endpoints that return the entity
// at the root (e.g. SCIM, custom_event).
func UnwrapEnvelope(respBody []byte) ([]byte, error) {
	var env baseOkResponse
	if err := json.Unmarshal(respBody, &env); err != nil {
		// not an object / not enveloped — hand the raw bytes back
		return respBody, nil
	}
	if env.Results == nil {
		return respBody, nil
	}
	return env.Results, nil
}

// DoUnwrap performs Do, unwraps the envelope, and unmarshals into out.
func (c *Client) DoUnwrap(ctx context.Context, method, path string, body, out any) error {
	respBody, err := c.Do(ctx, method, path, body)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	inner, err := UnwrapEnvelope(respBody)
	if err != nil {
		return err
	}
	if len(inner) == 0 {
		return nil
	}
	if err := json.Unmarshal(inner, out); err != nil {
		return fmt.Errorf("decoding unwrapped response body: %w", err)
	}
	return nil
}
