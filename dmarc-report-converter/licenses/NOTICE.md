# DMARC converter third-party notices

The converter's source release is selected by `sourceVersion` in `config.yaml`.
Its MIT license is copied from that release to `dmarc-report-converter/LICENSE`.
Source: https://github.com/tierpod/dmarc-report-converter.

The Go runtime's BSD-3-Clause license is copied from the build toolchain to
`go/LICENSE`. Vendored dependency licenses, copyright notices, and module
versions are copied to `vendor/` and `dmarc-report-converter/modules.txt`.
These dependencies retain their MIT, BSD-3-Clause, or Apache-2.0 terms.

The BusyBox base is GPL-2.0-only; see `GPL-2.0-only.txt`.
Source: https://busybox.net/downloads/.
Official image build configuration: https://github.com/docker-library/busybox.
Redistributing the binary requires compliance with GPLv2 section 3, including
providing the exact corresponding source and build configuration through a
permitted mechanism. These upstream links and this notice alone are not a
complete source distribution or a written source offer. Preserve the source and
configuration matching the pinned base image when redistributing it.

The CA certificate data is MPL-2.0; see `MPL-2.0.txt` and the package versions
in `alpine-packages.txt`. Corresponding source:
https://gitlab.alpinelinux.org/alpine/ca-certificates.

The original entrypoint script is AGPL-3.0-only; see `AGPL-3.0-only.txt`.
Its source is included at `/entrypoint.sh` and in this repository. Upstream
software is not relicensed by the repository's license.
