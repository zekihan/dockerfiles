# dockerfiles

## Release verification

The Publish images workflow runs nightly or manually on main. Select one image or
all images. Native checks gate every build; ordinary PR checks never build images.
New inputs publish `edge` to GHCR and Docker Hub. Unchanged successful inputs skip;
failed revisions require a new commit or manual recovery. Non-main dispatches fail.

Each image declares `version` plus a positive `revision` in `config.yaml`. A new
`<version>-r<revision>` identity is released automatically after a successful build,
and advances `latest`. An existing identity remains immutable even when dependency
updates refresh `edge`. Increase the revision for a new stable packaging release.
Manual force rebuilds edge; it cannot overwrite immutable releases. Old tags stay.

Proton Mail Bridge builds on native amd64 and arm64 runners. Other portable images
cross-compile on amd64; Postgres/VectorChord remains amd64-only and smoke-tests the
prepared artifact. The shared publisher copies one OCI archive to both registries
and verifies matching digests and anonymous access. Make the packages public before
accepting the first successful run.

After partial publication, rerun **failed jobs** within seven days to reuse the
original artifacts. Full rebuilds can produce different bytes and must not replace
an immutable release. After expiry, recover the original artifacts or declare a new
revision. Registry updates are not transactional.

DMARC's `sourceVersion` and `sourceCommit` must be updated together after reviewing
the upstream release. The build fetches the release tag and fails before checkout
or compilation if it resolves to a different commit. The verified commit is
stored in the image at `/usr/share/dmarc-report-converter/source-revision`.
Renovate version updates require a reviewed commit-pin update before they build.

Run `python3 -m unittest discover -s tests -v` (Git and mikefarah/yq required)
and `shellcheck dmarc-report-converter/verify-source.sh` locally. The tests use
temporary Git repositories to check moved tags and repeated source identity;
they never log in to a registry or publish an image.

## Licensing

Original Dockerfiles, scripts, configuration, and documentation in this repository
are licensed under AGPL-3.0-only; see [LICENSE](LICENSE). This license does not
replace the licenses of upstream software, container images, vendored files, or
third-party notices. Those components retain their original terms.

Each image includes its notices under `/usr/share/licenses/dockerfiles/`.
License labels summarize the identified components; they are not a complete
inventory of every dependency in an upstream base image.

- [go-mmproxy notices](go-mmproxy/licenses/NOTICE.md)
- [DMARC converter notices](dmarc-report-converter/licenses/NOTICE.md)
- [distroless notices](distroless/licenses/NOTICE.md)
- [Crunchy PostgreSQL / VectorChord notices](crunchy-postgres-vectorchord/licenses/NOTICE.md)

The Crunchy image uses software from the Crunchy Data Developer Program, whose
terms restrict redistribution. Publishing remains enabled. The notice is not
permission to redistribute; obtain an applicable agreement before distributing
the image to third parties.

- [Proton Mail Bridge](protonmailbridge/README.md) ([notices](protonmailbridge/licenses/NOTICE.md))

Workflow security policy and local validation: [docs/workflow-security.md](docs/workflow-security.md).
