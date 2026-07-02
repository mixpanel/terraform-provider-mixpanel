package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestRetryOn429WithRetryAfter verifies that the client retries on 429 and
// honors the Retry-After header.
func TestRetryOn429WithRetryAfter(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ServiceAccount: "sa", ServiceSecret: "secret"})
	start := time.Now()
	_, err := c.Do(context.Background(), "GET", "/test", nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts (1 failure + 1 success), got %d", attempts)
	}
	// Should have waited at least 1 second (Retry-After value)
	if elapsed < 1*time.Second {
		t.Errorf("expected to wait at least 1s for Retry-After, waited %v", elapsed)
	}
}

// TestRetryOn500 verifies that the client retries on 500 Internal Server Error.
func TestRetryOn500(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"internal server error"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ServiceAccount: "sa", ServiceSecret: "secret"})
	_, err := c.Do(context.Background(), "GET", "/test", nil)

	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts (1 failure + 1 success), got %d", attempts)
	}
}

// TestRetryOn502 verifies retry on 502 Bad Gateway.
func TestRetryOn502(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"bad gateway"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ServiceAccount: "sa", ServiceSecret: "secret"})
	_, err := c.Do(context.Background(), "GET", "/test", nil)

	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts (1 failure + 1 success), got %d", attempts)
	}
}

// TestRetryOn503 verifies retry on 503 Service Unavailable.
func TestRetryOn503(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"service unavailable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ServiceAccount: "sa", ServiceSecret: "secret"})
	_, err := c.Do(context.Background(), "GET", "/test", nil)

	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts (1 failure + 1 success), got %d", attempts)
	}
}

// TestRetryOn504 verifies retry on 504 Gateway Timeout.
func TestRetryOn504(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusGatewayTimeout)
			_, _ = w.Write([]byte(`{"error":"gateway timeout"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ServiceAccount: "sa", ServiceSecret: "secret"})
	_, err := c.Do(context.Background(), "GET", "/test", nil)

	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts (1 failure + 1 success), got %d", attempts)
	}
}

// TestNoRetryOn400 verifies that 400 Bad Request is NOT retried.
func TestNoRetryOn400(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ServiceAccount: "sa", ServiceSecret: "secret"})
	_, err := c.Do(context.Background(), "GET", "/test", nil)

	if err == nil {
		t.Fatal("expected error for 400, got nil")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry on 400), got %d", attempts)
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", apiErr.StatusCode)
	}
}

// TestMaxRetries verifies that retries are capped at maxRetries.
func TestMaxRetries(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"always fails"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ServiceAccount: "sa", ServiceSecret: "secret"})
	_, err := c.Do(context.Background(), "GET", "/test", nil)

	if err == nil {
		t.Fatal("expected error after exhausting retries, got nil")
	}
	// maxRetries + 1 = 6 total attempts
	expectedAttempts := maxRetries + 1
	if attempts != expectedAttempts {
		t.Errorf("expected %d attempts (1 initial + %d retries), got %d", expectedAttempts, maxRetries, attempts)
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", apiErr.StatusCode)
	}
}

// TestParseRetryAfterSeconds verifies parsing of Retry-After as delay-seconds.
func TestParseRetryAfterSeconds(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"Retry-After": []string{"60"},
		},
	}
	delay := parseRetryAfter(resp)
	if delay != 60*time.Second {
		t.Errorf("expected 60s, got %v", delay)
	}
}

// TestParseRetryAfterHTTPDate verifies parsing of Retry-After as HTTP-date.
func TestParseRetryAfterHTTPDate(t *testing.T) {
	future := time.Now().Add(5 * time.Second)
	resp := &http.Response{
		Header: http.Header{
			"Retry-After": []string{future.Format(http.TimeFormat)},
		},
	}
	delay := parseRetryAfter(resp)
	// Should be close to 5s (allow some variance for test execution time)
	if delay < 4*time.Second || delay > 6*time.Second {
		t.Errorf("expected ~5s, got %v", delay)
	}
}

// TestParseRetryAfterMissing verifies that missing Retry-After returns 0.
func TestParseRetryAfterMissing(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{},
	}
	delay := parseRetryAfter(resp)
	if delay != 0 {
		t.Errorf("expected 0, got %v", delay)
	}
}

// TestCalculateBackoff verifies exponential backoff calculation.
func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		attempt     int
		minExpected time.Duration
		maxExpected time.Duration
	}{
		{0, 1 * time.Second, 2 * time.Second},        // 1s + jitter
		{1, 2 * time.Second, 3 * time.Second},        // 2s + jitter
		{2, 4 * time.Second, 5 * time.Second},        // 4s + jitter
		{3, 8 * time.Second, 9 * time.Second},        // 8s + jitter
		{10, 60 * time.Second, 67 * time.Second},     // capped at 60s + jitter
	}

	for _, tt := range tests {
		t.Run("attempt_"+strconv.Itoa(tt.attempt), func(t *testing.T) {
			delay := calculateBackoff(tt.attempt)
			if delay < tt.minExpected || delay > tt.maxExpected {
				t.Errorf("attempt %d: expected delay in [%v, %v], got %v",
					tt.attempt, tt.minExpected, tt.maxExpected, delay)
			}
		})
	}
}

// TestIsRetryableStatus verifies the retry status code logic.
func TestIsRetryableStatus(t *testing.T) {
	tests := []struct {
		status     int
		retryable  bool
	}{
		{200, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{408, true},  // Request Timeout
		{429, true},  // Too Many Requests
		{500, true},  // Internal Server Error
		{502, true},  // Bad Gateway
		{503, true},  // Service Unavailable
		{504, true},  // Gateway Timeout
	}

	for _, tt := range tests {
		t.Run("status_"+strconv.Itoa(tt.status), func(t *testing.T) {
			got := isRetryableStatus(tt.status)
			if got != tt.retryable {
				t.Errorf("status %d: expected retryable=%v, got %v",
					tt.status, tt.retryable, got)
			}
		})
	}
}

// TestDoFormRetry verifies that DoForm also retries on retryable errors.
func TestDoFormRetry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"service unavailable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ServiceAccount: "sa", ServiceSecret: "secret"})
	_, err := c.DoForm(context.Background(), "POST", "/test", map[string]any{"key": "value"})

	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts (1 failure + 1 success), got %d", attempts)
	}
}

// TestContextCancellation verifies that context cancellation stops retries.
func TestContextCancellation(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		// Always fail to force retries
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after first attempt to interrupt retries
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	c := New(Config{BaseURL: srv.URL, ServiceAccount: "sa", ServiceSecret: "secret"})
	_, err := c.Do(ctx, "GET", "/test", nil)

	if err == nil {
		t.Fatal("expected error due to context cancellation, got nil")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context canceled error, got: %v", err)
	}
	// Should have stopped early due to context cancellation
	if attempts > 2 {
		t.Errorf("expected at most 2 attempts before cancellation, got %d", attempts)
	}
}
