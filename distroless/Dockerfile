FROM --platform=$BUILDPLATFORM docker.io/alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf AS builder

RUN mkdir -p \
    /tmp/distroless/etc \
    /tmp/distroless/run \
    /tmp/distroless/tmp \
    /tmp/distroless/usr/share/ca-certificates \
    /tmp/distroless/etc/ssl/certs \
    /tmp/distroless/usr/share/zoneinfo \
    && chmod 1777 /tmp/distroless/tmp

RUN apk --no-cache --update --repository https://dl-cdn.alpinelinux.org/alpine/edge/main add \
    ca-certificates \
    tzdata

RUN cp -R /usr/share/ca-certificates/* /tmp/distroless/usr/share/ca-certificates; \
    cp -R /etc/ssl/certs/* /tmp/distroless/etc/ssl/certs; \
    cp -R /usr/share/zoneinfo/* /tmp/distroless/usr/share/zoneinfo

RUN mkdir -p /tmp/distroless/usr/share/licenses/dockerfiles \
    && apk info -v ca-certificates tzdata > /tmp/distroless/usr/share/licenses/dockerfiles/alpine-packages.txt

FROM scratch AS final

COPY --from=builder /tmp/distroless /
COPY licenses/ /usr/share/licenses/dockerfiles/

ENTRYPOINT ["/"]

LABEL org.opencontainers.image.authors="zekihan@noqer.com" \
    org.opencontainers.image.url="https://noqer.com/" \
    org.opencontainers.image.documentation="https://docs.noqer.com/" \
    org.opencontainers.image.source="https://github.com/zekihan/dockerfiles" \
    org.opencontainers.image.vendor="Zekihan AZMAN" \
    org.opencontainers.image.licenses="MPL-2.0 AND LicenseRef-IANA-TZ-Public-Domain" \
    org.opencontainers.image.title="distroless" \
    org.opencontainers.image.description="Distroless is a minimal base image that contains only the necessary libraries and files to run an application, without any package manager or shell."
