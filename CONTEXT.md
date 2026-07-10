# Domain Glossary

## Rule
A mapping from an HTTP request pattern (method, URL path, headers, query parameters, body) to a pre-configured mock HTTP response. All specified criteria of a Rule must match for the Rule to trigger (AND semantics). When multiple Rules match a request, the first one defined takes precedence (first-match-wins).


All match dimensions, latency simulation, dynamic responses, hot reload, TLS, and an embeddable Go library are implemented (see ADR-0004, ADR-0005, ADR-0006).

### Path Matching
A Rule's URL path can be matched in one of four modes, chosen per-Rule:
- **Exact** — the request path must equal the Rule's path exactly.
- **Prefix** — the request path must equal the Rule's path, or start with the Rule's path followed by `/` (path-segment prefix).
- **Regex** — the request path must match the Rule's regular expression (validated at startup). Named capture groups (`(?P<id>…)`) are exposed to the response template.
- **Pattern** — the request path is matched against a path template in which each `{name}` placeholder matches exactly one path segment (`[^/]+`), mirroring `net/http` ServeMux wildcards and OpenAPI path templating. `/users/{id}` matches `/users/42` but not `/users/42/edit` or `/users`. Placeholder names must be well-formed and unique (validated at startup); the captured values are exposed to the response template as `{{.Param "id"}}`.

### Header and Query Matching
Headers and query parameters use exact value matching. All specified key-value pairs must match (AND semantics). Extra query parameters in the request are ignored; extra headers are ignored.

### Body Matching
Body matching supports three modes, chosen per-Rule:
- **Exact** — the request body must equal the Rule's value exactly.
- **Contains** — the request body must contain the Rule's value as a substring.
- **JSON** — the request body, parsed as JSON, must contain the Rule's value as a JSON subset: object fields in the value must be present in the body (extra fields ignored, recursively), arrays must match element-wise, scalars must be equal.
Defaults to exact if mode is omitted.

### Response
A Rule's response specifies an HTTP status code, optional headers, and a body (inline `body` or via `body_file`, mutually exclusive). If no `Content-Type` header is specified, the server sets `Content-Type: text/plain; charset=utf-8`.

`body_file` is checked for readability at startup and read from disk each time the response is served — edits to the file take effect without restart, and Save preserves the reference rather than inlining the content. If the file becomes unreadable at serve time, the server responds 500.

### Sequenced Responses
A Rule may specify an ordered list of Responses (`responses`) instead of a single Response (`response`); the two are mutually exclusive. The Nth request that matches the Rule receives the Nth Response in the list; once the list is exhausted, every further match receives the last Response ("last one sticks"). This models a resource that changes over successive calls — a job that returns 202 while pending then 200 when ready, an endpoint that fails 500 once then succeeds, paginated results. Each Response in the list is a full Response (own status, headers, body, delay, template).

A Rule with sequenced responses must carry an explicit `id`, because the sequence position is tracked per-Rule keyed by that id, and the position is preserved across a hot reload — a Rule without a stable id would have its position silently reset. Matching itself remains stateless: the sequence position never affects which Rule matches, only which Response that Rule returns. Resetting a Rule's sequence position (back to the first Response) happens only as part of an explicit Reset for test isolation, never as a side effect of reload.

### Latency
A Rule may include a `delay` field (e.g. `500ms`, `2s`) to simulate network latency. The server sleeps for the specified duration before writing the response.

### Dynamic Responses
A Rule with `template: true` processes its body through Go's `text/template`. Available data: `{{.Method}}`, `{{.Path}}`, `{{.Body}}`, `{{.Header "X"}}`, `{{.Query "k"}}`, and `{{.Param "id"}}` (a path parameter captured by a `pattern` or `regex` path; a missing name renders empty). Custom functions: `now`, `nowFormat`, `randomInt`, `randomString`, `counter`, `requestCount` (how many recorded requests match an optional method/path — sound beyond the Journal's retained window). When `template: true`, the response's header values are processed the same way (header keys are not), so a `Location` header can echo a path parameter. Without the template flag, the body is served as a literal string.

### Unmatched Requests
Requests matching no Rule receive HTTP 404 with no body. Users can simulate a default response by placing a Rule with no match criteria at the end of their Rule list.

## Go Library
`mock-server` embeds in Go tests as an in-process test double via the `mock` package (`github.com/flipmorsch/mock-server/mock`). It serves the same YAML config as the CLI on a random loopback port, and the request Journal backs its assertions. The in-process mock serves only mock Rules — it never exposes the `/__admin/` API or the `/_ui/` console, so its Journal stays in-process.

