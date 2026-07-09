# Changelog

## 1.0.1

- **Fix:** request counts (`requestCount` template func and `/__admin/requests/count`) were capped by the 200-entry journal ring buffer and silently undercounted past 200 requests. Total / method / exact-path counts are now sound via monotonic tallies, independent of the display window.
- **Security:** sensitive request headers (`Authorization`, `Cookie`, API keys) are redacted in the journal and `/__admin/` API; the server warns at startup when binding to a non-loopback address, since the journal and admin API are unauthenticated.

## 1.0.0

- **TLS** — `--tls` serves HTTPS with an auto-generated self-signed certificate (SHA-256 fingerprint logged at startup), or `--tls-cert`/`--tls-key` for a provided pair. Single HTTP-or-HTTPS listener; HTTP/2 via ALPN. See ADR-0005.
- **Hot reload** — send `SIGHUP` (headless only) to atomically reload rules from the config file; an invalid file leaves the running rules unchanged. See ADR-0004.

Both were the last deferred backlog items. 1.0 marks the YAML config schema, the CLI flags, and the `/__admin/` JSON API as stable (breaking changes → 2.0); the `/_ui/` fragment endpoints are internal and exempt.
