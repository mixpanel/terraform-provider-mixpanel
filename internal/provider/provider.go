// Package provider implements the Mixpanel Terraform provider. It wires the
// generated per-entity schema packages into terraform-plugin-framework resources
// and data sources and configures a shared API client.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/mixpanel/terraform-provider-mixpanel/internal/client"
)

// Ensure MixpanelProvider satisfies the provider.Provider interface.
var _ provider.Provider = (*MixpanelProvider)(nil)

// MixpanelProvider is the provider implementation.
type MixpanelProvider struct {
	// version is set at build time and surfaced in the provider metadata.
	version string
}

// MixpanelProviderModel maps provider schema attributes to Go values.
type MixpanelProviderModel struct {
	ServiceAccount       types.String `tfsdk:"service_account"`
	ServiceAccountSecret types.String `tfsdk:"service_account_secret"`
	ProjectID            types.String `tfsdk:"project_id"`
	OrganizationID       types.String `tfsdk:"organization_id"`
	BaseURL              types.String `tfsdk:"base_url"`
}

// New returns a provider factory for the given build version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &MixpanelProvider{version: version}
	}
}

func (p *MixpanelProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "mixpanel"
	resp.Version = p.version
}

func (p *MixpanelProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Mixpanel resources via the Mixpanel App API.",
		Attributes: map[string]schema.Attribute{
			"service_account": schema.StringAttribute{
				Optional:    true,
				Description: "Mixpanel service account username. May also be set via the MIXPANEL_SERVICE_ACCOUNT environment variable.",
			},
			"service_account_secret": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Mixpanel service account secret. May also be set via the MIXPANEL_SERVICE_ACCOUNT_SECRET environment variable.",
			},
			"project_id": schema.StringAttribute{
				Optional:    true,
				Description: "Default Mixpanel project ID for project-scoped resources. May also be set via the MIXPANEL_PROJECT_ID environment variable. Individual resources may override it.",
			},
			"organization_id": schema.StringAttribute{
				Optional:    true,
				Description: "Default Mixpanel organization ID for organization-scoped resources (e.g. service_account). May also be set via the MIXPANEL_ORGANIZATION_ID environment variable. Individual resources may override it.",
			},
			"base_url": schema.StringAttribute{
				Optional:    true,
				Description: "API base URL. Defaults to https://mixpanel.com. May also be set via the MIXPANEL_BASE_URL environment variable.",
			},
		},
	}
}

func (p *MixpanelProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg MixpanelProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Config attribute wins, else environment variable.
	serviceAccount := firstNonEmpty(cfg.ServiceAccount, "MIXPANEL_SERVICE_ACCOUNT")
	serviceSecret := firstNonEmpty(cfg.ServiceAccountSecret, "MIXPANEL_SERVICE_ACCOUNT_SECRET")
	projectID := firstNonEmpty(cfg.ProjectID, "MIXPANEL_PROJECT_ID")
	organizationID := firstNonEmpty(cfg.OrganizationID, "MIXPANEL_ORGANIZATION_ID")
	baseURL := firstNonEmpty(cfg.BaseURL, "MIXPANEL_BASE_URL")

	if serviceAccount == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("service_account"),
			"Missing Mixpanel Service Account",
			"Set the service_account attribute or the MIXPANEL_SERVICE_ACCOUNT environment variable.",
		)
	}
	if serviceSecret == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("service_account_secret"),
			"Missing Mixpanel Service Account Secret",
			"Set the service_account_secret attribute or the MIXPANEL_SERVICE_ACCOUNT_SECRET environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	c := client.New(client.Config{
		BaseURL:               baseURL,
		ServiceAccount:        serviceAccount,
		ServiceSecret:         serviceSecret,
		DefaultProjectID:      projectID,
		DefaultOrganizationID: organizationID,
	})

	// Make the client available to all resources and data sources.
	resp.DataSourceData = c
	resp.ResourceData = c
}

// Resources returns the full set of provider resources.
func (p *MixpanelProvider) Resources(ctx context.Context) []func() resource.Resource {
	return providerResources()
}

// DataSources returns the full set of provider data sources.
func (p *MixpanelProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return providerDataSources()
}

func firstNonEmpty(attr types.String, envKey string) string {
	if !attr.IsNull() && !attr.IsUnknown() && attr.ValueString() != "" {
		return attr.ValueString()
	}
	return os.Getenv(envKey)
}
