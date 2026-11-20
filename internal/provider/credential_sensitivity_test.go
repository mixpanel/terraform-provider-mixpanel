package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestCredentialSensitivity(t *testing.T) {
	ctx := context.Background()

	var serviceAccountResource resource.SchemaResponse
	(&ServiceAccountResource{}).Schema(ctx, resource.SchemaRequest{}, &serviceAccountResource)
	if serviceAccountResource.Diagnostics.HasError() {
		t.Fatalf("service-account resource schema returned diagnostics: %v", serviceAccountResource.Diagnostics)
	}
	resourceToken, ok := serviceAccountResource.Schema.Attributes["token"].(resourceschema.StringAttribute)
	if !ok {
		t.Fatalf("service-account resource token attribute has type %T, want StringAttribute", serviceAccountResource.Schema.Attributes["token"])
	}
	if !resourceToken.Sensitive {
		t.Fatal("service-account resource token must be sensitive")
	}
	if !resourceToken.Computed {
		t.Fatal("service-account resource token must remain computed")
	}
	resourceID, ok := serviceAccountResource.Schema.Attributes["serviceaccount_id"].(resourceschema.Int64Attribute)
	if !ok {
		t.Fatalf("service-account resource ID has type %T, want Int64Attribute", serviceAccountResource.Schema.Attributes["serviceaccount_id"])
	}
	if resourceID.Sensitive {
		t.Fatal("service-account resource ID must not be sensitive")
	}

	var projectResource resource.SchemaResponse
	(&ProjectResource{}).Schema(ctx, resource.SchemaRequest{}, &projectResource)
	if projectResource.Diagnostics.HasError() {
		t.Fatalf("project resource schema returned diagnostics: %v", projectResource.Diagnostics)
	}
	apiSecret, ok := projectResource.Schema.Attributes["api_secret"].(resourceschema.StringAttribute)
	if !ok {
		t.Fatalf("project api_secret attribute has type %T, want StringAttribute", projectResource.Schema.Attributes["api_secret"])
	}
	if !apiSecret.Sensitive {
		t.Fatal("project api_secret must be sensitive")
	}
	if !apiSecret.Computed {
		t.Fatal("project api_secret must remain computed")
	}
	projectID, ok := projectResource.Schema.Attributes["id"].(resourceschema.Int64Attribute)
	if !ok {
		t.Fatalf("project ID has type %T, want Int64Attribute", projectResource.Schema.Attributes["id"])
	}
	if projectID.Sensitive {
		t.Fatal("project ID must not be sensitive")
	}
}
