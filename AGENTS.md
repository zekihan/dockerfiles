# AGENTS.md

This file provides guidance to AI Agents when working with code in this repository.

## Overview

A monorepo of independent container images. Each top-level directory (`actions-runner`, `distroless`, `envsubst`, `it-tools`, `dmarc-report-converter`) is a self-contained image with its own `Dockerfile` and `config.yaml`. Adding a new image means creating a new directory plus a matching workflow.

## Per-image structure

Every image directory contains:
- `Dockerfile` — base images are always fully pinned: `repo:tag@sha256:...`. Renovate updates the digest/tag.
- `config.yaml` — build parameters:
  - `version` — the image version; also drives the published tags.
  - `arch` — comma-separated buildx platforms (e.g. `linux/amd64,linux/arm64`).
  - `revision` — positive packaging revision; a new `version-rN` identity requests an immutable release. Use manual force for an edge-only rebuild.
  - `sourceVersion` (dmarc only) — git tag of upstream source to build from.
  - `sourceCommit` (dmarc only) — reviewed full commit that the source tag must resolve to; update alongside `sourceVersion`.
  - `# renovate: depName=...` trailing comments on `version`/`sourceVersion` tell Renovate which upstream to track. Without this comment the version is manual (e.g. `distroless`, `envsubst` use a hand-set `1.0.0`).

## CI / build pipeline

`.github/workflows/publish.yaml` runs nightly or manually on main only. Select one
image or all images; force rebuilds unchanged inputs without replacing releases.
Changed per-image inputs trigger native checks, then a pinned shared publisher.
New declarations publish `version-rN` and advance `latest`; every successful build
updates multi-architecture `edge`. Failed revisions require a new commit or manual
recovery. Proton Mail Bridge uses native amd64 and arm64 runners; Postgres stays
amd64-only and smoke-tests the prepared image before publication. Ordinary PR
checks never build containers. Old published tags are retained.

Validate image changes by building the image. To build one locally:

```sh
docker buildx build --platform "$(yq '.arch' it-tools/config.yaml)" -t local/it-tools it-tools
# dmarc reads config.yaml at build time, so just point at its dir:
docker build dmarc-report-converter
```

Run `python3 -m unittest discover -s tests -v` for release security checks (requires Git and mikefarah/yq).

When changing build args that the Dockerfile reads from `config.yaml` (dmarc's `sourceVersion`), keep `config.yaml` and the Dockerfile in sync.

## Notable per-image patterns

- **distroless** — builds a `scratch` image by copying CA certs + zoneinfo out of an Alpine builder. No shell, no package manager.
- **dmarc-report-converter** — clones the upstream Go repo at `sourceVersion` and cross-compiles with a `TARGETPLATFORM`→`GOARCH`/`GOARM` case statement. `entrypoint.sh` reads `OUTPUT_DIR`/`CONFIG_DIR`/`CONFIG_FILE` env vars.
- **it-tools** — copies the prebuilt static site out of the upstream image into nginx-unprivileged (port 8080).
- **actions-runner** — extends the official GitHub Actions runner with build tooling and `yq`.

## Conventions

- Renovate (`renovate.json`, extends `github>zekihan/renovate-config`) drives dependency bumps. Commit messages follow Conventional Commits with a scope, e.g. `chore(container):`, `fix(container):`, `ci(github-action):`.
- All images carry a standard set of `org.opencontainers.image.*` LABELs (authors, vendor, component-specific license expression, per-image source/title/description). Match this block when adding an image. Keep upstream licenses and notices separate from the repository's AGPL-3.0-only license; include notices in the final image under `/usr/share/licenses/dockerfiles/`.
