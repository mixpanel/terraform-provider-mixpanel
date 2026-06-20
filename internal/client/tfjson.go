// Package client: tfjson.go provides the generic, deterministic bridge between
// terraform-plugin-framework values (carried as tftypes.Value on Plan.Raw /
// State.Raw) and the plain JSON wire bodies the Mixpanel API expects.
//
// The conversion is fully type-driven (it walks the schema's tftypes.Type and
// the raw tftypes.Value recursively), so it works for ANY generated schema
// without per-attribute code. The per-entity resource/data-source files only
// supply small descriptors (which keys are synthetic, which are jsonencode
// passthroughs); all the heavy lifting lives here and is hand-written/audited.
package client

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// AttrSpec describes how the generic bridge should treat the top-level schema
// attributes of one entity. It is produced by the code generator per resource.
type AttrSpec struct {
	// IDAttr is the schema attribute that holds the server-assigned identity
	// (e.g. "id"). It is computed and excluded from create/update request bodies.
	IDAttr string
	// ProjectIDAttr is the schema attribute that carries the project id override
	// ("project_id" when present). It is path-only and excluded from the body.
	ProjectIDAttr string
	// PathParamAttrs are synthetic attributes that exist only to carry path
	// parameters (e.g. "agent_flow_id", "customevent_id"). Excluded from the body.
	PathParamAttrs map[string]bool
	// JSONEncodeAttrs are top-level attributes exposed in the schema as
	// types.String holding jsonencode() of the raw API value. On the way out
	// the string is parsed back into JSON; on the way in the raw value is
	// json.Marshal'd into a string.
	JSONEncodeAttrs map[string]bool
	// JSONStringAttrs are top-level attributes whose API wire representation is a
	// STRINGIFIED JSON value: the OpenAPI type is `string` with
	// `format: json-object`, so the server expects (and returns) a JSON string
	// like "{}" — not a JSON object. They are exposed in the schema as
	// types.String (the user typically writes jsonencode(...) which yields that
	// string). Unlike JSONEncodeAttrs, the string must be passed through verbatim
	// on BOTH directions: WireFromRaw must NOT json.Unmarshal it into an object
	// (doing so puts {} on the wire and the API rejects "{} is not of type
	// 'string'"), and the read path stores the wire string unchanged. An
	// attribute listed here is treated as a plain string passthrough and is never
	// also a JSONEncodeAttr.
	JSONStringAttrs map[string]bool
	// JSONEncodeWireKey maps a Terraform attribute name (snake_case, required by
	// Terraform's attribute naming rules) to the original API/wire JSON key for
	// jsonencode attributes whose API key is not snake_case (e.g. the TF attr
	// "composed_properties" maps to wire key "composedProperties"). Attributes
	// absent from this map use their schema name verbatim on the wire.
	JSONEncodeWireKey map[string]string
	// OutputOnlyAttrs are read-only (Computed-only) schema attributes that the
	// API never accepts in a create/update request body. They are populated by
	// the server in the response and resolve to known values at plan time
	// (especially Computed bools with a static default), so without this set
	// they would be serialized into the POST/PATCH body and rejected by APIs
	// that disallow additional properties. WireFromRaw strips them.
	OutputOnlyAttrs map[string]bool

	// SpreadAttrs are jsonencode attributes (also listed in JSONEncodeAttrs)
	// whose decoded JSON OBJECT is spread (merged) into the TOP LEVEL of the
	// request body rather than nested under the attribute's wire key. This models
	// a polymorphic create/update body that the HashiCorp generator cannot
	// express: the body is a discriminated `oneOf` of N variant schemas (e.g. a
	// warehouse source's bigquery/snowflake/redshift/databricks/postgres
	// connection config). We collapse all variant-specific fields into one
	// jsonencode string attribute (e.g. "params") and spread its contents back to
	// the body root on the wire, so a user writes the variant fields as
	// jsonencode({...}) and they land flat where the API expects them. The
	// decoded value MUST be a JSON object; a non-object spread value is an error.
	// Spread attributes are never echoed back verbatim by the read GET (the read
	// schema is the flat response), so they are preserved from prior plan/state by
	// the merge-base read path like any other jsonencode passthrough.
	SpreadAttrs map[string]bool
}

