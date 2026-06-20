package client

import "testing"

import "github.com/hashicorp/terraform-plugin-go/tftypes"

// TestWireFromRawSpreadTopLevelPrecedence pins the warehouse_source collision fix:
// a SpreadAttr (params) whose decoded object carries a key that is ALSO a top-level
// typed attribute (warehouse_type) must NOT clobber the top-level value, and the
// result must be deterministic regardless of Go's randomized map iteration order.
// Previously the spread merged inline during the (random-order) root iteration, so
// whichever of `params.warehouse_type` / the real `warehouse_type` attr was written
// last won — a run-to-run flaky connector type.
func TestWireFromRawSpreadTopLevelPrecedence(t *testing.T) {
	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id":             tftypes.Number,
		"project_id":     tftypes.Number,
		"source_id":      tftypes.String,
		"source_name":    tftypes.String,
		"warehouse_type": tftypes.String,
		"params":         tftypes.String,
	}}

	// params intentionally carries a colliding "warehouse_type" key plus a unique
	// connection field that must be spread onto the body root.
	raw := tftypes.NewValue(objType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.Number, nil),
		"project_id":     tftypes.NewValue(tftypes.Number, nil),
		"source_id":      tftypes.NewValue(tftypes.String, nil),
		"source_name":    tftypes.NewValue(tftypes.String, "my-source"),
		"warehouse_type": tftypes.NewValue(tftypes.String, "snowflake"),
		"params":         tftypes.NewValue(tftypes.String, `{"warehouse_type":"bigquery","account":"acme"}`),
	})

	spec := AttrSpec{
		IDAttr:          "id",
		ProjectIDAttr:   "project_id",
		PathParamAttrs:  map[string]bool{"source_id": true},
		JSONEncodeAttrs: map[string]bool{"params": true},
		SpreadAttrs:     map[string]bool{"params": true},
	}

	// Run many times: any nondeterminism in spread/top-level precedence surfaces as
	// an occasional wrong value across iterations.
	for i := 0; i < 100; i++ {
		body, err := WireFromRaw(raw, spec)
		if err != nil {
			t.Fatalf("WireFromRaw: %v", err)
		}
		// Top-level typed attr wins the collision, every time.
		if got := body["warehouse_type"]; got != "snowflake" {
			t.Fatalf("iter %d: warehouse_type = %#v, want top-level \"snowflake\" (spread must not clobber it)", i, got)
		}
		// Non-colliding spread keys still land on the body root.
		if got := body["account"]; got != "acme" {
			t.Fatalf("iter %d: account = %#v, want spread value \"acme\"", i, got)
		}
		// params itself is consumed by the spread, not emitted as a wire key.
		if _, present := body["params"]; present {
			t.Fatalf("iter %d: params should be spread, not emitted as a key: %#v", i, body["params"])
		}
		if got := body["source_name"]; got != "my-source" {
			t.Fatalf("iter %d: source_name = %#v, want \"my-source\"", i, got)
		}
	}
}
