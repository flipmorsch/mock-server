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
  record/playback). **Next milestone: M2.** [shipped — see M2 below]

### M2 — Sequenced responses · ✅ shipped in v1.2.0
Delivered: per-rule ordered `responses:` list (Nth match → Nth response, last
sticks), stateless matching with the position tracked per-rule outside the rule
set (`atomic.Add` + clamp, surviving reload keyed by explicit `id:`), a validation
error for id-less `responses:` rules, `Reset()`/`POST /__admin/reset` rewinding
sequences, and a read-only UI treatment. See ADR-0007. **Next milestone: M3.**
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

### M3 — In-process assertions v2 · ✅ shipped in v1.5.0
Finishes M1's unkept headline: the near-miss / debug-first moat paying rent inside a
Go test — the chosen primary surface. From a grilling session (2026-07-09); a senior
panel (architect + backend + QA) proposed independently and two of three converged here.
- **Self-diagnosing `Verify*` failures.** Today `summary()` (`mock/mock.go:182`) dumps a
  flat `METHOD PATH → status` list — the request body, the one thing a failed
  `VerifyMatch(JSONBody:…)` exists to explain, is never printed. Fix: print each received
  request's body (truncated). For `VerifyMatch`/`VerifyAtLeast`/`VerifyAtMost` with a
  `JSONBody` filter, also name the closest received request and the first JSON path that
  differed — extend `rule.JSONBodyMatches` to return the first failing path instead of a
  bool. Count-only `Verify`/`VerifyCalled` get the body list (nothing to diff).
- **Widen `mock.Match`** with `Query` and `Headers` (and body `contains`). Pure plumbing:
  `rule.RequestFilter` / `requestFilterMatch` already implement all three; `Match` just
  never exposed them. Redaction (v1.0.1) is untouched — the 5 sensitive headers stay
  `[REDACTED]`, so an empty header value = presence check and exact-value assertions on
  them are deferred (no named need). Presence semantics live in the `mock` layer so the
  **frozen** `/__admin/` filter is unchanged.
- **`StartT(t, yaml)`** — a testing-aware constructor over a minimal `TB` interface
  (`Fatalf`/`Cleanup`/`Helper`, no `testing` import): fatals on parse error and registers
  `t.Cleanup(m.Close)`, so a forgotten `Close` can't leak a goroutine/port. `Start` stays.
- Additive to the `mock` package only; touches no frozen surface (YAML / CLI / `__admin/`).
- Delivered: `JSONBodyDiff` (first differing path), body + field-diff in `Verify*`
  failures, `Match.Query`/`Match.Headers` (presence-aware, redaction-safe, matched in
  the library layer so `/__admin/` stays frozen), and `StartT(t, yaml)`.
  **Next milestone: M4.**

### M4 — Path parameters · ✅ shipped in v1.6.0
The everyday REST-mock gap: match `/users/{id}` and echo the id back — impossible today
without a rule-per-id or a hand-written regex (the deferred-backlog "no path-param
template accessor" item, promoted).
- **`path_mode: pattern`** — `{name}` matches exactly one path segment (`[^/]+`),
  mirroring `net/http` ServeMux `{name}` and OpenAPI path templating. A new arm in
  `PathMatches` → the matcher, the journal filter, and the near-miss `Explain` engine all
  handle it for free (shared signature, no call-site changes).
- **`{{.Param "id"}}`** template accessor, mirroring `r.PathValue`; reads captures from
  both `pattern` mode and `regex` named captures. Threaded
  `ServeMock` → `writeResponse` → `ExecuteTemplate`.
- **Header-value templating** when `template: true` (keys stay literal), completing the
  canonical `201 Created` + `Location: /users/{{.Param "id"}}`. A missing param renders
  empty (no 500).
- The new `path_mode` value is an **additive** schema extension — minor-version-safe, not
  a 2.0 break, consistent with how M2 added `responses:`.
- Delivered: `path_mode: pattern` (`{name}` = one segment, unique-name validation),
  `PathParams` extraction, `{{.Param "id"}}` (pattern + regex named captures), and
  response header-value templating under `template: true`. **Next milestone: M5.**

### M5 — Distribution automation · S–M · deferred (was M3; owner deferred 2026-07-09)
- goreleaser + GH Actions: multi-arch builds, checksums, Docker image, optional
  Homebrew tap, `go install` (module path fixed in M1). Version via ldflags.
- Turns releases into a tag-push (1.0.0 was built by hand). Zero architectural risk.

### M6 — Reactive authoring island (UI stack) · L · planned
From a grilling session (2026-07-09). The authoring surface outgrew htmx+Alpine
(the driver: rich client interactivity — a working copy held in reactive client
state with undo). Replace it with a **buildless Vue 3 island**; the observation
surface (journal / SSE / near-miss explanations / shell) is untouched. Non-web
(native GUI, webview) and a full SPA were rejected; buildless (no Node/bundler)
preserves the single-static-binary identity and keeps M5 simple. See ADR-0009, ADR-0010.
- **Server JSON contract.** `GET /_ui/api/rules` (seed) + `POST /_ui/api/save`
  (whole working copy → `Check` → `Validate` → write → swap `config`); dry-run /
  probe / template-preview become JSON round-trips to the same server-side engines.
  **Delete** `workingCopy` + `AddRule`/`UpdateRule`/`DeleteRule`/`ReorderRules`/
  `UpdateListen` + `ruleFromForm`/`kvFromForm`/`validateField` + the form-encoded
  editor endpoints. `/_ui/` is internal, exempt from the frozen contract.
- **Vue island.** Embed `vue.esm-browser.js`; importmap in the Templ shell; rule
  list + editor as Vue components over a `reactive()` working copy with a
  snapshot-stack undo. Client-owned draft — refresh loses unsaved edits, guarded by
  the existing `beforeunload` warning. Matcher/template/probe stay authoritative in Go.
- **Seam.** Reuse the existing `HX-Trigger` → `CustomEvent` bus: the island
  subscribes to `mock:seed-rule` / `mock:edit-rule`; `ruleFromEntry` stays server-side.
- **Delete** `alpine.min.js`; trim `app.js` to observation-surface glue; keep the
  hand-written stylesheet.

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
