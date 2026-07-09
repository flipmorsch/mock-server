package rule

import (
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
)

// JSONBodyMatches reports whether got contains want as a JSON subset. It is
// JSONBodyDiff reduced to its boolean verdict — see there for the subset rules.
func JSONBodyMatches(want, got string) bool {
	_, _, _, ok := JSONBodyDiff(want, got)
	return ok
}

// JSONBodyDiff reports whether got (a JSON document) contains want as a subset:
//   - objects match partially — every field in want must be present in got with a
//     matching value; extra fields in got are ignored (recursively);
//   - arrays must be the same length and match element-wise;
//   - scalars must be equal.
//
// On match it returns ("", nil, nil, true). On mismatch it returns the path of the
// first divergence (e.g. "amount", "user.name", "items[2].id"), the wanted and
// actual values at that path, and false. Object keys are visited in sorted order so
// the reported path is deterministic. Either side failing to parse as JSON is a
// root-level mismatch (ok=false).
func JSONBodyDiff(want, got string) (path string, wantV, gotV any, ok bool) {
	var wv, gv any
	if json.Unmarshal([]byte(want), &wv) != nil {
		return "", want, got, false
	}
	if json.Unmarshal([]byte(got), &gv) != nil {
		return "", want, got, false
	}
	return jsonSubsetDiff("", wv, gv)
}

func jsonSubsetDiff(path string, want, got any) (string, any, any, bool) {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return path, want, got, false
		}
		keys := make([]string, 0, len(w))
		for k := range w {
			keys = append(keys, k)
		}
		sort.Strings(keys) // deterministic "first" failing path
		for _, k := range keys {
			child := k
			if path != "" {
				child = path + "." + k
			}
			gv, present := g[k]
			if !present {
				return child, w[k], nil, false
			}
			if p, wv, gv, ok := jsonSubsetDiff(child, w[k], gv); !ok {
				return p, wv, gv, false
			}
		}
		return "", nil, nil, true
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return path, want, got, false
		}
		for i := range w {
			child := path + "[" + strconv.Itoa(i) + "]"
			if p, wv, gv, ok := jsonSubsetDiff(child, w[i], g[i]); !ok {
				return p, wv, gv, false
			}
		}
		return "", nil, nil, true
	default:
		if !reflect.DeepEqual(want, got) {
			return path, want, got, false
		}
		return "", nil, nil, true
	}
}
