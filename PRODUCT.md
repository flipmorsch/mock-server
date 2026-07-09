# Product

## Register

product

## Users

Developers building and debugging HTTP integrations — terminal-comfortable engineers who mock external/internal APIs while working on backend services. They live in the debug loop: watch traffic → understand why a request did or didn't match → fix a rule → re-test. Density and speed matter more than hand-holding.

## Product Purpose

`mock-server` serves mock HTTP responses from a YAML config, and (with `--ui`) exposes a live debug console. It also embeds in Go tests as a library. Success is the debug loop being fast and the "why didn't my request match?" question always having an answer on screen. It competes with WireMock/Mockoon/Prism on the *debugging* experience, not on feature count.

## Brand Personality

Precise, dense, terminal-native. Calm under load. The interface is an instrument, not a landing page — it should read like a good TUI or a developer's console (Linear/Raycast density, monospace throughout), never like a consumer dashboard.

## Anti-references

- SaaS-cream / warm-neutral dashboards; rounded pastel consumer UI.
- Gradient-heavy or "hero-metric template" tool marketing UI.
- Spacious, low-density card grids where a dense list belongs.
- Anything that hides the traffic stream behind navigation.

## Design Principles

- **The journal is the home surface** (ADR-0003). The live request stream is always visible; the editor is subordinate to it.
- **Explain, don't just respond.** Near-miss match explanations are the moat; diagnostics are first-class, not an afterthought.
- **Density serves the expert.** Monospace, compact rows, information-rich. Don't pad for elegance at the cost of scannability.
- **The tool disappears into the task.** Earned familiarity over novelty; standard affordances, consistent vocabulary.
- **Lazy is a virtue.** Ship the smallest thing that works; no decoration that doesn't carry information.

## Accessibility & Inclusion

WCAG AA target: body/secondary text ≥4.5:1 (the dark theme's dim tier must clear it), placeholders included. Fully keyboard operable, visible focus indicators, and a `prefers-reduced-motion` path for every animation. Semantic status color is always paired with a non-color cue (method/status labels carry text, not just hue).
