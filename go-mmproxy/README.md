# go-mmproxy

Build a static scratch image from upstream go-mmproxy v2.3.1 at its exact source
commit, with a small fix for IPv6 PROXY clients arriving over an IPv4 transport.
The builder and upstream revision are pinned; buildx supports Linux AMD64 and
ARM64. The image remains a single upstream process with no shell or package
manager.

For a valid PROXY source, backend selection now follows the source address
family: TCP4 selects the configured IPv4 target and TCP6 selects the configured
IPv6 target, independently of the mmproxy frontend peer. When no valid PROXY
source is available, the frontend family still selects the target. Dynamic
destination remains disabled by default and is not enabled by the patch.

Build and inspect locally:

```sh
docker buildx build --platform linux/amd64 --load \
  -t local/go-mmproxy:2.3.1-ipv6.1 go-mmproxy
docker run --rm local/go-mmproxy:2.3.1-ipv6.1 -help
```

Run the family-selection regression from the matching upstream source after
applying `patches/ipv6-family.patch`:

```sh
gofmt -w internal/tcp/tcp.go internal/tcp/tcp_family_test.go
go test -timeout 30s ./...
```

The image build runs the focused family-selection regression in its native
Go builder stage before cross-compilation. The upstream tests that exercise
transparent loopback routing require Linux policy routes as configured by the
upstream CI workflow. The image build itself
cross-compiles with `CGO_ENABLED=0`, `GOOS=linux`, and buildx `TARGETARCH`, then
copies only the binary and third-party notices into scratch.

Pass listener, backend, and trusted-subnet arguments directly to the entrypoint.
The caller must configure mmproxy's policy routes in its network namespace and
grant the runtime `NET_ADMIN`. Soft Serve keeps its routing initializer and runs
this image as a sidecar with a fixed loopback backend and a mounted subnet list.

## Licensing

The executable and Go runtime retain their BSD-3-Clause terms. License and
copyright notices are included under `/usr/share/licenses/dockerfiles/`; see
[NOTICE.md](licenses/NOTICE.md). The repository's AGPL-3.0-only license applies
to its original build files, not to the upstream executable. Review and update
the bundled upstream notice when changing the source revision.
