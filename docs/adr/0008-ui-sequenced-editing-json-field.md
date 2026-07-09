# UI Sequenced-Response Editing: a JSON Form Field, Count-as-Mode

The Web UI edits sequenced responses (`responses:`) as well as single ones. Two
non-obvious choices make this work inside the templ/htmx/Alpine console:

- **The responses list crosses the `/_ui/` seam as one hidden JSON field**
  (`responses`), built from Alpine state on submit and `json.Unmarshal`ed in
  `ruleFromForm` — not as flattened form fields. A response element carries a
  nested variable-length `headers` map, which flat parallel-array encoding
  (`kvFromForm`'s trick) can't disambiguate across elements. JSON nests it for
  free, needs no bespoke prefix parser, and is testable with a plain form POST.
  There is precedent: `PUT /_ui/api/rules/reorder` already accepts browser JSON.
  This is a `/_ui/` internal field, exempt from the frozen YAML/API contract.
- **The count is the mode — no toggle.** A rule with one response serializes as
  singular `response:` (the existing flat form, unchanged, keeping per-field
  validation and dry-run); adding a second makes it `responses:`; deleting back
  to one restores single. A hidden `resp_mode` field (`single`/`sequence`),
  derived from the count client-side, is sent explicitly so the **server never
  infers** the mode from JSON presence — the `response`/`responses`
  mutual-exclusion invariant then holds by construction, and a stale `responses`
  blob left in a single-mode form is simply not read.

## Consequences

- **The editable sequence section is the carrier of both bug fixes.** The old
  read-only view emitted no inputs, so `hx-include="closest form"` sent nothing
  for a sequenced rule — which is why *both* edit and duplicate silently dropped
  the list. Rendering an Alpine-backed section that emits the hidden `responses`
  + `resp_mode` fixes edit and duplicate together.
- **`rule.Response` gained explicit json tags** (notably `json:"body_file"` — the
  underscore breaks encoding/json's case-fold — and `json:"-"` on the derived
  `DelayDuration`). Inert to yaml, the `/__admin/` API, and the `mock` package,
  none of which encode `Response`.
- **In sequence mode the UI forgoes per-field blur validation and dry-run
  `hx-include`** (they can't see JSON-encoded fields); the rule is validated
  whole on submit via `CheckRule`, which already checks each element.
- **The ADR-0007 id landmine is closed.** IDs are now assigned before validation
  on every write path (`CreateRule` mints→validates→commits; the update path sets
  the id before `CheckRule`), and `Save` runs `workingCopy.Check()` before the
  ID-minting `cloneConfig`. A UI-minted UUID is a stable id: it persists to disk,
  survives reload, and a duplicate gets a fresh id (hence a fresh sequence).
- **Editing a sequenced rule's list does not rewind its position** (Save reseeds
  by id, preserving the counter — consistent with ADR-0004/0007). The operator
  uses Reset for a fresh run; the editor notes this.

## Deferred

- **Live sequence position in the journal** (`→ get-job (seq 2/3)`) — genuine
  debug-first value, but orthogonal: it needs the serve path to record the
  served index on the journal entry. Separate follow-up.
- **Per-element template preview** and **drag-drop reorder** — the single-body
  preview and up/down buttons suffice; revisit on demand.
