package client

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestWireFromRawForUpdateDropsComputedEcho reproduces the dominant full-body
// PATCH bug: Optional+Computed schemas put server-populated read-only fields
// (id, created, can_edit, ...) into state, the plan carries them (via
// UseStateForUnknown), and without filtering they are echoed back at the write
// endpoint — which Mixpanel's extra="forbid"/allowlist validators reject with
// 400. With UpdateWritableAttrs set, WireFromRawForUpdate must keep ONLY the
// allowlisted attributes.
func TestWireFromRawForUpdateDropsComputedEcho(t *testing.T) {
	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id":          tftypes.String,
		"project_id":  tftypes.Number,
		"name":        tftypes.String,
		"description": tftypes.String,
		// Read-only computed echoes: known values from prior state. They are
		// deliberately NOT in OutputOnlyAttrs here to prove the allowlist alone
		// drops them.
		"created":  tftypes.String,
		"can_edit": tftypes.Bool,
		// jsonencode containers: "groups" is writable (allowlisted), "metadata"
		// is a computed echo (not allowlisted) and must be dropped too.
		"groups":   tftypes.String,
		"metadata": tftypes.String,
	}}
	raw := tftypes.NewValue(objType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "123"),
		"project_id":  tftypes.NewValue(tftypes.Number, 3),
		"name":        tftypes.NewValue(tftypes.String, "renamed"),
		"description": tftypes.NewValue(tftypes.String, "desc"),
		"created":     tftypes.NewValue(tftypes.String, "2026-01-01T00:00:00"),
		"can_edit":    tftypes.NewValue(tftypes.Bool, true),
		"groups":      tftypes.NewValue(tftypes.String, `[{"event":"x"}]`),
		"metadata":    tftypes.NewValue(tftypes.String, `{"server":"junk"}`),
	})
	spec := AttrSpec{
		IDAttr:        "id",
		ProjectIDAttr: "project_id",
		JSONEncodeAttrs: map[string]bool{
			"groups":   true,
			"metadata": true,
		},
		UpdateWritableAttrs: map[string]bool{
			"name":        true,
			"description": true,
			"groups":      true,
		},
	}

	body, err := WireFromRawForUpdate(raw, spec)
	if err != nil {
		t.Fatalf("WireFromRawForUpdate: %v", err)
	}
	for _, k := range []string{"created", "can_edit", "metadata", "id", "project_id"} {
		if _, ok := body[k]; ok {
			t.Errorf("update body must not echo read-only field %q, got %v", k, body[k])
		}
	}
	for _, k := range []string{"name", "description", "groups"} {
		if _, ok := body[k]; !ok {
			t.Errorf("update body must keep writable field %q; body=%v", k, body)
		}
	}
	if len(body) != 3 {
		t.Errorf("update body must contain exactly the 3 allowlisted keys, got %v", body)
	}

	// The same raw through the CREATE path with a different allowlist must
	// apply the create set independently.
	spec.CreateWritableAttrs = map[string]bool{"name": true, "groups": true}
	cbody, err := WireFromRawForCreate(raw, spec)
	if err != nil {
		t.Fatalf("WireFromRawForCreate: %v", err)
	}
	if _, ok := cbody["description"]; ok {
		t.Errorf("create body must drop non-allowlisted field description, got %v", cbody)
	}
	if len(cbody) != 2 {
		t.Errorf("create body must contain exactly name+groups, got %v", cbody)
	}
}

// TestWireFromRawForUpdateEmptyAllowlistBypasses pins the compatibility
// contract: a nil/empty allowlist disables filtering entirely (entities whose
// write surface is opaque, or not yet audited, keep full-body behavior).
func TestWireFromRawForUpdateEmptyAllowlistBypasses(t *testing.T) {
	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id":      tftypes.String,
		"name":    tftypes.String,
		"created": tftypes.String,
	}}
	raw := tftypes.NewValue(objType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "1"),
		"name":    tftypes.NewValue(tftypes.String, "n"),
		"created": tftypes.NewValue(tftypes.String, "c"),
	})
	spec := AttrSpec{IDAttr: "id"}
	body, err := WireFromRawForUpdate(raw, spec)
	if err != nil {
		t.Fatalf("WireFromRawForUpdate: %v", err)
	}
	if _, ok := body["created"]; !ok {
		t.Errorf("empty allowlist must bypass filtering (full body preserved), got %v", body)
	}
}

// TestWireFromRawForUpdateSpreadBypasses pins the spread-entity guard: an
// entity with SpreadAttrs must NEVER be filtered, because the body root
// carries variant keys flattened out of a jsonencode attr that a TF-attr-keyed
// allowlist cannot describe.
func TestWireFromRawForUpdateSpreadBypasses(t *testing.T) {
	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id":     tftypes.String,
		"params": tftypes.String,
	}}
	raw := tftypes.NewValue(objType, map[string]tftypes.Value{
		"id":     tftypes.NewValue(tftypes.String, "1"),
		"params": tftypes.NewValue(tftypes.String, `{"account_name":"acct","role":"r"}`),
	})
	spec := AttrSpec{
		IDAttr:          "id",
		JSONEncodeAttrs: map[string]bool{"params": true},
		SpreadAttrs:     map[string]bool{"params": true},
		// Deliberately hostile allowlist: must be ignored because of SpreadAttrs.
		UpdateWritableAttrs: map[string]bool{"something_else": true},
	}
	body, err := WireFromRawForUpdate(raw, spec)
	if err != nil {
		t.Fatalf("WireFromRawForUpdate: %v", err)
	}
	if body["account_name"] != "acct" || body["role"] != "r" {
		t.Errorf("spread entity must bypass filtering; body=%v", body)
	}
}
