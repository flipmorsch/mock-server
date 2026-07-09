# Hot Reload: Headless-Only, via SIGHUP

The server re-reads its YAML config and atomically swaps its rule set on `SIGHUP` — but only when running *without* `--ui`. Under `--ui`, file changes are ignored (the existing behavior) and the in-memory working copy stays the source of truth.

**Why headless-only.** With `--ui`, the running rule set can already diverge from disk — the working copy holds unsaved edits. Watching the file *as well* creates a three-way conflict (disk vs. working copy vs. running set) that can only be resolved by discarding someone's changes. Rather than build merge/prompt UX for a combination few people run intentionally, each mode keeps a single clear owner of the rule set: with `--ui`, the UI owns it; headless, the file owns it and is reloadable.

**Why SIGHUP, not a file watcher.** SIGHUP costs zero new dependencies (stdlib `os/signal`), where fsnotify would be the project's first watcher dependency. It is also correct for free: the server reloads *only when signalled*, so it never parses a half-written file. A file watcher fires on every intermediate write — editors save via multiple write syscalls or temp-file-plus-atomic-rename, so a watcher needs debounce logic *and* directory-watching (the rename breaks an inode watch). That is real code to get right, for a convenience feature. SIGHUP is also the classic Unix config-reload idiom (nginx, haproxy), and the audience is terminal-comfortable developers.

**Rejected alternative: fsnotify auto-watch.** Edit-and-it-just-applies is nicer UX, but costs one dependency plus debounce and atomic-rename handling, and risks reading a truncated file mid-save.

**Rejected alternative: watch the file even under `--ui`.** Forces conflict-resolution UX (prompt? auto-merge? reload-and-discard?) for a workflow the design already steers people away from.

**Consequences.**
- Reload is **manual** (send the signal), not automatic, and **Unix-only** — Windows never delivers SIGHUP, so reload is simply a no-op there; restart to pick up changes (the same path already required for a `listen` change).
- Reload swaps **rules only**. The `listen` address is not reloadable (rebinding requires a restart), matching the existing UI-save behavior.
- Reload is **atomic and validated**: the whole file is parsed and validated first; on any error the current rule set is kept unchanged and the error is logged. This mirrors the existing UI `Save()` swap, minus the disk write.
- The request journal and the `counter` template function's state both live outside the rule set, so an atomic swap leaves them intact. Reload changes rules, not the world clock — resetting `counter` mid-session would emit duplicate sequence values and surprise the client under test.
