FROM --platform=$BUILDPLATFORM docker.io/alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6 AS builder

ARG TARGETARCH

RUN apk add --no-cache ca-certificates curl yq-go

COPY config.yaml /build/config.yaml

RUN case "$TARGETARCH" in amd64|arm64) ;; *) echo 'Unsupported mmproxy architecture' >&2; exit 1 ;; esac \
    && version=$(yq '.version' /build/config.yaml) \
    && checksum=$(yq ".checksums.${TARGETARCH}" /build/config.yaml) \
    && curl --fail --show-error --location --retry 3 --connect-timeout 10 --max-time 120 \
        --proto '=https' --proto-redir '=https' \
        -o /go-mmproxy \
        "https://github.com/kzemek/go-mmproxy/releases/download/v${version}/go-mmproxy-v${version}-linux-${TARGETARCH}" \
    && printf '%s  %s\n' "$checksum" /go-mmproxy | sha256sum -c - \
    && chmod 0555 /go-mmproxy

FROM scratch AS final

COPY --from=builder /go-mmproxy /usr/local/bin/go-mmproxy
COPY licenses/ /usr/share/licenses/dockerfiles/

ENTRYPOINT ["/usr/local/bin/go-mmproxy"]

LABEL org.opencontainers.image.authors="zekihan@noqer.com" \
    org.opencontainers.image.url="https://noqer.com/" \
    org.opencontainers.image.documentation="https://github.com/zekihan/dockerfiles/tree/main/go-mmproxy" \
    org.opencontainers.image.source="https://github.com/zekihan/dockerfiles" \
    org.opencontainers.image.vendor="Zekihan AZMAN" \
    org.opencontainers.image.licenses="BSD-3-Clause" \
    org.opencontainers.image.title="go-mmproxy" \
    org.opencontainers.image.description="Static go-mmproxy binary for preserving client addresses behind PROXY protocol."
