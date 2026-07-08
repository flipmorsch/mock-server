package ui

import (
	"encoding/json"
	"fmt"
	"strings"
)

func highlightJSON(raw string) string {
	if !json.Valid([]byte(raw)) {
		return raw
	}

	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()

	var out strings.Builder
	depth := 0
	stack := make([]byte, 0, 16) // '{' or '['
	first := true
	needKey := false

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}

		if !first {
			if d, ok := tok.(json.Delim); ok && (d == '{' || d == '[') {
			} else if !needKey || len(stack) == 0 {
				out.WriteString(",\n")
				out.WriteString(spaces(depth * 2))
			}
		}
		first = false

		switch v := tok.(type) {
		case json.Delim:
			switch v {
			case '{', '[':
				if v == '{' {
					needKey = true
				}
				stack = append(stack, byte(v))
				out.WriteString(span("json-punct", string(v)))
				depth++
				out.WriteString("\n")
				out.WriteString(spaces(depth * 2))
			case '}', ']':
				depth--
				stack = stack[:len(stack)-1]
				if len(stack) > 0 && stack[len(stack)-1] == '{' {
					needKey = false
				}
				out.WriteString("\n")
				out.WriteString(spaces(depth * 2))
				out.WriteString(span("json-punct", string(v)))
			}
		case string:
			if needKey && len(stack) > 0 && stack[len(stack)-1] == '{' {
				b, _ := json.Marshal(v)
				out.WriteString(span("json-key", string(b)))
				out.WriteString(span("json-punct", ": "))
				needKey = false
			} else {
				b, _ := json.Marshal(v)
				out.WriteString(span("json-string", string(b)))
			}
		case json.Number:
			out.WriteString(span("json-number", v.String()))
		case bool:
			out.WriteString(span("json-bool", fmt.Sprint(v)))
		case nil:
			out.WriteString(span("json-null", "null"))
		}
	}

	return out.String()
}

func highlightBody(body string) string {
	html := highlightJSON(body)
	out := fmt.Sprintf("<pre class=\"json-body\">%s</pre>", html)
	return out
}

func span(cls, text string) string {
	return fmt.Sprintf("<span class=\"%s\">%s</span>", cls, text)
}

func spaces(n int) string {
	return strings.Repeat(" ", n)
}