// wireKey returns the JSON wire key for a schema attribute name. An explicit
// JSONEncodeWireKey alias always wins; otherwise the attribute name is used
// verbatim. Terraform requires attribute names to be snake_case, and the great
// majority of Mixpanel API wire keys are already snake_case, so the verbatim
// name is correct by default. The generator records the exact wire key for
// every attribute whose API name is NOT its snake_case form (e.g. the TF attr
// "composed_properties" maps to wire key "composedProperties") in
// JSONEncodeWireKey, so genuinely camelCase fields are handled explicitly
// rather than by guessing. (A previous blanket snake->camel conversion here
// wrongly mangled snake_case API fields such as "sort_property" into
// "sortProperty", which APIs with additionalProperties:false then rejected.)
func (s AttrSpec) wireKey(name string) string {
	if s.JSONEncodeWireKey != nil {
		if wk, ok := s.JSONEncodeWireKey[name]; ok {
			return wk
		}
	}
	return name
}

// WireFromRaw converts a Plan/State raw object value into a plain Go map ready
// for JSON encoding as an UNENVELOPED request body. Synthetic attributes
// (id, project_id, path params) are dropped. jsonencode attributes are parsed
// from their string form back into structured JSON. Null and unknown values are
// omitted so we never send unknowns or clobber server-managed fields.
func WireFromRaw(raw tftypes.Value, spec AttrSpec) (map[string]any, error) {
	if raw.IsNull() || !raw.IsKnown() {
		return map[string]any{}, nil
	}
	obj := map[string]tftypes.Value{}
	if err := raw.As(&obj); err != nil {
		return nil, fmt.Errorf("decoding root object: %w", err)
	}
	out := make(map[string]any, len(obj))
	// Spread attrs are applied in a SECOND pass so top-level typed attributes always
	// win a wire-key collision, deterministically. Applying them inline during the
	// (randomly-ordered) map iteration meant a spread key (e.g. from `params`) and a
	// real top-level attr (e.g. warehouse_type) writing the same wire key produced a
	// run-to-run nondeterministic body. Collected keyed by attr name so multiple
	// spreads also resolve in a stable (sorted) order.
	spreads := map[string]map[string]any{}
	for name, v := range obj {
		if name == spec.IDAttr || name == spec.ProjectIDAttr || spec.PathParamAttrs[name] {
			continue
		}
		if spec.OutputOnlyAttrs[name] {
			continue
		}
		if v.IsNull() || !v.IsKnown() {
			continue
		}
		if spec.JSONStringAttrs[name] {
			// v is a string whose value is ALSO the wire value: the API field is
			// type:string format:json-object (a stringified JSON like "{}"). Pass
			// the string through verbatim — do NOT decode it into a JSON object.
			var s string
			if err := v.As(&s); err != nil {
				return nil, fmt.Errorf("decoding json-string attr %q: %w", name, err)
			}
			if s == "" {
				continue
			}
			out[spec.wireKey(name)] = s
			continue
		}
		if spec.JSONEncodeAttrs[name] {
			// v is a string holding jsonencode() of the real value.
			var s string
			if err := v.As(&s); err != nil {
				return nil, fmt.Errorf("decoding jsonencode attr %q: %w", name, err)
			}
			if s == "" {
				continue
			}
			var decoded any
			if err := json.Unmarshal([]byte(s), &decoded); err != nil {
				return nil, fmt.Errorf("parsing jsonencode attr %q: %w", name, err)
			}
			if spec.SpreadAttrs[name] {
				// Spread the decoded object into the body root (polymorphic
				// oneOf body collapsed to one jsonencode attr). Must be an object.
				// Defer the merge to the second pass (top-level attrs take
				// precedence on a key collision).
				obj, ok := decoded.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("spread attr %q must be a JSON object, got %T", name, decoded)
				}
				spreads[name] = obj
				continue
			}
			out[spec.wireKey(name)] = decoded
			continue
		}
		nv, err := nativeFromTF(v)
		if err != nil {
			return nil, fmt.Errorf("attr %q: %w", name, err)
		}
		out[spec.wireKey(name)] = nv
	}
	// Second pass: merge spread objects into the body root. A top-level typed attr
	// already in `out` wins (it is the authoritative, schema-typed value), so a
	// spread key that collides with one is dropped rather than racing it. Spread
	// attrs are applied in sorted name order so multiple spreads are deterministic.
	if len(spreads) > 0 {
		names := make([]string, 0, len(spreads))
		for name := range spreads {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			for k, val := range spreads[name] {
				if _, exists := out[k]; exists {
					continue
				}
				out[k] = val
			}
		}
	}
	return out, nil
}

