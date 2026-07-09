# Roadmap (post-1.0)

From a grilling session: a senior panel (architect + backend + QA) proposed
independently, an adversarial pass stress-tested the result, and the three
pivotal calls below were decided by the project owner. v1.0.0 shipped the
matcher, latency, templates, `body_file`, the debug-first UI
(journal/SSE/match-explanations/probe), TLS, and SIGHUP hot reload.
**Stability contract:** the YAML schema, CLI flags, and `/__admin/` JSON API are
frozen (breaking changes → 2.0); `/_ui/` endpoints are internal and exempt.

**Guiding identity (ADR-0003):** debug-first. Every milestone should turn the
live-journal + near-miss-explanation moat into more value, not dilute it.

## Decisions (this session)

1. **Testing topology = in-process (Go tests).** The **Go library** with
   `.Verify()` on structs is the primary assertion surface; the HTTP `/__admin/`
   assertion API drops to the backlog (added only if a polyglot/CI need appears).
   This promotes the library form to the first milestone.
2. **Scope = HTTP request/response only.** gRPC, WebSocket, SSE, and streaming
   are an explicit **non-goal** (see below).
3. **Ship a v1.0.1 hardening release first**, before the milestones.

## Non-goals

- **gRPC / WebSocket / SSE / streaming.** The tool mocks HTTP request/response.
  These need transport work (HTTP/2, connection hijacking) orthogonal to the
  debug-first identity; revisit only on a concrete, named need.
- **Reopening settled decisions:** YAML-configured TLS (ADR-0005), multi-file
  Save ownership (ADR-0002), format-preserving Save (ADR-0002).

## v1.0.1 — Hardening · ✅ shipped in v1.0.1

Two live issues found while planning:
- **Count unsoundness (bug).** `requestCount` and `/__admin/requests/count` both
  scan the 200-entry journal ring buffer → counts silently cap/mislead past 200
  requests. Fix with monotonic counters independent of the display buffer.
  (`internal/rule/template.go`, `internal/server/journal.go`)
- **Secret-exposure risk.** The journal captures every request header (incl.
  `Authorization`/`Cookie`) and full bodies; `/__admin/` is unauthenticated and
  TLS is transport-only. Safe on the loopback default, but `--listen :8080` is one
  flag from exposing captured secrets and a `DELETE`-able journal. Fix with header
  redaction + a non-loopback bind warning.

## Milestones

### M1 — Go library / embeddable form · ✅ shipped in v1.1.0
The in-process answer makes this the headline. The reframed journal substrate is
folded in, because the library is its main consumer. Delivered: module rename,
`mock` package (`Start`/`URL`/`Close`/`Reset`/`Received`/`Count`/`Verify*`),
response capture, and JSON subset body matching. **Next milestone: M2.**
- **Module-path rename** `mock-server` → `github.com/flipmorsch/mock-server`
  (unblocks `import` and `go install`; safe now, no external importers).
- **Shallow, stable API** promoted out of `internal/`:
  `Start(t, rules)` / `URL()` / `Close()` / `Received()` / `Verify(...)`, random
  port. Keep the binary a thin `main` over it.
- **In-process `.Verify()` against Go structs** — `exactly` / `atLeast` /
  `atMost N` over a filter, JSON body matching (subset, vs today's raw
  exact/contains). Returns **self-diagnosing failures** via the existing near-miss
  ranking: a failed assertion names the request that came closest and the field
  that differed. This is the debug-first moat paying rent inside a Go test.
- **Substrate the library reads:** monotonic per-rule counts (from 1.0.1) and
  **response capture** (record status + response body in the journal, so
  `Received()` shows what the mock returned — also lays track for a future
  record/playback).

### M2 — Sequenced responses · M
- Per-rule ordered `responses:` list; the Nth matching request gets the Nth (last
  one sticks). Covers 202→200 polling, 500→200 retry, pagination.
- Matching stays **stateless** — `Explain`/near-miss untouched. State = a per-rule
  atomic index outside the rule set (the `counter` pattern, ADR-0004).
- **Hard requirement:** state is keyed by rule ID, but IDs are minted fresh on
  load/reload for id-less rules → **reject `responses:` rules without a stable
  explicit `id:`** (validation error), else state silently resets on SIGHUP.
  Define concurrent-hit semantics (CAS on the index).
- Reset for test isolation is now a library method (`m.Reset()`) plus an
  `/__admin/` endpoint. Defer the full named-scenario state machine until per-rule
  sequencing proves insufficient — the axes are orthogonal and compose later.

### M3 — Distribution automation · S–M
- goreleaser + GH Actions: multi-arch builds, checksums, Docker image, optional
  Homebrew tap, `go install` (module path fixed in M1). Version via ldflags.
- Turns releases into a tag-push (1.0.0 was built by hand). Zero architectural risk.

## Deferred backlog (with reasons)

- **HTTP `/__admin/` assertion API** — the out-of-process twin of M1's `.Verify()`
  (assertion endpoints returning `{satisfied, actual}`). Demoted by the in-process
  decision; the journal substrate for it lands in M1, so it's a thin add if a
  polyglot/CI harness ever needs it.
- **Record & playback proxy** — the biggest differentiator (realistic mocks from
  real traffic) but genuinely large: streaming vs the buffered `writeResponse`,
  binary/gzip bodies in text YAML, the 64KB journal body cap, upstream TLS, brittle
  generated rules, and a compliance tail (persisting captured secrets). M1's
  response-capture lays the substrate; revisit for 2.0.
- **Multi-file config** — pressing past ~30 rules, but collides with ADR-0002
  (which file does Save rewrite?). Ship headless-only `include` if demand appears.
- **Fault injection** — soft faults (jitter, truncated body, bad Content-Length)
  are cheap/additive → fold into a point release; hard/connection-level faults
  (reset, partial-write-hang) need `http.Hijacker`, unavailable under HTTP/2, so
  they're gated behind the deferred TLSNextProto HTTP/1.1 override.
- **OpenAPI import** — capped fidelity (no path-param template accessor yet),
  re-inflates the frozen schema, and Prism already owns the niche.
