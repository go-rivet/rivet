# Lua

This package embeds Lua 5.4.9 from [lua/lua](https://github.com/lua/lua/tree/v5.4.9)
(commit `312b9efaa1061c2c4cad08554dbc1351c3270eef`). The Lua C sources are
compiled directly by cgo; no platform-specific prebuilt libraries are needed.

The unmodified upstream C sources and headers are kept in `src/`, with the
library amalgamation included by a small cgo-only C file. This keeps the
vendored source isolated and lets non-cgo builds use an explicit unsupported
stub. The package exposes state creation, cleanup, and a short-lived locked
access callback for higher-level Lua integrations.
