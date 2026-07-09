# Embeddable Library: A Thin Public Wrapper Over internal/

The embeddable form (the `mock` package) is a small public wrapper over the
existing `internal/` packages, not a promotion of `internal/{rule,server,journal}`
to public. Rules are supplied as a YAML string (the same schema as the config
file), and verification returns errors that self-diagnose from the journal.

**Why a thin wrapper.** Promoting the internal packages would freeze their entire
surface as a public contract — the exact trap the roadmap warned against, since
those types (`rule.Rule`, `server.Server`, the journal internals) still change as
features land. A curated `mock` package (`Start`, `URL`, `Close`, `Reset`,
`Received`, `Count`/`CountMatch`, `Verify*`) exposes only what a test author
needs, leaving everything behind it free to evolve. The wrapper can import
`internal/` because both live under the same module path.

**Why YAML-string config, not a Go builder.** `Start(yaml)` reuses the single
config parser (`rule.ParseConfig`), so the library and the CLI can never drift to
two schemas. A typed Go builder for rules is more surface that would duplicate the
schema; defer it until users ask.

**Why the module was renamed.** `module mock-server` cannot be `go get`/`import`ed
or `go install`ed. The rename to `github.com/flipmorsch/mock-server` is the
prerequisite that makes both the library and `go install` possible.

**Rejected alternative: un-internalize rule/server/journal.** Maximum flexibility
for callers, but it publishes a large, churning surface as a contract and couples
every internal refactor to a major version.

**Rejected alternative: a Go builder API for rules.** Nicer typing for some, but a
second way to express rules that must be kept in sync with the YAML schema.

**Consequence: the public contract grows by one package.** The stability surface
is now the YAML schema, the CLI flags, the `/__admin/` API, **and** the `mock`
package. The `internal/` packages remain private and changeable; `/_ui/` stays
exempt.
