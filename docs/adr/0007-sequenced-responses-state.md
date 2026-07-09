# Sequenced Responses: State Keyed by Explicit Rule ID, Surviving Reload

A rule may carry an ordered `responses:` list instead of a single `response:`;
the Nth matching request gets the Nth response and the last one sticks (202→200
polling, 500→200 retry, pagination). Matching stays **stateless** — the position
is a per-rule `atomic.Int64` held outside the rule set (the `counter`/journal
pattern), so `Explain` and near-miss ranking are untouched. The index is keyed by
**rule ID** and **survives SIGHUP reload**: a reload copies the counter over for
any ID that persists, mirroring ADR-0004's rule — reload "changes rules, not the
world clock," and silently rewinding a client mid-poll-loop is the worse surprise.

## Consequences

- **`responses:` rules must declare an explicit `id:`** (validation error
  otherwise). IDs are minted fresh on load for id-less rules, so without a stable
  authored ID the sequence position would silently reset on every reload. The
  check works because validation runs *before* ID minting, so an empty ID
  unambiguously means the user omitted one.
- **Concurrency is `atomic.Add` + clamp-on-read**, not CAS. Two concurrent hits
  get distinct increasing indices (no duplicate, no skip); the counter runs past
  the list length harmlessly because the read clamps to the last element. CAS
  would only be needed to *stop* incrementing at the end — unnecessary given the
  clamp.
- **The index advances at match time, before the per-element `delay` sleep**, so
  ordering follows request arrival, not sleep completion — otherwise differing
  per-element delays could scramble sequence order and break a test asserting it.
- **`responses:` and `response:` are mutually exclusive** (validation error if
  both), and `responses:` is purely additive to the frozen YAML schema — existing
  singular-response configs are unaffected.
- **`Reset()` rewinds sequences.** The library `m.Reset()` and a new
  `POST /__admin/reset` both clear the journal *and* zero every sequence counter,
  for test isolation. `DELETE /__admin/requests` stays journal-only (unchanged
  frozen contract).
- **The Web UI treats sequenced rules as read-only** — YAML-authored, not
  UI-editable in this milestone. The struct round-trip preserves the field on
  Save; a handler guard rejects edits rather than flattening the list, and a rail
  badge marks them. Full UI editing and the named-scenario state machine are
  deferred.