// nativeFromTF recursively converts a tftypes.Value into a native Go value
// (map/slice/string/float64/bool/nil) suitable for json.Marshal.
func nativeFromTF(v tftypes.Value) (any, error) {
	if v.IsNull() || !v.IsKnown() {
		return nil, nil
	}
	t := v.Type()
	switch {
	case t.Is(tftypes.String):
		var s string
		if err := v.As(&s); err != nil {
			return nil, err
		}
		return s, nil
	case t.Is(tftypes.Bool):
		var b bool
		if err := v.As(&b); err != nil {
			return nil, err
		}
		return b, nil
	case t.Is(tftypes.Number):
		var n big.Float
		if err := v.As(&n); err != nil {
			return nil, err
		}
		f, _ := n.Float64()
		return f, nil
	case t.Is(tftypes.DynamicPseudoType):
		// Should not appear in framework-generated schemas, but be safe.
		var raw any
		if err := v.As(&raw); err != nil {
			return nil, err
		}
		return raw, nil
	}

	switch tt := t.(type) {
	case tftypes.Object:
		m := map[string]tftypes.Value{}
		if err := v.As(&m); err != nil {
			return nil, err
		}
		out := make(map[string]any, len(m))
		for k, ev := range m {
			if ev.IsNull() || !ev.IsKnown() {
				continue
			}
			nv, err := nativeFromTF(ev)
			if err != nil {
				return nil, err
			}
			out[k] = nv
		}
		return out, nil
	case tftypes.Map:
		m := map[string]tftypes.Value{}
		if err := v.As(&m); err != nil {
			return nil, err
		}
		out := make(map[string]any, len(m))
		for k, ev := range m {
			nv, err := nativeFromTF(ev)
			if err != nil {
				return nil, err
			}
			out[k] = nv
		}
		return out, nil
	case tftypes.List, tftypes.Set, tftypes.Tuple:
		var elems []tftypes.Value
		if err := v.As(&elems); err != nil {
			return nil, err
		}
		out := make([]any, 0, len(elems))
		for _, ev := range elems {
			nv, err := nativeFromTF(ev)
			if err != nil {
				return nil, err
			}
			out = append(out, nv)
		}
		return out, nil
	default:
		_ = tt
		return nil, fmt.Errorf("unsupported tftypes type %s", t.String())
	}
}

// RawFromWire builds a tftypes.Value conforming to schemaType from an API
// response map. project_id / path-param values come from extras (so the state
// retains the user-provided identity), jsonencode attributes are re-encoded back
// into strings, and any schema attribute absent from the response becomes null.
func RawFromWire(schemaType tftypes.Type, wire map[string]any, extras map[string]any, spec AttrSpec) (tftypes.Value, error) {
	obj, ok := schemaType.(tftypes.Object)
	if !ok {
		return tftypes.Value{}, fmt.Errorf("schema root is not an object: %s", schemaType.String())
	}
	vals := make(map[string]tftypes.Value, len(obj.AttributeTypes))
	for name, at := range obj.AttributeTypes {
		// Synthetic / identity attributes are sourced from extras first.
		if ev, present := extras[name]; present {
			tv, err := tfFromNative(at, ev, spec.JSONEncodeAttrs[name])
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("attr %q (extra): %w", name, err)
			}
			vals[name] = tv
			continue
		}
		raw, present := wire[spec.wireKey(name)]
		if !present || raw == nil {
			vals[name] = tftypes.NewValue(at, nil)
			continue
		}
		tv, err := tfFromNative(at, raw, spec.JSONEncodeAttrs[name])
		if err != nil {
			return tftypes.Value{}, fmt.Errorf("attr %q: %w", name, err)
		}
		vals[name] = tv
	}
	return tftypes.NewValue(obj, vals), nil
}

