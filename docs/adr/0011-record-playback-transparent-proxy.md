# Record & Playback: transparent proxy generates rules from real traffic

Recording mode turns the mock server into a transparent HTTP proxy. Requests arrive
at the mock, forward to an upstream, responses are captured, and a Rule is generated
and appended to an output file. When recording is off, the mock serves the generated
rules as normal — the user bootstraps realistic mocks from real traffic, then
refines by hand.

Recording is a **distinct operational mode** enabled by `--record`. It ignores the
rule set entirely: every request proxies. The journal still records exchanges, so
the debug-first moat is visible during capture. The generated rules use the
existing frozen YAML schema — this is a pure addition, no breaking changes (v1.8.0).

## Decisions

- **Transparent proxy, not passive observer.** The client must point at the mock
  instead of the real upstream. No certificate injection, no port mirroring — the
  mock is already an HTTP server.
- **One rule per exchange, no smart merging.** `/users/1` and `/users/42` produce
  two rules. Path-parameter detection and template merging are deferred until a
  concrete need. The value is fast bootstrapping, not perfect rules.
- **Text-only body capture.** `text/plain`, `text/html`, `application/json`,
  `application/xml` are captured inline. Binary content types are flagged
  `[binary, N bytes]` and skipped — avoids the YAML-binary-inlining problem and
  keeps the config file readable.
- **Method + path matching only.** Generated rules do not include request body,
  header, or query matching. Body matching in particular would require heuristics
  (exact vs. subset vs. contains) that are wrong more often than right; the user
  adds match dimensions during refinement.
- **Append to output file.** `--record-output` specifies a file; rules are appended
  and flushed on each capture. Crash-safe, and the user can watch the file grow.
- **Hop-by-hop headers stripped.** `Transfer-Encoding`, `Connection`, `Keep-Alive`,
  `Proxy-*`, `TE`, `Trailer`, `Upgrade` are stripped from captured responses —
  they're transport artifacts that break on replay.
- **Redacted headers stay redacted.** `Authorization`, `Cookie`, `Proxy-Authorization`,
  `Set-Cookie`, `X-Api-Key`, `X-Auth-Token` are stripped from captured responses —
  same invariant as the journal.
- **CLI-only for MVP.** Recording works headlessly. The UI's journal still shows
  recorded exchanges; no UI controls for start/stop recording until a named need
  appears.

## Considered Options

- **Match-first, proxy-on-miss:** Recording as a supplement to an existing rule set.
  Rejected — more complex to implement and explain. A distinct mode keeps the
  mental model clean.
- **Full UI integration:** "Start Recording" button, upstream URL field, live rule
  count. Rejected for MVP — recording is a power-user feature, and the journal
  already shows the traffic.
- **Binary body capture (base64 or body_file):** Rejected for MVP. Text covers
  90%+ of REST traffic; binary capture can be added when a concrete need appears.
- **Body matching in generated rules:** Rejected — wrong more often than right.
  Timestamps, random tokens, and request IDs make exact-match-generated rules
  useless out of the box. The user adds body matching during refinement.
