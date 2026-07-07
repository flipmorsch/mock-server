# mock-server

A standalone CLI tool that serves mock HTTP responses based on a YAML configuration file.

## Install

```sh
go build -o mock-server .
```

## Usage

```sh
mock-server [--listen addr:port] <config.yaml>
```

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
      path_mode: exact
    response:
      status: 200
      body: OK
```

- **`method`** — HTTP method to match (case-insensitive).
- **`path`** — URL path to match.
- **`path_mode`** — `exact` (default). Future: `prefix`, `regex`.
- **`response.status`** — HTTP status code.
- **`response.headers`** — Response headers (optional).
- **`response.body`** — Inline response body (mutually exclusive with `body_file`).
- **`response.body_file`** — Path to a file whose contents are used as the response body. Read at startup.

Multiple rules are evaluated in order. The first matching rule wins. If no rule matches, the server returns 404.

## Run Tests

```sh
go test ./...
```
