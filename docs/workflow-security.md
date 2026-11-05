# Workflow security (OPS-43)

External Actions are pinned to full upstream commit SHAs. The adjacent version
comments are review aids, not execution selectors. Renovate's GitHub Actions
manager must retain digest pins; do not replace a hash with a version tag.

Run the offline policy and regression suite before merging workflow changes:

```sh
python3 -m venv /tmp/workflow-policy-venv
/tmp/workflow-policy-venv/bin/pip install -r .github/scripts/requirements-policy.txt
/tmp/workflow-policy-venv/bin/python .github/scripts/check_action_pins.py
/tmp/workflow-policy-venv/bin/python -m unittest discover -s tests -v
```

The Dockerfiles regression suite additionally requires mikefarah/yq and Git.
The policy parses YAML, including quoted keys, aliases and flow mappings. It
rejects mutable Actions, persisted checkout credentials, workflow-level write
permissions, and workflow/job-level secret environment variables. Positive and
negative fixtures run without registry credentials or real publication.

## Pin review record

Verified on 2026-10-27 directly against each official upstream repository using
`git ls-remote` for the named release tag and its peeled `^{}` commit. This
establishes the release-to-commit identity; it is not a full audit of the bundled
Action code or proof of upstream publisher security. Review changes to Action
inputs, runtime and bundled source when updating these pins.

| Action release | Immutable commit |
| --- | --- |
| [actions/checkout@v7.0.1](https://github.com/actions/checkout/tree/v7.0.1) | [3d3c42e5aac5ba805825da76410c181273ba90b1](https://github.com/actions/checkout/commit/3d3c42e5aac5ba805825da76410c181273ba90b1) |
| [actions/download-artifact@v4.3.0](https://github.com/actions/download-artifact/tree/v4.3.0) | [d3f86a106a0bac45b974a628896c90dbdf5c8093](https://github.com/actions/download-artifact/commit/d3f86a106a0bac45b974a628896c90dbdf5c8093) |
| [actions/upload-artifact@v4.6.2](https://github.com/actions/upload-artifact/tree/v4.6.2) | [ea165f8d65b6e75b540449e92b4886f43607fa02](https://github.com/actions/upload-artifact/commit/ea165f8d65b6e75b540449e92b4886f43607fa02) |
| [docker/build-push-action@v7.4.0](https://github.com/docker/build-push-action/tree/v7.4.0) | [c3c9e263c25d99ce0380d002d59b67737d91b0dc](https://github.com/docker/build-push-action/commit/c3c9e263c25d99ce0380d002d59b67737d91b0dc) |
| [docker/login-action@v4.6.0](https://github.com/docker/login-action/tree/v4.6.0) | [dbcb813823bdd20940b903addbd779551569679f](https://github.com/docker/login-action/commit/dbcb813823bdd20940b903addbd779551569679f) |
| [docker/metadata-action@v6.2.0](https://github.com/docker/metadata-action/tree/v6.2.0) | [dc802804100637a589fabce1cb79ff13a1411302](https://github.com/docker/metadata-action/commit/dc802804100637a589fabce1cb79ff13a1411302) |
| [docker/setup-buildx-action@v4.4.1](https://github.com/docker/setup-buildx-action/tree/v4.4.1) | [f87e5991a6d7451dcb8d9637bfbc97413f497069](https://github.com/docker/setup-buildx-action/commit/f87e5991a6d7451dcb8d9637bfbc97413f497069) |
| [docker/setup-qemu-action@v4.4.0](https://github.com/docker/setup-qemu-action/tree/v4.4.0) | [99012661954931238ded8c8b007157a8430204e1](https://github.com/docker/setup-qemu-action/commit/99012661954931238ded8c8b007157a8430204e1) |

## Build and publication boundary

The pinned shared image workflow builds without publication credentials and uploads
an OCI archive with its checksum and immutable artifact ID. Postgres smoke-tests
the native image loaded from that archive before upload. Native ARM archives are
combined by embedded trusted code for Proton Mail Bridge. Publication downloads
the exact artifacts, verifies their checksums, and preserves manifest digests when
copying to both registries. Source checkout in the publisher is used for ancestry
checks only; it never executes repository scripts.

Caller tests enforce check dependencies, explicit registry secrets, immutable
shared pins and the architecture exceptions. Archive integrity, failure recovery,
stale promotion and immutable-tag guards are tested in `release-workflows`.
See [publication and recovery](../README.md#release-verification).
