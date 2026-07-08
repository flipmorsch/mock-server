# Full YAML Rewrite on Save

When the user saves changes through the Web UI, the server overwrites the entire YAML configuration file with a fresh serialization of the in-memory state. Original formatting, comments, and key ordering are not preserved.

**Why.** Preserving YAML formatting requires an AST-level parser that tracks whitespace and comments (like `yaml.v3` with `yaml.Node`). That parser has a different API surface than the struct-tag deserialization we use at startup. Maintaining two YAML paths — one for reading (struct-based) and one for writing (node-based with format preservation) — doubles the YAML surface area and introduces the risk of reading and writing subtly different structures.

The alternative (format-preserving save) would require either:
- Switching entirely to node-based YAML manipulation, losing struct-based ergonomics, or
- Maintaining both paths and ensuring they stay in sync

Both are higher complexity for a feature most users won't need. The primary path for mock configuration is now the UI; the YAML file becomes a persistence format, not a hand-edited artifact. A user who prefers hand-editing YAML can still do so — without `--ui`, the server behaves exactly as before.

**Consequence: users who alternate between hand-editing YAML and the UI will lose comments and formatting on every UI save.** The UI includes a warning on first save to set this expectation.
