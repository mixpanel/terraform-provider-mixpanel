package client

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestWireFromRawStripsOutputOnly reproduces the playlist Create 400 bug: the
// generated schema marks server/permission booleans (allow_staff_override,
// can_pin, ...) as Computed-only with booldefault.StaticBool(false), so they
// resolve to a known `false` at plan time and were being serialized into the
// POST body. PlaylistApiPayload has additionalProperties:false, so the API
// rejected them. OutputOnlyAttrs must cause WireFromRaw to drop them while
// keeping the writable attributes (name, sort_property, description).
func TestWireFromRawStripsOutputOnly(t *testing.T) {
	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id":                     tftypes.String,
		"project_id":             tftypes.Number,
		"playlist_id":            tftypes.String,
		"name":                   tftypes.String,
		"description":            tftypes.String,
		"sort_property":          tftypes.String,
		"allow_staff_override":   tftypes.Bool,
		"can_pin":                tftypes.Bool,
		"can_share":              tftypes.Bool,
		"can_update_basic":       tftypes.Bool,
		"can_view":               tftypes.Bool,
		"is_shared_with_project": tftypes.Bool,
		"is_superadmin":          tftypes.Bool,
		"last_modified_by_email": tftypes.String,
	}}

	raw := tftypes.NewValue(objType, map[string]tftypes.Value{
		// identity + path param: always stripped
		"id":          tftypes.NewValue(tftypes.String, nil),
		"project_id":  tftypes.NewValue(tftypes.Number, nil),
		"playlist_id": tftypes.NewValue(tftypes.String, nil),
		// writable attributes: must survive
		"name":          tftypes.NewValue(tftypes.String, "tf live test"),
		"description":   tftypes.NewValue(tftypes.String, "tf live test"),
		"sort_property": tftypes.NewValue(tftypes.String, "recency"),
		// computed-only booleans (known false from booldefault): must be dropped
		"allow_staff_override":   tftypes.NewValue(tftypes.Bool, false),
		"can_pin":                tftypes.NewValue(tftypes.Bool, false),
		"can_share":              tftypes.NewValue(tftypes.Bool, false),
		"can_update_basic":       tftypes.NewValue(tftypes.Bool, false),
		"can_view":               tftypes.NewValue(tftypes.Bool, false),
		"is_shared_with_project": tftypes.NewValue(tftypes.Bool, false),
		"is_superadmin":          tftypes.NewValue(tftypes.Bool, false),
		"last_modified_by_email": tftypes.NewValue(tftypes.String, nil),
	})

	spec := AttrSpec{
		IDAttr:         "id",
		ProjectIDAttr:  "project_id",
		PathParamAttrs: map[string]bool{"playlist_id": true},
		OutputOnlyAttrs: map[string]bool{
			"allow_staff_override":   true,
			"can_pin":                true,
			"can_share":              true,
			"can_update_basic":       true,
			"can_view":               true,
			"is_shared_with_project": true,
			"is_superadmin":          true,
			"last_modified_by_email": true,
		},
	}

	body, err := WireFromRaw(raw, spec)
	if err != nil {
		t.Fatalf("WireFromRaw: %v", err)
	}

	forbidden := []string{
		"allow_staff_override", "can_pin", "can_share", "can_update_basic",
		"can_view", "is_shared_with_project", "is_superadmin",
		"last_modified_by_email", "id", "project_id", "playlist_id",
	}
	for _, k := range forbidden {
		if _, ok := body[k]; ok {
			t.Errorf("output-only/synthetic attr %q leaked into request body: %#v", k, body)
		}
	}

	want := map[string]string{
		"name":          "tf live test",
		"description":   "tf live test",
		"sort_property": "recency",
	}
	for k, v := range want {
		got, ok := body[k]
		if !ok {
			t.Errorf("writable attr %q missing from request body", k)
			continue
		}
		if got != v {
			t.Errorf("attr %q = %v, want %v", k, got, v)
		}
	}
	if len(body) != len(want) {
		t.Errorf("body has %d keys, want exactly %d (%v)", len(body), len(want), body)
	}
}