### Construction
- `StartT(t, yaml)` — the test constructor. It parses the config, failing the test via `t.Fatalf` if the config is invalid, and registers `t.Cleanup` so the mock closes automatically at test end (a forgotten close cannot leak a goroutine or port). `t` is any value satisfying a minimal `TB` interface (`Helper`/`Fatalf`/`Cleanup`), which `*testing.T` does; the package does not import `testing`.
- `Start(yaml) (*Server, error)` — the non-test form; the caller closes it with `Close()`.

### Assertions
`Received()` returns the recorded requests, each carrying the response the mock gave. `Count(method, path)` and `CountMatch(Match)` tally matches; `Verify`, `VerifyCalled`, `VerifyMatch`, `VerifyAtLeast`, and `VerifyAtMost` assert a count and return an error otherwise. A `Match` selects requests by `Method`, `Path`, `JSONBody` (JSON subset, as in Body Matching above), `Query`, and `Headers`: a non-empty query/header value must match exactly, an empty value asserts presence only. A method/path-only count uses monotonic tallies (sound beyond the Journal's retained window); adding a query, header, or body dimension scopes the count to that window.

The sensitive headers redacted in the Journal (`Authorization`, `Proxy-Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, `X-Auth-Token`) are stored as `[REDACTED]`, so a header `Match` on them can assert presence (empty value) but never their real value.

### Self-diagnosing failures
A failed `Verify*` lists every received request with its (truncated) body. When the `Match` carried a `JSONBody`, each method/path-matching request is additionally annotated with the first JSON path that differed and its got/want values (`↳ JSONBody.amount: got 300, want 500`) — the near-miss diagnostic identity applied inside a Go test.

`Reset()` clears the received requests and rewinds every response sequence to its first element, for isolation between subtests.

## Web UI
A graphical interface for managing Rules and server configuration, embedded in the mock server binary. Activated with the `--ui` CLI flag; disabled by default. The UI is served under the `/_ui/` path prefix, which is a reserved namespace — user-defined Rules never match requests to `/_ui/` paths.

### Rule Management
The UI supports creating, reading, updating, duplicating, deleting, and reordering Rules (drag-and-drop). Changes are held as a working copy in the browser (the authoring island) until the user explicitly saves; unsaved edits do not survive a page reload. See ADR-0010.

### Save
An explicit user action that writes the current working copy to the YAML configuration file on disk and immediately updates the running Rule set to match. A failed Save leaves the running Rule set unchanged and shows the error inline in the UI. Unsaved changes trigger a browser warning before navigating away.

### Rule Identity
Each Rule carries an auto-generated RFC-4122 UUID (`id` field), assigned on creation and persisted into the YAML file on save. Rules are addressed by UUID in the API. IDs are compared as opaque strings — legacy IDs in older formats remain valid.

### Test
Two test modes per Rule:
- **Dry-run** — evaluates whether the Rule would match a user-supplied request (method, path, headers, body). No actual HTTP request is sent.
- **Probe** — sends a real HTTP request to the mock server's own listener and displays the mock response (or 404 if the Rule doesn't match).

### Request Journal
A live log of recent HTTP requests visible in the UI, stored as an in-memory ring buffer of the last 200 requests. Updates in real-time. No persistence. Each entry also records the response the mock returned (status and body), and — when a sequenced Rule matched — which response in the sequence served that request. Sensitive request headers (`Authorization`, `Cookie`, API keys) are redacted. Request counts are tracked with monotonic tallies, so they stay accurate beyond the 200-entry display window.

### Match Explanation
A per-request diagnostic answering "why did this request get this response." For an unmatched request, it shows the Rules that came closest to matching, ranked by closeness, each with the exact match dimension that failed (expected vs. actual value). For a matched request, it identifies the winning Rule; the verdicts of earlier skipped Rules are available on demand (diagnosing an early broad Rule shadowing a later specific one under first-match-wins). Explanations appear on Journal entries and in dry-run results.

Two actions close the loop from a Match Explanation:
- **Jump-to-rule** — a near-miss links to its Rule, opening it for editing with the failing criterion highlighted.
- **Rule-from-request** — an unmatched Journal entry can seed a new Rule, pre-filled from the captured request.

### Validation
Rule fields are validated on blur (per-field) and at Save time (cross-field). Invalid fields show inline errors. The server validates the entire configuration on Save and rejects invalid saves.

### Save Behavior
Save serializes the entire in-memory state to the YAML file (full rewrite). Formatting of the original file is not preserved. The saved file contains only fields the user set — unset optional fields and internal derived values are omitted, so the output matches the documented Config File Structure. The `listen` address is written to the file on save, but the server stays bound to its original address until restart; the UI notifies the user when a restart is needed.

If the config file does not exist at startup with `--ui`, the server starts with an empty Rule set. The first Save creates the file.

External modifications to the config file while the server is running are not detected. The next Save overwrites them.

### Template Preview
A preview panel renders the Rule's template body against user-supplied sample data (method, path, headers, body), available when `template: true` is set. Default sample: `GET /sample` with empty body and headers.

### Template Autocomplete
An interactive suggestion dropdown that appears in response body textareas when the user types `{{`. After `{{` it shows all available template variables and functions; after `{{.` it shows only dot-prefixed field accessors. Selecting an item inserts the full tag with argument placeholders (e.g. `{{.Header "name"}}`), with the first placeholder selected for immediate replacement. The popup is positioned below the textarea, navigable by arrow keys, Enter, Tab, or click, and dismissed with Esc. It re-evaluates on every keystroke — appearing, filtering, or dismissing based on cursor context within `{{ }}` delimiters. Filtering is prefix-match. The static tag reference text is replaced by a brief "Type `{{` for autocomplete" hint.

### Technology
The UI is split into two surfaces. The **observation surface** (request journal, match explanations, shell, nav) is server-rendered [Templ](https://templ.guide), with the live journal fed by Server-Sent Events. The **authoring surface** (rule list + rule editor) is a reactive [Vue 3](https://vuejs.org) island loaded buildless via an ESM importmap — no bundler, no Node; `templ generate` remains the only build step. The two communicate through DOM `CustomEvent`s on `document.body`. Styling is a hand-written embedded stylesheet (no CSS framework). All assets are embedded via `//go:embed` into the single static binary. See ADR-0009.

### API
The authoring island seeds itself from `GET /_ui/api/rules` (JSON) and persists via `POST /_ui/api/save` (the whole working copy as JSON); dry-run, probe, and template-preview are JSON round-trips to the same server-side engines. The observation surface is server-rendered, updated live by a Server-Sent Events stream at `/_ui/api/events`; its cross-surface actions (jump-to-rule, rule-from-request) are dispatched as DOM `CustomEvent`s. The programmatic JSON API lives under `/__admin/`. The server uses Go 1.22+ `net/http` enhanced ServeMux for routing (no external router dependency).

## Configuration
Rules are defined in a YAML configuration file. The server reads this file at startup and writes to it when the user saves via the Web UI. The `listen` address is configurable both in the file and through the UI.

The server configuration itself is minimal: a listen address and port (default `127.0.0.1:8080`). TLS is configured through CLI flags, not the config file (see TLS below).

### TLS
The server serves HTTPS when TLS is enabled. `--tls` enables it; with no certificate supplied, the server generates an ephemeral self-signed certificate at startup and logs its SHA-256 fingerprint. `--tls-cert` and `--tls-key` supply a certificate/key pair (given together) and imply `--tls`. The listener is either HTTP or HTTPS — never both, and there is no HTTP→HTTPS redirect. The Web UI and `/__admin/` API inherit TLS on the same listener. Enabling TLS turns on HTTP/2 (via ALPN). TLS is a bind-time concern: it is not affected by hot reload and requires a restart to change. See ADR-0005.

### CLI
```
mock-server [--listen addr:port] [--ui] [--tls] [--tls-cert file --tls-key file] <config.yaml>
```
The config file path is required. `--listen` overrides the listen address from the config file. `--ui` activates the embedded Web UI. `--tls` / `--tls-cert` / `--tls-key` configure HTTPS (see TLS above). Standard `--help` and `--version` flags are supported.

The server logs to stdout: a startup line with the listen address, and one line per request (method, path, status, matched Rule or "no match"). Errors are logged to stderr.

Rules are fixed at startup from the config file, but may be mutated at runtime via the Web UI. Restart the server to discard unsaved UI changes.

### Hot Reload
When the server runs without the Web UI, sending it a `SIGHUP` signal reloads the Rule set from the configuration file without a restart. The reload is atomic and validated: the file is re-read and validated in full, and only a valid file replaces the running Rules — an invalid file leaves the running Rules unchanged. The `listen` address is not affected (rebinding requires a restart), and the request Journal is preserved across a reload. Hot reload is a Unix convenience: `SIGHUP` is not delivered on Windows.

With the Web UI enabled (`--ui`), hot reload is disabled — the in-memory working copy owns the Rule set, so `SIGHUP` is ignored rather than reloading or terminating the server. See ADR-0004.

Invalid configuration causes the server to exit immediately with a descriptive error message to stderr. No partial startup.
Implemented in Go, distributed as a single static binary.
The tool is a standalone CLI binary and also embeds as a Go library (see Go Library above).

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
