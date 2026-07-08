# Embedded Web UI

The mock server has a Web UI for configuring Rules visually. We embedded it in the server binary (gated behind `--ui`) rather than building a separate tool that edits the YAML file.

**Why.** A separate tool would need to parse and serialize YAML, then the server re-parses it — two serialization boundaries with drift risk. A separate tool also can't offer live test probes (it can't send requests to a server that isn't running). Embedding means the UI has direct access to the running Rule set, the request journal, and the matcher — all in-process, no network indirection.

**Rejected alternative: standalone web app that reads/writes the YAML file.** Would have been simpler to build (no server modifications) but would require the user to run two processes and restart the mock server after every config change. The test probe would be impossible without coordinating with a running server.

**Rejected alternative: Electron/Tauri desktop app.** Adds a heavy build dependency and a large binary for a tool whose core audience is developers comfortable with a terminal. The server already speaks HTTP — serving HTML over HTTP is the natural extension.

**Consequence: the mock server is no longer a pure HTTP mock server.** It now has a reserved `/_ui/` path prefix and serves its own UI. This adds complexity to the request routing: the server must intercept `/_ui/` requests before rule matching.
