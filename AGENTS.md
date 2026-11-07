# AGENTS.md

This file provides guidance to AI Agents when working with code in this repository.

## Overview

A monorepo of independent container images: `distroless`, `dmarc-report-converter`,
`go-mmproxy`, `protonmailbridge`, `crunchy-postgres-vectorchord`, and `crowdsec-agent`. Each top-level
image directory contains its Dockerfile and build configuration. The shared
`.github/workflows/publish.yaml` workflow handles every image; adding an image
requires wiring its per-image decide, checks, and publish jobs into that workflow
and its `workflow_dispatch` image option.

## Per-image structure

Every image directory contains:
- `Dockerfile` — base images are always fully pinned: `repo:tag@sha256:...`. Renovate updates the digest/tag.
- `config.yaml` — build parameters:
  - `version` — the image version; also drives the published tags.
  - `arch` — comma-separated buildx platforms (e.g. `linux/amd64,linux/arm64`).
  - `revision` — positive packaging revision; a new `version-rN` identity requests an immutable release. Use manual force for an edge-only rebuild.
  - `sourceVersion` (dmarc only) — git tag of upstream source to build from.
  - `sourceCommit` (dmarc only) — reviewed full commit that the source tag must resolve to; update alongside `sourceVersion`.
  - `# renovate: depName=...` trailing comments on `version`/`sourceVersion` tell Renovate which upstream to track. Without this comment the version is manual (for example, `distroless` uses a hand-set version).

## CI / build pipeline

`.github/workflows/publish.yaml` runs nightly or manually on main only. Select one
image or all images; force rebuilds unchanged inputs without replacing releases.
Changed per-image inputs trigger native checks, then a pinned shared publisher.
New declarations publish `version-rN` and advance `latest`; every successful build
updates multi-architecture `edge`. Failed revisions require a new commit or manual
recovery. Proton Mail Bridge uses native amd64 and arm64 runners; Postgres stays
amd64-only and smoke-tests the prepared image before publication. Ordinary PR
checks never build containers. Old published tags are retained.

Validate image changes by building the image. For example:

```sh
docker buildx build --platform "$(yq '.arch' distroless/config.yaml)" -t local/distroless distroless
# dmarc reads config.yaml at build time, so point at its directory:
docker build dmarc-report-converter
```

Run `python3 -m unittest discover -s tests -v` for release security checks (requires Git and mikefarah/yq).

When changing build args that the Dockerfile reads from `config.yaml` (dmarc's `sourceVersion`), keep `config.yaml` and the Dockerfile in sync.

## Notable per-image patterns

- **distroless** — builds a `scratch` image by copying CA certs and zoneinfo out of an Alpine builder. No shell or package manager.
- **dmarc-report-converter** — clones the upstream Go repo at `sourceVersion` and cross-compiles by target platform. The tag must resolve to the reviewed `sourceCommit`.
- **go-mmproxy** — applies the IPv6 family patch (OPS-214) and builds for the configured architectures.
- **protonmailbridge** — builds on native amd64 and arm64 runners and runs as non-root UID 65532.
- **crunchy-postgres-vectorchord** — amd64-only; smoke-tests the prepared artifact before publication.
- **crowdsec-agent** — upstream CrowdSec plus a static Go supervisor (`crowdsec-agent/supervisor`) for health, hub refresh and init-container preparation. The final stage only adds files (no `RUN`), and `/usr/local/sbin/crowdsec` symlinks to the supervisor so the upstream entrypoint goes through it. `apply-whitelists.sh` is copied unchanged from homelab. `version` tracks the CrowdSec tag; keep it in step with the `FROM` pin. Run `go test ./...` in `supervisor/` and `smoke-test.sh` against a local build.

## Conventions

- Renovate (`renovate.json`, extends `github>zekihan/renovate-config`) drives dependency bumps. Commit messages follow Conventional Commits with a scope, e.g. `chore(container):`, `fix(container):`, `ci(github-action):`.
- All images carry a standard set of `org.opencontainers.image.*` LABELs (authors, vendor, component-specific license expression, per-image source/title/description). Match this block when adding an image. Keep upstream licenses and notices separate from the repository's AGPL-3.0-only license; include notices in the final image under `/usr/share/licenses/dockerfiles/`.
