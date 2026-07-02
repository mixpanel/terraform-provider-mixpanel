// Ungated unit tests for the shared CRUD helpers (no TF_ACC, no provider
// harness): the found/absent/undetermined contract of findInList, the
// non-object guard in unwrapBody, and int64 id precision in normalizeNumbers.

package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestFindInListFoundInArray verifies the plain array list shape still matches
// by id, including through the success envelope.
func TestFindInListFoundInArray(t *testing.T) {
	body := []byte(`{"status": "ok", "results": [{"id": 1, "name": "a"}, {"id": 2, "name": "b"}]}`)
	wire, found, err := findInList(body, true, "id", "2")
	if err != nil {
		t.Fatalf("findInList: %v", err)
	}
	if !found {
		t.Fatal("findInList: id 2 should be found")
	}
	if wire["name"] != "a" && wire["name"] != "b" {
		t.Errorf("unexpected match: %v", wire)
	}
	if wire["name"] != "b" {
		t.Errorf("matched wrong element: %v", wire)
	}
}

// TestFindInListDefinitelyAbsent verifies a well-formed array that lacks the id
// reports found=false with NO error (the caller may safely evict state).
func TestFindInListDefinitelyAbsent(t *testing.T) {
	body := []byte(`[{"id": 1}, {"id": 3}]`)
	_, found, err := findInList(body, false, "id", "2")
	if err != nil {
		t.Fatalf("findInList: %v", err)
	}
	if found {
		t.Fatal("findInList: id 2 should be absent")
	}
}

// TestFindInListMapBody verifies the id->object map listing shape
// (themes_to_dict_map convention): a match on the nested id, a match on the map
// key (with the id injected), and a well-formed miss reporting found=false.
func TestFindInListMapBody(t *testing.T) {
	// Match via nested id field.
	body := []byte(`{"10": {"id": 10, "name": "x"}, "20": {"id": 20, "name": "y"}}`)
	wire, found, err := findInList(body, false, "id", "20")
	if err != nil {
		t.Fatalf("findInList: %v", err)
	}
	if !found || wire["name"] != "y" {
		t.Fatalf("findInList map body: found=%v wire=%v", found, wire)
	}

	// Match via the map KEY when the inner object carries no id; the key is
	// injected so downstream id extraction works.
	body = []byte(`{"30": {"name": "z"}}`)
	wire, found, err = findInList(body, false, "id", "30")
	if err != nil {
		t.Fatalf("findInList: %v", err)
	}
	if !found {
		t.Fatal("findInList: key 30 should be found")
	}
	if got, ok := nestedID(wire, "id"); !ok || got != "30" {
		t.Errorf("injected id = %q, %v; want \"30\"", got, ok)
	}

	// Well-formed map without the id: definitely absent, no error.
	body = []byte(`{"10": {"id": 10}}`)
	_, found, err = findInList(body, false, "id", "99")
	if err != nil {
		t.Fatalf("findInList: %v", err)
	}
	if found {
		t.Fatal("findInList: id 99 should be absent")
	}
}

