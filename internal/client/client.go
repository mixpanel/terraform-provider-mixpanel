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
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the Mixpanel API host. Paths are appended to it, e.g.
// /api/app/projects/{project_id}/dashboards.
const DefaultBaseURL = "https://mixpanel.com"

// Retry configuration constants
const (
	maxRetries   = 5                // Maximum number of retry attempts
	baseBackoff  = 1 * time.Second  // Base delay for exponential backoff
	maxBackoff   = 60 * time.Second // Maximum delay for exponential backoff
	jitterFactor = 0.1              // Jitter factor (10% of delay)
)

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
	// Only re-attach credentials on a SAME-HOST redirect. Go's stdlib deliberately
	// strips the Authorization header when a redirect crosses to a different host so
	// the Basic-auth service-account credentials are never leaked to another domain;
	// re-adding it unconditionally would defeat that protection.
	if req.URL.Host == prev.URL.Host {
		req.Header.Set("Authorization", prev.Header.Get("Authorization"))
	}
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
	msg := fmt.Sprintf("mixpanel API %s %s: status %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
	// Org-scoped resources (teams, projects, org settings, user/role grants) are
	// permission-gated: the service account must be an organization admin/owner.
	// A project-scoped service account authenticates fine (reads work) but gets a
	// 403 on these writes. Surface that clearly so users know it's a privilege
	// issue, not a bad request or wrong credentials.
	if e.StatusCode == 403 && strings.Contains(e.Path, "/organizations/") {
		msg += "\n\nThis is an organization-scoped operation that requires an organization-admin " +
			"service account. The configured service account authenticates but lacks org-admin " +
			"permission on this organization. Use a service account that is an admin/owner of the org."
	}
	return msg
}

// isRetryableStatus returns true if the HTTP status code is retryable.
// Retries on:
// - 408 Request Timeout
// - 429 Too Many Requests (rate limit)
// - 500 Internal Server Error
// - 502 Bad Gateway
// - 503 Service Unavailable
// - 504 Gateway Timeout
//
// Does NOT retry on:
// - 4xx errors (client errors) except 408/429 - these are terminal
func isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout, // 408
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	default:
		return false
	}
}

// parseRetryAfter parses the Retry-After header from an HTTP response.
// It supports both delay-seconds (integer) and HTTP-date formats.
// Returns the delay duration, or 0 if the header is not present or invalid.
func parseRetryAfter(resp *http.Response) time.Duration {
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter == "" {
		return 0
	}

	// Try parsing as delay-seconds (integer)
	if seconds, err := strconv.ParseInt(retryAfter, 10, 64); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP-date
	if t, err := http.ParseTime(retryAfter); err == nil {
		delay := time.Until(t)
		if delay > 0 {
			return delay
		}
	}

	return 0
}

// calculateBackoff calculates the exponential backoff delay with jitter.
// Uses exponential backoff starting at 1 second, doubling each attempt up to
// a maximum of 60 seconds. Adds 0-10% random jitter to prevent thundering herd.
//
// Formula: min(1.0 * 2^attempt, 60.0) + random(0, delay * 0.1)
func calculateBackoff(attempt int) time.Duration {
	delay := baseBackoff * time.Duration(math.Pow(2, float64(attempt)))
	if delay > maxBackoff {
		delay = maxBackoff
	}
	jitter := time.Duration(rand.Float64() * float64(delay) * jitterFactor)
	return delay + jitter
}

