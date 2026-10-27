# OPS-43 owner acceptance

- [ ] Confirm Docker Hub tokens are restricted to the intended repositories and required publication operations. Credential scope cannot be inferred from workflow source; do not expose token values in evidence.
- [ ] Review the upstream Action pin record in `docs/workflow-security.md` before merging subsequent dependency updates.
- [ ] Run an explicitly authorized release/publication acceptance check after merge when Actions capacity is available. This branch was validated locally with synthetic fixtures only; no registry write, release, image publication, live settings change, credential rotation, or workflow retry was performed.
- [ ] Confirm any required protected environment approvals / repository Action allowlist through the owner. No live repository protections were changed by this code-only proposal.
- [ ] Exercise GitHub artifact upload/download and both OCI multi-platform and PostgreSQL Docker-archive registry copy on an authorized release runner. Local tests mock registry writes; this branch did not push any images.
