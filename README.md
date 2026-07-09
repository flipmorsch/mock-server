# mock-server

A standalone CLI that serves mock HTTP responses from a YAML config file — with request matching across every dimension, latency simulation, templated responses, TLS, hot reload, and an embedded debug UI.

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
- **`path`** + **`path_mode`** — `exact` (default), `prefix` (path-segment prefix), or `regex`.
- **`headers`** / **`query`** — exact key/value matches (AND semantics; extras ignored).
- **`body`** — `{mode: exact|contains, value: ...}` (defaults to `exact`).

All specified criteria must match (AND). Rules are evaluated in order; the first match wins. No match → 404.

### Response

- **`status`** — HTTP status code (default 200).
- **`headers`** — response headers (defaults to `Content-Type: text/plain; charset=utf-8`).
- **`body`** / **`body_file`** — inline body, or a file read from disk on each request (mutually exclusive).
- **`delay`** — latency before responding, e.g. `500ms`, `2s`.
- **`template`** — when `true`, the body is rendered as a Go `text/template` (`.Method`, `.Path`, `.Body`, `.Header "X"`, `.Query "k"`; funcs `now`, `nowFormat`, `randomInt`, `randomString`, `counter`).

## Hot reload

Running **without** `--ui`, send `SIGHUP` to reload the rules from the file without a restart:

```sh
kill -HUP <pid>
```

The reload is atomic and validated — an invalid file leaves the running rules unchanged. Disabled under `--ui` (the UI owns the in-memory rules). Unix only.

## Run tests

```sh
go test ./...
```
