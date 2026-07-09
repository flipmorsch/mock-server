# Reactive Authoring Island: buildless Vue 3 replaces htmx+Alpine for editing

The Web UI's **authoring surface** (rule list + rule editor) outgrew htmx+Alpine:
holding the whole working copy in reactive client state with undo and live
derivation is past Alpine's ceiling. We replace it with a **reactive island** — a
[Vue 3](https://vuejs.org) app mounted onto the Templ-rendered shell — while the
**observation surface** (live journal SSE stream, near-miss match explanations,
shell/nav) stays server-rendered Templ + htmx. Vue loads **buildless** via an ESM
importmap over `//go:embed`'d assets; `templ generate` stays the only build step
and the single static binary is preserved.

## Why this shape

- The driver is rich *client* interactivity, and a browser is already the richest
  client-reactive runtime there is — the capability gap is Alpine's, not the web's.
- **Islands, not a full SPA.** The journal is the debug-first moat (ADR-0003), it
  is read-only, and htmx+SSE serve it well. The clean seam is authoring (stateful,
  client-reactive) vs. observation (streaming, server-rendered). An SPA would
  rebuild the moat for no user gain.
- **Buildless** keeps the shipped identity (README/PRODUCT.md: "single static
  binary, no build step beyond `templ generate`") and keeps the upcoming M5
  goreleaser pipeline simple. Vue officially supports both CDN/importmap and
  Vite+SFC, so adding a build step later is a config change, not a re-framework.
- **Vue over Preact/Lit** for a form-dense editor: `v-model` collapses the
  request/response/header/query/sequence fields, `reactive()` + a snapshot stack
  gives undo cheaply, `computed` drives validation — and it is the largest
  ecosystem to grow into.

## Considered Options

- **Full web SPA (Go as a JSON API).** Retires htmx+Templ+the fragment API and
  rebuilds the journal moat; forces a bundler and a `dist/` embed. Rejected — cost
  without benefit for two-thirds of the UI that already works.
- **Non-web (native Go GUI via Gio/Fyne; webview via Wails/Tauri).** The audience
  runs the mock over SSH, in containers, next to CI, where a web UI port-forwards
  trivially and a native window cannot follow. Native also breaks single-binary
  cross-compile (CGo/GL) and does not solve the interactivity driver any better
  than a browser. ADR-0001 already rejected desktop as too heavy.
- **Bundler build (Svelte/Solid/Vue-SFC).** Better DX, but adds Node to the
  release pipeline, a `dist/` to embed, and retires a documented promise — for
  ergonomics not yet needed.

## Consequences

- `alpine.min.js` is removed; **htmx stays** for the observation surface. The
  cross-surface seam reuses the existing `HX-Trigger` → `CustomEvent` bus: the Vue
  island subscribes on `document.body` (e.g. `mock:seed-rule`, `mock:edit-rule`)
  rather than introducing a new transport.
- Matcher, `text/template` render, dry-run, and probe **stay server-side** (Go
  owns them); the Vue app orchestrates them over JSON and renders results. No
  matcher/template logic is duplicated in JS.
- `/_ui/` remains internal and exempt from the frozen YAML/CLI/`__admin` contract,
  so its endpoints can return JSON instead of HTML fragments without a 2.0 break.
- Working-copy ownership moves client-side as a direct consequence — see ADR-0010.