// RawFromWireMerged builds resource state after a create/update by overlaying the
// API response onto the planned value. For every top-level schema attribute:
//
//   - if the plan carries a KNOWN, non-null value, that value is preserved (this
//     keeps Terraform's "planned value must equal applied value" contract for
//     Required/Optional attributes, and for Optional+Computed attributes the user
//     set explicitly — even when the API does not echo the field back, or echoes
//     it back enriched with server-assigned sub-fields such as a subscription id);
//   - otherwise (plan value null or unknown, e.g. a Computed-only id or an unset
//     Optional+Computed attribute) the value is taken from the API response, with
//     identity / path-param values sourced from extras as in RawFromWire.
//
// Read has no plan to merge against and continues to use RawFromWire (passing a
// null base here degrades to exactly that behaviour).
func RawFromWireMerged(schemaType tftypes.Type, base tftypes.Value, wire map[string]any, extras map[string]any, spec AttrSpec) (tftypes.Value, error) {
	obj, ok := schemaType.(tftypes.Object)
	if !ok {
		return tftypes.Value{}, fmt.Errorf("schema root is not an object: %s", schemaType.String())
	}
	// Decode the planned object so we can read per-attribute plan values.
	planAttrs := map[string]tftypes.Value{}
	if !base.IsNull() && base.IsKnown() {
		if err := base.As(&planAttrs); err != nil {
			return tftypes.Value{}, fmt.Errorf("decoding plan object: %w", err)
		}
	}
	vals := make(map[string]tftypes.Value, len(obj.AttributeTypes))
	for name, at := range obj.AttributeTypes {
		// Identity / synthetic attributes are always sourced from extras when present.
		if ev, present := extras[name]; present {
			tv, err := tfFromNative(at, ev, spec.JSONEncodeAttrs[name])
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("attr %q (extra): %w", name, err)
			}
			vals[name] = tv
			continue
		}
		// Preserve a non-null planned value verbatim, but ONLY when it is fully
		// known (no nested unknowns). A container attribute (object/list) whose
		// leaves are Computed (e.g. ruleset.variants[].is_sticky / screenshot) is
		// IsKnown()==true at the top even while those leaves are still unknown
		// "(known after apply)"; preserving it verbatim would leave the unknowns
		// in state and trip Terraform's "invalid result object after apply" check.
		// Falling through rebuilds the attribute from the API response, which has
		// the server-resolved values.
		if pv, present := planAttrs[name]; present && !pv.IsNull() && fullyKnown(pv) {
			vals[name] = pv
			continue
		}
		// Otherwise fill from the API response (null when absent).
		raw, present := wire[spec.wireKey(name)]
		if !present || raw == nil {
			vals[name] = tftypes.NewValue(at, nil)
			continue
		}
		tv, err := tfFromNative(at, raw, spec.JSONEncodeAttrs[name])
		if err != nil {
			return tftypes.Value{}, fmt.Errorf("attr %q: %w", name, err)
		}
		vals[name] = tv
	}
	return tftypes.NewValue(obj, vals), nil
}

// fullyKnown reports whether v and ALL of its nested elements/attributes are
// known. tftypes.Value.IsKnown() is shallow: a known container can still hold
// unknown children. We need a deep check so the plan-preserve fast path in
// RawFromWireMerged does not carry "(known after apply)" leaves into final
// state (which Terraform rejects post-apply).
func fullyKnown(v tftypes.Value) bool {
	if !v.IsKnown() {
		return false
	}
	if v.IsNull() {
		return true
	}
	switch v.Type().(type) {
	case tftypes.Object, tftypes.Map:
		m := map[string]tftypes.Value{}
		if err := v.As(&m); err != nil {
			return false
		}
		for _, ev := range m {
			if !fullyKnown(ev) {
				return false
			}
		}
	case tftypes.List, tftypes.Set, tftypes.Tuple:
		var elems []tftypes.Value
		if err := v.As(&elems); err != nil {
			return false
		}
		for _, ev := range elems {
			if !fullyKnown(ev) {
				return false
			}
		}
	}
	return true
}

