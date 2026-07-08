# Domain Glossary

## Rule
A mapping from an HTTP request pattern (method, URL path, headers, query parameters, body) to a pre-configured mock HTTP response. All specified criteria of a Rule must match for the Rule to trigger (AND semantics). When multiple Rules match a request, the first one defined takes precedence (first-match-wins).


All match dimensions, latency simulation, and dynamic responses are implemented. TLS and hot reload remain deferred.

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

## Web UI
A graphical interface for managing Rules and server configuration, embedded in the mock server binary. Activated with the `--ui` CLI flag; disabled by default. The UI is served under the `/_ui/` path prefix, which is a reserved namespace — user-defined Rules never match requests to `/_ui/` paths.

### Rule Management
The UI supports creating, reading, updating, duplicating, deleting, and reordering Rules (drag-and-drop). Changes are held in-memory as a working copy until the user explicitly saves.

### Save
An explicit user action that writes the current working copy to the YAML configuration file on disk and immediately updates the running Rule set to match. A failed Save leaves the running Rule set unchanged and shows the error inline in the UI. Unsaved changes trigger a browser warning before navigating away.

### Rule Identity
Each Rule carries an auto-generated UUID (`id` field), assigned on creation and persisted into the YAML file on save. Rules are addressed by UUID in the API.

### Test
Two test modes per Rule:
- **Dry-run** — evaluates whether the Rule would match a user-supplied request (method, path, headers, body). No actual HTTP request is sent.
- **Probe** — sends a real HTTP request to the mock server's own listener and displays the mock response (or 404 if the Rule doesn't match).

### Request Journal
A live log of recent HTTP requests visible in the UI, stored as an in-memory ring buffer of the last 200 requests. Updates in real-time. No persistence.

### Validation
Rule fields are validated on blur (per-field) and at Save time (cross-field). Invalid fields show inline errors. The server validates the entire configuration on Save and rejects invalid saves.

### Save Behavior
Save serializes the entire in-memory state to the YAML file (full rewrite). Formatting of the original file is not preserved. The `listen` address is written to the file on save, but the server stays bound to its original address until restart; the UI notifies the user when a restart is needed.

If the config file does not exist at startup with `--ui`, the server starts with an empty Rule set. The first Save creates the file.

External modifications to the config file while the server is running are not detected. The next Save overwrites them.

### Template Preview
A preview panel renders the Rule's template body against user-supplied sample data (method, path, headers, body), available when `template: true` is set. Default sample: `GET /sample` with empty body and headers.

### Technology
Built with server-rendered HTML using [Templ](https://templ.guide) components and [htmx](https://htmx.org) for interactivity. Drag-and-drop reordering uses [Alpine.js](https://alpinejs.dev). Styling via Tailwind CSS (standalone CLI, no JS build). All static assets are embedded via `//go:embed`.

### API
The UI communicates with the server through REST endpoints under `/_ui/api/` returning JSON and htmx partial endpoints under `/_ui/partials/` returning HTML fragments. The server uses Go 1.22+ `net/http` enhanced ServeMux for routing (no external router dependency).

## Configuration
Rules are defined in a YAML configuration file. The server reads this file at startup and writes to it when the user saves via the Web UI. The `listen` address is configurable both in the file and through the UI.

The server configuration itself is minimal: a listen address and port (default `127.0.0.1:8080`). No TLS.


### CLI
```
mock-server [--listen addr:port] [--ui] <config.yaml>
```
The config file path is required. `--listen` overrides the listen address from the config file. `--ui` activates the embedded Web UI. Standard `--help` and `--version` flags are supported.

The server logs to stdout: a startup line with the listen address, and one line per request (method, path, status, matched Rule or "no match"). Errors are logged to stderr.

Rules are fixed at startup from the config file, but may be mutated at runtime via the Web UI. Restart the server to discard unsaved UI changes.

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
