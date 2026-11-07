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

The patch is tracked as OPS-214 and waits on upstream
[kzemek/go-mmproxy#39](https://github.com/kzemek/go-mmproxy/pull/39). Drop it
once a release after v2.3.1 includes that fix.

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

The entrypoint is `/usr/local/bin/mmproxy-entrypoint`, a small static Go program
that execs `/usr/local/bin/go-mmproxy` with the given arguments. By default it
does nothing else, so existing consumers behave as before. Pass listener,
backend, and trusted-subnet arguments directly to it.

Set `MMPROXY_SETUP_ROUTING=true` to have the entrypoint first apply the policy
routing mmproxy needs in the current network namespace (the equivalent of
`ip route add local 0.0.0.0/0 dev lo table 123`, `ip rule add from 127.0.0.1/8
iif lo table 123`, and the `-6` forms with `::/0` and `::1/128`). It talks to the
kernel over netlink, so the image still has no shell or iproute2. The setup is
idempotent: routes are replaced and rules carry a fixed priority and are
created exclusively, so a container restart inside the same pod network
namespace neither duplicates nor fails on existing entries. The container needs
`NET_ADMIN`; setup failure exits non-zero before go-mmproxy starts.

| Variable | Default | Meaning |
| --- | --- | --- |
| `MMPROXY_SETUP_ROUTING` | unset (off) | `true` applies the routing before exec |
| `MMPROXY_ROUTING_TABLE` | `123` | routing table number (not 253-255) |
| `MMPROXY_ROUTING_PRIORITY` | `32765` | rule priority, 1-32765 (ahead of `main`) |

The helper's tests run in the image build, including a network-namespace
check that is skipped where user namespaces or `ip` are unavailable. Run them
locally with `go test ./...` in `entrypoint/`. Soft Serve runs this image as a
sidecar with a fixed loopback backend, a mounted subnet list, and
`MMPROXY_SETUP_ROUTING=true`.

## Licensing

The executable and Go runtime retain their BSD-3-Clause terms. License and
copyright notices are included under `/usr/share/licenses/dockerfiles/`; see
[NOTICE.md](licenses/NOTICE.md). The repository's AGPL-3.0-only license applies
to its original build files, not to the upstream executable. Review and update
the bundled upstream notice when changing the source revision.
