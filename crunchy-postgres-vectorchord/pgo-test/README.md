# Disposable PGO acceptance test (OPS-26)

This opt-in local harness creates a uniquely named kind cluster with a private
kubeconfig, installs an explicitly supplied PGO manifest, and destroys only that
kind cluster on exit. It does not use your current Kubernetes context, publish
images, modify production resources, or run automatically in GitHub Actions.

## Prerequisites and resource budget

Use an amd64 Linux Docker host, Python 3.9+, kubectl and kind. Allow approximately
8 CPUs, 16 GiB RAM and 30 GiB free disk, plus 15–30 minutes. The candidate currently
supports linux/amd64; an arm64 laptop is not acceptance evidence for this image.
The default kind storage provisioner supplies the disposable PVCs (four database
PVCs and two backup PVCs, each requesting 2 GiB). This is a functional test, not
node-loss, performance, durability or multi-zone qualification.

Supply a reviewed, rendered, self-contained PGO 6.x installation YAML supporting
PostgreSQL 18, configured to watch `pgo-test`. Pin all operator and supporting
images and record the operator release. Installation must include its namespace,
CRD, RBAC and controller. The harness records the manifest SHA-256. No installation
is downloaded or silently selected by this script. Review this manifest as
privileged code before use. Registry access to the immutable candidate, matching
Crunchy pgBackRest image, operator and kind node image must already be available
without changing credentials. Applicable Crunchy licensing still applies.

Use the exact candidate digest from the image build under evaluation; do not
substitute a release tag. All three image arguments require sha256 digests.
The harness pulls those existing images into disposable nodes; it never publishes.
If the candidate is only local, arrange its availability in an approved disposable
environment separately. No registry push is authorized by this test.

```sh
python3 crunchy-postgres-vectorchord/pgo-test/run.py \
  --allow-disposable-cluster \
  --image 'REGISTRY/CANDIDATE@sha256:ACTUAL_64_HEX_DIGEST' \
  --backrest-image 'REGISTRY/PG_BACKREST@sha256:ACTUAL_64_HEX_DIGEST' \
  --node-image 'kindest/node@sha256:ACTUAL_64_HEX_DIGEST' \
  --operator-manifest /absolute/path/to/reviewed-pgo.yaml \
  --output /absolute/path/to/new-evidence.json
```

The placeholders intentionally cannot run. Each phase waits at most `--timeout`
seconds (default 900); individual commands also have bounded timeouts. Ctrl-C
runs cleanup. A killed process or unavailable Docker engine can prevent cleanup:
inspect the exact generated `ops26-*` name in evidence and remove only that kind
cluster with `kind delete cluster --name NAME`. Never use wildcard deletion.

## Assertions and promotion criteria

1. PGO reconciles two ready database pods with the exact candidate image. Seed
   VectorChord's index/query fixture and wait until its 1,000 rows reach a replica.
2. Patroni performs a controlled switchover to that replica. Require the old
   replica to become primary, verify it is out of recovery, and run the original
   indexed nearest-neighbor assertion (id 500, `embeddings_idx` in the plan).
3. Run a synchronous full pgBackRest backup, select its completed full-backup
   label, then remove the source table. Restore a second PostgresCluster from
   that exact backup with `--type=immediate`, using the same candidate image.
4. Require two ready recovered database pods and verify the original row count,
   extension-backed index plan and query result without recreating any data/index.

A passing evidence file must contain all four stages and `cleanup: passed`.
Retain candidate digest, operator manifest hash/release, backup label, host
architecture and local smoke-test results with the release review. Repeat for
each candidate/operator combination before considering production promotion.
This harness does not authorize deployment, image publication or a database
upgrade. A mocked test pass is not PGO acceptance evidence.

Failure evidence contains only pod names, phases, readiness, runtime image IDs and restart counts,
plus the failing stage. It deliberately excludes logs, events, resource specs,
Secrets and arbitrary subprocess stderr, which can expose credentials or data.
If more diagnosis is needed, reproduce in another disposable run; do not broaden
collection to production. Cleanup failures produce a failing exit code and name
the remaining kind cluster. Output files are created under a restrictive umask.

Protocol references: [PGO recovery and cloning](https://access.crunchydata.com/documentation/postgres-operator/latest/tutorials/backups-disaster-recovery/disaster-recovery)
and [Patroni switchover](https://patroni.readthedocs.io/en/latest/patronictl.html#patronictl-switchover).
