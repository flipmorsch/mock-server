# Domain Glossary

## Rule
A mapping from an HTTP request pattern (method, URL path, headers, query parameters, body) to a pre-configured mock HTTP response. All specified criteria of a Rule must match for the Rule to trigger (AND semantics). When multiple Rules match a request, the first one defined takes precedence (first-match-wins).


All match dimensions, latency simulation, and dynamic responses are implemented. Runtime configuration, TLS, and hot reload remain deferred.

### Path Matching
A Rule's URL path can be matched in one of three modes, chosen per-Rule:
- **Exact** — the request path must equal the Rule's path exactly.
- **Prefix** — the request path must equal the Rule's path, or start with the Rule's path followed by `/` (path-segment prefix).
- **Regex** — the request path must match the Rule's regular expression (validated at startup).

### Header and Query Matching
Headers and query parameters use exact value matching. All specified key-value pairs must match (AND semantics). Extra query parameters in the request are ignored; extra headers are ignored.

### Body Matching
Body matching supports two modes, chosen per-Rule:
- **Exact** — the request body must equal the Rule's value exactly.
- **Contains** — the request body must contain the Rule's value as a substring.
Defaults to exact if mode is omitted.

### Response
A Rule's response specifies an HTTP status code, optional headers, and a body (inline `body` or via `body_file`, mutually exclusive). If no `Content-Type` header is specified, the server sets `Content-Type: text/plain; charset=utf-8`.

### Latency
A Rule may include a `delay` field (e.g. `500ms`, `2s`) to simulate network latency. The server sleeps for the specified duration before writing the response.

### Dynamic Responses
A Rule with `template: true` processes its body through Go's `text/template`. Available data: `{{.Method}}`, `{{.Path}}`, `{{.Body}}`, `{{.Header "X"}}`, `{{.Query "k"}}`. Custom functions: `now`, `nowFormat`, `randomInt`, `randomString`, `counter`. Without the template flag, the body is served as a literal string.

### Unmatched Requests
Requests matching no Rule receive HTTP 404 with no body. Users can simulate a default response by placing a Rule with no match criteria at the end of their Rule list.

## Configuration
Rules are defined in a YAML configuration file read at server startup. No runtime API for adding or removing Rules.

The server configuration itself is minimal: a listen address and port (default `127.0.0.1:8080`). No TLS.


### CLI
```
mock-server [--listen addr:port] <config.yaml>
```
The config file path is required. `--listen` overrides the listen address from the config file. Standard `--help` and `--version` flags are supported.

The server logs to stdout: a startup line with the listen address, and one line per request (method, path, status, matched Rule or "no match"). Errors are logged to stderr.


No hot reload. Rules are fixed at startup. Restart the server to pick up config changes.

Invalid configuration causes the server to exit immediately with a descriptive error message to stderr. No partial startup.
Implemented in Go, distributed as a single static binary.
The tool is a standalone CLI binary. A library/embeddable form may follow.

### Config File Structure
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

  - name: "create user"
    request:
      method: POST
      path: /users
      path_mode: exact
      headers:
        content-type: application/json
      body:
        mode: contains
        value: '"name"'
    response:
      status: 201
      body: |
        {"id": 2, "name": "Bob"}

  - name: "large response"
    request:
      method: GET
      path: /data
      path_mode: exact
    response:
      status: 200
      headers:
        content-type: application/json
      body_file: ./fixtures/large-response.json
```
