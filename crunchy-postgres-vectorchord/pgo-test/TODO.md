# OPS-26 remaining acceptance

- Run this harness on an approved disposable amd64 Docker host with kind,
  a reviewed PostgreSQL-18-compatible PGO installation manifest, and immutable
  candidate/pgBackRest/node image digests. No such inputs were supplied here.
- Capture successful reconciliation, replica switchover, full pgBackRest backup,
  physical recovery, indexed queries and cleanup evidence. These live checks have
  NOT been run. Local environment has no kind binary and Docker socket access was
  denied; no production cluster was contacted and no retry bypassed that denial.
- Run the existing image build and container smoke test on that host. Docker
  access is required; offline regression tests do not establish compatibility.
- Validate the selected PGO release's Patroni CLI defaults and pgBackRest runtime
  image against the supplied operator manifest in that disposable run.

Do not promote based solely on offline tests. No publication, live credential or
network changes, deployment or production acceptance are part of this change.
