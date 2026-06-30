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
		NewBehaviorResource,
		NewBookmarkResource,
		NewBusinessContextResource,
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
		NewFormulaResource,
		NewHeatMapResource,
		NewHeatMapCollectionResource,
		NewLexiconTagResource,
		NewMetricResource,
		NewOrgRequestAccessSettingsResource,
		NewOrgSessionSettingsResource,
		NewPlaylistResource,
		NewProjectResource,
		NewRollupProjectResource,
		NewServiceAccountResource,
		NewSparkSettingsResource,
		NewTeamResource,
		NewThemeResource,
		NewTwofactorSettingsResource,
		NewUserProjectRoleResource,
		NewWarehouseSourceResource,
		NewWebhookResource,
		NewWorkspaceResource,
	}
}

// providerDataSources lists every data-source constructor registered by the provider.
func providerDataSources() []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewAgentFlowDataSource,
		NewAnnotationDataSource,
		NewBehaviorDataSource,
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
		NewOrgRequestAccessSettingsDataSource,
		NewOrgSessionSettingsDataSource,
		NewPlaylistDataSource,
		NewProjectOutgoingIntegrationDataSource,
		NewRollupProjectDataSource,
		NewServiceAccountDataSource,
		NewSparkSettingsDataSource,
		NewTagDataSource,
		NewThemeDataSource,
		NewTwofactorSettingsDataSource,
		NewWarehouseSourceDataSource,
		NewWorkspaceDataSource,
		// Plural "list" data sources (GREEN-10): bulk id + composite import-id discovery.
		NewAgentFlowListDataSource,
		NewAnnotationListDataSource,
		NewCohortListDataSource,
		NewCustomAlertListDataSource,
		NewCustomEventListDataSource,
		NewCustomPropertyListDataSource,
		NewCustomRoleListDataSource,
		NewEmailDigestListDataSource,
		NewExperimentListDataSource,
		NewFeatureFlagListDataSource,
		NewHeatMapListDataSource,
		NewPlaylistListDataSource,
		NewSchemaGraphDataSource,
	}
}
