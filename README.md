# mock-server

A standalone CLI that serves mock HTTP responses from a YAML config file — with request matching across every dimension, latency simulation, templated responses, sequenced responses, TLS, hot reload, and an embedded debug UI.

## Install

```sh
go build -o mock-server .
```

## Usage

```sh
mock-server [--listen addr:port] [--ui] [--tls] [--tls-cert file --tls-key file] <config.yaml>
```

- `--listen` — override the listen address (default `127.0.0.1:8080`).
- `--ui` — enable the embedded Web UI at `/_ui/` (live request journal, match explanations, dry-run + probe testing).
- `--tls` — serve HTTPS. With no cert supplied, an ephemeral self-signed certificate is generated and its SHA-256 fingerprint is logged.
- `--tls-cert` / `--tls-key` — serve HTTPS with a provided certificate/key pair (given together; implies `--tls`).
- `--version`, `--help`.

## Config File

```yaml
listen: "127.0.0.1:8080"

rules:
  - name: "get users"
    request:
      method: GET
      path: /users
      path_mode: exact
    response:
      status: 200
      headers:
        content-type: application/json
      body: |
        [{"id": 1, "name": "Alice"}]

  - name: "health check"
    request:
      method: GET
      path: /health
    response:
      status: 200
      body: OK
```

### Request matching

- **`method`** — HTTP method (case-insensitive).
- **`path`** + **`path_mode`** — `exact` (default), `prefix` (path-segment prefix), `regex`, or `pattern` (`/users/{id}` — each `{name}` matches one segment, like ServeMux/OpenAPI; captures are readable in the response template).
- **`headers`** / **`query`** — exact key/value matches (AND semantics; extras ignored).
- **`body`** — `{mode: exact|contains, value: ...}` (defaults to `exact`).

All specified criteria must match (AND). Rules are evaluated in order; the first match wins. No match → 404.

### Response

- **`status`** — HTTP status code (default 200).
- **`headers`** — response headers (defaults to `Content-Type: text/plain; charset=utf-8`).
- **`body`** / **`body_file`** — inline body, or a file read from disk on each request (mutually exclusive).
- **`delay`** — latency before responding, e.g. `500ms`, `2s`.
- **`template`** — when `true`, the body **and header values** are rendered as a Go `text/template` (`.Method`, `.Path`, `.Body`, `.Header "X"`, `.Query "k"`, `.Param "id"` for `pattern`/`regex` path captures; funcs `now`, `nowFormat`, `randomInt`, `randomString`, `counter`). A missing path param renders empty, so a `201` can carry `Location: /users/{{.Param "id"}}`.

### Sequenced responses

Give a rule an ordered `responses:` list instead of a single `response:` to return a different response on each successive matching request (the last one sticks) — for polling, retry, and pagination scenarios:

```yaml
rules:
  - id: create-job      # required: the sequence position is keyed by id
    request: {method: GET, path: /jobs/1}
    responses:
      - {status: 202, body: '{"state":"pending"}'}
      - {status: 202, body: '{"state":"pending"}'}
      - {status: 200, body: '{"state":"done"}'}
```

Each element is a full response (its own status, headers, body/`body_file`, `delay`, `template`). `responses:` and `response:` are mutually exclusive, and the rule **must** have an explicit `id:` — the position is tracked per-rule by id and preserved across a `SIGHUP` reload, so an id-less rule (whose id is minted fresh on load) is rejected. Reset the position with `m.Reset()` (library) or `POST /__admin/reset`.

You can also build a sequence in the Web UI: on a rule's Response tab, **+ add response** turns a single response into a list (reorder with ↑/↓, delete back to one to revert). UI-created rules get a stable id automatically. Editing the list does not rewind a running sequence — use Reset for that.

## Hot reload

Running **without** `--ui`, send `SIGHUP` to reload the rules from the file without a restart:

```sh
kill -HUP <pid>
```

The reload is atomic and validated — an invalid file leaves the running rules unchanged. Disabled under `--ui` (the UI owns the in-memory rules). Unix only.

## Library (embed in Go tests)

Use mock-server in-process as a test double:

```sh
go get github.com/flipmorsch/mock-server@latest
```

```go
import "github.com/flipmorsch/mock-server/mock"

func TestCheckout(t *testing.T) {
	m := mock.StartT(t, `
rules:
  - request: {method: POST, path: /charge}
    response: {status: 200, body: '{"ok":true}'}
`)
	// StartT fatals on a bad config and auto-closes the mock at test end (t.Cleanup).

	checkout(m.URL()) // point the code under test at the mock, then exercise it

	// Assert what your code sent. On mismatch the error prints the request body and
	// names the first field that differed:
	//   POST /charge → 200
	//     body: {"amount":300}
	//     ↳ JSONBody.amount: got 300, want 500
	if err := m.VerifyMatch(mock.Match{
		Method: "POST", Path: "/charge", JSONBody: `{"amount":500}`,
	}, 1); err != nil {
		t.Error(err)
	}
}
```

`StartT(t, …)` serves the same YAML as the CLI on a random loopback port and closes
itself when the test ends; use `Start` (which returns an error) outside tests.
Helpers: `URL`, `Close`, `Reset`, `Received`, `Count`/`CountMatch`, and
`Verify`/`VerifyCalled`/`VerifyMatch`/`VerifyAtLeast`/`VerifyAtMost`. A `Match`
selects requests by `Method`, `Path`, `JSONBody` (subset — partial objects,
element-wise arrays), `Query`, and `Headers`: a non-empty value must match exactly,
an empty value asserts presence. Sensitive headers are redacted in the journal, so
they're matchable by presence only.

## Run tests

```sh
go test ./...
```