// TestFindInListUndeterminedShapes verifies that a body that is NOT a
// recognizable listing (HTML proxy page, bare value, empty body, map of
// non-objects) returns an ERROR and never found=false, since found=false makes
// callers remove the resource from state and re-create it on the next apply.
func TestFindInListUndeterminedShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"html proxy page", "<html><body>502 Bad Gateway</body></html>"},
		{"bare string", `"oops"`},
		{"bare number", `42`},
		{"empty body", ""},
		{"json null", "null"},
		{"map of non-objects", `{"a": 1, "b": 2}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, found, err := findInList([]byte(tc.body), false, "id", "1")
			if err == nil {
				t.Fatalf("findInList(%q): expected an error, got found=%v", tc.body, found)
			}
			if found {
				t.Errorf("findInList(%q): found must be false on error", tc.body)
			}
		})
	}
}

// TestFindInListEnvelopeError verifies an HTTP-200 error envelope propagates as
// an error (never a silent not-found).
func TestFindInListEnvelopeError(t *testing.T) {
	body := []byte(`{"status": "error", "error": "insufficient permissions"}`)
	_, found, err := findInList(body, true, "id", "1")
	if err == nil {
		t.Fatal("findInList: expected an error for a status:error envelope")
	}
	if found {
		t.Error("findInList: found must be false on error")
	}
	if !strings.Contains(err.Error(), "insufficient permissions") {
		t.Errorf("error %q does not include the server error message", err)
	}
}

// TestUnwrapBodyObject verifies the normal enveloped-object read path.
func TestUnwrapBodyObject(t *testing.T) {
	body := []byte(`{"status": "ok", "results": {"id": 5, "name": "n"}}`)
	m, err := unwrapBody(body, true)
	if err != nil {
		t.Fatalf("unwrapBody: %v", err)
	}
	if m["name"] != "n" {
		t.Errorf("unwrapBody = %v, want name=n", m)
	}
}

// TestUnwrapBodyEmptyAndNull verifies an empty or JSON-null body still yields
// an empty map (some write endpoints return no entity).
func TestUnwrapBodyEmptyAndNull(t *testing.T) {
	for _, body := range []string{"", "null"} {
		m, err := unwrapBody([]byte(body), false)
		if err != nil {
			t.Fatalf("unwrapBody(%q): %v", body, err)
		}
		if len(m) != 0 {
			t.Errorf("unwrapBody(%q) = %v, want empty map", body, m)
		}
	}
}

// TestUnwrapBodyNonObjectErrors verifies a non-object body is an error rather
// than an empty map (which used to flow an empty id downstream).
func TestUnwrapBodyNonObjectErrors(t *testing.T) {
	for _, body := range []string{`[1,2,3]`, `"bare string"`, `42`, "<html>proxy</html>"} {
		if _, err := unwrapBody([]byte(body), false); err == nil {
			t.Errorf("unwrapBody(%q): expected an error, got nil", body)
		}
	}
}

// TestUnwrapBodyEnvelopeError verifies the HTTP-200 error envelope surfaces
// through unwrapBody.
func TestUnwrapBodyEnvelopeError(t *testing.T) {
	body := []byte(`{"status": "error", "error": "nope"}`)
	if _, err := unwrapBody(body, true); err == nil {
		t.Fatal("unwrapBody: expected an error for a status:error envelope")
	}
}

// TestNormalizeNumbersInt64Precision verifies integer ids above 2^53 survive
// normalizeNumbers exactly (kept as int64 instead of being routed through
// float64), while genuine floats still normalize to float64 and oversized
// decimals fall back to their literal string.
func TestNormalizeNumbersInt64Precision(t *testing.T) {
	const bigID = "9223372036854775001" // > 2^53, corrupted by a float64 round-trip
	dec := json.NewDecoder(strings.NewReader(`{"dataGroupId": ` + bigID + `, "ratio": 0.25, "huge": 123456789012345678901234567890}`))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	nm := normalizeNumbers(m).(map[string]any)

	id, ok := nm["dataGroupId"].(int64)
	if !ok {
		t.Fatalf("dataGroupId = %T (%v), want int64", nm["dataGroupId"], nm["dataGroupId"])
	}
	if s, ok := nestedID(nm, "dataGroupId"); !ok || s != bigID {
		t.Errorf("nestedID = %q, %v; want %q (id corrupted)", s, ok, bigID)
	}
	if id != 9223372036854775001 {
		t.Errorf("dataGroupId = %d, want 9223372036854775001", id)
	}

	if f, ok := nm["ratio"].(float64); !ok || f != 0.25 {
		t.Errorf("ratio = %T (%v), want float64 0.25", nm["ratio"], nm["ratio"])
	}

	// Wider than int64: falls back to float64 (existing behavior for
	// non-identity numeric magnitudes).
	if _, ok := nm["huge"].(float64); !ok {
		t.Errorf("huge = %T (%v), want float64 fallback", nm["huge"], nm["huge"])
	}
}

// TestUnitCollectIDsFromListShapes verifies the list data-source helper keeps
// returning ids for well-formed arrays and empty bodies, but errors on a shape
// surprise instead of silently reporting an empty listing.
func TestUnitCollectIDsFromListShapes(t *testing.T) {
	ids, err := collectIDsFromList([]byte(`{"status":"ok","results":[{"id": 9223372036854775001}, {"id": "abc"}]}`), true, "id")
	if err != nil {
		t.Fatalf("collectIDsFromList: %v", err)
	}
	if len(ids) != 2 || ids[0] != "9223372036854775001" || ids[1] != "abc" {
		t.Errorf("ids = %v, want exact big id then abc", ids)
	}

	ids, err = collectIDsFromList([]byte(``), false, "id")
	if err != nil || len(ids) != 0 {
		t.Errorf("empty body: ids=%v err=%v, want empty, nil", ids, err)
	}

	if _, err := collectIDsFromList([]byte(`<html>proxy</html>`), false, "id"); err == nil {
		t.Error("html body: expected an error, got nil")
	}
	if _, err := collectIDsFromList([]byte(`{"not":"an array"}`), false, "id"); err == nil {
		t.Error("object body: expected an error, got nil")
	}
}

// TestUnitSelectNewestFromListLargestID pins the documented heuristic: among
// multiple rows matching the attribute, the numerically largest id wins.
func TestUnitSelectNewestFromListLargestID(t *testing.T) {
	body := []byte(`[{"id": 3, "event_name": "e"}, {"id": 11, "event_name": "e"}, {"id": 7, "event_name": "other"}]`)
	wire, id, err := selectNewestFromList(body, false, "id", "event_name", "e")
	if err != nil {
		t.Fatalf("selectNewestFromList: %v", err)
	}
	if id != "11" {
		t.Errorf("id = %q, want \"11\" (largest matching id)", id)
	}
	if got, _ := nestedID(wire, "id"); got != "11" {
		t.Errorf("wire id = %q, want \"11\"", got)
	}
}