// Do performs an HTTP request against an absolute API path. If body is non-nil it
// is JSON-encoded. The raw response body bytes are returned for 2xx responses
// (callers typically pass them through UnwrapEnvelope). DELETE that returns JSON
// (the Mixpanel convention, not 204) is handled like any other 2xx.
//
// Retry behavior:
// - Retries on 408, 429, 500, 502, 503, 504 status codes
// - Honors Retry-After header on 429, else uses jittered exponential backoff
// - Caps at 5 retry attempts (6 total requests including the initial attempt)
// - 4xx errors (except 408/429) are terminal and not retried
// - Network errors and timeouts are retried
func (c *Client) Do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var bodyBytes []byte
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		bodyBytes = buf
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Recreate request body reader for each attempt
		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
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
			// Network error - retry if we haven't exhausted attempts
			lastErr = err
			if attempt < maxRetries {
				delay := calculateBackoff(attempt)
				select {
				case <-time.After(delay):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return nil, err
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response body: %w", err)
		}

		// Success - return immediately
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return respBody, nil
		}

		// Check if this is a retryable error
		if isRetryableStatus(resp.StatusCode) && attempt < maxRetries {
			var delay time.Duration
			if resp.StatusCode == http.StatusTooManyRequests {
				// Honor Retry-After header on 429
				if retryAfter := parseRetryAfter(resp); retryAfter > 0 {
					delay = retryAfter
				} else {
					delay = calculateBackoff(attempt)
				}
			} else {
				// Use exponential backoff for 5xx errors
				delay = calculateBackoff(attempt)
			}

			lastErr = &APIError{
				Method:     method,
				Path:       path,
				StatusCode: resp.StatusCode,
				Body:       string(respBody),
			}

			select {
			case <-time.After(delay):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// Non-retryable error or exhausted retries - return error immediately
		return nil, &APIError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	// Exhausted retries
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, &APIError{
		Method:     method,
		Path:       path,
		StatusCode: 0,
		Body:       "exhausted retries",
	}
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
// Retry behavior is the same as Do (see Do documentation for details).
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

	formEncoded := form.Encode()
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.URL(path), strings.NewReader(formEncoded))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth(c.ServiceAccount, c.ServiceSecret)

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			// Network error - retry if we haven't exhausted attempts
			lastErr = err
			if attempt < maxRetries {
				delay := calculateBackoff(attempt)
				select {
				case <-time.After(delay):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return nil, err
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response body: %w", err)
		}

		// Success - return immediately
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return respBody, nil
		}

		// Check if this is a retryable error
		if isRetryableStatus(resp.StatusCode) && attempt < maxRetries {
			var delay time.Duration
			if resp.StatusCode == http.StatusTooManyRequests {
				// Honor Retry-After header on 429
				if retryAfter := parseRetryAfter(resp); retryAfter > 0 {
					delay = retryAfter
				} else {
					delay = calculateBackoff(attempt)
				}
			} else {
				// Use exponential backoff for 5xx errors
				delay = calculateBackoff(attempt)
			}

			lastErr = &APIError{
				Method:     method,
				Path:       path,
				StatusCode: resp.StatusCode,
				Body:       string(respBody),
			}

			select {
			case <-time.After(delay):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// Non-retryable error or exhausted retries
		return nil, &APIError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	// Exhausted retries
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, &APIError{
		Method:     method,
		Path:       path,
		StatusCode: 0,
		Body:       "exhausted retries",
	}
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

// baseOkResponse is the Mixpanel envelope. On success the server sends
// {"status": "ok", "results": <entity>}; on failure it sends
// {"status": "error", "error": <message>} (webapp app_api/response.py), where
// <message> is usually a string but can be a structured object (e.g. the
// agentic endpoints return {"code": ..., "message": ...}). Some endpoints emit
// the error envelope with HTTP 200, so the status field must be checked even
// on a 2xx response.
type baseOkResponse struct {
	Status  string          `json:"status"`
	Results json.RawMessage `json:"results"`
	Error   json.RawMessage `json:"error"`
}

// envelopeErrorMessage renders the envelope `error` payload for humans: a JSON
// string is unquoted, any other JSON value (object, array, number) is included
// verbatim, and an absent/empty payload yields "".
func envelopeErrorMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

// UnwrapEnvelope extracts the `results` field of a BaseOkResponseModel response.
// If the payload is not enveloped (no top-level `results`), the original bytes are
// returned unchanged so it also works for the few endpoints that return the entity
// at the root (e.g. SCIM, custom_event).
//
// A body that decodes as the envelope with status "error" is returned as an
// error carrying the server's error message: several App API endpoints send
// {"status":"error","error":...} with HTTP 200, which would otherwise be
// silently treated as a success payload.
func UnwrapEnvelope(respBody []byte) ([]byte, error) {
	var env baseOkResponse
	if err := json.Unmarshal(respBody, &env); err != nil {
		// not an object / not enveloped — hand the raw bytes back
		return respBody, nil
	}
	if env.Status == "error" {
		msg := envelopeErrorMessage(env.Error)
		if msg == "" {
			msg = strings.TrimSpace(string(respBody))
		}
		return nil, fmt.Errorf("mixpanel API returned an error envelope (status %q): %s", env.Status, msg)
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
	// Unwrap even when the caller discards the body (out == nil): an HTTP-200
	// error envelope ({"status":"error",...}) must still fail the call.
	inner, err := UnwrapEnvelope(respBody)
	if err != nil {
		// Add the endpoint context so an HTTP-200 error envelope is attributable.
		return fmt.Errorf("mixpanel API %s %s: %w", method, path, err)
	}
	if out == nil || len(inner) == 0 {
		return nil
	}
	if err := json.Unmarshal(inner, out); err != nil {
		return fmt.Errorf("decoding unwrapped response body: %w", err)
	}
	return nil
}
