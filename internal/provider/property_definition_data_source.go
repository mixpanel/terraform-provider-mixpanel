// MANUALLY MAINTAINED — DO NOT REGENERATE (not driven by gen/crudgen.py).
//
// Data source over the Lexicon property-definitions module. See
// property_definition_resource.go for the verified endpoint semantics
// (collection-path GET selected by name + resourceType, synthetic id==0 body
// when no definition row exists, dual project/workspace mounts).

package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/mixpanel/terraform-provider-mixpanel/internal/client"
)

var (
	_ datasource.DataSource              = (*PropertyDefinitionDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*PropertyDefinitionDataSource)(nil)
)

// NewPropertyDefinitionDataSource constructs the property_definition data source.
func NewPropertyDefinitionDataSource() datasource.DataSource {
	return &PropertyDefinitionDataSource{}
}

type PropertyDefinitionDataSource struct {
	client               *client.Client
	workspacePathBuilder *client.WorkspacePathBuilder
}

// PropertyDefinitionDataSourceModel mirrors PropertyDefinitionModel plus the
// read-only `exists` convenience flag (the API synthesizes an id==0 body for
// unknown property names instead of returning 404).
type PropertyDefinitionDataSourceModel struct {
	ID           types.Int64  `tfsdk:"id"`
	ProjectID    types.Int64  `tfsdk:"project_id"`
	Name         types.String `tfsdk:"name"`
	ResourceType types.String `tfsdk:"resource_type"`
	DisplayName  types.String `tfsdk:"display_name"`
	Description  types.String `tfsdk:"description"`
	ExampleValue types.String `tfsdk:"example_value"`
	Type         types.String `tfsdk:"type"`
	Hidden       types.Bool   `tfsdk:"hidden"`
	Dropped      types.Bool   `tfsdk:"dropped"`
	Sensitive    types.Bool   `tfsdk:"sensitive"`
	Exists       types.Bool   `tfsdk:"exists"`
}

func (d *PropertyDefinitionDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_property_definition"
}

func (d *PropertyDefinitionDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the Lexicon metadata of one event or profile property.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:    true,
				Description: "The Lexicon definition row id (0 when no definition row exists yet).",
			},
			"project_id": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "The project ID (defaults to the provider project).",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The property key to look up.",
			},
			"resource_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "\"Event\" (default) or \"User\".",
				Validators: []validator.String{
					stringvalidator.OneOf("Event", "User"),
				},
			},
			"display_name":  schema.StringAttribute{Computed: true},
			"description":   schema.StringAttribute{Computed: true},
			"example_value": schema.StringAttribute{Computed: true},
			"type":          schema.StringAttribute{Computed: true},
			"hidden":        schema.BoolAttribute{Computed: true},
			"dropped":       schema.BoolAttribute{Computed: true},
			"sensitive":     schema.BoolAttribute{Computed: true},
			"exists": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether a Lexicon definition row exists for this property (the API returns a synthetic empty body instead of 404 when it does not).",
			},
		},
	}
}

func (d *PropertyDefinitionDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected DataSource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}
	d.client = c
	d.workspacePathBuilder = client.NewWorkspacePathBuilder(c)
}

func (d *PropertyDefinitionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m PropertyDefinitionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := d.client.ProjectID("")
	if !m.ProjectID.IsNull() && !m.ProjectID.IsUnknown() {
		projectID = d.client.ProjectID(strconv.FormatInt(m.ProjectID.ValueInt64(), 10))
	}
	resourceType := "Event"
	if !m.ResourceType.IsNull() && !m.ResourceType.IsUnknown() && m.ResourceType.ValueString() != "" {
		resourceType = m.ResourceType.ValueString()
	}

	// Same dual-mount preference as the resource (workspace mount first,
	// project mount on 404).
	bases := []string{"/api/app/projects/" + projectID + "/data-definitions"}
	if d.workspacePathBuilder != nil {
		if wid, err := d.workspacePathBuilder.WorkspaceID(ctx, projectID); err == nil && wid != "" {
			bases = []string{"/api/app/workspaces/" + wid + "/data-definitions", bases[0]}
		}
	}
	var respBody []byte
	var err error
	for i, base := range bases {
		respBody, err = d.client.Do(ctx, "GET", base+propertySelectorSuffix(m.Name.ValueString(), resourceType), nil)
		if err == nil {
			break
		}
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 && i < len(bases)-1 {
			continue
		}
		resp.Diagnostics.AddError("Reading property_definition", err.Error())
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading property_definition", err.Error())
		return
	}
	wire, err := unwrapBody(respBody, true)
	if err != nil {
		resp.Diagnostics.AddError("Decoding property_definition response", err.Error())
		return
	}

	getString := func(key string) types.String {
		if v, ok := wire[key].(string); ok {
			return types.StringValue(v)
		}
		return types.StringValue("")
	}
	getBool := func(key string) types.Bool {
		if v, ok := wire[key].(bool); ok {
			return types.BoolValue(v)
		}
		return types.BoolValue(false)
	}
	id := int64(0)
	if v, ok := propertyWireID(wire); ok {
		id = v
	}
	m.ID = types.Int64Value(id)
	m.Exists = types.BoolValue(id != 0)
	m.ResourceType = types.StringValue(resourceType)
	m.DisplayName = getString("displayName")
	m.Description = getString("description")
	m.ExampleValue = getString("exampleValue")
	m.Type = getString("type")
	m.Hidden = getBool("hidden")
	m.Dropped = getBool("dropped")
	m.Sensitive = getBool("sensitive")

	pid, perr := strconv.ParseInt(projectID, 10, 64)
	if perr != nil {
		resp.Diagnostics.AddError("Resolving project_id", fmt.Sprintf("project id %q is not numeric: %s", projectID, perr))
		return
	}
	m.ProjectID = types.Int64Value(pid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
