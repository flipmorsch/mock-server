# The Working Copy Moves Client-Side

With the authoring surface now a Vue island (ADR-0009), the **working copy** (the
unsaved edits held until Save) moves from the server into the browser. The server
previously held `workingCopy` and mutated it per htmx action; now the Vue app owns
it. The server exposes two JSON endpoints for the island — `GET /_ui/api/rules`
seeds the app, `POST /_ui/api/save` sends the whole working copy — and the
granular server mutators plus form parsing are deleted.

## Why

- It is the point of the change: optimistic in-memory editing and instant undo
  require the working copy to live where the edits happen. Server-owned would be
  htmx-over-JSON and would fight undo with a round-trip per keystroke.
- **Live matching is untouched.** The matcher serves the committed `config`, never
  the working copy — `Save()` already does `s.config = serving`. Moving the draft
  client-side therefore cannot change what the running server matches.
- **It is a net deletion.** `workingCopy` + `AddRule`/`UpdateRule`/`DeleteRule`/
  `ReorderRules`/`UpdateListen` + `ruleFromForm`/`kvFromForm`/`validateField` + the
  form-encoded editor endpoints collapse into seed + save. The existing `Save()`
  write-path (validate → write file → swap `config`) is reused, fed JSON instead
  of accumulated mutations.

## Consequences

- **Unsaved edits no longer survive a browser refresh or share across tabs** — the
  server no longer holds them. The existing `beforeunload` unsaved-changes warning
  guards accidental loss; `sessionStorage`-backing the draft is a small later add
  if it proves painful. Accepted for a local single-developer debug tool.
- **Server-side validation stays authoritative on Save.** The client does cheap
  per-field validation for instant feedback only; `POST /_ui/api/save` runs the
  same `Check`/`Validate` as today and returns errors as JSON. A failed save
  leaves the committed `config` unchanged (unchanged from today).
- `ruleFromEntry` (rule-from-request) stays server-side; its output reaches the
  client working copy through the ADR-0009 seam (an event payload), not an HTML
  fragment.
