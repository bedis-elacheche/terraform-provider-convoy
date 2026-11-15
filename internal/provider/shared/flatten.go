package shared

import (
	"encoding/json"
	"reflect"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
)

// flattenAll flattens a JSON value into dotted keys, descending into objects
// and arrays alike. Convoy stores subscription filters partially flattened
// ("data.amount": {"$gte": 5}); flattening both sides fully gives a canonical
// form that is independent of how far Convoy flattened.
func flattenAll(prefix string, v any, out map[string]any) {
	join := func(k string) string {
		if prefix == "" {
			return k
		}
		return prefix + "." + k
	}
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 0 && prefix != "" {
			out[prefix] = map[string]any{}
		}
		for k, val := range t {
			flattenAll(join(k), val, out)
		}
	case []any:
		if len(t) == 0 && prefix != "" {
			out[prefix] = []any{}
		}
		for i, val := range t {
			flattenAll(join(strconv.Itoa(i)), val, out)
		}
	default:
		out[prefix] = t
	}
}

// FilterDocsEqual reports whether two filter documents match the same events.
func FilterDocsEqual(a, b string) bool {
	var va, vb any
	if json.Unmarshal([]byte(a), &va) != nil || json.Unmarshal([]byte(b), &vb) != nil {
		return a == b
	}
	fa, fb := map[string]any{}, map[string]any{}
	flattenAll("", va, fa)
	flattenAll("", vb, fb)
	return reflect.DeepEqual(fa, fb)
}

// FilterDocFromAPI returns the prior value when it is equivalent to what the
// API returned (modulo flattening), and the API document otherwise.
func FilterDocFromAPI(prior jsontypes.Normalized, raw json.RawMessage) jsontypes.Normalized {
	if IsEmptyJSON(raw) {
		return JSONFromAPI(prior, raw)
	}
	if Known(prior) && FilterDocsEqual(prior.ValueString(), string(raw)) {
		return prior
	}
	return JSONFromAPI(prior, raw)
}
