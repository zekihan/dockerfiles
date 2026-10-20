FROM golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 AS build
RUN apt-get update && apt-get install -y --no-install-recommends libsecret-1-dev libfido2-dev libcbor-dev libssl-dev pkg-config \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY config.yaml /build-config.yaml
# Keep the release version and immutable source revision together in config.yaml.
RUN revision=$(sed -n "s/^sourceRevision: //p" /build-config.yaml) \
    && test "${#revision}" = 40 \
    && git init && git remote add origin https://github.com/ProtonMail/proton-bridge.git \
    && git fetch --depth 1 origin "$revision" \
    && git checkout --detach FETCH_HEAD
RUN make build-nogui

FROM debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates openssl pass gnupg libsecret-1-0 libfido2-1 libcbor0.8 util-linux \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /src/bridge /src/proton-bridge /protonmail/
COPY entrypoint.sh /setup/bridge.sh
COPY licenses/ /usr/share/licenses/dockerfiles/
ENV HOME=/root
ENTRYPOINT ["bash", "/setup/bridge.sh"]
CMD ["serve"]

LABEL org.opencontainers.image.authors="zekihan@noqer.com" \
    org.opencontainers.image.url="https://noqer.com/" \
    org.opencontainers.image.documentation="https://github.com/zekihan/dockerfiles/tree/main/protonmailbridge" \
    org.opencontainers.image.source="https://github.com/zekihan/dockerfiles" \
    org.opencontainers.image.vendor="Zekihan AZMAN" \
    org.opencontainers.image.licenses="AGPL-3.0-only AND GPL-3.0-or-later" \
    org.opencontainers.image.title="protonmailbridge" \
    org.opencontainers.image.description="Proton Mail Bridge CLI with persistent encrypted credentials and certificate export."
