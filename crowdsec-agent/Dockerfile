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

# The upstream image bundles yq v4.50.1 (x/net 0.48, x/text 0.32) and apk
# rsync 3.5.0-r0. Replace both with fixed releases without a RUN in the
# final stage: files and package records land via COPY --from=builder /out/ /.
# Keep the base pin in step with the final stage below.
COPY --from=docker.io/crowdsecurity/crowdsec:v1.8.2@sha256:65b74942f7e7ed5c60fde4ac15fc4ba715904c398c8b16dd451f4907d90347d4 /lib/apk/db/installed /base-installed

RUN set -eu \
    && case "$TARGETARCH" in \
        amd64) yqarch=amd64; yqsha=129534601871538efe0ba4c5e62a34fb1d5f469bcb975c4fddeb467d82974973; apkarch=x86_64; rsyncsha=852e7fa78f8b9dd8022c97dbfe0c52ea3909d4faee2712f05fd676cec43133cb; idn2sha=9e6121c48274280c4258f1b3da895c5e63266639cd7ff611e9a562311e210263; unistring_sha=62fe5110253cb5ba3a1673e33097b0bca9903b5e016ed50aca4977bd9b905009;; \
        arm64) yqarch=arm64; yqsha=89f69310b61b68d1245065a1241d1fc843f7a36cd6103ffd77ad987613bd1b67; apkarch=aarch64; rsyncsha=c79c427e7a89e97e8b834205897b8e5d99f2e06d5ca8b0fb24667001c4282479; idn2sha=7023b0733eb32fac513a865d4630a13e266c0ce30196cd3e74948ee8fffae7a7; unistring_sha=a74bede34b08f19eec13e53fca147c6409070a49261e0ce1df9d85a7e841d0cf;; \
        *) echo "unsupported TARGETARCH: $TARGETARCH" >&2; exit 1;; \
    esac \
    && wget -qO /tmp/yq "https://github.com/mikefarah/yq/releases/download/v4.54.3/yq_linux_$yqarch" \
    && echo "$yqsha  /tmp/yq" | sha256sum -c - \
    && mkdir -p /rsync-root/etc/apk /out/usr/local/bin /out/usr/bin /out/usr/lib /out/lib/apk/db \
    && cp /etc/apk/repositories /rsync-root/etc/apk/repositories \
    && echo "$apkarch" > /rsync-root/etc/apk/arch \
    && apk add --root /rsync-root --initdb --no-scripts --allow-untrusted rsync \
    && echo "$rsyncsha  /rsync-root/usr/bin/rsync" | sha256sum -c - \
    && echo "$idn2sha  /rsync-root/usr/lib/libidn2.so.0.4.0" | sha256sum -c - \
    && echo "$unistring_sha  /rsync-root/usr/lib/libunistring.so.5.2.1" | sha256sum -c - \
    && awk 'BEGIN{ RS=""; ORS="\n\n" } { name=""; for(i=1;i<=NF;i++){ if($i ~ /^P:/){ name=$i; sub(/^P:/,"",name); break } } } NR==FNR{ if(name=="rsync"||name=="libidn2"||name=="libunistring") new[name]=$0; next } NR!=FNR{ if(name=="rsync"){ print new["rsync"]; next } print } END{ print new["libidn2"]; print new["libunistring"] }' /rsync-root/lib/apk/db/installed /base-installed > /out/lib/apk/db/installed \
    && install -m 0755 /tmp/yq /out/usr/local/bin/yq \
    && install -m 0755 /rsync-root/usr/bin/rsync /out/usr/bin/rsync \
    && cp -a /rsync-root/usr/lib/libidn2.so.* /rsync-root/usr/lib/libunistring.so.* /out/usr/lib/

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
