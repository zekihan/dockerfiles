FROM --platform=$BUILDPLATFORM docker.io/golang:1.27.3-alpine@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS builder

SHELL ["/bin/ash", "-o", "pipefail", "-c"]

ARG TARGETARCH

WORKDIR /src
COPY supervisor/go.mod supervisor/go.sum ./
RUN go mod download && go mod verify
COPY supervisor/ ./
COPY apply-whitelists.sh /out/usr/local/lib/crowdsec-supervisor/apply-whitelists.sh

RUN set -eu \
    && gofmt -l . | (! grep .) \
    && go vet ./... \
    && go test -count=1 ./... \
    && CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" go build -trimpath -ldflags='-s -w' \
        -o /out/usr/local/bin/crowdsec-supervisor . \
    && mkdir -p /out/usr/local/sbin /out/usr/share/licenses/dockerfiles \
    && ln -s /usr/local/bin/crowdsec-supervisor /out/usr/local/sbin/crowdsec \
    && chmod 0755 /out/usr/local/bin/crowdsec-supervisor \
    && chmod 0644 /out/usr/local/lib/crowdsec-supervisor/apply-whitelists.sh \
    && cp /usr/local/go/LICENSE /out/usr/share/licenses/dockerfiles/go-LICENSE \
    && yaml="$(go list -m -f '{{.Dir}}' go.yaml.in/yaml/v3)" \
    && cp "$yaml/LICENSE" /out/usr/share/licenses/dockerfiles/go-yaml-LICENSE \
    && cp "$yaml/NOTICE" /out/usr/share/licenses/dockerfiles/go-yaml-NOTICE \
    && chmod 0644 /out/usr/share/licenses/dockerfiles/*

FROM docker.io/crowdsecurity/crowdsec:v1.8.2@sha256:65b74942f7e7ed5c60fde4ac15fc4ba715904c398c8b16dd451f4907d90347d4 AS final

COPY --from=builder /out/ /
COPY licenses/ /usr/share/licenses/dockerfiles/

LABEL org.opencontainers.image.authors="zekihan@noqer.com" \
    org.opencontainers.image.url="https://noqer.com/" \
    org.opencontainers.image.documentation="https://github.com/zekihan/dockerfiles/tree/main/crowdsec-agent" \
    org.opencontainers.image.source="https://github.com/zekihan/dockerfiles" \
    org.opencontainers.image.vendor="Zekihan AZMAN" \
    org.opencontainers.image.licenses="AGPL-3.0-only AND MIT AND Apache-2.0 AND BSD-3-Clause" \
    org.opencontainers.image.title="crowdsec-agent" \
    org.opencontainers.image.description="Upstream CrowdSec with a static supervisor for VictoriaLogs acquisition health, scheduled hub refreshes, and agent/LAPI state preparation."
