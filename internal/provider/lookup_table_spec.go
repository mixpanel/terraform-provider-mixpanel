// MANUALLY MAINTAINED — DO NOT REGENERATE (not driven by gen/crudgen.py).

package provider

import "github.com/mixpanel/terraform-provider-mixpanel/internal/client"

// LookupTableAttrSpec describes the lookup_table entity to the generic
// tftypes<->JSON bridge, mirroring the per-entity AttrSpec convention of the
// generated resources. The hand-written lookup_table resource uses a typed
// model rather than the bridge (its CRUD is the signed-URL upload handshake,
// not plain JSON round-trips), so this spec is documentation of the entity's
// identity/scope attributes for tooling parity: `id` is the dimension
// data-group id, a full-range int64 the API serializes as a string.
func LookupTableAttrSpec() client.AttrSpec {
	return client.AttrSpec{
		IDAttr:            "id",
		ProjectIDAttr:     "project_id",
		PathParamAttrs:    map[string]bool{},
		JSONEncodeAttrs:   map[string]bool{},
		JSONStringAttrs:   map[string]bool{},
		JSONEncodeWireKey: map[string]string{},
		OutputOnlyAttrs:   map[string]bool{},
		SpreadAttrs:       map[string]bool{},
	}
}
