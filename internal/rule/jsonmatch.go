package rule

import (
	"encoding/json"
	"reflect"
)

// JSONBodyMatches reports whether got (a JSON document) contains want (a JSON
// document) as a subset:
//   - objects match partially — every field in want must be present in got with a
//     matching value; extra fields in got are ignored (recursively);
//   - arrays must be the same length and match element-wise;
//   - scalars must be equal.
//
// Either side failing to parse as JSON returns false.
func JSONBodyMatches(want, got string) bool {
	var wv, gv any
	if json.Unmarshal([]byte(want), &wv) != nil {
		return false
	}
	if json.Unmarshal([]byte(got), &gv) != nil {
		return false
	}
	return jsonSubset(wv, gv)
}

func jsonSubset(want, got any) bool {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, wv := range w {
			gv, ok := g[k]
			if !ok || !jsonSubset(wv, gv) {
				return false
			}
		}
		return true
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for i := range w {
			if !jsonSubset(w[i], g[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(want, got)
	}
}
