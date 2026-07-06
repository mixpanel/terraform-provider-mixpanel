// Unit tests for the shared-entities helpers (sharing.go) against the mock
// server's shared-entities routes. These run without TF_ACC: they exercise the
// HTTP contract (upsert/delete/read) directly, not the Terraform lifecycle.
package provider

import (
	"context"
	"testing"

	"github.com/mixpanel/terraform-provider-mixpanel/internal/client"
)

func TestSharing_helperLifecycle(t *testing.T) {
	srv := newMockServer(t, mockOpts{enveloped: true})
	c := client.New(client.Config{
		BaseURL:          srv.URL,
		ServiceAccount:   "test",
		ServiceSecret:    "test",
		DefaultProjectID: "1",
	})
	ctx := context.Background()

	// Initially unshared.
	shared, err := readEntityProjectShare(ctx, c, "1", sharedEntityTypeCohort, "42")
	if err != nil {
		t.Fatalf("read (initial): %v", err)
	}
	if shared {
		t.Fatal("expected no project share before upsert")
	}

	// Share, read back.
	if err := shareEntityWithProject(ctx, c, "1", sharedEntityTypeCohort, "42", true); err != nil {
		t.Fatalf("share: %v", err)
	}
	shared, err = readEntityProjectShare(ctx, c, "1", sharedEntityTypeCohort, "42")
	if err != nil {
		t.Fatalf("read (after share): %v", err)
	}
	if !shared {
		t.Fatal("expected project share after upsert")
	}

	// A different project id does not match.
	shared, err = readEntityProjectShare(ctx, c, "2", sharedEntityTypeCohort, "42")
	if err != nil {
		t.Fatalf("read (other project): %v", err)
	}
	if shared {
		t.Fatal("share for project 1 must not read as shared for project 2")
	}

	// Unshare, read back.
	if err := unshareEntityFromProject(ctx, c, "1", sharedEntityTypeCohort, "42"); err != nil {
		t.Fatalf("unshare: %v", err)
	}
	shared, err = readEntityProjectShare(ctx, c, "1", sharedEntityTypeCohort, "42")
	if err != nil {
		t.Fatalf("read (after unshare): %v", err)
	}
	if shared {
		t.Fatal("expected no project share after delete")
	}
}

// TestSharing_uuidEntityID covers the feature-flag case: entity ids are UUIDs
// and must round-trip as strings (shareWireID must not coerce them).
func TestSharing_uuidEntityID(t *testing.T) {
	srv := newMockServer(t, mockOpts{enveloped: true})
	c := client.New(client.Config{BaseURL: srv.URL, ServiceAccount: "test", ServiceSecret: "test", DefaultProjectID: "1"})
	ctx := context.Background()

	const uuid = "0198c0de-0000-4000-8000-000000000abc"
	if err := shareEntityWithProject(ctx, c, "1", sharedEntityTypeFeatureFlag, uuid, true); err != nil {
		t.Fatalf("share uuid entity: %v", err)
	}
	shared, err := readEntityProjectShare(ctx, c, "1", sharedEntityTypeFeatureFlag, uuid)
	if err != nil {
		t.Fatalf("read uuid entity: %v", err)
	}
	if !shared {
		t.Fatal("expected uuid entity to be shared")
	}
}
