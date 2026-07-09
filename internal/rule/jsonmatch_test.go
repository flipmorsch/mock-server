package rule

import "testing"

func TestJSONBodyMatches(t *testing.T) {
	cases := []struct {
		name       string
		want, got  string
		wantResult bool
	}{
		{"object subset", `{"name":"Bob"}`, `{"name":"Bob","age":30}`, true},
		{"missing key", `{"x":1}`, `{"y":1}`, false},
		{"value mismatch", `{"n":1}`, `{"n":2}`, false},
		{"nested partial", `{"a":{"b":1}}`, `{"a":{"b":1,"c":2}}`, true},
		{"number equal", `{"amount":500}`, `{"amount":500,"currency":"usd"}`, true},
		{"array equal", `[1,2]`, `[1,2]`, true},
		{"array length differs", `[1]`, `[1,2]`, false},
		{"array of partial objects", `{"items":[{"id":1}]}`, `{"items":[{"id":1,"x":9}]}`, true},
		{"want not json", `{bad`, `{}`, false},
		{"got not json", `{}`, `nope`, false},
		{"exact scalar", `true`, `true`, true},
	}
	for _, c := range cases {
		if got := JSONBodyMatches(c.want, c.got); got != c.wantResult {
			t.Errorf("%s: JSONBodyMatches(%q, %q) = %v, want %v", c.name, c.want, c.got, got, c.wantResult)
		}
	}
}

func TestJSONBodyDiff(t *testing.T) {
	cases := []struct {
		name      string
		want, got string
		wantPath  string // "" means match
	}{
		{"match", `{"a":1}`, `{"a":1,"b":2}`, ""},
		{"scalar mismatch", `{"amount":500}`, `{"amount":300}`, "amount"},
		{"nested object path", `{"user":{"name":"a"}}`, `{"user":{"name":"b"}}`, "user.name"},
		{"array index path", `{"items":[{"id":1},{"id":2}]}`, `{"items":[{"id":1},{"id":9}]}`, "items[1].id"},
		{"missing key path", `{"x":1}`, `{"y":1}`, "x"},
		{"array length", `{"items":[1,2]}`, `{"items":[1]}`, "items"},
		{"type mismatch", `{"a":{"b":1}}`, `{"a":5}`, "a"},
		{"deterministic first key", `{"a":1,"b":2}`, `{"a":9,"b":9}`, "a"}, // sorted -> "a" always first
		{"want not json", `{bad`, `{}`, ""},                                // parse fail: ok=false, path root ""
	}
	for _, c := range cases {
		path, _, _, ok := JSONBodyDiff(c.want, c.got)
		if c.wantPath == "" && c.name != "want not json" {
			if !ok {
				t.Errorf("%s: expected match, got diff at %q", c.name, path)
			}
			continue
		}
		if c.name == "want not json" {
			if ok {
				t.Errorf("%s: expected mismatch on unparseable input", c.name)
			}
			continue
		}
		if ok {
			t.Errorf("%s: expected mismatch at %q, got match", c.name, c.wantPath)
		} else if path != c.wantPath {
			t.Errorf("%s: diff path = %q, want %q", c.name, path, c.wantPath)
		}
	}
}
