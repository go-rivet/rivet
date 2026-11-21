# Rivet

[![CI](https://github.com/go-rivet/rivet/actions/workflows/ci.yml/badge.svg)](https://github.com/go-rivet/rivet/actions/workflows/ci.yml)
[![Docker](https://github.com/go-rivet/docker/actions/workflows/build-and-push.yml/badge.svg)](https://github.com/go-rivet/docker/actions/workflows/build-and-push.yml)
[![E2E](https://github.com/go-rivet/rivet/actions/workflows/e2e.yml/badge.svg)](https://github.com/go-rivet/rivet/actions/workflows/e2e.yml)
[![GitHub release](https://img.shields.io/github/v/release/go-rivet/rivet?color=blue)](https://github.com)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)][license-url]


**Rivet** is a minimal, fast task runner written in Go with an **embedded Lua scripting engine**. Full documentation is available at **[docs.go-rivet.dev](https://docs.go-rivet.dev/)**.

## Quickstart

Create a `Taskfile.yml` and run `rivet greet`:

```yaml
version: 42
vars:
  NAMES: [Ada, Linus, Grace]
tasks:
  greet:
    cmds:
      - lua: |
          for _, name in ipairs(ctx.vars.NAMES) do
            if name ~= "Linus" then
              rivet.task("hello", {NAME = name})
            end
          end
  hello:
    cmds:
      - echo "Hello, {{.NAME}}!"
```


## Why Rivet?

* **Embedded Lua VM:** Write loops and logic directly in Lua instead of complex YAML templates and shell scripts.
* **Single Static Binary:** Standalone executable with zero runtime dependencies.
* **Minimal Overhead & Container Ready:** Built for speed with official images available from GitHub Container Registry.


## Installation

* **Go Install:** `go install github.com/go-rivet/rivet/cmd/rivet@latest`
* **Automated Script (Linux):** `curl -sSL https://raw.githubusercontent.com/go-rivet/rivet/main/install.sh | sh`
* **Prebuilt Binaries:** Available for Linux and Windows on [GitHub Releases](https://github.com/go-rivet/rivet/releases).
* **Docker:** Pull from `ghcr.io/go-rivet/rivet:latest` or `ghcr.io/go-rivet/rivet:latest-alpine`.


## Development

Use `make` commands for building and testing:

```bash
# Setup a development environment.
make deps

# Build Rivet.
make build
make install

# Release build (Linux and Windows).
make release

# Development commands.
make test
make test-all
make lint

# Other make commands:
make help
```


## License

This project is licensed under the MIT License. 

* Copyright (c) 2026 Timothy Rule (Modifications and current project)
* Copyright (c) 2016 Andrey Nering (Original base project)

See the [LICENSE][license-url] file for the full license text.


[task-url]: https://taskfile.dev
[license-url]: LICENSE
