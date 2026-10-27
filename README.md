# dockerfiles

## Release verification

The Distroless, DMARC converter, Go-mmproxy and Proton Mail Bridge publishing
jobs run only for pushes or manual dispatches on `refs/heads/main`. They check
out the triggering commit, record it in the run summary and image revision label,
and expose the Docker Hub password only to the login step. A manual dispatch on
a branch or tag skips the entire publishing job.

DMARC's `sourceVersion` and `sourceCommit` must be updated together after reviewing
the upstream release. The build fetches the release tag and fails before checkout
or compilation if it resolves to a different commit. The verified commit is
stored in the image at `/usr/share/dmarc-report-converter/source-revision`.
Renovate version updates require a reviewed commit-pin update before they build.

Run `python3 -m unittest discover -s tests -v` (Git and mikefarah/yq required)
and `shellcheck dmarc-report-converter/verify-source.sh` locally. The tests use
temporary Git repositories to check moved tags and repeated source identity;
they never log in to a registry or publish an image.

Live acceptance still requires an authorized maintainer to dispatch each workflow
from a non-main branch and confirm the job is skipped, then dispatch from main
and check the recorded SHA and published tags. Protected GitHub Environment
reviewers, branch protections and external credential scopes are operator-managed
controls; this change does not configure or verify those settings. Environment
setup and live publication require separate authorization.

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
