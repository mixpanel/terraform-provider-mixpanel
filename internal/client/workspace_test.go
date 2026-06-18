package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDefaultWorkspaceID verifies the canonical-workspace selection order
// (global > default > first) and that the correct project-scoped list URL is
// requested. This is the resolution the feature_flag resource relies on to build
// the workspace-bearing CRUD path that the backend's require_workspace_is_set
// decorator demands.
func TestDefaultWorkspaceID(t *testing.T) {
	cases := []struct {
		name    string
		results string
		want    string
	}{
		{
			name:    "global wins over default and first",
			results: `[{"id":111,"is_global":false,"is_default":false},{"id":222,"is_global":false,"is_default":true},{"id":4531560,"is_global":true,"is_default":false}]`,
			want:    "4531560",
		},
		{
			name:    "default when no global",
			results: `[{"id":111,"is_global":false,"is_default":false},{"id":222,"is_global":false,"is_default":true}]`,
			want:    "222",
		},
		{
			name:    "first when neither global nor default",
			results: `[{"id":777,"is_global":false,"is_default":false},{"id":888,"is_global":false,"is_default":false}]`,
			want:    "777",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"status":"ok","results":` + tc.results + `}`))
			}))
			defer srv.Close()

			c := New(Config{BaseURL: srv.URL, ServiceAccount: "u", ServiceSecret: "p"})
			got, err := c.DefaultWorkspaceID(context.Background(), "1234567")
			if err != nil {
				t.Fatalf("DefaultWorkspaceID: %v", err)
			}
			if got != tc.want {
				t.Fatalf("workspace id = %q, want %q", got, tc.want)
			}
			if want := "/api/app/projects/1234567/workspaces"; gotPath != want {
				t.Fatalf("requested path = %q, want %q", gotPath, want)
			}
		})
	}
}

// TestDefaultWorkspaceIDEmpty surfaces an error when a project reports no
// workspaces, so the feature_flag path is never built with a blank
// {workspace_id} segment (which the API would reject ambiguously).
func TestDefaultWorkspaceIDEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","results":[]}`))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, ServiceAccount: "u", ServiceSecret: "p"})
	if _, err := c.DefaultWorkspaceID(context.Background(), "1"); err == nil {
		t.Fatal("expected error for project with no workspaces, got nil")
	}
}
