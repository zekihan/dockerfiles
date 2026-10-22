FROM --platform=$BUILDPLATFORM golang:1.25@sha256:699337d620559a59b4a2bb298ad59611e535d2ee755a34cf2d2a98f37578dc80 AS builder

ARG TARGETARCH
ARG SOURCE_REVISION=e5c3c990eed960dacd34038c088d6ce4c4b35c97

RUN apt-get update \
    && apt-get install --no-install-recommends -y git \
    && rm -rf /var/lib/apt/lists/*

COPY config.yaml /build/config.yaml
COPY patches/ipv6-family.patch /build/ipv6-family.patch

RUN set -eu \
    && grep -Fx "sourceRevision: ${SOURCE_REVISION}" /build/config.yaml \
    && git init /src \
    && cd /src \
    && git remote add origin https://github.com/kzemek/go-mmproxy.git \
    && git fetch --depth 1 origin "${SOURCE_REVISION}" \
    && git checkout --detach FETCH_HEAD \
    && test "$(git rev-parse HEAD)" = "${SOURCE_REVISION}" \
    && git apply /build/ipv6-family.patch \
    && test "$(git diff --name-only)" = "internal/tcp/tcp.go" \
    && test "$(git ls-files --others --exclude-standard)" = "internal/tcp/tcp_family_test.go" \
    && go test ./internal/tcp -run TestChooseTargetAddrFamily -count=1 \
    && CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" go build -trimpath -ldflags='-s -w' -o /go-mmproxy . \
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
    org.opencontainers.image.description="Static go-mmproxy with PROXY source-family backend selection for transparent IPv6 client forwarding."
