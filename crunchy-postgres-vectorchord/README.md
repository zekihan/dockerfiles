# Crunchy PostgreSQL with VectorChord

## Licensing and redistribution

The base image comes from the Crunchy Data Developer Program. Its
[terms of use](https://www.crunchydata.com/developers/terms-of-use) prohibit
redistribution to third parties without an applicable support subscription or
signed written agreement. Open-source components retain their individual
licenses; those licenses do not by themselves establish permission to
redistribute Crunchy's packaged product.

Publishing remains enabled. This notice does not grant redistribution permission
or waive Crunchy's terms. Confirm that an applicable agreement permits the
intended distribution before publishing or sharing this image with third parties.

VectorChord 1.1.1 offers AGPLv3 or Elastic License v2. The repository license
covers this repository's original build files and scripts only. See the
[bundled notice](licenses/NOTICE.md) for component terms and source references.

PGO-compatible Crunchy PostgreSQL 18.4 plus VectorChord 1.1.1, initially for
`linux/amd64`. The final stage inherits the Crunchy runtime unchanged and adds
only VectorChord library, control, and SQL files. pgvector remains supplied by
Crunchy. No default command, user, or database configuration is overridden.

## Build and test

```sh
docker buildx build --platform linux/amd64 --load \
  -t local/crunchy-postgres-vectorchord:test crunchy-postgres-vectorchord
bash crunchy-postgres-vectorchord/smoke-test.sh local/crunchy-postgres-vectorchord:test
```

The smoke test uses a disposable, non-root container and temporary database. It
checks extension creation, an indexed nearest-neighbor query, restart, and logical
backup/restore. It does not verify PGO reconciliation, replica promotion, or
pgBackRest recovery. The opt-in [disposable PGO harness](pgo-test/README.md)
adds reconciliation, controlled replica promotion and physical backup/recovery
validation; run it for the exact candidate before production promotion. See
[remaining acceptance checks](pgo-test/TODO.md) for currently unexecuted checks.

## Build dependencies

The Crunchy base image is pinned by digest. The builder extracts PostgreSQL
18.4 PGDG RHEL 9 headers (verified by SHA-256); the base does not ship server
headers. It retains Crunchy's `pg_config` and checks that the header version
matches the runtime. These are matching upstream-version headers, not Crunchy
vendor development packages. Runtime smoke testing is required for every build.

VectorChord is compiled from a checksummed release archive inside the same UBI
base with a pinned Rust toolchain. No Debian-built extension binaries are copied
into UBI. Build dependencies and headers are absent from the final stage.

## PGO / Immich configuration

Once published, set `crunchyPostgres.imagePostgres` to the custom image pinned by
digest. Add `vchord.so` alongside `pg_stat_statements` in the cluster's preload
setting; retain other operator-managed entries. During database bootstrap, run:

```sql
CREATE EXTENSION IF NOT EXISTS vchord CASCADE;
CREATE EXTENSION IF NOT EXISTS earthdistance CASCADE;
```

Use `DB_VECTOR_EXTENSION=vectorchord` for Immich. All database replicas and
restore environments need the same extension image. This image creates no
users, databases, or credentials itself.

## Updates

Renovate's Dockerfile manager tracks the Crunchy base. PostgreSQL minor updates
must also update the matching PGDG header package and checksum. VectorChord and
Rust versions/checksums are deliberately updated together by hand. Update the
combined `config.yaml` version when PostgreSQL or VectorChord changes; bump
`nonce` for other rebuilds. Keep PostgreSQL on major 18 in this image line.

CI builds and tests pull requests without publishing. After smoke tests pass,
main-branch pushes and manual main-branch runs publish the exact tested image to
Docker Hub and GHCR. It uses existing `DOCKER_USERNAME`, `DOCKER_PASSWORD`, and
`GITHUB_TOKEN` credentials. Tags include the combined version (for example,
`18.4-vectorchord1.1.1`) and a revision/run/attempt tag
(`<version>-r<nonce>-<run_number>-<run_attempt>`). No `latest` or major-only tag
is published. Pin the published digest in GitOps.

Extension upgrades may require `ALTER EXTENSION` and index rebuilding; changing
the image alone does not perform those operations.
