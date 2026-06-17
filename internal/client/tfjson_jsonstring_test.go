package client

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestWireFromRawJSONStringPassthrough reproduces the bookmark Create 400 bug:
// the bookmark API field `params` (and `metadata`) is type:string
// format:json-object, i.e. a STRINGIFIED JSON value. The user writes
// params = jsonencode({}) which yields the string "{}". Treating it as a
// JSONEncodeAttr json.Unmarshal'd that string back into an object and put
// params:{} on the wire, so the API rejected it with
// {"error":"params: {} is not of type 'string'"}. As a JSONStringAttr the string
// must pass through verbatim, so the wire body must contain params:"{}" (a JSON
// string), not params:{} (a JSON object).
func TestWireFromRawJSONStringPassthrough(t *testing.T) {
	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id":          tftypes.Number,
		"project_id":  tftypes.Number,
		"bookmark_id": tftypes.String,
		"name":        tftypes.String,
		"type":        tftypes.String,
		"params":      tftypes.String,
		"metadata":    tftypes.String,
	}}

	raw := tftypes.NewValue(objType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.Number, nil),
		"project_id":  tftypes.NewValue(tftypes.Number, nil),
		"bookmark_id": tftypes.NewValue(tftypes.String, nil),
		"name":        tftypes.NewValue(tftypes.String, "tf live test"),
		"type":        tftypes.NewValue(tftypes.String, "segmentation"),
		"params":      tftypes.NewValue(tftypes.String, "{}"),
		"metadata":    tftypes.NewValue(tftypes.String, `{"a":1}`),
	})

	spec := AttrSpec{
		IDAttr:          "id",
		ProjectIDAttr:   "project_id",
		PathParamAttrs:  map[string]bool{"bookmark_id": true},
		JSONStringAttrs: map[string]bool{"params": true, "metadata": true},
	}

	body, err := WireFromRaw(raw, spec)
	if err != nil {
		t.Fatalf("WireFromRaw: %v", err)
	}

	// params/metadata must remain JSON STRINGS on the wire, not objects.
	if got, ok := body["params"].(string); !ok || got != "{}" {
		t.Errorf("params = %#v (type %T), want string \"{}\"", body["params"], body["params"])
	}
	if got, ok := body["metadata"].(string); !ok || got != `{"a":1}` {
		t.Errorf("metadata = %#v (type %T), want string %q", body["metadata"], body["metadata"], `{"a":1}`)
	}

	// Marshal the whole body the way client.Do would, and confirm the on-wire
	// JSON has params as a string literal, not an object.
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	wire := string(b)
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if string(decoded["params"]) != `"{}"` {
		t.Errorf("on-wire params = %s, want the JSON string \"{}\"; full body=%s", decoded["params"], wire)
	}

	// Scalars still pass through.
	if body["name"] != "tf live test" || body["type"] != "segmentation" {
		t.Errorf("scalar attrs wrong: name=%v type=%v", body["name"], body["type"])
	}
}

// TestRawFromWireJSONStringReadback confirms the read path keeps params/metadata
// as plain strings (they are NOT in JSONEncodeAttrs, so no re-marshalling), which
// makes plan==state for a value the server echoes back verbatim.
func TestRawFromWireJSONStringReadback(t *testing.T) {
	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id":         tftypes.Number,
		"project_id": tftypes.Number,
		"name":       tftypes.String,
		"params":     tftypes.String,
		"metadata":   tftypes.String,
	}}
	spec := AttrSpec{
		IDAttr:          "id",
		ProjectIDAttr:   "project_id",
		JSONStringAttrs: map[string]bool{"params": true, "metadata": true},
	}
	wire := map[string]any{
		"name":     "tf live test",
		"params":   "{}",
		"metadata": `{"a":1}`,
	}
	extras := map[string]any{"id": "90820149", "project_id": "1234567"}

	val, err := RawFromWire(objType, wire, extras, spec)
	if err != nil {
		t.Fatalf("RawFromWire: %v", err)
	}
	m := map[string]tftypes.Value{}
	if err := val.As(&m); err != nil {
		t.Fatalf("decode state object: %v", err)
	}
	var params string
	if err := m["params"].As(&params); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	if params != "{}" {
		t.Errorf("state params = %q, want %q", params, "{}")
	}
	var metadata string
	if err := m["metadata"].As(&metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadata != `{"a":1}` {
		t.Errorf("state metadata = %q, want %q", metadata, `{"a":1}`)
	}
}
