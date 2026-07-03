// Unit tests for the read-vs-apply merge semantics of RawFromWireMerged
// (drift detection architecture): MergeApply preserves fully-known planned
// values (except jsonencode attrs the wire carries), MergeRead prefers the
// wire wherever it carries a field and falls back to prior state only for
// echo gaps and unconvertible shapes.
package client

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func mergeModeSchema() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"id":          tftypes.String,
			"name":        tftypes.String,
			"description": tftypes.String,
			"groups":      tftypes.String, // jsonencode passthrough
			"secret":      tftypes.String, // never echoed by the API
		},
	}
}

func mergeModeSpec() AttrSpec {
	return AttrSpec{
		IDAttr:          "id",
		JSONEncodeAttrs: map[string]bool{"groups": true},
	}
}

func mergeModeBase(name, description, groups, secret string) tftypes.Value {
	obj := mergeModeSchema()
	return tftypes.NewValue(obj, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "42"),
		"name":        tftypes.NewValue(tftypes.String, name),
		"description": tftypes.NewValue(tftypes.String, description),
		"groups":      tftypes.NewValue(tftypes.String, groups),
		"secret":      tftypes.NewValue(tftypes.String, secret),
	})
}

func attrString(t *testing.T, v tftypes.Value, name string) (string, bool) {
	t.Helper()
	m := map[string]tftypes.Value{}
	if err := v.As(&m); err != nil {
		t.Fatalf("decoding result object: %v", err)
	}
	av, ok := m[name]
	if !ok || av.IsNull() {
		return "", false
	}
	var s string
	if err := av.As(&s); err != nil {
		t.Fatalf("attr %q not a string: %v", name, err)
	}
	return s, true
}

// MergeRead: the wire wins for every attribute it carries (typed AND
// jsonencode), prior state survives only where the wire omits the field.
func TestRawFromWireMerged_ReadPrefersWire(t *testing.T) {
	base := mergeModeBase("config name", "config desc", `["a"]`, "s3cret")
	wire := map[string]any{
		"name":   "renamed in webapp",
		"groups": []any{"a", "b"},
		// description and secret omitted by the GET.
	}
	extras := map[string]any{"id": "42"}
	out, err := RawFromWireMerged(mergeModeSchema(), MergeRead, base, wire, extras, mergeModeSpec())
	if err != nil {
		t.Fatalf("RawFromWireMerged: %v", err)
	}
	if got, _ := attrString(t, out, "name"); got != "renamed in webapp" {
		t.Errorf("name: wire must win on read, got %q", got)
	}
	if got, _ := attrString(t, out, "groups"); got != `["a","b"]` {
		t.Errorf("groups: wire must win on read, got %q", got)
	}
	if got, _ := attrString(t, out, "description"); got != "config desc" {
		t.Errorf("description: prior state must survive a wire omission, got %q", got)
	}
	if got, _ := attrString(t, out, "secret"); got != "s3cret" {
		t.Errorf("secret: write-only field must survive a wire omission, got %q", got)
	}
}

// MergeRead: a JSON null on the wire is treated as an omission (several
// endpoints return null for fields they accept but do not store), so the
// prior value is preserved rather than clobbered.
func TestRawFromWireMerged_ReadWireNullKeepsPrior(t *testing.T) {
	base := mergeModeBase("n", "d", `{}`, "s")
	wire := map[string]any{"description": nil}
	out, err := RawFromWireMerged(mergeModeSchema(), MergeRead, base, wire, map[string]any{"id": "42"}, mergeModeSpec())
	if err != nil {
		t.Fatalf("RawFromWireMerged: %v", err)
	}
	if got, _ := attrString(t, out, "description"); got != "d" {
		t.Errorf("description: wire null must keep prior, got %q", got)
	}
}

// MergeRead: a wire shape the schema cannot hold falls back to the prior
// value for that attribute instead of failing the whole refresh.
func TestRawFromWireMerged_ReadUnconvertibleKeepsPrior(t *testing.T) {
	base := mergeModeBase("n", "d", `{}`, "s")
	// name echoed as an object; the schema has a plain bool-incompatible type
	// only for non-string targets, so use a bool attribute shape mismatch via
	// description (string target coerces), and check with a genuinely
	// incompatible target: add a bool attr.
	schema := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id":     tftypes.String,
		"active": tftypes.Bool,
	}}
	prior := tftypes.NewValue(schema, map[string]tftypes.Value{
		"id":     tftypes.NewValue(tftypes.String, "42"),
		"active": tftypes.NewValue(tftypes.Bool, true),
	})
	wire := map[string]any{"active": "yes"} // string where a bool is expected
	out, err := RawFromWireMerged(schema, MergeRead, prior, wire, map[string]any{"id": "42"}, AttrSpec{IDAttr: "id"})
	if err != nil {
		t.Fatalf("RawFromWireMerged must not fail on an unconvertible wire shape in read mode: %v", err)
	}
	m := map[string]tftypes.Value{}
	if err := out.As(&m); err != nil {
		t.Fatal(err)
	}
	var b bool
	if err := m["active"].As(&b); err != nil || !b {
		t.Errorf("active: unconvertible wire shape must keep prior value true, got %v (err %v)", m["active"], err)
	}
	// In apply mode the same mismatch must still be a loud error.
	if _, err := RawFromWireMerged(schema, MergeApply, tftypes.NewValue(schema, nil), wire, map[string]any{"id": "42"}, AttrSpec{IDAttr: "id"}); err == nil {
		t.Error("MergeApply with a null base must surface the conversion error")
	}
	_ = base
}

// MergeApply: fully-known planned values are preserved verbatim even when the
// wire carries a different value (planned-value contract), while jsonencode
// attributes are refreshed from the wire when it carries the field and
// preserved when it omits it (the 5787cbd behavior).
func TestRawFromWireMerged_ApplyPrefersPlan(t *testing.T) {
	plan := mergeModeBase("planned name", "planned desc", `["a"]`, "s3cret")
	wire := map[string]any{
		"name":   "server-side name",
		"groups": []any{"a", "b"}, // server enriched the JSON
	}
	extras := map[string]any{"id": "42"}
	out, err := RawFromWireMerged(mergeModeSchema(), MergeApply, plan, wire, extras, mergeModeSpec())
	if err != nil {
		t.Fatalf("RawFromWireMerged: %v", err)
	}
	if got, _ := attrString(t, out, "name"); got != "planned name" {
		t.Errorf("name: plan must win on apply, got %q", got)
	}
	if got, _ := attrString(t, out, "groups"); got != `["a","b"]` {
		t.Errorf("groups: wire must win for a jsonencode attr the response carries, got %q", got)
	}
	if got, _ := attrString(t, out, "secret"); got != "s3cret" {
		t.Errorf("secret: plan must be preserved when the wire omits the field, got %q", got)
	}
}
