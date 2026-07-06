// Acceptance tests for mixpanel_custom_event resource.
// Run with: TF_ACC=1 go test -v -run TestAccCustomEvent
//
// Live-verified payload shape (2026-07-03, devbox project 3): the resource's
// writable attributes are `name` and `alternatives` (a JSON string of
// [{event, serialized?, valid_segfilter?}] sent as a single form field —
// custom events use form-encoding, webapp custom_events/views.py
// create_customevent does json.loads(params.pop("alternatives"))). The
// `custom_event` attribute is the read-only response wrapper, NOT an input.
// Delete is a soft delete: GET of a deleted custom event still returns 200
// with "deleted": 1, so the destroy check parses the body.

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// accCheckCustomEventDestroy verifies via the live API that custom events
// are soft-deleted: GET keeps returning 200 for deleted custom events, with
// the "deleted" flag set on the wrapped object.
func accCheckCustomEventDestroy() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		base := os.Getenv("MIXPANEL_BASE_URL")
		if base == "" {
			base = "https://mixpanel.com"
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "mixpanel_custom_event" {
				continue
			}
			id := rs.Primary.Attributes["customevent_id"]
			if id == "" {
				id = rs.Primary.ID
			}
			projectID := rs.Primary.Attributes["project_id"]
			if projectID == "" {
				projectID = os.Getenv("MIXPANEL_PROJECT_ID")
			}
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/app/custom_events/%s/%s/", base, projectID, id), nil)
			if err != nil {
				return err
			}
			req.SetBasicAuth(os.Getenv("MIXPANEL_SERVICE_ACCOUNT"), os.Getenv("MIXPANEL_SERVICE_ACCOUNT_SECRET"))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("checking custom event %s after destroy: %w", id, err)
			}
			if resp.StatusCode == http.StatusNotFound {
				resp.Body.Close()
				continue // hard-gone is fine too
			}
			var body struct {
				CustomEvent struct {
					Deleted any `json:"deleted"`
				} `json:"custom_event"`
			}
			err = json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			if err != nil {
				return fmt.Errorf("decoding custom event %s after destroy: %w", id, err)
			}
			// deleted is 1/true when soft-deleted, 0/false/null otherwise.
			switch v := body.CustomEvent.Deleted.(type) {
			case bool:
				if !v {
					return fmt.Errorf("custom event %s still exists after destroy (deleted=false)", id)
				}
			case float64:
				if v == 0 {
					return fmt.Errorf("custom event %s still exists after destroy (deleted=0)", id)
				}
			default:
				return fmt.Errorf("custom event %s still exists after destroy (deleted=%v)", id, v)
			}
		}
		return nil
	}
}

// accCustomEventConfig renders a custom event with the given alternatives HCL.
func accCustomEventConfig(projectID, name, alternativesHCL string) string {
	return accProviderConfig(projectID) + fmt.Sprintf(`
resource "mixpanel_custom_event" "test" {
  name         = %q
  project_id   = %s
  alternatives = `+alternativesHCL+`
}
`, name, projectID)
}

func TestAccCustomEvent_basic(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-custom-event")
	projectID := projectPool.nextProject()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckCustomEventDestroy(),
		Steps: []resource.TestStep{
			{
				// Create
				Config: accCustomEventConfig(projectID, name, `jsonencode([{ event = "page_view" }])`),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_event.test"),
					resource.TestCheckResourceAttr("mixpanel_custom_event.test", "name", name),
					// customevent_id is the identity field
					accCheckResourceAttrIsInt("mixpanel_custom_event.test", "customevent_id"),
					// The read-only response wrapper is populated after create.
					accCheckResourceAttrSet("mixpanel_custom_event.test", "custom_event.id"),
				),
			},
			{
				// Import. alternatives is create/update input only (the read
				// response nests it under custom_event), so it cannot be
				// verified across import.
				ResourceName:                         "mixpanel_custom_event.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "customevent_id",
				ImportStateIdFunc:                    importIDFunc("mixpanel_custom_event.test", "customevent_id", "project_id"),
				ImportStateVerifyIgnore:              []string{"custom_event", "alternatives"},
			},
		},
	})
}

func TestAccCustomEvent_withFilters(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-custom-event-filter")
	projectID := projectPool.nextProject()

	// An alternative carrying a property filter (serialized selector +
	// valid_segfilter object — live-verified shape).
	filteredAlt := `jsonencode([
    {
      event            = "page_view"
      serialized       = "(defined (string(properties[\"url\"])))"
      valid_segfilter  = { operator = "defined", children = [] }
    }
  ])`
	updatedAlt := `jsonencode([
    {
      event            = "page_view"
      serialized       = "(defined (string(properties[\"path\"])))"
      valid_segfilter  = { operator = "defined", children = [] }
    }
  ])`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckCustomEventDestroy(),
		Steps: []resource.TestStep{
			{
				// Create with a filtered alternative
				Config: accCustomEventConfig(projectID, name, filteredAlt),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_event.test"),
					resource.TestCheckResourceAttr("mixpanel_custom_event.test", "name", name),
				),
			},
			{
				// Update the filter
				Config: accCustomEventConfig(projectID, name, updatedAlt),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_event.test"),
				),
			},
		},
	})
}

func TestAccCustomEvent_formEncoding(t *testing.T) {
	skipIfNotAcceptance(t)

	name := accRandomName("tf-acc-custom-event-form")
	projectID := projectPool.nextProject()

	// Custom events use form-encoding per gaps-and-gotchas §1: name travels
	// as a plain form field and alternatives as ONE JSON-encoded form field.
	// Multiple alternatives exercise the encoding of a non-trivial blob.
	multiAlt := `jsonencode([
    { event = "test_event" },
    { event = "test_event_v2" }
  ])`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6,
		PreCheck:                 func() { accTestPreCheck(t) },
		CheckDestroy:             accCheckCustomEventDestroy(),
		Steps: []resource.TestStep{
			{
				Config: accCustomEventConfig(projectID, name, multiAlt),
				Check: resource.ComposeTestCheckFunc(
					accCheckResourceExists("mixpanel_custom_event.test"),
					accCheckResourceAttrIsInt("mixpanel_custom_event.test", "customevent_id"),
				),
			},
			{
				// Verify idempotency
				Config:             accCustomEventConfig(projectID, name, multiAlt),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}
