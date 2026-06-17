// Package client: workspace.go resolves the canonical workspace for a project.
//
// Several entity CRUD routes (e.g. feature flags) only function on their
// workspace-bearing variant (/api/app/projects/{project_id}/workspaces/
// {workspace_id}/...). The project-only variant is gated by a
// require_workspace_is_set decorator on the backend and returns
// HTTP 400 {"error":"Workspace is required"}. When the practitioner does not pin
// a workspace_id, the provider targets the project's canonical workspace: the
// single is_global=true "All Project Data" workspace, falling back to the
// is_default workspace, then the first workspace returned.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
)

// workspaceListEntry is the subset of the workspace list payload we rely on.
// The list endpoint returns BaseOkResponseModel wrapping a list of workspace
// dicts (see webapp .../workspaces/__types__/workspace_list_resp.schema.json).
type workspaceListEntry struct {
	ID        json.Number `json:"id"`
	IsGlobal  bool        `json:"is_global"`
	IsDefault bool        `json:"is_default"`
}

// DefaultWorkspaceID returns the canonical workspace id for a project as a
// string suitable for templating into a URL path. Selection order: the global
// workspace, then the default workspace, then the first entry. An error is
// returned only on transport / decode failure or when the project has no
// workspaces.
func (c *Client) DefaultWorkspaceID(ctx context.Context, projectID string) (string, error) {
	path := fmt.Sprintf("/api/app/projects/%s/workspaces", projectID)
	respBody, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}
	inner, err := UnwrapEnvelope(respBody)
	if err != nil {
		return "", err
	}
	var list []workspaceListEntry
	if err := json.Unmarshal(inner, &list); err != nil {
		return "", fmt.Errorf("decoding workspace list: %w", err)
	}
	if len(list) == 0 {
		return "", fmt.Errorf("project %s has no workspaces", projectID)
	}
	var global, def, first string
	for i, w := range list {
		id := normalizeWorkspaceID(w.ID)
		if id == "" {
			continue
		}
		if i == 0 || first == "" {
			first = id
		}
		if w.IsGlobal && global == "" {
			global = id
		}
		if w.IsDefault && def == "" {
			def = id
		}
	}
	switch {
	case global != "":
		return global, nil
	case def != "":
		return def, nil
	default:
		return first, nil
	}
}

// normalizeWorkspaceID renders a JSON number id without scientific notation.
func normalizeWorkspaceID(n json.Number) string {
	if n == "" {
		return ""
	}
	if f, _, err := big.ParseFloat(n.String(), 10, 64, big.ToNearestEven); err == nil {
		return f.Text('f', -1)
	}
	return n.String()
}
