// MANUALLY MAINTAINED — DO NOT REGENERATE (not driven by gen/crudgen.py).

package provider

import "github.com/mixpanel/terraform-provider-mixpanel/internal/client"

// PropertyDefinitionAttrSpec describes the property_definition entity to the
// generic tftypes<->JSON bridge, mirroring the per-entity AttrSpec convention
// of the generated resources. The hand-written property_definition resource
// and data source use a typed model rather than the bridge, so this spec is
// documentation of the entity's identity/scope attributes for tooling parity:
// the entity has no server-assigned path id (the selector is name +
// resourceType on the collection path) and `id` is the Lexicon row id echoed
// in responses.
func PropertyDefinitionAttrSpec() client.AttrSpec {
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
