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
