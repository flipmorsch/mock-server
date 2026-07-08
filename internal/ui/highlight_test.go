package ui

import (
	"strings"
	"testing"
)

func TestHighlightBodyValidJSON(t *testing.T) {
	raw := `{"name":"Alice","age":30,"active":true,"data":null,"tags":["a","b"]}`
	html := highlightBody(raw)

	if !strings.Contains(html, "json-key") {
		t.Error("missing json-key spans")
	}
	if !strings.Contains(html, "json-string") {
		t.Error("missing json-string spans")
	}
	if !strings.Contains(html, "json-number") {
		t.Error("missing json-number spans")
	}
	if !strings.Contains(html, "json-bool") {
		t.Error("missing json-bool spans")
	}
	if !strings.Contains(html, "json-null") {
		t.Error("missing json-null spans")
	}
	if !strings.Contains(html, "json-punct") {
		t.Error("missing json-punct spans")
	}
	if !strings.Contains(html, "<pre class=\"json-body\">") {
		t.Error("missing <pre> wrapper")
	}
}

func TestHighlightBodyInvalidJSON(t *testing.T) {
	raw := "just some text"
	html := highlightBody(raw)

	if strings.Contains(html, "json-key") {
		t.Error("non-JSON should not have json-key spans")
	}
	if html != "<pre class=\"json-body\">just some text</pre>" {
		t.Errorf("non-JSON should be raw text in pre, got: %s", html)
	}
}

func TestHighlightBodyInvalidJSONEscaped(t *testing.T) {
	html := highlightBody("<script>alert(1)</script>")
	if strings.Contains(html, "<script>") {
		t.Errorf("non-JSON body must be HTML-escaped, got: %s", html)
	}
}

func TestHighlightBodyLayout(t *testing.T) {
	html := highlightBody(`{"a":1,"b":[true,null],"c":{}}`)
	want := `<pre class="json-body"><span class="json-punct">{</span>
  <span class="json-key">"a"</span><span class="json-punct">: </span><span class="json-number">1</span><span class="json-punct">,</span>
  <span class="json-key">"b"</span><span class="json-punct">: </span><span class="json-punct">[</span>
    <span class="json-bool">true</span><span class="json-punct">,</span>
    <span class="json-null">null</span>
  <span class="json-punct">]</span><span class="json-punct">,</span>
  <span class="json-key">"c"</span><span class="json-punct">: </span><span class="json-punct">{</span><span class="json-punct">}</span>
<span class="json-punct">}</span></pre>`
	if html != want {
		t.Errorf("layout mismatch:\ngot:\n%s\nwant:\n%s", html, want)
	}
}

func TestHighlightBodyNested(t *testing.T) {
	raw := `{"user":{"name":"Bob","scores":[1,2,3]}}`
	html := highlightBody(raw)

	if !strings.Contains(html, "json-key") {
		t.Error("nested JSON missing json-key")
	}
	if !strings.Contains(html, "json-number") {
		t.Error("nested JSON missing json-number")
	}
}
