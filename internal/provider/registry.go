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
		NewDataGovernanceSettingsResource,
		NewDataGroupResource,
		NewDatasetResource,
		NewEventDropFilterResource,
		NewEmailDigestResource,
		NewEventDefinitionResource,
		NewExperimentResource,
		NewFeatureFlagResource,
		NewHeatMapResource,
		NewHeatMapCollectionResource,
		NewPlaylistResource,
		NewRollupProjectResource,
		NewServiceAccountResource,
		NewThemeResource,
		NewWebhookResource,
		NewWorkspaceResource,
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
		NewDatasetDataSource,
		NewEmailDigestDataSource,
		NewEventDefinitionDataSource,
		NewExperimentDataSource,
		NewFeatureFlagDataSource,
		NewHeatMapDataSource,
		NewHeatMapCollectionDataSource,
		NewMetricDataSource,
		NewPlaylistDataSource,
		NewProjectOutgoingIntegrationDataSource,
		NewRollupProjectDataSource,
		NewServiceAccountDataSource,
		NewTagDataSource,
		NewThemeDataSource,
		NewWarehouseSourceDataSource,
		NewWorkspaceDataSource,
		// Plural "list" data sources (GREEN-10): bulk id + composite import-id discovery.
		NewAgentFlowListDataSource,
		NewAnnotationListDataSource,
		NewCustomAlertListDataSource,
		NewCustomPropertyListDataSource,
		NewCustomRoleListDataSource,
		NewEmailDigestListDataSource,
		NewExperimentListDataSource,
		NewFeatureFlagListDataSource,
		NewHeatMapListDataSource,
		NewPlaylistListDataSource,
	}
}
