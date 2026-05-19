# Rivet

[![CI](https://github.com/go-rivet/rivet/actions/workflows/ci.yml/badge.svg)](https://github.com/go-rivet/rivet/actions/workflows/ci.yml)
[![Docker](https://github.com/go-rivet/docker/actions/workflows/build-and-push.yml/badge.svg)](https://github.com/go-rivet/docker/actions/workflows/build-and-push.yml)
[![E2E](https://github.com/go-rivet/rivet/actions/workflows/e2e.yml/badge.svg)](https://github.com/go-rivet/rivet/actions/workflows/e2e.yml)
[![GitHub release](https://img.shields.io/github/v/release/go-rivet/rivet?color=blue)](https://github.com)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)][license-url]

Rivet - Minimal Task.

## Usage

Rivet is built specifically for advanced workflows and minimalist environments. Unlike the upstream [Task][task-url] project, it strips out non-essential overhead to focus strictly on performance and reliable task execution.

For extensive documentation or community-driven support, please refer to the upstream [Task project][task-url].

## Installation

### Automated Install Script (Linux/Darwin)

You can download and run the installer script, which automatically detects your Operating System, hardware architecture.

```bash
# Download and install the latest version of Rivet.
curl -sSL https://raw.githubusercontent.com/go-rivet/rivet/main/install.sh | sh

# Download and install a specific version of Rivet.
RIVET_VERSION="v0.2.0" curl -sSL https://raw.githubusercontent.com/go-rivet/rivet/main/install.sh | sh
```

### Release Artifacts

Statically compiled binaries are available for multiple operating systems and architectures. Download the matching executable for your host platform directly from the **GitHub Releases** panel:

*   **Linux:** `linux/amd64`, `linux/arm64`, `linux/arm/v7`, `linux/386`
*   **macOS:** `darwin/amd64`, `darwin/arm64`
*   **Windows:** `windows/amd64`, `windows/arm64`, `windows/386`


#### Download via curl
You can fetch and extract a specific asset directly through the terminal. Replace `v0.2.0` and `linux_amd64` with your target release tag and host platform strings:

```bash
# Define your target variables
VERSION="v0.2.0"
PLATFORM="linux_amd64"

# Download, extract, and make the binary executable
curl -sSL "https://github.com\({VERSION}/rivet_\){PLATFORM}.tar.gz" | tar -xz
chmod +x rivet
mv rivet /usr/local/bin/
```

### Scratch Docker Image

```bash
# Extract the binary into your current host directory.
docker create --name temp-rivet ghcr.io/go-rivet/rivet:latest
docker cp temp-rivet:/rivet ./rivet
docker rm temp-rivet

# Verify the extracted file runs locally.
./rivet --version
Rivet v0.2.0
Commit: ffc9bc74
Built:  2026-05-17T12:32:58Z
```

### Alpine Docker Image

```bash
# Drop into an interactive bash shell.
docker run --rm -it ghcr.io/go-rivet/rivet:latest-alpine
```

## Development

All build and testing routines are managed via standard automation commands. Binaries are compiled to be 100% statically linked with zero dependencies on system host libraries.

```bash
# Setup a development environment.
make deps

# Build Rivet.
make build
make install

# Release build (all supported platforms).
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
