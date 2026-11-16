# SPDX-FileCopyrightText: 2026 Timothy Rule
# SPDX-License-Identifier: MIT

ARG GO_VERSION=1.26.0
FROM golang:${GO_VERSION}-bookworm

ARG ZIG_VERSION=0.14.1
ARG TARGETARCH

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl git make xz-utils zip \
    && rm -rf /var/lib/apt/lists/*

RUN set -eu; \
    case "${TARGETARCH}" in \
      amd64) zig_arch=x86_64-linux; zig_sha=24aeeec8af16c381934a6cd7d95c807a8cb2cf7df9fa40d359aa884195c4716c ;; \
      arm64) zig_arch=aarch64-linux; zig_sha=f7a654acc967864f7a050ddacfaa778c7504a0eca8d2b678839c21eea47c992b ;; \
      *) echo "Unsupported build architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    zig_archive="zig-${zig_arch}-${ZIG_VERSION}.tar.xz"; \
    curl -fsSLo "/tmp/${zig_archive}" "https://ziglang.org/download/${ZIG_VERSION}/${zig_archive}"; \
    echo "${zig_sha}  /tmp/${zig_archive}" | sha256sum -c -; \
    mkdir -p /opt/zig; \
    tar -xJf "/tmp/${zig_archive}" -C /opt/zig --strip-components=1; \
    ln -s /opt/zig/zig /usr/local/bin/zig; \
    rm "/tmp/${zig_archive}"

WORKDIR /src
