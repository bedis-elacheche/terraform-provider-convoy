// Package shared holds plumbing used by every resource, data source,
// ephemeral resource and action of the Convoy provider.
package shared

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/convoy"
)

// ProviderData is handed from the provider to every resource.
type ProviderData struct {
	Client         *convoy.Client
	ProjectID      string
	OrganisationID string
}

// FromProviderData converts the opaque provider data.
func FromProviderData(data any, diags *diag.Diagnostics) *ProviderData {
	if data == nil {
		return nil
	}
	pd, ok := data.(*ProviderData)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *shared.ProviderData, got %T", data))
		return nil
	}
	return pd
}

// ResolveProjectID picks the resource-level project_id, falling back to the
// provider's default project, mirroring how the Stripe provider resolves the
// connected-account context.
func (p *ProviderData) ResolveProjectID(v types.String, diags *diag.Diagnostics) string {
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		return v.ValueString()
	}
	if p != nil && p.ProjectID != "" {
		return p.ProjectID
	}
	diags.AddError("Missing project ID",
		"Set project_id on the resource or configure a default project_id on the provider (or the CONVOY_PROJECT_ID environment variable).")
	return ""
}

// ParseImportID splits an import ID of the form "<project_id>/<id>" (or
// "<project_id>/<parent_id>/<id>" when parts == 3). When the provider has a
// default project the project prefix may be omitted.
func ParseImportID(raw string, parts int, defaultProject string) ([]string, error) {
	segs := strings.Split(raw, "/")
	if len(segs) == parts-1 && defaultProject != "" {
		segs = append([]string{defaultProject}, segs...)
	}
	if len(segs) != parts {
		want := "<project_id>/<id>"
		if parts == 3 {
			want = "<project_id>/<subscription_id>/<id>"
		}
		return nil, fmt.Errorf("unexpected import ID %q, expected %s", raw, want)
	}
	for _, s := range segs {
		if s == "" {
			return nil, fmt.Errorf("unexpected import ID %q: empty segment", raw)
		}
	}
	return segs, nil
}

// ------------------------------------------------------------ conversions

// StrOrNull returns a null string for "".
func StrOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// StrPtrOrNull returns a null string for a nil pointer.
func StrPtrOrNull(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

// OptStr returns nil when the value is null/unknown.
func OptStr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// OptBool returns nil when the value is null/unknown.
func OptBool(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

// OptInt64 returns nil when the value is null/unknown.
func OptInt64(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := v.ValueInt64()
	return &i
}

// Known reports whether a value is neither null nor unknown.
func Known(v attr.Value) bool { return v != nil && !v.IsNull() && !v.IsUnknown() }

// StringList converts a list/set of strings into a Go slice (nil when
// null/unknown).
func StringList(ctx context.Context, v attr.Value, diags *diag.Diagnostics) []string {
	if !Known(v) {
		return nil
	}
	var out []string
	switch t := v.(type) {
	case types.List:
		diags.Append(t.ElementsAs(ctx, &out, false)...)
	case types.Set:
		diags.Append(t.ElementsAs(ctx, &out, false)...)
	}
	return out
}

// ListOfStrings builds a types.List from a slice; nil/empty -> empty list.
func ListOfStrings(in []string) types.List {
	vals := make([]attr.Value, 0, len(in))
	for _, s := range in {
		vals = append(vals, types.StringValue(s))
	}
	return types.ListValueMust(types.StringType, vals)
}

// SetOfStrings builds a types.Set from a slice; nil/empty -> empty set.
func SetOfStrings(in []string) types.Set {
	vals := make([]attr.Value, 0, len(in))
	for _, s := range in {
		vals = append(vals, types.StringValue(s))
	}
	return types.SetValueMust(types.StringType, vals)
}

// MapOfStrings converts a map attribute into a Go map.
func MapOfStrings(ctx context.Context, v types.Map, diags *diag.Diagnostics) map[string]string {
	if !Known(v) {
		return nil
	}
	out := map[string]string{}
	diags.Append(v.ElementsAs(ctx, &out, false)...)
	return out
}

// ------------------------------------------------------------------- JSON

// JSONRaw returns the raw JSON from a jsonencode()d attribute.
func JSONRaw(v jsontypes.Normalized) json.RawMessage {
	if !Known(v) || strings.TrimSpace(v.ValueString()) == "" {
		return nil
	}
	return json.RawMessage(v.ValueString())
}

// IsEmptyJSON reports whether raw is absent, null, or an empty object.
func IsEmptyJSON(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) == 0 || string(t) == "null" || string(t) == "{}"
}

// JSONFromAPI converts an API JSON document into a normalized JSON value.
// Empty documents map to null unless the prior value was itself an empty
// document. Formatting-only differences against the prior value are handled
// by jsontypes.Normalized semantic equality.
func JSONFromAPI(prior jsontypes.Normalized, raw json.RawMessage) jsontypes.Normalized {
	if IsEmptyJSON(raw) {
		if Known(prior) && IsEmptyJSON(json.RawMessage(prior.ValueString())) {
			return prior
		}
		return jsontypes.NewNormalizedNull()
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return jsontypes.NewNormalizedValue(string(raw))
	}
	b, _ := json.Marshal(v)
	return jsontypes.NewNormalizedValue(string(b))
}

// KeepSensitive returns the prior value for secret fields that Convoy
// encrypts, redacts or never echoes back, and otherwise the API value.
func KeepSensitive(prior types.String, api string) types.String {
	if Known(prior) {
		return prior
	}
	return StrOrNull(api)
}
