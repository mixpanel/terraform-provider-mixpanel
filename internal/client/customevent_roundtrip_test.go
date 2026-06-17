package client

import (
	"math/big"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestCustomEventRoundTrip reproduces the "inconsistent result after apply"
// scenario for mixpanel_custom_event: the user sets top-level `name` and
// `alternatives`, but the API echoes them back only nested under `custom_event`.
// RawFromWireMerged must preserve the planned top-level values verbatim while
// filling the Computed-only nested `custom_event` object and identity from the
// response.
func TestCustomEventRoundTrip(t *testing.T) {
	nestedType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id":   tftypes.Number,
		"name": tftypes.String,
	}}
	schemaType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"name":           tftypes.String,
		"alternatives":   tftypes.String, // jsonencode passthrough (string in schema)
		"custom_event":   nestedType,     // Computed-only nested echo
		"customevent_id": tftypes.Number, // identity
		"project_id":     tftypes.Number,
	}}

	// Plan: user set name + alternatives; computed attrs unknown/null.
	altJSON := `[{"event":"Page View"}]`
	plan := tftypes.NewValue(schemaType, map[string]tftypes.Value{
		"name":           tftypes.NewValue(tftypes.String, "My CE"),
		"alternatives":   tftypes.NewValue(tftypes.String, altJSON),
		"custom_event":   tftypes.NewValue(nestedType, nil),
		"customevent_id": tftypes.NewValue(tftypes.Number, nil),
		"project_id":     tftypes.NewValue(tftypes.Number, nil),
	})

	// API response (unwrapped): {"custom_event": {"id": 2054084, "name": "My CE"}}
	wire := map[string]any{
		"custom_event": map[string]any{
			"id":   float64(2054084),
			"name": "My CE",
		},
	}
	extras := map[string]any{
		"customevent_id": "2054084",
		"project_id":     "1234567",
	}
	spec := AttrSpec{
		IDAttr:          "customevent_id",
		ProjectIDAttr:   "project_id",
		JSONEncodeAttrs: map[string]bool{"alternatives": true},
		// custom_event.to_json() uses snake_case keys; the generic wireKey would
		// camelCase the top-level "custom_event" key to "customEvent" and fail to
		// read the nested object back. The verbatim alias (mirrors the generated
		// CustomEventAttrSpec) forces the snake-case wire key.
		JSONEncodeWireKey: map[string]string{"custom_event": "custom_event"},
	}

	out, err := RawFromWireMerged(schemaType, plan, wire, extras, spec)
	if err != nil {
		t.Fatalf("RawFromWireMerged: %v", err)
	}
	got := map[string]tftypes.Value{}
	if err := out.As(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// name preserved from plan (NOT null).
	var name string
	if err := got["name"].As(&name); err != nil || name != "My CE" {
		t.Errorf("name = %q (err %v), want preserved %q", name, err, "My CE")
	}
	// alternatives preserved from plan (NOT null).
	var alts string
	if err := got["alternatives"].As(&alts); err != nil || alts != altJSON {
		t.Errorf("alternatives = %q (err %v), want preserved %q", alts, err, altJSON)
	}
	// identity filled from extras.
	var id big.Float
	if err := got["customevent_id"].As(&id); err != nil {
		t.Fatalf("customevent_id decode: %v", err)
	}
	if id.Text('f', -1) != "2054084" {
		t.Errorf("customevent_id = %s, want 2054084", id.Text('f', -1))
	}
	// nested custom_event filled from response.
	if got["custom_event"].IsNull() {
		t.Errorf("custom_event nested object is null, want filled from response")
	}
}
