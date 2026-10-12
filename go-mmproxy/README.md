# go-mmproxy

Package the official static go-mmproxy release binary in a scratch image for
linux/amd64 and linux/arm64. Download and verify its architecture-specific SHA-256
during the image build. The runtime has no shell, downloader, or package manager.

`config.yaml` supplies the image/upstream version, supported platforms, and release
checksums. Update the version and both checksums together after verifying the
upstream release. Version updates are manual because checksums must change with
the binaries. Bump `nonce` to republish an unchanged release.

The matching GitHub Actions workflow publishes to Docker Hub and GHCR following
the repository's existing tagging conventions.

```sh
docker buildx build --platform linux/amd64,linux/arm64 --load \
  -t local/go-mmproxy:2.3.1 go-mmproxy
docker run --rm local/go-mmproxy:2.3.1 -help
```

Pass listener, backend, and trusted-subnet arguments directly to the entrypoint.
The caller must configure mmproxy's policy routes in its network namespace and
grant the runtime `NET_ADMIN`. Soft Serve keeps its routing initializer and runs
this image as a sidecar with a fixed loopback backend and a mounted subnet list.

## Licensing

The executable and Go runtime retain their BSD-3-Clause terms. License and
copyright notices are included under `/usr/share/licenses/dockerfiles/`; see
[NOTICE.md](licenses/NOTICE.md). The repository's AGPL-3.0-only license applies to
its original build files, not to the upstream executable. Review and update the
bundled upstream notice when changing the release version.
