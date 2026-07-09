# TLS: Self-Signed by Default, Provide-Your-Own, CLI-Only

`--tls` serves HTTPS on the single listener. With no cert supplied, the server generates an ephemeral self-signed certificate at startup. `--tls-cert` / `--tls-key` supply a real pair (and imply `--tls`). TLS is configured through CLI flags only — there is no YAML `tls:` block. The server is either HTTP or HTTPS, never both; there is no HTTP→HTTPS redirect.

**Why self-signed by default.** This is a dev/test mock. Forcing the user to generate a certificate just to point an HTTPS client at a local mock is friction with no payoff. Stdlib `crypto/x509` + `crypto/ecdsa` generate a valid ephemeral cert in ~40 lines with zero new dependencies. The provide-your-own branch is ~2 lines (`ServeTLS(certFile, keyFile)`) and covers the real cases the default can't — cert-pinning tests and specific SANs. Supporting both is (self-signed) + 2 lines and leaves no gap.

**Why CLI-only, not YAML.** Cert/key paths and the listener are host- and environment-specific bind-time state — the same category as `--listen`, which is already declared non-reloadable. Putting TLS in the YAML file would grow the config schema at the exact moment it is being frozen for 1.0, force the Web UI to render and manage certificate settings, and muddy the hot-reload semantics (reload swaps rules, not the listener).

**Rejected alternative: a YAML `tls:` block.** Grows the frozen 1.0 schema, drags the UI into cert management, and blurs the "reload changes rules, not the listener" line.

**Rejected alternative: user-provided cert only.** Makes every HTTPS user wrangle a certificate for a throwaway mock — the friction the self-signed default exists to remove.

**Rejected alternative: a separate plaintext port for the UI/admin.** Scope creep, plus a plaintext admin surface is a footgun. If the server is HTTPS, its UI and `/__admin/` API are HTTPS too.

**Consequences.**
- The embedded UI (`--ui`) and `/__admin/` API inherit TLS on the same listener.
- The UI's real-probe test calls the server's *own loopback listener*, so under TLS its client must switch to `https://` and use `InsecureSkipVerify` — there is no MITM surface on loopback, and this also works when a user-supplied cert's SANs don't include `127.0.0.1`.
- Enabling TLS silently turns on **HTTP/2** via ALPN (stdlib default). This is usually desirable for a mock; it can be disabled later by setting `TLSNextProto` to an empty map if HTTP/1.1 fidelity is needed.
- Self-signed trust is handled by logging the certificate's SHA-256 fingerprint at startup; clients use `-k` / skip-verify, or the provide-your-own path for proper trust.
- Adopting TLS (and the SIGHUP handler) requires replacing the bare `http.ListenAndServe` with an `http.Server` value; set `ReadHeaderTimeout` while there.
