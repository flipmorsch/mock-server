package ui

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

const (
	sepNone  = iota // start of doc, or right after a key's ": "
	sepOpen         // just opened a container: newline+indent, no comma
	sepValue        // a value just ended: comma+newline+indent
)

func highlightJSON(raw string) string {
	if !json.Valid([]byte(raw)) {
		return html.EscapeString(raw)
	}

	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()

	var out strings.Builder
	depth := 0
	stack := make([]byte, 0, 16) // '{' or '['
	sep := sepNone
	expectKey := false

	emitSep := func() {
		switch sep {
		case sepOpen:
			out.WriteString("\n")
			out.WriteString(spaces(depth * 2))
		case sepValue:
			out.WriteString(span("json-punct", ","))
			out.WriteString("\n")
			out.WriteString(spaces(depth * 2))
		}
	}
	endValue := func() {
		sep = sepValue
		expectKey = len(stack) > 0 && stack[len(stack)-1] == '{'
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}

		switch v := tok.(type) {
		case json.Delim:
			switch v {
			case '{', '[':
				emitSep()
				out.WriteString(span("json-punct", string(v)))
				stack = append(stack, byte(v))
				depth++
				sep = sepOpen
				expectKey = v == '{'
			case '}', ']':
				depth--
				stack = stack[:len(stack)-1]
				if sep != sepOpen { // empty containers close on the same line
					out.WriteString("\n")
					out.WriteString(spaces(depth * 2))
				}
				out.WriteString(span("json-punct", string(v)))
				endValue()
			}
		case string:
			emitSep()
			b, _ := json.Marshal(v)
			if expectKey {
				out.WriteString(span("json-key", string(b)))
				out.WriteString(span("json-punct", ": "))
				sep = sepNone
				expectKey = false
			} else {
				out.WriteString(span("json-string", string(b)))
				endValue()
			}
		case json.Number:
			emitSep()
			out.WriteString(span("json-number", v.String()))
			endValue()
		case bool:
			emitSep()
			out.WriteString(span("json-bool", fmt.Sprint(v)))
			endValue()
		case nil:
			emitSep()
			out.WriteString(span("json-null", "null"))
			endValue()
		}
	}

	return out.String()
}

func highlightBody(body string) string {
	return fmt.Sprintf("<pre class=\"json-body\">%s</pre>", highlightJSON(body))
}

func span(cls, text string) string {
	return fmt.Sprintf("<span class=\"%s\">%s</span>", cls, text)
}

func spaces(n int) string {
	return strings.Repeat(" ", n)
}
