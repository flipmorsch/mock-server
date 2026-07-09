# Changelog

## 1.0.0

- **TLS** — `--tls` serves HTTPS with an auto-generated self-signed certificate (SHA-256 fingerprint logged at startup), or `--tls-cert`/`--tls-key` for a provided pair. Single HTTP-or-HTTPS listener; HTTP/2 via ALPN. See ADR-0005.
- **Hot reload** — send `SIGHUP` (headless only) to atomically reload rules from the config file; an invalid file leaves the running rules unchanged. See ADR-0004.

Both were the last deferred backlog items. 1.0 marks the YAML config schema, the CLI flags, and the `/__admin/` JSON API as stable (breaking changes → 2.0); the `/_ui/` fragment endpoints are internal and exempt.
