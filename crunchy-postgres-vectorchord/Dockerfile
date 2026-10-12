FROM registry.developers.crunchydata.com/crunchydata/crunchy-postgres:ubi9-18.4-2621@sha256:73d62247952de9eb3086a23ceda8a79b243cef75f710e4f40aabacad577d12e8 AS base

FROM base AS builder
USER root
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

# Keep PGDG headers confined to the builder; preserve Crunchy's pg_config and runtime.
ARG PG_DEVEL_VERSION=18.4-2PGDG.rhel9.8
ARG PG_DEVEL_SHA256=ddeea4c990649e3c68ac0a43d091231f4797409aa638382e5c944117745d29ed
ARG VECTORCHORD_VERSION=1.1.1
ARG VECTORCHORD_SHA256=d70b5595bfc852f1f24c05c0a40272e7deecbb0ddf8ffdddec5afa42c2392b1e
ARG RUST_VERSION=1.93.0
ARG RUST_SHA256=b9d9f01a96a2542852ccfddd82194276ba1c86bc76353309ff636b737fc0a772

RUN microdnf install -y --disablerepo='epel*' clang clang-devel llvm-devel cpio tar xz git openssl-devel \
    && microdnf clean all

WORKDIR /build
RUN test "$(uname -m)" = x86_64 \
    && curl -fsSL --retry 3 -o pg-devel.rpm "https://download.postgresql.org/pub/repos/yum/18/redhat/rhel-9-x86_64/postgresql18-devel-${PG_DEVEL_VERSION}.x86_64.rpm" \
    && echo "${PG_DEVEL_SHA256}  pg-devel.rpm" | sha256sum -c - \
    && mkdir headers && cd headers \
    && rpm2cpio ../pg-devel.rpm | cpio -idm \
    && cp -a usr/pgsql-18/include/. /usr/pgsql-18/include/ \
    && grep -F '#define PG_VERSION "'"$(pg_config --version | cut -d ' ' -f 2)"'"' /usr/pgsql-18/include/server/pg_config*.h

RUN curl -fsSL --retry 3 -o rust.tar.xz "https://static.rust-lang.org/dist/rust-${RUST_VERSION}-x86_64-unknown-linux-gnu.tar.xz" \
    && echo "${RUST_SHA256}  rust.tar.xz" | sha256sum -c - \
    && tar -xf rust.tar.xz \
    && ./rust-${RUST_VERSION}-x86_64-unknown-linux-gnu/install.sh --prefix=/usr/local --components=rustc,cargo,rust-std-x86_64-unknown-linux-gnu,rustfmt-preview --disable-ldconfig \
    && rustfmt --version

ENV PG_CONFIG=/usr/pgsql-18/bin/pg_config \
    CC=clang \
    CARGO_BUILD_JOBS=4
RUN curl -fsSL --retry 3 -o vectorchord.tar.gz "https://github.com/supervc-stack/VectorChord/archive/refs/tags/${VECTORCHORD_VERSION}.tar.gz" \
    && echo "${VECTORCHORD_SHA256}  vectorchord.tar.gz" | sha256sum -c - \
    && mkdir vectorchord && tar -xzf vectorchord.tar.gz --strip-components=1 -C vectorchord
WORKDIR /build/vectorchord
RUN cargo fetch --locked \
    && CARGO_NET_OFFLINE=true make build \
    && make install DESTDIR=/out

RUN mkdir -p /out/usr/share/licenses/dockerfiles/vectorchord \
    && cp LICENSE /out/usr/share/licenses/dockerfiles/vectorchord/LICENSE

FROM base AS final
COPY --from=builder /out/ /
COPY licenses/ /usr/share/licenses/dockerfiles/

LABEL org.opencontainers.image.authors="zekihan@noqer.com" \
    org.opencontainers.image.url="https://noqer.com/" \
    org.opencontainers.image.documentation="https://github.com/zekihan/dockerfiles/tree/main/crunchy-postgres-vectorchord" \
    org.opencontainers.image.source="https://github.com/zekihan/dockerfiles" \
    org.opencontainers.image.vendor="Zekihan AZMAN" \
    org.opencontainers.image.licenses="LicenseRef-Crunchy-Data-Developer-Program AND (AGPL-3.0-only OR Elastic-2.0)" \
    org.opencontainers.image.title="Crunchy PostgreSQL with VectorChord" \
    org.opencontainers.image.description="Crunchy PostgreSQL 18 with VectorChord for Immich."
