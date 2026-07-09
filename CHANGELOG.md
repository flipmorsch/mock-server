# Changelog

## 1.3.0

- **Edit sequenced responses in the Web UI.** The response editor gains an ordered, editable list: add a second response to make a rule sequenced, reorder with up/down, delete back to one to return to a single response. Sequenced rules are no longer read-only in the UI. See ADR-0008.
- **Fix:** duplicating a sequenced rule silently dropped its `responses` list (the editor form never carried it), producing a single-response copy. Duplicate now preserves the full sequence (with a fresh id, hence a fresh position).
- Internally, the responses list crosses the UI boundary as one hidden JSON field, and IDs are now assigned before validation on every write path — which also closes a latent gap where an id-less sequenced rule could bypass the "explicit id" guard on save (noted in ADR-0007).

## 1.2.0

- **Sequenced responses** — a rule may carry an ordered `responses:` list instead of a single `response:`; the Nth matching request gets the Nth response and the last one sticks. Covers 202→200 polling, 500→200 retry, and pagination. Matching stays stateless; the position is tracked per-rule outside the rule set and survives `SIGHUP` reload. See ADR-0007.
- **Sequenced rules require an explicit `id:`** — the position is keyed by id, so a rule without a stable id would silently reset on reload; this is now a validation error. `response:` and `responses:` are mutually exclusive.
- **Reset rewinds sequences** — `m.Reset()` and the new `POST /__admin/reset` clear the journal *and* rewind every sequence to its first response (test isolation). `DELETE /__admin/requests` is unchanged (journal-only).
- The Web UI shows sequenced rules read-only (they're YAML-authored); editing one there is rejected rather than flattening the list.

## 1.1.0

- **Embeddable Go library** (`github.com/flipmorsch/mock-server/mock`) — run the mock in-process from Go tests: `mock.Start(yaml)` on a random loopback port, `URL`/`Close`/`Reset`/`Received`, and `Verify`/`VerifyCalled`/`VerifyMatch`/`VerifyAtLeast`/`VerifyAtMost`/`Count`/`CountMatch`. Failed assertions list what was actually received. See ADR-0006.
- **Module renamed** to `github.com/flipmorsch/mock-server` (enables `go get` and `go install`).
- **JSON subset body matching** — request bodies can be matched with `mode: json` (rules) or `Match{JSONBody: ...}` (library): partial objects, element-wise arrays, equal scalars.
- **Response capture** — the journal now records the status and body the mock returned (visible in `/__admin/requests` and `Received()`).

## 1.0.1

- **Fix:** request counts (`requestCount` template func and `/__admin/requests/count`) were capped by the 200-entry journal ring buffer and silently undercounted past 200 requests. Total / method / exact-path counts are now sound via monotonic tallies, independent of the display window.
- **Security:** sensitive request headers (`Authorization`, `Cookie`, API keys) are redacted in the journal and `/__admin/` API; the server warns at startup when binding to a non-loopback address, since the journal and admin API are unauthenticated.

## 1.0.0

- **TLS** — `--tls` serves HTTPS with an auto-generated self-signed certificate (SHA-256 fingerprint logged at startup), or `--tls-cert`/`--tls-key` for a provided pair. Single HTTP-or-HTTPS listener; HTTP/2 via ALPN. See ADR-0005.
- **Hot reload** — send `SIGHUP` (headless only) to atomically reload rules from the config file; an invalid file leaves the running rules unchanged. See ADR-0004.

Both were the last deferred backlog items. 1.0 marks the YAML config schema, the CLI flags, and the `/__admin/` JSON API as stable (breaking changes → 2.0); the `/_ui/` fragment endpoints are internal and exempt.
