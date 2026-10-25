FROM --platform=$BUILDPLATFORM docker.io/alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6 AS certs
RUN apk update && apk add ca-certificates
RUN mkdir -p /licenses && apk info -v ca-certificates > /licenses/alpine-packages.txt

FROM --platform=$BUILDPLATFORM docker.io/golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS builder

ARG TARGETPLATFORM

RUN apk update && \
    apk add --no-cache \
        ca-certificates && \
    apk add --no-cache --virtual=build-dependencies \
        git \
        curl \
        yq-go

COPY config.yaml config.yaml
COPY verify-source.sh /usr/local/bin/verify-source.sh

RUN export VERSION=$(yq '.sourceVersion' config.yaml) && \
    SOURCE_COMMIT=$(yq '.sourceCommit' config.yaml) && \
    export GOOS="linux" && export CGO_ENABLED=0 && \
    case ${TARGETPLATFORM} in \
        "linux/amd64")    ARCH="amd64"          ;; \
        "linux/arm64")    ARCH="arm64"          ;; \
        "linux/riscv64")  ARCH="riscv64"        ;; \
        "linux/ppc64le")  ARCH="ppc64le"        ;; \
        "linux/s390x")    ARCH="s390x"          ;; \
        "linux/386")      ARCH="386"            ;; \
        "linux/arm/v7")   ARCH="arm"; ARM="7"   ;; \
        "linux/arm/v6")   ARCH="arm"; ARM="6"   ;; \
    esac && \
    export GOARCH="${ARCH}" && export GOARM="${ARM}" && \
    sh /usr/local/bin/verify-source.sh https://github.com/tierpod/dmarc-report-converter.git "$VERSION" "$SOURCE_COMMIT" /tmp/repo && \
    cd /tmp/repo && \
    go build -mod=vendor -ldflags="-s -w -X \"main.version=${VERSION}\"" -o "/usr/bin/dmarc-report-converter" ./cmd/dmarc-report-converter

RUN mkdir -p /licenses/dmarc-report-converter /licenses/go \
    && cp /tmp/repo/LICENSE /licenses/dmarc-report-converter/LICENSE \
    && cp /tmp/repo/vendor/modules.txt /licenses/dmarc-report-converter/modules.txt \
    && cp /usr/local/go/LICENSE /licenses/go/LICENSE \
    && cd /tmp/repo \
    && find vendor -type f \( -iname '*license*' -o -iname '*copying*' -o -iname '*notice*' \) \
        -exec sh -c 'for path do mkdir -p "/licenses/$(dirname "$path")"; cp "$path" "/licenses/$path"; done' sh {} +

FROM docker.io/busybox:1.38.0@sha256:fd7dc98638c8e305f4dc34e979f1c0fdfdcaeb0fbf8fcff77ae834b6da3d7e6e AS final

COPY --from=certs /etc/ssl/certs /etc/ssl/certs
COPY --from=builder /usr/bin/dmarc-report-converter /usr/bin/dmarc-report-converter
COPY ./entrypoint.sh ./entrypoint.sh
COPY --from=builder /tmp/repo/source-revision /usr/share/dmarc-report-converter/source-revision
COPY --from=certs /licenses/ /usr/share/licenses/dockerfiles/
COPY --from=builder /licenses/ /usr/share/licenses/dockerfiles/
COPY licenses/ /usr/share/licenses/dockerfiles/

ENTRYPOINT ["./entrypoint.sh"]

LABEL org.opencontainers.image.authors="zekihan@noqer.com" \
    org.opencontainers.image.url="https://noqer.com/" \
    org.opencontainers.image.documentation="https://docs.noqer.com/" \
    org.opencontainers.image.source="https://github.com/zekihan/dockerfiles/dmarc-report-converter" \
    org.opencontainers.image.vendor="Zekihan AZMAN" \
    org.opencontainers.image.licenses="AGPL-3.0-only AND MIT AND BSD-3-Clause AND Apache-2.0 AND GPL-2.0-only AND MPL-2.0" \
    org.opencontainers.image.title="Dmarc Report Converter" \
    org.opencontainers.image.description="Dmarc Report Converter is a tool to help you convert your dmarc reports to a more readable format."