// tfFromNative converts a native Go JSON value into a tftypes.Value of type t.
// When jsonEncode is true, the native value is marshalled to a JSON string and
// stored as a tftypes string (the jsonencode passthrough representation).
func tfFromNative(t tftypes.Type, v any, jsonEncode bool) (tftypes.Value, error) {
	if jsonEncode {
		if v == nil {
			return tftypes.NewValue(t, nil), nil
		}
		b, err := json.Marshal(v)
		if err != nil {
			return tftypes.Value{}, err
		}
		return tftypes.NewValue(tftypes.String, string(b)), nil
	}
	if v == nil {
		return tftypes.NewValue(t, nil), nil
	}

	switch {
	case t.Is(tftypes.String):
		// Coerce non-strings defensively (some APIs return numeric-ish ids).
		switch s := v.(type) {
		case string:
			return tftypes.NewValue(tftypes.String, s), nil
		default:
			return tftypes.NewValue(tftypes.String, fmt.Sprintf("%v", v)), nil
		}
	case t.Is(tftypes.Bool):
		b, ok := v.(bool)
		if !ok {
			return tftypes.NewValue(t, nil), nil
		}
		return tftypes.NewValue(tftypes.Bool, b), nil
	case t.Is(tftypes.Number):
		switch n := v.(type) {
		case float64:
			return tftypes.NewValue(tftypes.Number, big.NewFloat(n)), nil
		case json.Number:
			f, _, err := big.ParseFloat(n.String(), 10, 512, big.ToNearestEven)
			if err != nil {
				return tftypes.Value{}, err
			}
			return tftypes.NewValue(tftypes.Number, f), nil
		case int:
			return tftypes.NewValue(tftypes.Number, big.NewFloat(float64(n))), nil
		case string:
			// Identity values flow in from extras as strings even when the schema
			// attribute is a Number (e.g. a numeric id rendered for the URL path).
			f, _, err := big.ParseFloat(n, 10, 512, big.ToNearestEven)
			if err != nil {
				return tftypes.NewValue(t, nil), nil
			}
			return tftypes.NewValue(tftypes.Number, f), nil
		default:
			return tftypes.NewValue(t, nil), nil
		}
	case t.Is(tftypes.DynamicPseudoType):
		return tftypes.NewValue(t, nil), nil
	}

	switch tt := t.(type) {
	case tftypes.Object:
		m, ok := v.(map[string]any)
		if !ok {
			return tftypes.NewValue(t, nil), nil
		}
		vals := make(map[string]tftypes.Value, len(tt.AttributeTypes))
		for name, at := range tt.AttributeTypes {
			ev, present := m[name]
			if !present || ev == nil {
				vals[name] = tftypes.NewValue(at, nil)
				continue
			}
			tv, err := tfFromNative(at, ev, false)
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("%q: %w", name, err)
			}
			vals[name] = tv
		}
		return tftypes.NewValue(tt, vals), nil
	case tftypes.Map:
		m, ok := v.(map[string]any)
		if !ok {
			return tftypes.NewValue(t, nil), nil
		}
		vals := make(map[string]tftypes.Value, len(m))
		for k, ev := range m {
			tv, err := tfFromNative(tt.ElementType, ev, false)
			if err != nil {
				return tftypes.Value{}, err
			}
			vals[k] = tv
		}
		return tftypes.NewValue(tt, vals), nil
	case tftypes.List:
		arr, ok := v.([]any)
		if !ok {
			return tftypes.NewValue(t, nil), nil
		}
		vals := make([]tftypes.Value, 0, len(arr))
		for _, ev := range arr {
			tv, err := tfFromNative(tt.ElementType, ev, false)
			if err != nil {
				return tftypes.Value{}, err
			}
			vals = append(vals, tv)
		}
		return tftypes.NewValue(tt, vals), nil
	case tftypes.Set:
		arr, ok := v.([]any)
		if !ok {
			return tftypes.NewValue(t, nil), nil
		}
		vals := make([]tftypes.Value, 0, len(arr))
		for _, ev := range arr {
			tv, err := tfFromNative(tt.ElementType, ev, false)
			if err != nil {
				return tftypes.Value{}, err
			}
			vals = append(vals, tv)
		}
		return tftypes.NewValue(tt, vals), nil
	case tftypes.Tuple:
		arr, ok := v.([]any)
		if !ok {
			return tftypes.NewValue(t, nil), nil
		}
		vals := make([]tftypes.Value, 0, len(tt.ElementTypes))
		for i, et := range tt.ElementTypes {
			if i >= len(arr) {
				vals = append(vals, tftypes.NewValue(et, nil))
				continue
			}
			tv, err := tfFromNative(et, arr[i], false)
			if err != nil {
				return tftypes.Value{}, err
			}
			vals = append(vals, tv)
		}
		return tftypes.NewValue(tt, vals), nil
	default:
		return tftypes.NewValue(t, nil), nil
	}
}

// IDFromWire extracts the identity attribute from an unwrapped response body and
// returns it as a string (numbers are rendered without scientific notation).
func IDFromWire(wire map[string]any, idAttr string) (string, bool) {
	v, ok := wire[idAttr]
	if !ok || v == nil {
		return "", false
	}
	switch x := v.(type) {
	case string:
		return x, true
	case float64:
		return new(big.Float).SetFloat64(x).Text('f', -1), true
	case json.Number:
		return x.String(), true
	default:
		return fmt.Sprintf("%v", x), true
	}
}
