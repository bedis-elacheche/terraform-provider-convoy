package shared

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
)

func TestFilterDocsEqual(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{`{"data":{"amount":{"$gte":5}}}`, `{"data.amount":{"$gte":5}}`, true},
		{`{"a":{"b":1},"c":2}`, `{"c":2,"a.b":1}`, true},
		{`{"$or":[{"a":{"b":1}},{"c":2}]}`, `{"$or":[{"a.b":1},{"c":2}]}`, true},
		{`{"a":{"b":1}}`, `{"a.b":2}`, false},
		{`{"a":[1,2]}`, `{"a":[2,1]}`, false},
	}
	for _, tc := range cases {
		if got := FilterDocsEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("FilterDocsEqual(%s, %s) = %v", tc.a, tc.b, got)
		}
	}
}

func TestFilterDocFromAPI(t *testing.T) {
	prior := jsontypes.NewNormalizedValue(`{"data": {"x": 1}}`)
	if got := FilterDocFromAPI(prior, json.RawMessage(`{"data.x":1}`)); !got.Equal(prior) {
		t.Errorf("equivalent document should keep prior, got %s", got)
	}
	if got := FilterDocFromAPI(prior, json.RawMessage(`{"data.x":2}`)); got.ValueString() != `{"data.x":2}` {
		t.Errorf("drift not reported, got %s", got)
	}
	if got := FilterDocFromAPI(jsontypes.NewNormalizedNull(), json.RawMessage(`{}`)); !got.IsNull() {
		t.Errorf("empty document should be null, got %s", got)
	}
}

func TestParseImportID(t *testing.T) {
	if p, err := ParseImportID("p/e", 2, ""); err != nil || p[0] != "p" || p[1] != "e" {
		t.Errorf("unexpected %v %v", p, err)
	}
	if p, err := ParseImportID("e", 2, "def"); err != nil || p[0] != "def" {
		t.Errorf("default project not used: %v %v", p, err)
	}
	if _, err := ParseImportID("e", 2, ""); err == nil {
		t.Error("expected error without default project")
	}
	if p, err := ParseImportID("p/s/f", 3, ""); err != nil || p[2] != "f" {
		t.Errorf("unexpected %v %v", p, err)
	}
	if _, err := ParseImportID("p//f", 3, ""); err == nil {
		t.Error("expected error on empty segment")
	}
}
