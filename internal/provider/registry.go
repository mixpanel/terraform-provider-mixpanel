// Code generated stub by gen_stubs.py — provider resource/data-source registry.

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// providerResources lists every resource constructor registered by the provider.
func providerResources() []func() resource.Resource {
	return []func() resource.Resource{
		NewAgentFlowResource,
		NewAnnotationResource,
		NewBookmarkResource,
		NewCanvasResource,
		NewCohortResource,
		NewConnectorResource,
		NewCustomAlertResource,
		NewCustomEventResource,
		NewCustomPropertyResource,
		NewCustomRoleResource,
		NewDashboardResource,
		NewDataGroupResource,
		NewEmailDigestResource,
		NewExperimentResource,
		NewFeatureFlagResource,
		NewHeatMapResource,
		NewHeatMapCollectionResource,
		NewPlaylistResource,
		NewRollupProjectResource,
		NewServiceAccountResource,
		NewThemeResource,
		NewWebhookResource,
	}
}

// providerDataSources lists every data-source constructor registered by the provider.
func providerDataSources() []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewAgentFlowDataSource,
		NewAnnotationDataSource,
		NewBookmarkDataSource,
		NewCanvasDataSource,
		NewCohortDataSource,
		NewConnectorDataSource,
		NewCustomAlertDataSource,
		NewCustomEventDataSource,
		NewCustomPropertyDataSource,
		NewCustomRoleDataSource,
		NewDashboardDataSource,
		NewEmailDigestDataSource,
		NewExperimentDataSource,
		NewFeatureFlagDataSource,
		NewHeatMapDataSource,
		NewHeatMapCollectionDataSource,
		NewMetricDataSource,
		NewPlaylistDataSource,
		NewRollupProjectDataSource,
		NewServiceAccountDataSource,
		NewThemeDataSource,
		NewWarehouseSourceDataSource,
	}
}
