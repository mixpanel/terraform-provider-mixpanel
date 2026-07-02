package client

import (
	"math/big"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestUnitTFFromNativeTypeMismatchErrors verifies that a wire value whose JSON
// type does not match the schema type is a hard error carrying the attribute
// name, not a silent null (which used to erase real server data from state
// with no signal).
func TestUnitTFFromNativeTypeMismatchErrors(t *testing.T) {
	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"enabled": tftypes.Bool,
		"count":   tftypes.Number,
		"tags":    tftypes.List{ElementType: tftypes.String},
		"config":  tftypes.Object{AttributeTypes: map[string]tftypes.Type{"k": tftypes.String}},
	}}
	spec := AttrSpec{}

	cases := []struct {
		name string
		wire map[string]any
		attr string
	}{
		{"bool gets string", map[string]any{"enabled": "yes"}, "enabled"},
		{"number gets bool", map[string]any{"count": true}, "count"},
		{"number gets non-numeric string", map[string]any{"count": "abc"}, "count"},
		{"list gets object", map[string]any{"tags": map[string]any{"a": "b"}}, "tags"},
		{"object gets array", map[string]any{"config": []any{1, 2}}, "config"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RawFromWire(objType, tc.wire, nil, spec)
			if err == nil {
				t.Fatalf("RawFromWire: expected a type-mismatch error for %v, got nil", tc.wire)
			}
			if !strings.Contains(err.Error(), tc.attr) {
				t.Errorf("error %q does not name attribute %q", err, tc.attr)
			}
		})
	}
}

// TestUnitTFFromNativeEmptyStringNumberExtraStaysNull verifies the deliberate
// exception: identity extras flow in as strings, and an EMPTY string on a
// number attribute means "absent" and stays null rather than erroring.
func TestUnitTFFromNativeEmptyStringNumberExtraStaysNull(t *testing.T) {
	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id": tftypes.Number,
	}}
	val, err := RawFromWire(objType, map[string]any{}, map[string]any{"id": ""}, AttrSpec{IDAttr: "id"})
	if err != nil {
		t.Fatalf("RawFromWire: %v", err)
	}
	attrs := map[string]tftypes.Value{}
	if err := val.As(&attrs); err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	if !attrs["id"].IsNull() {
		t.Errorf("id = %v, want null for an empty-string extra", attrs["id"])
	}
}

// TestUnitTFFromNativeInt64Preserved verifies the int64 branch (activated by
// normalizeNumbers keeping integers as int64) converts exactly: a value above
// 2^53 round-trips without float corruption, into both a Number attribute and
// a String id attribute.
func TestUnitTFFromNativeInt64Preserved(t *testing.T) {
	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"data_group_id": tftypes.Number,
		"id":            tftypes.String,
	}}
	const bigID = int64(9223372036854775001)
	wire := map[string]any{
		"data_group_id": bigID,
		"id":            bigID,
	}
	val, err := RawFromWire(objType, wire, nil, AttrSpec{})
	if err != nil {
		t.Fatalf("RawFromWire: %v", err)
	}
	attrs := map[string]tftypes.Value{}
	if err := val.As(&attrs); err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	var n big.Float
	if err := attrs["data_group_id"].As(&n); err != nil {
		t.Fatalf("data_group_id: %v", err)
	}
	if got := n.Text('f', -1); got != "9223372036854775001" {
		t.Errorf("data_group_id = %s, want 9223372036854775001 (float corruption)", got)
	}
	var s string
	if err := attrs["id"].As(&s); err != nil {
		t.Fatalf("id: %v", err)
	}
	if s != "9223372036854775001" {
		t.Errorf("id = %q, want %q", s, "9223372036854775001")
	}

	// IDFromWire (the id extraction path) must render it exactly too.
	if got, ok := IDFromWire(wire, "data_group_id"); !ok || got != "9223372036854775001" {
		t.Errorf("IDFromWire = %q, %v; want exact id", got, ok)
	}
}
